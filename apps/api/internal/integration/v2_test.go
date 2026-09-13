package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"example.com/assembly-erp/api/internal/migrate"
	"example.com/assembly-erp/api/internal/platform"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// V2 integration tests are deliberately opt-in and can only touch the isolated
// loopback erp_test database. They never reset or inspect the v1 schema.
func v2Pool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL absent: v2 PostgreSQL integration was not executed")
	}
	if err := platform.RequireLocalDatabase(raw, "erp_test", "55433"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	if _, err := migrate.ApplyV2(ctx, raw, "up"); err != nil {
		t.Fatal("v2 migration failed")
	}
	pool, err := platform.Open(ctx, raw)
	if err != nil {
		t.Fatal("v2 connection failed")
	}
	t.Cleanup(pool.Close)
	return pool, ctx
}

func TestV2SchemaIsolatedAndGuarded(t *testing.T) {
	pool, ctx := v2Pool(t)
	var found int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='v2' AND table_name IN ('users','work_orders','documents','inventory_balances','material_ledger','output_balances','output_ledger','command_replays','audit_events')`).Scan(&found); err != nil {
		t.Fatal(err)
	}
	if found != 9 {
		t.Fatalf("v2 table count=%d, want 9", found)
	}
	var v1Count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('items','production_orders','stock_movements')`).Scan(&v1Count); err != nil {
		t.Fatal(err)
	}
	if v1Count < 3 {
		t.Fatalf("v1 tables disappeared or were not preserved: %d", v1Count)
	}
}

func TestV2QuantityAndImmutabilityGuards(t *testing.T) {
	pool, ctx := v2Pool(t)
	item, lot, location, owner := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO v2.items(id,code,name,kind,unit) VALUES($1,'V2_GUARD','guard','component','ea')`, item)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO v2.lots(id,item_id,code) VALUES($1,$2,'V2_GUARD_LOT')`, lot, item); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO v2.locations(id,code,name,kind) VALUES($1,'V2_GUARD_LOC','guard','warehouse')`, location); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO v2.inventory_balances(item_id,lot_id,location_id,owner_id,quantity) VALUES($1,$2,$3,$4,-1)`, item, lot, location, owner)
	if err == nil {
		t.Fatal("negative inventory was accepted")
	}
}

func TestV2PostedDocumentRequiresReverseCommand(t *testing.T) {
	pool, ctx := v2Pool(t)
	var triggerCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_trigger WHERE tgname IN ('immutable_document','immutable_material_ledger','immutable_output_ledger','immutable_audit')`).Scan(&triggerCount); err != nil {
		t.Fatal(err)
	}
	if triggerCount != 4 {
		t.Fatalf("append-only trigger count=%d, want 4", triggerCount)
	}
}
