package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/assembly-erp/api/internal/catalog"
	"example.com/assembly-erp/api/internal/domain"
	"example.com/assembly-erp/api/internal/inventory"
	"example.com/assembly-erp/api/internal/migrate"
	"example.com/assembly-erp/api/internal/platform"
	"example.com/assembly-erp/api/internal/production"
	"example.com/assembly-erp/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type harness struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	db         *store.Store
	items      *catalog.Service
	stock      *inventory.Service
	production *production.Service
}
type fixture struct {
	a, b, product domain.Item
	order         domain.OrderDetail
}

func (h harness) newItem(t *testing.T, kind string) domain.Item {
	t.Helper()
	code := "T_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	item, err := h.items.Create(h.ctx, domain.ItemInput{Code: code, Name: code, Kind: kind, Unit: "EA"})
	if err != nil {
		t.Fatal("fixture item creation failed", domain.PublicError(err).Code)
	}
	return item
}

func (h harness) recipe(t *testing.T) fixture {
	t.Helper()
	f := fixture{a: h.newItem(t, "component"), b: h.newItem(t, "component"), product: h.newItem(t, "finished_good")}
	_, err := h.items.ReplaceBOM(h.ctx, f.product.ID, domain.BOMInput{Components: []domain.ComponentInput{{ItemID: f.a.ID.String(), QuantityPerUnit: 1}, {ItemID: f.b.ID.String(), QuantityPerUnit: 2}}})
	if err != nil {
		t.Fatal("fixture BOM failed")
	}
	for _, row := range []struct {
		id       uuid.UUID
		quantity int64
	}{{f.a.ID, 10}, {f.b.ID, 20}} {
		if _, _, err = h.stock.Receive(h.ctx, uuid.NewString(), domain.ReceiptInput{ItemID: row.id.String(), Quantity: row.quantity}); err != nil {
			t.Fatal("fixture receipt failed")
		}
	}
	f.order, _, err = h.production.Create(h.ctx, uuid.NewString(), domain.OrderInput{FinishedItemID: f.product.ID.String(), PlannedQuantity: 5})
	if err != nil {
		t.Fatal("fixture order failed")
	}
	return f
}

func (h harness) snapshot(t *testing.T) string {
	t.Helper()
	tx, err := h.pool.BeginTx(h.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal("snapshot transaction failed")
	}
	defer tx.Rollback(context.Background())
	var snapshot strings.Builder
	for _, table := range []string{"items", "boms", "bom_lines", "inventory_balances", "stock_receipts", "production_orders", "production_order_materials", "production_results", "stock_movements", "idempotency_requests"} {
		var value string
		if err := tx.QueryRow(h.ctx, "SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM "+table+" t").Scan(&value); err != nil {
			t.Fatal("snapshot query failed")
		}
		snapshot.WriteString(table + value)
	}
	return snapshot.String()
}

func status(t *testing.T, err error, wanted int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected HTTP-equivalent error %d, received success", wanted)
	}
	public := domain.PublicError(err)
	if public.Status != wanted {
		t.Fatalf("error status=%d code=%s; want=%d", public.Status, public.Code, wanted)
	}
}

func (h harness) balance(t *testing.T, id uuid.UUID) int64 {
	t.Helper()
	row, err := h.db.Queries.GetBalance(h.ctx, id)
	if err != nil {
		t.Fatal("balance query failed")
	}
	return row.Quantity
}

func TestPostgreSQLIntegration(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL absent: real PostgreSQL integration was NOT executed")
	}
	if err := platform.RequireLocalDatabase(raw, "erp_test", "55433"); err != nil {
		t.Fatal("unsafe test database target refused")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := migrate.Apply(ctx, raw, "up"); err != nil {
		t.Fatal("test migration failed; connection details withheld")
	}
	pool, err := platform.Open(ctx, raw)
	if err != nil {
		t.Fatal("test database connection failed; connection details withheld")
	}
	defer pool.Close()
	database := store.New(pool)
	h := harness{ctx, pool, database, catalog.New(database), inventory.New(database), production.New(database)}

	t.Run("partial-production-and-all-defective", func(t *testing.T) {
		f := h.recipe(t)
		_, _, err := h.production.Record(ctx, f.order.ID, uuid.NewString(), domain.ResultInput{GoodQuantity: 2, DefectiveQuantity: 1})
		if err != nil {
			t.Fatal("partial result failed", domain.PublicError(err).Code)
		}
		current, err := h.production.Get(ctx, f.order.ID)
		if err != nil || current.GoodQuantity != 2 || current.DefectiveQuantity != 1 || current.RemainingQuantity != 2 || current.Status != "in_progress" {
			t.Fatal("partial aggregation mismatch")
		}
		_, _, err = h.production.Record(ctx, f.order.ID, uuid.NewString(), domain.ResultInput{GoodQuantity: 0, DefectiveQuantity: 2})
		if err != nil {
			t.Fatal("all-defective remainder failed")
		}
		current, _ = h.production.Get(ctx, f.order.ID)
		if current.Status != "completed" || current.GoodQuantity != 2 || current.DefectiveQuantity != 3 {
			t.Fatal("completed totals mismatch")
		}
		if h.balance(t, f.a.ID) != 5 || h.balance(t, f.b.ID) != 10 || h.balance(t, f.product.ID) != 2 {
			t.Fatal("production inventory mismatch")
		}
	})

	for _, stage := range []string{"result.after_insert", "result.after_consumption", "result.after_totals"} {
		t.Run("atomic-rollback-"+stage, func(t *testing.T) {
			f := h.recipe(t)
			before := h.snapshot(t)
			key := uuid.NewString()
			faulty := store.WithFault(pool, func(point string) error {
				if point == stage {
					return errors.New("injected write failure")
				}
				return nil
			})
			_, _, err := production.New(faulty).Record(ctx, f.order.ID, key, domain.ResultInput{GoodQuantity: 1, DefectiveQuantity: 1})
			status(t, err, 500)
			if after := h.snapshot(t); after != before {
				t.Fatal("partial writes survived the failed transaction")
			}
			_, replay, err := h.production.Record(ctx, f.order.ID, key, domain.ResultInput{GoodQuantity: 1, DefectiveQuantity: 1})
			if err != nil || replay {
				t.Fatal("failed request key was persisted or retry failed")
			}
		})
	}

	for _, stage := range []string{"receipt.after_insert", "receipt.after_movement"} {
		t.Run("atomic-rollback-"+stage, func(t *testing.T) {
			component := h.newItem(t, "component")
			before := h.snapshot(t)
			key := uuid.NewString()
			faulty := store.WithFault(pool, func(point string) error {
				if point == stage {
					return errors.New("injected write failure")
				}
				return nil
			})
			_, _, err := inventory.New(faulty).Receive(ctx, key, domain.ReceiptInput{ItemID: component.ID.String(), Quantity: 4})
			status(t, err, 500)
			if h.snapshot(t) != before {
				t.Fatal("receipt failed to roll back every table")
			}
			_, replay, err := h.stock.Receive(ctx, key, domain.ReceiptInput{ItemID: component.ID.String(), Quantity: 4})
			if err != nil || replay {
				t.Fatal("failed receipt request key was persisted")
			}
		})
	}

	t.Run("atomic-item-bom-and-order", func(t *testing.T) {
		f := h.recipe(t)
		for _, stage := range []string{"item.after_balance", "bom.after_replace", "order.after_snapshot"} {
			before := h.snapshot(t)
			faulty := store.WithFault(pool, func(point string) error {
				if point == stage {
					return errors.New("injected write failure")
				}
				return nil
			})
			var err error
			switch stage {
			case "item.after_balance":
				_, err = catalog.New(faulty).Create(ctx, domain.ItemInput{Code: "FAIL_" + uuid.NewString()[:8], Name: "failure", Kind: "component", Unit: "EA"})
			case "bom.after_replace":
				_, err = catalog.New(faulty).ReplaceBOM(ctx, f.product.ID, domain.BOMInput{Components: []domain.ComponentInput{{ItemID: f.a.ID.String(), QuantityPerUnit: 99}}})
			case "order.after_snapshot":
				_, _, err = production.New(faulty).Create(ctx, uuid.NewString(), domain.OrderInput{FinishedItemID: f.product.ID.String(), PlannedQuantity: 2})
			}
			status(t, err, 500)
			if h.snapshot(t) != before {
				t.Fatal("atomic rollback failed at", stage)
			}
		}
	})

	t.Run("same-key-concurrency-and-route-bound-hash", func(t *testing.T) {
		f := h.recipe(t)
		key := uuid.NewString()
		start := make(chan struct{})
		var wg sync.WaitGroup
		type outcome struct {
			result domain.Result
			replay bool
			err    error
		}
		outcomes := make(chan outcome, 8)
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				r, replay, err := h.production.Record(ctx, f.order.ID, key, domain.ResultInput{GoodQuantity: 1, DefectiveQuantity: 1})
				outcomes <- outcome{r, replay, err}
			}()
		}
		close(start)
		wg.Wait()
		close(outcomes)
		created := 0
		var id uuid.UUID
		for outcome := range outcomes {
			if outcome.err != nil {
				t.Fatal("concurrent same-key request failed", domain.PublicError(outcome.err).Code)
			}
			if !outcome.replay {
				created++
			}
			if id == uuid.Nil {
				id = outcome.result.ID
			}
			if outcome.result.ID != id {
				t.Fatal("same-key requests produced different resources")
			}
		}
		if created != 1 {
			t.Fatalf("same-key created %d resources", created)
		}
		other, _, err := h.production.Create(ctx, uuid.NewString(), domain.OrderInput{FinishedItemID: f.product.ID.String(), PlannedQuantity: 2})
		if err != nil {
			t.Fatal("fixture order failed")
		}
		_, _, err = h.production.Record(ctx, other.ID, key, domain.ResultInput{GoodQuantity: 1, DefectiveQuantity: 1})
		status(t, err, 409)
	})

	t.Run("BOM-replacement-and-snapshot-serialization", func(t *testing.T) {
		f := h.recipe(t)
		for iteration := range 10 {
			start := make(chan struct{})
			changes := make(chan error, 1)
			a, b := int64(1), int64(2)
			if iteration%2 == 0 {
				a, b = 3, 4
			}
			go func() {
				<-start
				_, err := h.items.ReplaceBOM(ctx, f.product.ID, domain.BOMInput{Components: []domain.ComponentInput{{ItemID: f.a.ID.String(), QuantityPerUnit: a}, {ItemID: f.b.ID.String(), QuantityPerUnit: b}}})
				changes <- err
			}()
			close(start)
			created, _, err := h.production.Create(ctx, uuid.NewString(), domain.OrderInput{FinishedItemID: f.product.ID.String(), PlannedQuantity: 1})
			changeErr := <-changes
			if err != nil || changeErr != nil {
				t.Fatal("concurrent BOM operation failed")
			}
			quantities := map[uuid.UUID]int64{}
			for _, m := range created.Materials {
				quantities[m.ItemID] = m.QuantityPerUnit
			}
			if !((quantities[f.a.ID] == 1 && quantities[f.b.ID] == 2) || (quantities[f.a.ID] == 3 && quantities[f.b.ID] == 4)) {
				t.Fatal("mixed BOM revisions copied")
			}
		}
		original, _ := h.production.Get(ctx, f.order.ID)
		quantities := map[uuid.UUID]int64{}
		for _, m := range original.Materials {
			quantities[m.ItemID] = m.QuantityPerUnit
		}
		if quantities[f.a.ID] != 1 || quantities[f.b.ID] != 2 {
			t.Fatal("old order snapshot mutated")
		}
	})

	t.Run("lock-timeout-rolls-back-key", func(t *testing.T) {
		component := h.newItem(t, "component")
		key := uuid.NewString()
		lock, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal("lock transaction failed")
		}
		defer lock.Rollback(context.Background())
		if _, err = lock.Exec(ctx, "SELECT item_id FROM inventory_balances WHERE item_id=$1 FOR UPDATE", component.ID); err != nil {
			t.Fatal("lock setup failed")
		}
		short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		_, _, err = h.stock.Receive(short, key, domain.ReceiptInput{ItemID: component.ID.String(), Quantity: 1})
		cancel()
		status(t, err, 503)
		if err = lock.Rollback(ctx); err != nil {
			t.Fatal("lock cleanup failed")
		}
		_, replay, err := h.stock.Receive(ctx, key, domain.ReceiptInput{ItemID: component.ID.String(), Quantity: 1})
		if err != nil || replay || h.balance(t, component.ID) != 1 {
			t.Fatal("timed-out request committed partial data")
		}
	})

	t.Run("stock-upper-bound-rejects-all-writes", func(t *testing.T) {
		f := h.recipe(t)
		for _, id := range []uuid.UUID{f.a.ID, f.product.ID} {
			_, err := pool.Exec(ctx, "UPDATE inventory_balances SET quantity=$2 WHERE item_id=$1", id, domain.MaxStock)
			if err != nil {
				t.Fatal("upper-bound fixture failed")
			}
			t.Cleanup(func() {
				_, err := pool.Exec(context.Background(), "UPDATE inventory_balances SET quantity=(SELECT COALESCE(sum(delta),0) FROM stock_movements WHERE item_id=$1) WHERE item_id=$1", id)
				if err != nil {
					t.Error("upper-bound fixture cleanup failed")
				}
			})
		}
		before := h.snapshot(t)
		_, _, err := h.stock.Receive(ctx, uuid.NewString(), domain.ReceiptInput{ItemID: f.a.ID.String(), Quantity: 1})
		status(t, err, 409)
		if h.snapshot(t) != before {
			t.Fatal("overflow receipt partially committed")
		}
		_, _, err = h.production.Record(ctx, f.order.ID, uuid.NewString(), domain.ResultInput{GoodQuantity: 1})
		status(t, err, 409)
		if h.snapshot(t) != before {
			t.Fatal("overflow production partially committed")
		}
	})

	t.Run("database-check-and-foreign-key-constraints", func(t *testing.T) {
		component := h.newItem(t, "component")
		for _, value := range []int64{-1, domain.MaxStock + 1} {
			tx, _ := pool.Begin(ctx)
			_, err := tx.Exec(ctx, "UPDATE inventory_balances SET quantity=$2 WHERE item_id=$1", component.ID, value)
			tx.Rollback(ctx)
			var pg *pgconn.PgError
			if !errors.As(err, &pg) || pg.Code != "23514" {
				t.Fatal("database quantity CHECK not enforced")
			}
		}
		tx, _ := pool.Begin(ctx)
		_, err := tx.Exec(ctx, "INSERT INTO inventory_balances(item_id) VALUES($1)", uuid.New())
		tx.Rollback(ctx)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "23503" {
			t.Fatal("database foreign key not enforced")
		}
	})

	t.Run("global-ledger-and-aggregate-reconciliation", func(t *testing.T) {
		for name, sql := range map[string]string{
			"inventory": `SELECT count(*) FROM inventory_balances b LEFT JOIN (SELECT item_id,sum(delta) quantity FROM stock_movements GROUP BY item_id) m USING(item_id) WHERE b.quantity<>COALESCE(m.quantity,0)`,
			"orders":    `SELECT count(*) FROM production_orders o LEFT JOIN (SELECT order_id,sum(good_quantity) good,sum(defective_quantity) defective FROM production_results GROUP BY order_id) r ON r.order_id=o.id WHERE o.good_quantity<>COALESCE(r.good,0) OR o.defective_quantity<>COALESCE(r.defective,0)`,
		} {
			var failures int64
			if err := pool.QueryRow(ctx, sql).Scan(&failures); err != nil || failures != 0 {
				t.Fatal(fmt.Sprintf("%s invariant mismatches: %d", name, failures))
			}
		}
	})
}
