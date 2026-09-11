package production

import (
	"context"
	"errors"

	"example.com/assembly-erp/api/internal/db"
	"example.com/assembly-erp/api/internal/domain"
	"example.com/assembly-erp/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Service struct{ DB *store.Store }

func New(database *store.Store) *Service { return &Service{DB: database} }

func order(row db.ProductionOrder) domain.Order {
	remaining, status := domain.OrderState(row.PlannedQuantity, row.GoodQuantity, row.DefectiveQuantity)
	return domain.Order{ID: row.ID, FinishedItemID: row.FinishedItemID, PlannedQuantity: row.PlannedQuantity, GoodQuantity: row.GoodQuantity, DefectiveQuantity: row.DefectiveQuantity, RemainingQuantity: remaining, Status: status, CreatedAt: row.CreatedAt.UTC()}
}

func detail(ctx context.Context, q *db.Queries, row db.ProductionOrder) (domain.OrderDetail, error) {
	out := domain.OrderDetail{Order: order(row), Materials: []domain.Material{}}
	materials, err := q.ListOrderMaterials(ctx, row.ID)
	if err != nil {
		return out, err
	}
	for _, m := range materials {
		out.Materials = append(out.Materials, domain.Material{ItemID: m.ItemID, Code: m.Code, Name: m.Name, Unit: m.Unit, QuantityPerUnit: m.QuantityPerUnit})
	}
	return out, nil
}

func result(row db.ProductionResult) domain.Result {
	return domain.Result{ID: row.ID, OrderID: row.OrderID, GoodQuantity: row.GoodQuantity, DefectiveQuantity: row.DefectiveQuantity, Note: row.Note, CreatedAt: row.CreatedAt.UTC()}
}

func (s *Service) Create(ctx context.Context, key string, input domain.OrderInput) (domain.OrderDetail, bool, error) {
	var out domain.OrderDetail
	requestKey, err := domain.ID(key)
	if err != nil {
		return out, false, err
	}
	finishedID, err := domain.ID(input.FinishedItemID)
	if err != nil {
		return out, false, err
	}
	if err = domain.Quantity(input.PlannedQuantity, 1); err != nil {
		return out, false, err
	}
	input.FinishedItemID = finishedID.String()
	var replay bool
	err = s.DB.Transaction(ctx, func(q *db.Queries) error {
		id, repeated, err := store.Claim(ctx, q, "production_order", requestKey, input)
		if err != nil {
			return err
		}
		replay = repeated
		if replay {
			row, err := q.GetOrder(ctx, id)
			if err != nil {
				return err
			}
			out, err = detail(ctx, q, row)
			return err
		}
		finished, err := q.LockItem(ctx, finishedID)
		if err != nil {
			return err
		}
		if finished.Kind != "finished_good" {
			return domain.Input("완제품만 생산 지시를 생성할 수 있습니다.")
		}
		header, err := q.LockBOM(ctx, finishedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Conflict("BOM_REQUIRED", "BOM을 등록한 후 생산 지시를 생성해 주세요.")
		}
		if err != nil {
			return err
		}
		materials, err := q.ListBOMLines(ctx, header.ID)
		if err != nil {
			return err
		}
		if len(materials) == 0 {
			return domain.Conflict("BOM_REQUIRED", "BOM에는 부품이 하나 이상 필요합니다.")
		}
		row, err := q.CreateOrder(ctx, db.CreateOrderParams{ID: id, FinishedItemID: finishedID, PlannedQuantity: input.PlannedQuantity})
		if err != nil {
			return err
		}
		for _, m := range materials {
			if err = q.CreateOrderMaterial(ctx, db.CreateOrderMaterialParams{OrderID: id, ComponentItemID: m.ItemID, QuantityPerUnit: m.QuantityPerUnit}); err != nil {
				return err
			}
		}
		if err = s.DB.Checkpoint("order.after_snapshot"); err != nil {
			return err
		}
		out, err = detail(ctx, q, row)
		return err
	})
	return out, replay, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (domain.OrderDetail, error) {
	row, err := s.DB.Queries.GetOrder(ctx, id)
	if err != nil {
		return domain.OrderDetail{}, err
	}
	return detail(ctx, s.DB.Queries, row)
}

func (s *Service) List(ctx context.Context, query domain.Query) (domain.Page[domain.Order], error) {
	out := domain.Page[domain.Order]{Data: []domain.Order{}}
	if err := domain.ValidateQuery(query); err != nil {
		return out, err
	}
	total, err := s.DB.Queries.CountOrders(ctx, query.Status)
	if err != nil {
		return out, err
	}
	rows, err := s.DB.Queries.ListOrders(ctx, db.ListOrdersParams{StatusFilter: query.Status, RowLimit: query.PageSize, RowOffset: query.Offset()})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out.Data = append(out.Data, order(row))
	}
	out.Pagination = query.Pagination(total)
	return out, nil
}

func (s *Service) Record(ctx context.Context, orderID uuid.UUID, key string, input domain.ResultInput) (domain.Result, bool, error) {
	var out domain.Result
	requestKey, err := domain.ID(key)
	if err != nil {
		return out, false, err
	}
	if err = domain.Quantity(input.GoodQuantity, 0); err != nil {
		return out, false, err
	}
	if err = domain.Quantity(input.DefectiveQuantity, 0); err != nil {
		return out, false, err
	}
	processed := input.GoodQuantity + input.DefectiveQuantity
	if processed == 0 {
		return out, false, domain.Input("양품과 불량의 합계는 1 이상이어야 합니다.")
	}
	if input.Note, err = domain.Text(input.Note, 0, 500, "실적 메모"); err != nil {
		return out, false, err
	}
	canonical := struct {
		OrderID uuid.UUID          `json:"order_id"`
		Input   domain.ResultInput `json:"input"`
	}{orderID, input}
	var replay bool
	err = s.DB.Transaction(ctx, func(q *db.Queries) error {
		id, repeated, err := store.Claim(ctx, q, "production_result", requestKey, canonical)
		if err != nil {
			return err
		}
		replay = repeated
		if replay {
			row, err := q.GetResult(ctx, id)
			out = result(row)
			return err
		}
		current, err := q.LockOrder(ctx, orderID)
		if err != nil {
			return err
		}
		remaining := current.PlannedQuantity - current.GoodQuantity - current.DefectiveQuantity
		if remaining == 0 {
			return domain.Conflict("ORDER_COMPLETED", "완료된 지시에는 새로운 실적을 등록할 수 없습니다.")
		}
		if processed > remaining {
			return domain.Conflict("PLAN_EXCEEDED", "이번 처리 수량이 잔여 생산 수량을 초과합니다.")
		}
		materials, err := q.ListOrderMaterials(ctx, orderID)
		if err != nil {
			return err
		}
		if len(materials) == 0 {
			return errors.New("order material snapshot is missing")
		}
		ids := []uuid.UUID{current.FinishedItemID}
		for _, m := range materials {
			ids = append(ids, m.ItemID)
		}
		balances, err := store.LockBalances(ctx, q, ids)
		if err != nil {
			return err
		}
		shortages := []domain.Shortage{}
		for _, m := range materials {
			required := m.QuantityPerUnit * processed
			if balances[m.ItemID] < required {
				shortages = append(shortages, domain.Shortage{ItemID: m.ItemID, RequiredQuantity: required, AvailableQuantity: balances[m.ItemID]})
			}
		}
		if len(shortages) > 0 {
			problem := domain.Conflict("INSUFFICIENT_STOCK", "생산에 필요한 부품 재고가 부족합니다.")
			problem.Details = struct {
				Shortages []domain.Shortage `json:"shortages"`
			}{shortages}
			return problem
		}
		if balances[current.FinishedItemID]+input.GoodQuantity > domain.MaxStock {
			return domain.Conflict("STOCK_LIMIT", "완제품 현재고가 상한을 초과합니다.")
		}
		row, err := q.CreateResult(ctx, db.CreateResultParams{ID: id, OrderID: orderID, GoodQuantity: input.GoodQuantity, DefectiveQuantity: input.DefectiveQuantity, Note: input.Note})
		if err != nil {
			return err
		}
		if err = s.DB.Checkpoint("result.after_insert"); err != nil {
			return err
		}
		for _, m := range materials {
			if err = store.Move(ctx, q, balances, m.ItemID, -m.QuantityPerUnit*processed, "production_consumption", nil, &id); err != nil {
				return err
			}
			if err = s.DB.Checkpoint("result.after_consumption"); err != nil {
				return err
			}
		}
		if input.GoodQuantity > 0 {
			if err = store.Move(ctx, q, balances, current.FinishedItemID, input.GoodQuantity, "production_receipt", nil, &id); err != nil {
				return err
			}
		}
		if _, err = q.AddOrderTotals(ctx, db.AddOrderTotalsParams{ID: orderID, GoodQuantity: input.GoodQuantity, DefectiveQuantity: input.DefectiveQuantity}); err != nil {
			return err
		}
		if err = s.DB.Checkpoint("result.after_totals"); err != nil {
			return err
		}
		out = result(row)
		return nil
	})
	return out, replay, err
}

func (s *Service) Result(ctx context.Context, id uuid.UUID) (domain.Result, error) {
	row, err := s.DB.Queries.GetResult(ctx, id)
	return result(row), err
}

func (s *Service) Results(ctx context.Context, orderID uuid.UUID, query domain.Query) (domain.Page[domain.Result], error) {
	out := domain.Page[domain.Result]{Data: []domain.Result{}}
	if err := domain.ValidateQuery(query); err != nil {
		return out, err
	}
	if _, err := s.DB.Queries.GetOrder(ctx, orderID); err != nil {
		return out, err
	}
	total, err := s.DB.Queries.CountResults(ctx, orderID)
	if err != nil {
		return out, err
	}
	rows, err := s.DB.Queries.ListResults(ctx, db.ListResultsParams{OrderID: orderID, RowLimit: query.PageSize, RowOffset: query.Offset()})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out.Data = append(out.Data, result(row))
	}
	out.Pagination = query.Pagination(total)
	return out, nil
}
