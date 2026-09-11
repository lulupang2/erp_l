package catalog

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

func item(row db.Item) domain.Item {
	return domain.Item{ID: row.ID, Code: row.Code, Name: row.Name, Kind: row.Kind, Unit: row.Unit, CreatedAt: row.CreatedAt.UTC()}
}

func (s *Service) Create(ctx context.Context, input domain.ItemInput) (domain.Item, error) {
	in, err := domain.NormalizeItem(input)
	if err != nil {
		return domain.Item{}, err
	}
	var result domain.Item
	err = s.DB.Transaction(ctx, func(q *db.Queries) error {
		row, err := q.CreateItem(ctx, db.CreateItemParams{ID: uuid.New(), Code: in.Code, Name: in.Name, Kind: in.Kind, Unit: in.Unit})
		if err != nil {
			return err
		}
		if err = q.CreateBalance(ctx, row.ID); err != nil {
			return err
		}
		result = item(row)
		return s.DB.Checkpoint("item.after_balance")
	})
	return result, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (domain.Item, error) {
	row, err := s.DB.Queries.GetItem(ctx, id)
	return item(row), err
}

func (s *Service) List(ctx context.Context, query domain.Query) (domain.Page[domain.Item], error) {
	out := domain.Page[domain.Item]{Data: []domain.Item{}}
	if err := domain.ValidateQuery(query); err != nil {
		return out, err
	}
	total, err := s.DB.Queries.CountItems(ctx, db.CountItemsParams{KindFilter: query.Kind, Search: query.Search})
	if err != nil {
		return out, err
	}
	rows, err := s.DB.Queries.ListItems(ctx, db.ListItemsParams{KindFilter: query.Kind, Search: query.Search, RowLimit: query.PageSize, RowOffset: query.Offset()})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out.Data = append(out.Data, item(row))
	}
	out.Pagination = query.Pagination(total)
	return out, nil
}

func bom(ctx context.Context, q *db.Queries, header db.Bom) (domain.BOM, error) {
	out := domain.BOM{ID: header.ID, FinishedItemID: header.FinishedItemID, UpdatedAt: header.UpdatedAt.UTC(), Components: []domain.Material{}}
	lines, err := q.ListBOMLines(ctx, header.ID)
	if err != nil {
		return out, err
	}
	for _, line := range lines {
		out.Components = append(out.Components, domain.Material{ItemID: line.ItemID, Code: line.Code, Name: line.Name, Unit: line.Unit, QuantityPerUnit: line.QuantityPerUnit})
	}
	return out, nil
}

func (s *Service) BOM(ctx context.Context, id uuid.UUID) (domain.BOM, error) {
	var result domain.BOM
	err := s.DB.Transaction(ctx, func(q *db.Queries) error {
		product, err := q.GetItem(ctx, id)
		if err != nil {
			return err
		}
		if product.Kind != "finished_good" {
			return domain.Input("BOM은 완제품에만 지정할 수 있습니다.")
		}
		// Lock the header so its timestamp and lines belong to the same revision.
		header, err := q.LockBOM(ctx, id)
		if err != nil {
			return err
		}
		result, err = bom(ctx, q, header)
		return err
	})
	return result, err
}

func (s *Service) ReplaceBOM(ctx context.Context, id uuid.UUID, input domain.BOMInput) (domain.BOM, error) {
	var result domain.BOM
	if len(input.Components) == 0 {
		return result, domain.Input("BOM에는 부품이 하나 이상 필요합니다.")
	}
	ids := make([]uuid.UUID, len(input.Components))
	seen := map[uuid.UUID]bool{}
	for index, component := range input.Components {
		parsed, err := domain.ID(component.ItemID)
		if err != nil {
			return result, err
		}
		if seen[parsed] {
			return result, domain.Input("BOM에 같은 부품을 중복으로 넣을 수 없습니다.")
		}
		if err := domain.Quantity(component.QuantityPerUnit, 1); err != nil {
			return result, err
		}
		seen[parsed] = true
		ids[index] = parsed
	}
	err := s.DB.Transaction(ctx, func(q *db.Queries) error {
		// Both initial BOM creation and order creation lock the parent first.
		// This also serializes the case in which there is no header to lock yet.
		product, err := q.LockItem(ctx, id)
		if err != nil {
			return err
		}
		if product.Kind != "finished_good" {
			return domain.Input("BOM은 완제품에만 지정할 수 있습니다.")
		}
		for _, componentID := range ids {
			component, err := q.GetItem(ctx, componentID)
			if err != nil {
				return err
			}
			if component.Kind != "component" {
				return domain.Input("BOM 자재에는 부품만 사용할 수 있습니다.")
			}
		}
		header, err := q.LockBOM(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			header, err = q.CreateBOM(ctx, db.CreateBOMParams{ID: uuid.New(), FinishedItemID: id})
		} else if err == nil {
			header, err = q.TouchBOM(ctx, header.ID)
		}
		if err != nil {
			return err
		}
		if err = q.DeleteBOMLines(ctx, header.ID); err != nil {
			return err
		}
		for i, line := range input.Components {
			if err = q.CreateBOMLine(ctx, db.CreateBOMLineParams{BomID: header.ID, ComponentItemID: ids[i], QuantityPerUnit: line.QuantityPerUnit}); err != nil {
				return err
			}
		}
		if err = s.DB.Checkpoint("bom.after_replace"); err != nil {
			return err
		}
		result, err = bom(ctx, q, header)
		return err
	})
	return result, err
}
