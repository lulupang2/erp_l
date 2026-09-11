package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"example.com/assembly-erp/api/internal/db"
	"example.com/assembly-erp/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool    *pgxpool.Pool
	Queries *db.Queries
	fault   func(string) error
}

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool, Queries: db.New(pool)} }

// WithFault is an in-process test seam only; no HTTP parameter or environment
// variable can enable failure injection in the running server.
func WithFault(pool *pgxpool.Pool, fault func(string) error) *Store {
	s := New(pool)
	s.fault = fault
	return s
}

func (s *Store) Checkpoint(stage string) error {
	if s.fault != nil {
		return s.fault(stage)
	}
	return nil
}

func (s *Store) Transaction(ctx context.Context, work func(*db.Queries) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err = work(s.Queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func Claim(ctx context.Context, q *db.Queries, operation string, key uuid.UUID, input any) (id uuid.UUID, replay bool, err error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return uuid.Nil, false, err
	}
	hash := sha256.Sum256(encoded)
	digest := hex.EncodeToString(hash[:])
	id, err = q.ClaimRequest(ctx, db.ClaimRequestParams{Operation: operation, Key: key, RequestHash: digest, ResourceID: uuid.New()})
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, err
	}
	// ON CONFLICT waited for the competing transaction. READ COMMITTED gives this
	// next statement a fresh snapshot containing its committed result.
	existing, err := q.GetRequest(ctx, db.GetRequestParams{Operation: operation, Key: key})
	if err != nil {
		return uuid.Nil, false, err
	}
	if existing.RequestHash != digest {
		return uuid.Nil, false, domain.Conflict("IDEMPOTENCY_CONFLICT", "같은 요청 키를 다른 입력에 사용할 수 없습니다.")
	}
	return existing.ResourceID, true, nil
}

func LockBalances(ctx context.Context, q *db.Queries, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
	ids = append([]uuid.UUID(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return bytes.Compare(ids[i][:], ids[j][:]) < 0 })
	balances := make(map[uuid.UUID]int64, len(ids))
	for _, id := range ids {
		if _, exists := balances[id]; exists {
			continue
		}
		row, err := q.LockBalance(ctx, id)
		if err != nil {
			return nil, err
		}
		balances[id] = row.Quantity
	}
	return balances, nil
}

func Move(ctx context.Context, q *db.Queries, balances map[uuid.UUID]int64, item uuid.UUID, delta int64, kind string, receipt, result *uuid.UUID) error {
	next := balances[item] + delta
	if next < 0 || next > domain.MaxStock {
		return domain.Conflict("STOCK_LIMIT", "현재고가 허용 범위를 벗어납니다.")
	}
	if delta == 0 {
		return errors.New("zero stock movement is forbidden")
	}
	if err := q.SetBalance(ctx, db.SetBalanceParams{ItemID: item, Quantity: next}); err != nil {
		return err
	}
	if err := q.CreateMovement(ctx, db.CreateMovementParams{ID: uuid.New(), ItemID: item, Delta: delta, MovementType: kind, ReceiptID: receipt, ResultID: result}); err != nil {
		return err
	}
	balances[item] = next
	return nil
}
