package inventory

import (
	"context"

	"example.com/assembly-erp/api/internal/db"
	"example.com/assembly-erp/api/internal/domain"
	"example.com/assembly-erp/api/internal/store"
	"github.com/google/uuid"
)

type Service struct{ DB *store.Store }

func New(database *store.Store) *Service { return &Service{DB: database} }

func receipt(row db.StockReceipt) domain.Receipt {
	return domain.Receipt{ID: row.ID, ItemID: row.ItemID, Quantity: row.Quantity, Note: row.Note, CreatedAt: row.CreatedAt.UTC()}
}

func (s *Service) Receive(ctx context.Context, key string, input domain.ReceiptInput) (domain.Receipt, bool, error) {
	var result domain.Receipt
	requestKey, err := domain.ID(key)
	if err != nil {
		return result, false, err
	}
	itemID, err := domain.ID(input.ItemID)
	if err != nil {
		return result, false, err
	}
	if err = domain.Quantity(input.Quantity, 1); err != nil {
		return result, false, err
	}
	if input.Note, err = domain.Text(input.Note, 0, 500, "입고 메모"); err != nil {
		return result, false, err
	}
	input.ItemID = itemID.String()
	var replay bool
	err = s.DB.Transaction(ctx, func(q *db.Queries) error {
		id, repeated, err := store.Claim(ctx, q, "stock_receipt", requestKey, input)
		if err != nil {
			return err
		}
		replay = repeated
		if replay {
			row, err := q.GetReceipt(ctx, id)
			result = receipt(row)
			return err
		}
		component, err := q.GetItem(ctx, itemID)
		if err != nil {
			return err
		}
		if component.Kind != "component" {
			return domain.Input("수동 입고는 부품만 가능합니다.")
		}
		balances, err := store.LockBalances(ctx, q, []uuid.UUID{itemID})
		if err != nil {
			return err
		}
		if balances[itemID]+input.Quantity > domain.MaxStock {
			return domain.Conflict("STOCK_LIMIT", "입고 후 현재고가 상한을 초과합니다.")
		}
		row, err := q.CreateReceipt(ctx, db.CreateReceiptParams{ID: id, ItemID: itemID, Quantity: input.Quantity, Note: input.Note})
		if err != nil {
			return err
		}
		if err = s.DB.Checkpoint("receipt.after_insert"); err != nil {
			return err
		}
		if err = store.Move(ctx, q, balances, itemID, input.Quantity, "manual_receipt", &id, nil); err != nil {
			return err
		}
		if err = s.DB.Checkpoint("receipt.after_movement"); err != nil {
			return err
		}
		result = receipt(row)
		return nil
	})
	return result, replay, err
}

func (s *Service) Receipt(ctx context.Context, id uuid.UUID) (domain.Receipt, error) {
	row, err := s.DB.Queries.GetReceipt(ctx, id)
	return receipt(row), err
}

func (s *Service) List(ctx context.Context, query domain.Query) (domain.Page[domain.Inventory], error) {
	out := domain.Page[domain.Inventory]{Data: []domain.Inventory{}}
	if err := domain.ValidateQuery(query); err != nil {
		return out, err
	}
	total, err := s.DB.Queries.CountItems(ctx, db.CountItemsParams{KindFilter: query.Kind, Search: query.Search})
	if err != nil {
		return out, err
	}
	rows, err := s.DB.Queries.ListInventory(ctx, db.ListInventoryParams{KindFilter: query.Kind, Search: query.Search, RowLimit: query.PageSize, RowOffset: query.Offset()})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out.Data = append(out.Data, domain.Inventory{ItemID: row.ItemID, Code: row.Code, Name: row.Name, Kind: row.Kind, Unit: row.Unit, Quantity: row.Quantity, UpdatedAt: row.UpdatedAt.UTC()})
	}
	out.Pagination = query.Pagination(total)
	return out, nil
}

func (s *Service) Movements(ctx context.Context, query domain.Query) (domain.Page[domain.Movement], error) {
	out := domain.Page[domain.Movement]{Data: []domain.Movement{}}
	if err := domain.ValidateQuery(query); err != nil {
		return out, err
	}
	total, err := s.DB.Queries.CountMovements(ctx, query.ItemID)
	if err != nil {
		return out, err
	}
	rows, err := s.DB.Queries.ListMovements(ctx, db.ListMovementsParams{ItemFilter: query.ItemID, RowLimit: query.PageSize, RowOffset: query.Offset()})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out.Data = append(out.Data, domain.Movement{ID: row.ID, ItemID: row.ItemID, Delta: row.Delta, MovementType: row.MovementType, ReceiptID: row.ReceiptID, ResultID: row.ResultID, CreatedAt: row.CreatedAt.UTC()})
	}
	out.Pagination = query.Pagination(total)
	return out, nil
}
