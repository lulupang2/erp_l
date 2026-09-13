package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"example.com/assembly-erp/api/internal/factory"
	"example.com/assembly-erp/api/internal/migrate"
	"example.com/assembly-erp/api/internal/platform"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type accEnv struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

type accHarness struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	app    *fiber.App
	actors map[string]factory.Actor
	refs   map[string]uuid.UUID
}

func accDB(raw string) error {
	return platform.RequireLocalDatabase(raw, "erp_test", "55433")
}

func TestV2FactoryAcceptanceResetGuard(t *testing.T) {
	for _, raw := range []string{
		"postgres://u:x@db.example.com:5432/erp_test?sslmode=verify-full",
		"postgres://u:x@127.0.0.1:55433/erp_demo",
		"postgres://u:x@127.0.0.1:5432/erp_test",
	} {
		if accDB(raw) == nil {
			t.Fatalf("unsafe reset target allowed: %s", raw)
		}
	}
}

func newAcc(t *testing.T) *accHarness {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL absent: v2 factory acceptance not executed")
	}
	if err := accDB(raw); err != nil {
		t.Fatal("unsafe acceptance database refused")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	p, err := platform.Open(ctx, raw)
	if err != nil {
		t.Fatal("acceptance database connect failed")
	}
	if _, err = p.Exec(ctx, "DROP SCHEMA IF EXISTS v2 CASCADE"); err != nil {
		p.Close()
		t.Fatal("isolated v2 reset failed")
	}
	p.Close()
	if _, err = migrate.ApplyV2(ctx, raw, "up"); err != nil {
		t.Fatal("v2 migration failed")
	}
	p, err = platform.Open(ctx, raw)
	if err != nil {
		t.Fatal("acceptance pool failed")
	}
	t.Cleanup(p.Close)
	h := &accHarness{
		ctx:    ctx,
		pool:   p,
		actors: map[string]factory.Actor{},
		refs:   map[string]uuid.UUID{},
	}
	for name, role := range map[string]string{
		"admin":     "admin",
		"planner":   "planner",
		"materials": "materials",
		"operator":  "operator",
		"operator2": "operator",
		"quality":   "quality",
	} {
		h.actor(t, name, role)
	}
	h.app = fiber.New()
	factory.Register(h.app, factory.New(p), func(c fiber.Ctx) error {
		a, ok := h.actors[c.Get("X-Test-Actor")]
		if !ok {
			return c.SendStatus(401)
		}
		c.Locals("v2_actor", a)
		return c.Next()
	})
	t.Cleanup(func() { _ = h.app.Shutdown() })
	h.seed(t)
	return h
}

func (h *accHarness) actor(t *testing.T, name, role string) {
	a := factory.Actor{ID: uuid.New(), Roles: []string{role}}
	if _, e := h.pool.Exec(h.ctx, "INSERT INTO v2.users(id,username,password_hash) VALUES($1,$2,$3)", a.ID, "acc_"+name, "test-only"); e != nil {
		t.Fatal(e)
	}
	if _, e := h.pool.Exec(h.ctx, "INSERT INTO v2.user_roles(user_id,role) VALUES($1,$2)", a.ID, role); e != nil {
		t.Fatal(e)
	}
	h.actors[name] = a
}

func (h *accHarness) call(actor, method, path string, body any, key uuid.UUID) (int, accEnv, error) {
	var r io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return 0, accEnv{}, e
		}
		r = bytes.NewReader(b)
	}
	q := httptest.NewRequest(method, "http://127.0.0.1"+path, r)
	q.Header.Set("X-Test-Actor", actor)
	if body != nil {
		q.Header.Set("Content-Type", "application/json")
	}
	if key != uuid.Nil {
		q.Header.Set("Idempotency-Key", key.String())
	}
	resp, e := h.app.Test(q)
	if e != nil {
		return 0, accEnv{}, e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(resp.Body)
	if e != nil {
		return resp.StatusCode, accEnv{}, e
	}
	var out accEnv
	if len(b) > 0 {
		e = json.Unmarshal(b, &out)
	}
	return resp.StatusCode, out, e
}

func (h *accHarness) expect(t *testing.T, a, m, p string, b any, k uuid.UUID, w int) accEnv {
	t.Helper()
	if k == uuid.Nil && m == http.MethodPost {
		k = uuid.New()
	}
	s, o, e := h.call(a, m, p, b, k)
	if e != nil {
		t.Fatal(e)
	}
	if s != w {
		c := ""
		if o.Error != nil {
			c = o.Error.Code
		}
		t.Fatalf("%s %s status=%d code=%s want=%d", m, p, s, c, w)
	}
	return o
}

func accID(t *testing.T, e accEnv) uuid.UUID {
	var v struct {
		ID uuid.UUID `json:"id"`
	}
	if json.Unmarshal(e.Data, &v) != nil || v.ID == uuid.Nil {
		t.Fatal("missing id")
	}
	return v.ID
}

func accCode(t *testing.T, e accEnv, w string) {
	if e.Error == nil || e.Error.Code != w {
		g := ""
		if e.Error != nil {
			g = e.Error.Code
		}
		t.Fatalf("code=%s want=%s", g, w)
	}
}

func (h *accHarness) ref(t *testing.T, a, k string, b any) uuid.UUID {
	return accID(t, h.expect(t, a, "POST", "/api/v2/reference/"+k, b, uuid.Nil, 201))
}

func (h *accHarness) seed(t *testing.T) {
	h.refs["case"] = h.ref(t, "planner", "items", map[string]any{"code": "ACC_CASE", "name": "Case", "kind": "component", "unit": "ea"})
	h.refs["switch"] = h.ref(t, "planner", "items", map[string]any{"code": "ACC_SWITCH", "name": "Switch", "kind": "component", "unit": "ea"})
	h.refs["fg"] = h.ref(t, "planner", "items", map[string]any{"code": "ACC_FG", "name": "FG", "kind": "finished", "unit": "ea"})
	h.refs["wh"] = h.ref(t, "materials", "locations", map[string]any{"code": "ACC_WH", "name": "WH", "kind": "warehouse"})
	h.refs["floor"] = h.ref(t, "materials", "locations", map[string]any{"code": "ACC_FLOOR", "name": "Floor", "kind": "floor"})
	h.refs["fgwh"] = h.ref(t, "materials", "locations", map[string]any{"code": "ACC_FGWH", "name": "FGWH", "kind": "finished"})
	h.refs["caseLot"] = h.ref(t, "materials", "lots", map[string]any{"item_id": h.refs["case"], "code": "ACC_CASE_LOT"})
	h.refs["switchLot"] = h.ref(t, "materials", "lots", map[string]any{"item_id": h.refs["switch"], "code": "ACC_SWITCH_LOT"})
	h.refs["defect"] = h.ref(t, "quality", "defect-reasons", map[string]any{"code": "ACC_DEFECT", "name": "Defect"})
	h.refs["bom"] = h.ref(t, "planner", "bom-revisions", map[string]any{
		"finished_item_id": h.refs["fg"],
		"revision":         "1",
		"lines": []map[string]any{
			{"component_item_id": h.refs["case"], "quantity": 1},
			{"component_item_id": h.refs["switch"], "quantity": 2},
		},
	})
	h.refs["qc"] = h.ref(t, "quality", "inspection-revisions", map[string]any{
		"finished_item_id": h.refs["fg"],
		"revision":         "1",
		"items": []map[string]any{
			{"item_code": "FINAL", "name": "Final", "required": true, "acceptance": "pass"},
		},
	})
	h.expect(t, "admin", "POST", "/api/v2/reference/bom-revisions/"+h.refs["bom"].String()+"/approve", nil, uuid.Nil, 200)
	h.expect(t, "admin", "POST", "/api/v2/reference/inspection-revisions/"+h.refs["qc"].String()+"/approve", nil, uuid.Nil, 200)
}

func (h *accHarness) order(t *testing.T, target int64) uuid.UUID {
	id := accID(t, h.expect(t, "planner", "POST", "/api/v2/orders", map[string]any{
		"finished_item_id":       h.refs["fg"],
		"target_quantity":        target,
		"bom_revision_id":        h.refs["bom"],
		"inspection_revision_id": h.refs["qc"],
		"assigned_user_id":       h.actors["operator"].ID,
		"planned_date":           "2026-09-13",
	}, uuid.Nil, 201))
	h.expect(t, "planner", "POST", "/api/v2/orders/"+id.String()+"/issue", map[string]any{}, uuid.Nil, 200)
	return id
}

func (h *accHarness) doc(t *testing.T, a string, b any) uuid.UUID {
	return accID(t, h.expect(t, a, "POST", "/api/v2/documents", b, uuid.Nil, 201))
}

func (h *accHarness) post(t *testing.T, a string, id uuid.UUID, w int) accEnv {
	return h.expect(t, a, "POST", "/api/v2/documents/"+id.String()+"/post", nil, uuid.New(), w)
}

func (h *accHarness) receive(t *testing.T, c, s int64) {
	d := h.doc(t, "materials", map[string]any{
		"kind":        "component_receipt",
		"location_id": h.refs["wh"],
		"quantity":    c + s,
		"lines": []map[string]any{
			{"lot_id": h.refs["caseLot"], "location_id": h.refs["wh"], "quantity": c},
			{"lot_id": h.refs["switchLot"], "location_id": h.refs["wh"], "quantity": s},
		},
	})
	h.post(t, "materials", d, 200)
}

func (h *accHarness) issue(t *testing.T, o uuid.UUID, c, s int64) {
	d := h.doc(t, "materials", map[string]any{
		"kind":        "issue",
		"order_id":    o,
		"location_id": h.refs["floor"],
		"lines": []map[string]any{
			{"lot_id": h.refs["caseLot"], "location_id": h.refs["wh"], "quantity": c},
			{"lot_id": h.refs["switchLot"], "location_id": h.refs["wh"], "quantity": s},
		},
	})
	h.post(t, "materials", d, 200)
}

func (h *accHarness) session(t *testing.T, o uuid.UUID) uuid.UUID {
	return accID(t, h.expect(t, "operator", "POST", "/api/v2/work-sessions", map[string]any{"order_id": o}, uuid.Nil, 201))
}

func (h *accHarness) production(t *testing.T, o, s uuid.UUID, q int64) uuid.UUID {
	d := h.doc(t, "operator", map[string]any{
		"kind":            "production",
		"order_id":        o,
		"work_session_id": s,
		"quantity":        q,
		"lines": []map[string]any{
			{"lot_id": h.refs["caseLot"], "location_id": h.refs["floor"], "quantity": q},
			{"lot_id": h.refs["switchLot"], "location_id": h.refs["floor"], "quantity": 2 * q},
		},
	})
	h.post(t, "operator", d, 200)
	return d
}

func (h *accHarness) inspection(t *testing.T, o, src uuid.UUID, a, r int64, result string) uuid.UUID {
	b := map[string]any{
		"kind":                 "inspection",
		"order_id":             o,
		"source_document_id":   src,
		"accepted_quantity":    a,
		"rejected_quantity":    r,
		"inspection_results": []map[string]any{{"item_code": "FINAL", "result": result}},
	}
	if r > 0 {
		b["defect_reason_id"] = h.refs["defect"]
	}
	return h.doc(t, "quality", b)
}

func (h *accHarness) goods(t *testing.T, o, src uuid.UUID, q int64) uuid.UUID {
	return h.doc(t, "materials", map[string]any{
		"kind":               "goods_receipt",
		"order_id":           o,
		"source_document_id": src,
		"location_id":        h.refs["fgwh"],
		"quantity":           q,
	})
}

func (h *accHarness) prepared(t *testing.T, target, q int64) (uuid.UUID, uuid.UUID, uuid.UUID) {
	o := h.order(t, target)
	h.receive(t, q, 2*q)
	h.issue(t, o, q, 2*q)
	s := h.session(t, o)
	return o, s, h.production(t, o, s, q)
}

func TestV2FactoryPostgreSQLAcceptance(t *testing.T) {
	h := newAcc(t)

	t.Run("representative-flow-and-terminal-state", func(t *testing.T) {
		o := h.order(t, 2)
		h.receive(t, 3, 6)
		h.issue(t, o, 3, 6)
		s := h.session(t, o)
		p := h.production(t, o, s, 2)
		i := h.inspection(t, o, p, 2, 0, "pass")
		h.post(t, "quality", i, 200)
		g := h.goods(t, o, i, 2)
		h.post(t, "materials", g, 200)
		r := h.doc(t, "materials", map[string]any{
			"kind":        "return",
			"order_id":    o,
			"location_id": h.refs["floor"],
			"lines": []map[string]any{
				{"lot_id": h.refs["caseLot"], "location_id": h.refs["wh"], "quantity": 1},
				{"lot_id": h.refs["switchLot"], "location_id": h.refs["wh"], "quantity": 2},
			},
		})
		h.post(t, "materials", r, 200)
		h.expect(t, "operator", "POST", "/api/v2/work-sessions/"+s.String()+"/end", nil, uuid.Nil, 200)
		h.expect(t, "planner", "POST", "/api/v2/orders/"+o.String()+"/close", map[string]any{}, uuid.Nil, 200)
		late := h.doc(t, "materials", map[string]any{
			"kind":        "issue",
			"order_id":    o,
			"location_id": h.refs["floor"],
			"lines": []map[string]any{
				{"lot_id": h.refs["caseLot"], "location_id": h.refs["wh"], "quantity": 1},
			},
		})
		accCode(t, h.post(t, "materials", late, 409), "INVALID_STATE")
	})

	t.Run("cross-order-source-blocked", func(t *testing.T) {
		_, _, p := h.prepared(t, 1, 1)
		o2 := h.order(t, 1)
		i := h.inspection(t, o2, p, 1, 0, "pass")
		accCode(t, h.post(t, "quality", i, 409), "SOURCE_ORDER_MISMATCH")
	})

	t.Run("session-and-hold-guards", func(t *testing.T) {
		o := h.order(t, 1)
		e := h.expect(t, "operator2", "POST", "/api/v2/work-sessions", map[string]any{"order_id": o}, uuid.Nil, 403)
		accCode(t, e, "FORBIDDEN")
		s := h.session(t, o)
		h.expect(t, "planner", "POST", "/api/v2/orders/"+o.String()+"/hold", map[string]any{}, uuid.Nil, 200)
		p := h.doc(t, "operator", map[string]any{
			"kind":            "production",
			"order_id":        o,
			"work_session_id": s,
			"quantity":        1,
		})
		accCode(t, h.post(t, "operator", p, 409), "INVALID_STATE")
	})

	t.Run("required-inspection-failure-blocks-accepted-output", func(t *testing.T) {
		o, _, p := h.prepared(t, 2, 2)
		i := h.inspection(t, o, p, 1, 1, "fail")
		accCode(t, h.post(t, "quality", i, 409), "INSPECTION_CRITERIA_FAILED")
	})

	t.Run("duplicate-and-concurrent-receipt-capped", func(t *testing.T) {
		o, _, p := h.prepared(t, 10, 5)
		i := h.inspection(t, o, p, 5, 0, "pass")
		h.post(t, "quality", i, 200)
		g := h.goods(t, o, i, 2)
		k := uuid.New()
		h.expect(t, "materials", "POST", "/api/v2/documents/"+g.String()+"/post", nil, k, 200)
		h.expect(t, "materials", "POST", "/api/v2/documents/"+g.String()+"/post", nil, k, 200)
		a, b := h.goods(t, o, i, 2), h.goods(t, o, i, 2)
		start := make(chan struct{})
		results := make(chan int, 2)
		var wg sync.WaitGroup
		for _, d := range []uuid.UUID{a, b} {
			d := d
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				s, _, _ := h.call("materials", "POST", "/api/v2/documents/"+d.String()+"/post", nil, uuid.New())
				results <- s
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		ok, conf := 0, 0
		for s := range results {
			if s == 200 {
				ok++
			} else if s == 409 {
				conf++
			} else {
				t.Fatalf("unexpected concurrent receipt status %d", s)
			}
		}
		if ok != 1 || conf != 1 {
			t.Fatalf("concurrent receipts ok=%d conflict=%d", ok, conf)
		}
		var accepted, received int64
		if e := h.pool.QueryRow(h.ctx, "SELECT accepted_quantity,received_quantity FROM v2.work_order_view WHERE id=$1", o).Scan(&accepted, &received); e != nil {
			t.Fatal(e)
		}
		if accepted != 1 || received != 4 {
			t.Fatalf("accepted=%d received=%d", accepted, received)
		}
	})
}
