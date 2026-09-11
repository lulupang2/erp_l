package demo

import (
	"context"
	"errors"

	"example.com/assembly-erp/api/internal/catalog"
	"example.com/assembly-erp/api/internal/domain"
	"example.com/assembly-erp/api/internal/inventory"
	"example.com/assembly-erp/api/internal/platform"
	"example.com/assembly-erp/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func Execute(ctx context.Context, rawURL, action, confirmation string) error {
	if action != "seed" && action != "reset" {
		return errors.New("unknown demo command")
	}
	if action == "reset" && confirmation != "ERP_DEMO_RESET" {
		return errors.New("explicit demo reset confirmation is required")
	}
	if err := platform.RequireLocalDatabase(rawURL, "erp_demo", "55432"); err != nil {
		return err
	}
	pool, err := platform.Open(ctx, rawURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if action == "reset" {
		_, err = pool.Exec(ctx, `TRUNCATE stock_movements, production_results, production_order_materials,
			production_orders, stock_receipts, inventory_balances, bom_lines, boms, items, idempotency_requests`)
		return err
	}
	s := store.New(pool)
	items, stock := catalog.New(s), inventory.New(s)
	ensureItem := func(code, name, kind string) (domain.Item, error) {
		rows, err := items.List(ctx, domain.Query{Page: 1, PageSize: 100, Search: code})
		if err != nil {
			return domain.Item{}, err
		}
		for _, item := range rows.Data {
			if item.Code == code {
				if item.Name != name || item.Kind != kind || item.Unit != "개" {
					return item, errors.New("demo item conflicts with existing data")
				}
				return item, nil
			}
		}
		return items.Create(ctx, domain.ItemInput{Code: code, Name: name, Kind: kind, Unit: "개"})
	}
	cases, err := ensureItem("CASE", "케이스", "component")
	if err != nil {
		return err
	}
	switches, err := ensureItem("SWITCH", "스위치", "component")
	if err != nil {
		return err
	}
	keyboard, err := ensureItem("KEYBOARD", "키보드", "finished_good")
	if err != nil {
		return err
	}
	wanted := map[uuid.UUID]int64{cases.ID: 1, switches.ID: 80}
	current, err := items.BOM(ctx, keyboard.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = items.ReplaceBOM(ctx, keyboard.ID, domain.BOMInput{Components: []domain.ComponentInput{{ItemID: cases.ID.String(), QuantityPerUnit: 1}, {ItemID: switches.ID.String(), QuantityPerUnit: 80}}})
	} else if err == nil {
		if len(current.Components) != 2 {
			return errors.New("demo BOM conflicts with existing data")
		}
		for _, line := range current.Components {
			if wanted[line.ItemID] != line.QuantityPerUnit {
				return errors.New("demo BOM conflicts with existing data")
			}
		}
	}
	if err != nil {
		return err
	}
	for _, entry := range []struct {
		Item     domain.Item
		Quantity int64
	}{{cases, 10}, {switches, 800}} {
		key := uuid.NewSHA1(uuid.NameSpaceURL, []byte("assembly-erp/demo/v1/"+entry.Item.Code)).String()
		_, _, err = stock.Receive(ctx, key, domain.ReceiptInput{ItemID: entry.Item.ID.String(), Quantity: entry.Quantity, Note: "데모 초기 입고"})
		if err != nil {
			return err
		}
	}
	return nil
}
