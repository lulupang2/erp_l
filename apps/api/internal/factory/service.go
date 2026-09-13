package factory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"example.com/assembly-erp/api/internal/domain"
	"example.com/assembly-erp/api/internal/v2identity"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const nilOwner = "00000000-0000-0000-0000-000000000000"

type Service struct {
	Pool        *pgxpool.Pool
	hookMu      sync.RWMutex
	failureHook func(string) error
}

func New(pool *pgxpool.Pool) *Service { return &Service{Pool: pool} }
func (s *Service) SetFailureHook(hook func(string) error) {
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	s.failureHook = hook
}
func (s *Service) fail(stage string) error {
	s.hookMu.RLock()
	hook := s.failureHook
	s.hookMu.RUnlock()
	if hook != nil {
		return hook(stage)
	}
	return nil
}

type Actor = v2identity.Actor

func CurrentActor(c fiber.Ctx) (Actor, error) {
	actor, ok := c.Locals("v2_actor").(Actor)
	if !ok || actor.ID == uuid.Nil {
		return Actor{}, Unauthorized()
	}
	return actor, nil
}
func conflict(code, msg string) error { return &domain.Error{Status: 409, Code: code, Message: msg} }
func requireRole(actor Actor, roles ...string) error {
	for _, want := range roles {
		for _, got := range actor.Roles {
			if got == want {
				return nil
			}
		}
	}
	return Forbidden()
}
func requireState(status string, allowed ...string) error {
	if !slices.Contains(allowed, status) {
		return conflict("INVALID_STATE", "현재 상태에서 해당 명령을 수행할 수 없습니다.")
	}
	return nil
}
func public(err error) *domain.Error {
	var database *pgconn.PgError
	if errors.As(err, &database) {
		switch database.Code {
		case "23503":
			return domain.Input("참조한 품목·로트·위치·지시가 존재하지 않습니다.")
		case "23514":
			return domain.Conflict("CONSTRAINT_VIOLATION", "수량 또는 상태 제약을 위반하는 요청입니다.")
		}
	}
	return domain.PublicError(err)
}
func jsonBody(c fiber.Ctx, value any) error {
	if err := c.Bind().Body(value); err != nil {
		return domain.Input("요청 형식이 올바르지 않습니다.")
	}
	return nil
}
func key(c fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Get("Idempotency-Key"))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, domain.Input("Idempotency-Key UUID가 필요합니다.")
	}
	return id, nil
}
func auditEvent(ctx context.Context, tx pgx.Tx, actor Actor, operation string, entity uuid.UUID, reason string, details any) error {
	data, err := json.Marshal(details)
	if err != nil {
		return err
	}
	role := ""
	if len(actor.Roles) > 0 {
		role = actor.Roles[0]
	}
	_, err = tx.Exec(ctx, "INSERT INTO v2.audit_events(actor_id,actor_role,operation,entity_id,reason,details) VALUES($1,$2,$3,$4,$5,$6)", actor.ID, role, operation, entity, reason, data)
	return err
}
func inputQty(q int64, min int64) error {
	if q < min || q > 1_000_000 {
		return domain.Input("수량은 허용 범위의 정수여야 합니다.")
	}
	return nil
}
func hashInput(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func quantityValue(value any) (int64, bool) {
	switch number := value.(type) {
	case float64:
		quantity := int64(number)
		return quantity, float64(quantity) == number
	case json.Number:
		quantity, err := number.Int64()
		return quantity, err == nil
	case int64:
		return number, true
	case int:
		return int64(number), true
	default:
		return 0, false
	}
}

func Register(app *fiber.App, s *Service, authenticate fiber.Handler) {
	api := app.Group("/api/v2", authenticate)
	api.Get("/reference", s.reference)
	api.Post("/reference/:kind", s.createReference)
	api.Post("/reference/:kind/:id/:action", s.referenceAction)
	api.Get("/orders", s.orders)
	api.Post("/orders", s.createOrder)
	api.Get("/orders/:id", s.order)
	api.Put("/orders/:id", s.updateOrder)
	api.Post("/orders/:id/:action", s.orderAction)
	api.Get("/work-sessions", s.sessions)
	api.Post("/work-sessions", s.startSession)
	api.Post("/work-sessions/:id/end", s.endSession)
	api.Get("/documents", s.documents)
	api.Post("/documents", s.createDocument)
	api.Get("/documents/:id", s.document)
	api.Put("/documents/:id", s.updateDocument)
	api.Post("/documents/:id/post", s.postDocument)
	api.Post("/documents/:id/reverse", s.reverseDocument)
	api.Get("/inventory", s.inventory)
	api.Get("/output-lots", s.outputLots)
	api.Get("/ledger", s.ledger)
	api.Get("/reconciliation", s.reconciliation)
	api.Get("/trace", s.trace)
	api.Get("/audit", s.audit)
}

// PublicAccess supplies the fixed portfolio actor. It keeps audit attribution
// and the transaction-level role checks intact without exposing login routes.
func PublicAccess(c fiber.Ctx) error {
	c.Locals("v2_actor", v2identity.PublicActor)
	return c.Next()
}

func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func liveRequireRole(ctx context.Context, tx pgx.Tx, actor Actor, roles ...string) error {
	var active bool
	if err := tx.QueryRow(ctx, "SELECT active FROM v2.users WHERE id=$1", actor.ID).Scan(&active); err != nil {
		return err
	}
	if !active {
		return Unauthorized()
	}
	rows, err := tx.Query(ctx, "SELECT role FROM v2.user_roles WHERE user_id=$1", actor.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	current := map[string]bool{}
	for rows.Next() {
		var role string
		if err = rows.Scan(&role); err != nil {
			return err
		}
		current[role] = true
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, role := range roles {
		if current[role] {
			return nil
		}
	}
	return Forbidden()
}

func replay(ctx context.Context, tx pgx.Tx, actor Actor, operation string, id uuid.UUID, body any) (json.RawMessage, bool, error) {
	var required []string
	switch {
	case strings.HasPrefix(operation, "reference:create:"):
		if strings.HasSuffix(operation, ":items") || strings.HasSuffix(operation, ":locations") || strings.HasSuffix(operation, ":lots") {
			required = []string{"materials", "planner", "admin"}
		} else {
			required = []string{"planner", "quality", "admin"}
		}
	case strings.HasPrefix(operation, "reference:"):
		required = []string{"admin"}
	case strings.HasPrefix(operation, "order:"):
		required = []string{"planner", "admin"}
	case operation == "session:start":
		required = []string{"operator"}
	case strings.HasPrefix(operation, "session:end:"):
		required = []string{"operator", "admin"}
	case operation == "document:create":
		if in, ok := body.(documentInput); ok {
			required = documentRole(in.Kind)
		} else if in, ok := body.(*documentInput); ok {
			required = documentRole(in.Kind)
		}
	case strings.HasPrefix(operation, "document:reverse:"):
		required = []string{"admin"}
	case strings.HasPrefix(operation, "document:post:"):
		lookupID, err := uuid.Parse(strings.TrimPrefix(operation, "document:post:"))
		if err != nil {
			return nil, false, domain.Input("문서 UUID가 필요합니다.")
		}
		var kind string
		if err := tx.QueryRow(ctx, "SELECT kind FROM v2.documents WHERE id=$1", lookupID).Scan(&kind); err != nil {
			return nil, false, err
		}
		required = documentRole(kind)
	}
	if len(required) > 0 {
		if err := liveRequireRole(ctx, tx, actor, required...); err != nil {
			return nil, false, err
		}
	}
	if id == uuid.Nil {
		return nil, false, domain.Input("요청 식별자가 필요합니다.")
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID.String()+":"+operation+":"+id.String()); err != nil {
		return nil, false, err
	}
	hash := hashInput(body)
	var previousHash string
	var response []byte
	err := tx.QueryRow(ctx, "SELECT request_hash,response FROM v2.command_replays WHERE account_id=$1 AND operation=$2 AND request_key=$3 FOR UPDATE", actor.ID, operation, id).Scan(&previousHash, &response)
	if err == nil {
		if previousHash != hash {
			return nil, false, conflict("IDEMPOTENCY_CONFLICT", "같은 요청 키를 다른 입력에 사용할 수 없습니다.")
		}
		return response, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}
	return nil, false, nil
}
func saveReplay(ctx context.Context, tx pgx.Tx, actor Actor, operation string, id uuid.UUID, body any, response any) error {
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO v2.command_replays(account_id,operation,request_key,request_hash,response) VALUES($1,$2,$3,$4,$5)", actor.ID, operation, id, hashInput(body), data)
	return err
}
func respond(c fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	e := public(err)
	return c.Status(e.Status).JSON(fiber.Map{"error": e})
}
func one(c fiber.Ctx, status int, data any) error {
	return c.Status(status).JSON(fiber.Map{"data": data})
}

func (s *Service) reference(c fiber.Ctx) error {
	actor, err := CurrentActor(c)
	if err != nil {
		return respond(c, err)
	}
	if err = requireRole(actor, "admin", "planner", "materials", "quality"); err != nil {
		return respond(c, err)
	}
	ctx := c.Context()
	out := map[string]any{}
	queries := map[string]string{"items": "SELECT id,code,name,kind,unit,active FROM v2.items ORDER BY code", "locations": "SELECT id,code,name,kind,active FROM v2.locations ORDER BY code", "lots": "SELECT id,code,item_id,active FROM v2.lots ORDER BY code", "defect_reasons": "SELECT id,code,name,active FROM v2.defect_reasons ORDER BY code", "bom_revisions": "SELECT id,finished_item_id,revision,status,lines FROM v2.bom_revision_view ORDER BY finished_item_id,revision", "inspection_revisions": "SELECT id,finished_item_id,revision,status,items FROM v2.inspection_revision_view ORDER BY finished_item_id,revision"}
	for name, q := range queries {
		rows, er := s.Pool.Query(ctx, q)
		if er != nil {
			return respond(c, er)
		}
		vals := []map[string]any{}
		for rows.Next() {
			var id uuid.UUID
			var a, b, c3, d string
			var active bool
			var blob []byte
			switch name {
			case "items":
				if er = rows.Scan(&id, &a, &b, &c3, &d, &active); er == nil {
					vals = append(vals, map[string]any{"id": id, "code": a, "name": b, "kind": c3, "unit": d, "active": active})
				}
			case "locations":
				if er = rows.Scan(&id, &a, &b, &c3, &active); er == nil {
					vals = append(vals, map[string]any{"id": id, "code": a, "name": b, "kind": c3, "active": active})
				}
			case "lots":
				if er = rows.Scan(&id, &a, &b, &active); er == nil {
					vals = append(vals, map[string]any{"id": id, "code": a, "item_id": b, "active": active})
				}
			case "defect_reasons":
				if er = rows.Scan(&id, &a, &b, &active); er == nil {
					vals = append(vals, map[string]any{"id": id, "code": a, "name": b, "active": active})
				}
			default:
				if er = rows.Scan(&id, &a, &b, &c3, &blob); er == nil {
					var decoded any
					_ = json.Unmarshal(blob, &decoded)
					vals = append(vals, map[string]any{"id": id, "finished_item_id": a, "revision": b, "status": c3, "lines": decoded, "items": decoded})
				}
			}
			if er != nil {
				rows.Close()
				return respond(c, er)
			}
		}
		rows.Close()
		out[name] = vals
	}
	return one(c, 200, out)
}

func (s *Service) createReference(c fiber.Ctx) error {
	actor, err := CurrentActor(c)
	if err != nil {
		return respond(c, err)
	}
	kind := c.Params("kind")
	if kind == "items" || kind == "locations" || kind == "lots" {
		if err = requireRole(actor, "admin", "materials", "planner"); err != nil {
			return respond(c, err)
		}
	} else if err = requireRole(actor, "admin", "planner", "quality"); err != nil {
		return respond(c, err)
	}
	idempotency, err := key(c)
	if err != nil {
		return respond(c, err)
	}
	var in map[string]any
	if err = jsonBody(c, &in); err != nil {
		return respond(c, err)
	}
	id := uuid.New()
	result := map[string]any{"id": id, "kind": kind}
	replayFound := false
	ctx := c.Context()
	err = s.tx(ctx, func(tx pgx.Tx) error {
		previous, replayed, err := replay(ctx, tx, actor, "reference:create:"+kind, idempotency, in)
		if err != nil {
			return err
		}
		if replayed {
			if err = json.Unmarshal(previous, &result); err != nil {
				return err
			}
			replayFound = true
			return nil
		}
		switch kind {
		case "items":
			_, err = tx.Exec(ctx, "INSERT INTO v2.items(id,code,name,kind,unit) VALUES($1,$2,$3,$4,$5)", id, strings.ToUpper(strings.TrimSpace(fmt.Sprint(in["code"]))), strings.TrimSpace(fmt.Sprint(in["name"])), fmt.Sprint(in["kind"]), fmt.Sprint(in["unit"]))
		case "locations":
			_, err = tx.Exec(ctx, "INSERT INTO v2.locations(id,code,name,kind) VALUES($1,$2,$3,$4)", id, strings.ToUpper(strings.TrimSpace(fmt.Sprint(in["code"]))), strings.TrimSpace(fmt.Sprint(in["name"])), fmt.Sprint(in["kind"]))
		case "lots":
			item, parseErr := uuid.Parse(fmt.Sprint(in["item_id"]))
			if parseErr != nil {
				return domain.Input("품목 UUID가 필요합니다.")
			}
			_, err = tx.Exec(ctx, "INSERT INTO v2.lots(id,item_id,code) VALUES($1,$2,$3)", id, item, fmt.Sprint(in["code"]))
		case "defect-reasons":
			_, err = tx.Exec(ctx, "INSERT INTO v2.defect_reasons(id,code,name) VALUES($1,$2,$3)", id, fmt.Sprint(in["code"]), fmt.Sprint(in["name"]))
		case "bom-revisions":
			err = s.createBOM(ctx, tx, actor, id, in)
		case "inspection-revisions":
			err = s.createInspection(ctx, tx, actor, id, in)
		default:
			return domain.Missing()
		}
		if err != nil {
			return err
		}
		if err = auditEvent(ctx, tx, actor, "reference_create", id, "", map[string]any{"kind": kind}); err != nil {
			return err
		}
		return saveReplay(ctx, tx, actor, "reference:create:"+kind, idempotency, in, result)
	})
	if err != nil {
		return respond(c, err)
	}
	if replayFound {
		return one(c, 200, result)
	}
	return one(c, 201, result)
}
func (s *Service) createBOM(ctx context.Context, tx pgx.Tx, actor Actor, id uuid.UUID, in map[string]any) error {
	finished, e := uuid.Parse(fmt.Sprint(in["finished_item_id"]))
	if e != nil {
		return domain.Input("완제품 UUID가 필요합니다.")
	}
	if _, e = tx.Exec(ctx, "INSERT INTO v2.bom_revisions(id,finished_item_id,revision,created_by) VALUES($1,$2,$3,$4)", id, finished, fmt.Sprint(in["revision"]), actor.ID); e != nil {
		return e
	}
	lines, _ := in["lines"].([]any)
	if len(lines) == 0 {
		return domain.Input("BOM에는 부품이 필요합니다.")
	}
	for _, raw := range lines {
		line, ok := raw.(map[string]any)
		if !ok {
			return domain.Input("BOM 항목 형식이 올바르지 않습니다.")
		}
		component, e := uuid.Parse(fmt.Sprint(line["component_item_id"]))
		if e != nil {
			return domain.Input("부품 UUID가 필요합니다.")
		}
		q, ok := quantityValue(line["quantity"])
		if !ok {
			return domain.Input("BOM 수량은 정수여야 합니다.")
		}
		if e = inputQty(q, 1); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO v2.bom_lines(revision_id,component_item_id,quantity) VALUES($1,$2,$3)", id, component, q); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) createInspection(ctx context.Context, tx pgx.Tx, actor Actor, id uuid.UUID, in map[string]any) error {
	finished, e := uuid.Parse(fmt.Sprint(in["finished_item_id"]))
	if e != nil {
		return domain.Input("완제품 UUID가 필요합니다.")
	}
	if _, e = tx.Exec(ctx, "INSERT INTO v2.inspection_revisions(id,finished_item_id,revision,created_by) VALUES($1,$2,$3,$4)", id, finished, fmt.Sprint(in["revision"]), actor.ID); e != nil {
		return e
	}
	items, _ := in["items"].([]any)
	if len(items) == 0 {
		return domain.Input("검사 항목이 필요합니다.")
	}
	for _, raw := range items {
		v, ok := raw.(map[string]any)
		if !ok {
			return domain.Input("검사 항목 형식이 올바르지 않습니다.")
		}
		if _, e = tx.Exec(ctx, "INSERT INTO v2.inspection_items(revision_id,item_code,name,required,acceptance) VALUES($1,$2,$3,$4,$5)", id, fmt.Sprint(v["item_code"]), fmt.Sprint(v["name"]), v["required"], fmt.Sprint(v["acceptance"])); e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) referenceAction(c fiber.Ctx) error {
	actor, err := CurrentActor(c)
	if err != nil {
		return respond(c, err)
	}
	if err = requireRole(actor, "admin"); err != nil {
		return respond(c, err)
	}
	id, e := uuid.Parse(c.Params("id"))
	if e != nil {
		return respond(c, domain.Input("UUID가 필요합니다."))
	}
	idempotency, e := key(c)
	if e != nil {
		return respond(c, e)
	}
	kind, action := c.Params("kind"), c.Params("action")
	table := ""
	if kind == "bom-revisions" {
		table = "bom_revisions"
	}
	if kind == "inspection-revisions" {
		table = "inspection_revisions"
	}
	if table == "" || (action != "approve" && action != "retire") {
		return respond(c, domain.Input("기준정보 상태 명령이 올바르지 않습니다."))
	}
	status := map[bool]string{true: "approved", false: "retired"}[action == "approve"]
	result := map[string]any{"id": id, "status": status}
	replayFound := false
	e = s.tx(c.Context(), func(tx pgx.Tx) error {
		previous, replayed, e := replay(c.Context(), tx, actor, "reference:"+action+":"+id.String(), idempotency, map[string]any{"kind": kind, "action": action, "id": id})
		if e != nil {
			return e
		}
		if replayed {
			if e = json.Unmarshal(previous, &result); e != nil {
				return e
			}
			replayFound = true
			return nil
		}
		var query string
		if action == "approve" {
			query = "UPDATE v2." + table + " SET status='approved',approved_by=$2,approved_at=now() WHERE id=$1 AND status='draft'"
		} else {
			query = "UPDATE v2." + table + " SET status='retired' WHERE id=$1 AND status='approved'"
		}
		tag, e := tx.Exec(c.Context(), query, id, actor.ID)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return conflict("INVALID_STATE", "상태 변경이 허용되지 않습니다.")
		}
		if e = auditEvent(c.Context(), tx, actor, "reference_"+action, id, "", map[string]any{"kind": kind}); e != nil {
			return e
		}
		return saveReplay(c.Context(), tx, actor, "reference:"+action+":"+id.String(), idempotency, map[string]any{"kind": kind, "action": action, "id": id}, result)
	})
	if e != nil {
		return respond(c, e)
	}
	if replayFound {
		return one(c, 200, result)
	}
	return one(c, 200, result)
}

func (s *Service) orders(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner", "materials", "operator", "quality"); e != nil {
		return respond(c, e)
	}
	rows, e := s.Pool.Query(c.Context(), "SELECT id,finished_item_id,target_quantity,start_allowance,bom_revision_id,inspection_revision_id,assigned_user_id,planned_date,status,version,created_by,created_at,new_output_quantity,pending_quantity,accepted_quantity,rejected_quantity,rework_quantity,disposed_quantity,received_quantity,remaining_quantity,floor_quantity,active_sessions,has_variances FROM v2.work_order_view ORDER BY created_at DESC")
	if e != nil {
		return respond(c, e)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, fi, bom, ins, assigned, created uuid.UUID
		var target, allow, ver, newOut, pending, accepted, rejected, rework, disposed, received, remain, floor, active int64
		var date, status string
		var variance bool
		var at time.Time
		if e = rows.Scan(&id, &fi, &target, &allow, &bom, &ins, &assigned, &date, &status, &ver, &created, &at, &newOut, &pending, &accepted, &rejected, &rework, &disposed, &received, &remain, &floor, &active, &variance); e != nil {
			return respond(c, e)
		}
		out = append(out, map[string]any{"id": id, "finished_item_id": fi, "target_quantity": target, "start_allowance": allow, "bom_revision_id": bom, "inspection_revision_id": ins, "assigned_user_id": assigned, "planned_date": date, "status": status, "version": ver, "created_by": created, "created_at": at, "new_output_quantity": newOut, "pending_quantity": pending, "accepted_quantity": accepted, "rejected_quantity": rejected, "rework_quantity": rework, "disposed_quantity": disposed, "received_quantity": received, "remaining_quantity": remain, "floor_quantity": floor, "active_sessions": active, "has_variances": variance})
	}
	return one(c, 200, out)
}

type orderInput struct {
	FinishedItemID       uuid.UUID `json:"finished_item_id"`
	TargetQuantity       int64     `json:"target_quantity"`
	BomRevisionID        uuid.UUID `json:"bom_revision_id"`
	InspectionRevisionID uuid.UUID `json:"inspection_revision_id"`
	AssignedUserID       uuid.UUID `json:"assigned_user_id"`
	PlannedDate          string    `json:"planned_date"`
	Version              int64     `json:"version,omitempty"`
}

func (s *Service) createOrder(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner"); e != nil {
		return respond(c, e)
	}
	var in orderInput
	if e = jsonBody(c, &in); e != nil {
		return respond(c, e)
	}
	if e = inputQty(in.TargetQuantity, 1); e != nil {
		return respond(c, e)
	}
	idempotency, e := key(c)
	if e != nil {
		return respond(c, e)
	}
	id := uuid.New()
	var result map[string]any
	replayFound := false
	e = s.tx(c.Context(), func(tx pgx.Tx) error {
		prev, replayed, e := replay(c.Context(), tx, actor, "order:create", idempotency, in)
		if e != nil || replayed {
			if replayed {
				if e = json.Unmarshal(prev, &result); e != nil {
					return e
				}
				replayFound = true
			}
			return e
		}
		if in.FinishedItemID == uuid.Nil || in.AssignedUserID == uuid.Nil {
			return domain.Input("완제품과 담당 사용자 UUID가 필요합니다.")
		}
		var finishedOK bool
		if e = tx.QueryRow(c.Context(), "SELECT EXISTS(SELECT 1 FROM v2.items WHERE id=$1 AND kind='finished' AND active)", in.FinishedItemID).Scan(&finishedOK); e != nil {
			return e
		}
		if !finishedOK {
			return conflict("INVALID_FINISHED_ITEM", "활성 완제품 품목이 필요합니다.")
		}
		var assigneeOK bool
		if e = tx.QueryRow(c.Context(), "SELECT EXISTS(SELECT 1 FROM v2.users u JOIN v2.user_roles r ON r.user_id=u.id WHERE u.id=$1 AND u.active AND r.role='operator')", in.AssignedUserID).Scan(&assigneeOK); e != nil {
			return e
		}
		if !assigneeOK {
			return conflict("ASSIGNEE_NOT_OPERATOR", "활성 작업자 계정을 담당자로 지정해야 합니다.")
		}
		var bomSnap, insSnap []byte
		if e = tx.QueryRow(c.Context(), "SELECT lines FROM v2.bom_revision_view WHERE id=$1 AND finished_item_id=$2 AND status='approved'", in.BomRevisionID, in.FinishedItemID).Scan(&bomSnap); e != nil {
			return conflict("REVISION_NOT_APPROVED", "해당 완제품의 승인된 BOM 개정이 필요합니다.")
		}
		if e = tx.QueryRow(c.Context(), "SELECT items FROM v2.inspection_revision_view WHERE id=$1 AND finished_item_id=$2 AND status='approved'", in.InspectionRevisionID, in.FinishedItemID).Scan(&insSnap); e != nil {
			return conflict("REVISION_NOT_APPROVED", "해당 완제품의 승인된 검사 기준이 필요합니다.")
		}
		_, e = tx.Exec(c.Context(), "INSERT INTO v2.work_orders(id,finished_item_id,target_quantity,start_allowance,bom_revision_id,inspection_revision_id,assigned_user_id,planned_date,bom_snapshot,inspection_snapshot,created_by) VALUES($1,$2,$3,$3,$4,$5,$6,$7,$8,$9,$10)", id, in.FinishedItemID, in.TargetQuantity, in.BomRevisionID, in.InspectionRevisionID, in.AssignedUserID, in.PlannedDate, bomSnap, insSnap, actor.ID)
		if e != nil {
			return e
		}
		result = map[string]any{"id": id, "status": "draft", "target_quantity": in.TargetQuantity, "start_allowance": in.TargetQuantity}
		if e = saveReplay(c.Context(), tx, actor, "order:create", idempotency, in, result); e != nil {
			return e
		}
		return nil
	})
	if e != nil {
		return respond(c, e)
	}
	return one(c, map[bool]int{true: 200, false: 201}[replayFound], result)
}
func (s *Service) order(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner", "materials", "operator", "quality"); e != nil {
		return respond(c, e)
	}
	id, e := uuid.Parse(c.Params("id"))
	if e != nil {
		return respond(c, domain.Input("UUID가 필요합니다."))
	}
	row := s.Pool.QueryRow(c.Context(), "SELECT row_to_json(v) FROM v2.work_order_view v WHERE id=$1", id)
	var data []byte
	if e = row.Scan(&data); e != nil {
		return respond(c, e)
	}
	var out any
	_ = json.Unmarshal(data, &out)
	return one(c, 200, out)
}
func (s *Service) updateOrder(c fiber.Ctx) error {
	return respond(c, conflict("DRAFT_VERSION_REQUIRED", "초안 변경은 발행 명령 API를 통해 처리합니다."))
}

func (s *Service) orderAction(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	action := c.Params("action")
	roles := map[string][]string{"issue": {"planner"}, "hold": {"planner"}, "resume": {"planner"}, "allowance": {"planner"}, "target": {"planner"}, "close": {"planner"}, "early-close": {"planner"}, "cancel": {"planner"}}
	if e = requireRole(actor, roles[action]...); e != nil {
		return respond(c, e)
	}
	id, e := uuid.Parse(c.Params("id"))
	if e != nil {
		return respond(c, domain.Input("UUID가 필요합니다."))
	}
	var body struct {
		Reason      string `json:"reason"`
		Quantity    int64  `json:"quantity"`
		Acknowledge bool   `json:"acknowledge_variances"`
	}
	if e = jsonBody(c, &body); e != nil {
		return respond(c, e)
	}
	idempotency, e := key(c)
	if e != nil {
		return respond(c, e)
	}
	var status string
	switch action {
	case "issue":
		status = "issued"
	case "hold":
		status = "held"
	case "resume":
		status = "in_progress"
	case "cancel":
		status = "cancelled"
	case "early-close":
		if strings.TrimSpace(body.Reason) == "" {
			return respond(c, domain.Input("조기종결 사유가 필요합니다."))
		}
		status = "early_closed"
	case "close":
		status = "closed"
	case "allowance", "target":
		if e = inputQty(body.Quantity, 1); e != nil {
			return respond(c, e)
		}
		if strings.TrimSpace(body.Reason) == "" {
			return respond(c, domain.Input("착수 허용량·목표 변경 사유가 필요합니다."))
		}
	}
	var result = map[string]any{"id": id, "action": action}
	replayFound := false
	e = s.tx(c.Context(), func(tx pgx.Tx) error {
		previous, replayed, e := replay(c.Context(), tx, actor, "order:"+action+":"+id.String(), idempotency, body)
		if e != nil {
			return e
		}
		if replayed {
			if e = json.Unmarshal(previous, &result); e != nil {
				return e
			}
			replayFound = true
			return nil
		}
		var old string
		var target, allow int64
		var variance bool
		if e = tx.QueryRow(c.Context(), "SELECT status,target_quantity,start_allowance,EXISTS(SELECT 1 FROM v2.documents WHERE order_id=$1 AND status='posted' AND has_variance) FROM v2.work_orders WHERE id=$1 FOR UPDATE", id).Scan(&old, &target, &allow, &variance); e != nil {
			return e
		}
		valid := map[string][]string{
			"issue":       {"draft"},
			"hold":        {"issued", "in_progress"},
			"resume":      {"held"},
			"allowance":   {"issued", "in_progress", "held"},
			"target":      {"draft", "issued", "in_progress", "held"},
			"close":       {"in_progress"},
			"early-close": {"issued", "in_progress"},
			"cancel":      {"draft", "issued"},
		}
		if !slices.Contains(valid[action], old) {
			return conflict("INVALID_STATE", "현재 지시 상태에서 해당 명령을 수행할 수 없습니다.")
		}
		if action == "issue" {
			var revisionsOK, assigneeOK bool
			if e = tx.QueryRow(c.Context(), `SELECT
				EXISTS(SELECT 1 FROM v2.bom_revisions b WHERE b.id=o.bom_revision_id AND b.finished_item_id=o.finished_item_id AND b.status='approved')
				AND EXISTS(SELECT 1 FROM v2.inspection_revisions i WHERE i.id=o.inspection_revision_id AND i.finished_item_id=o.finished_item_id AND i.status='approved'),
				EXISTS(SELECT 1 FROM v2.users u JOIN v2.user_roles r ON r.user_id=u.id WHERE u.id=o.assigned_user_id AND u.active AND r.role='operator')
				FROM v2.work_orders o WHERE o.id=$1`, id).Scan(&revisionsOK, &assigneeOK); e != nil {
				return e
			}
			if !revisionsOK {
				return conflict("REVISION_NOT_APPROVED", "발행 시점에도 승인 상태인 BOM과 검사 기준이 필요합니다.")
			}
			if !assigneeOK {
				return conflict("ASSIGNEE_NOT_OPERATOR", "활성 작업자 계정이 배정되어야 지시를 발행할 수 있습니다.")
			}
		}
		if action == "cancel" && old == "issued" {
			var activity bool
			if e = tx.QueryRow(c.Context(), "SELECT EXISTS(SELECT 1 FROM v2.documents WHERE order_id=$1 AND status='posted') OR EXISTS(SELECT 1 FROM v2.work_sessions WHERE order_id=$1)", id).Scan(&activity); e != nil {
				return e
			}
			if activity {
				return conflict("ACTIVITY_EXISTS", "처리 이력이 있는 지시는 직접 취소할 수 없습니다.")
			}
		}
		if action == "early-close" {
			var activity bool
			if e = tx.QueryRow(c.Context(), "SELECT EXISTS(SELECT 1 FROM v2.documents WHERE order_id=$1 AND status='posted') OR EXISTS(SELECT 1 FROM v2.work_sessions WHERE order_id=$1)", id).Scan(&activity); e != nil {
				return e
			}
			if !activity {
				return conflict("ACTIVITY_REQUIRED", "처리 이력이 없는 지시는 조기종결 대신 취소해야 합니다.")
			}
		}
		if action == "close" && variance && !body.Acknowledge {
			return conflict("VARIANCE_ACK_REQUIRED", "BOM 대비 소비 차이를 확인해야 마감할 수 있습니다.")
		}
		if action == "close" || action == "early-close" {
			var remaining, pending, accepted, rejected, rework, floor, active int64
			if e = tx.QueryRow(c.Context(), "SELECT remaining_quantity,pending_quantity,accepted_quantity,rejected_quantity,rework_quantity,floor_quantity,active_sessions FROM v2.work_order_view WHERE id=$1", id).Scan(&remaining, &pending, &accepted, &rejected, &rework, &floor, &active); e != nil {
				return e
			}
			if (action == "close" && remaining > 0) || pending > 0 || accepted > 0 || rejected > 0 || rework > 0 || floor > 0 || active > 0 {
				return conflict("SETTLEMENT_REQUIRED", "마감에는 목표 입고와 모든 대기 산출·현장 자재·활성 세션의 정리가 필요합니다.")
			}
		}
		if action == "resume" {
			var previous string
			err := tx.QueryRow(c.Context(), "SELECT details->>'previous_status' FROM v2.audit_events WHERE entity_id=$1 AND operation='order_hold' ORDER BY recorded_at DESC,id DESC LIMIT 1", id).Scan(&previous)
			if err == nil && (previous == "issued" || previous == "in_progress") {
				status = previous
			} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		switch action {
		case "allowance":
			allow += body.Quantity
			if allow > 1_000_000 {
				return domain.Input("착수 허용량 합계가 상한을 초과합니다.")
			}
			_, e = tx.Exec(c.Context(), "UPDATE v2.work_orders SET start_allowance=$2,version=version+1 WHERE id=$1", id, allow)
		case "target":
			var received int64
			if e = tx.QueryRow(c.Context(), "SELECT received_quantity FROM v2.work_order_view WHERE id=$1", id).Scan(&received); e != nil {
				return e
			}
			if body.Quantity < received {
				return conflict("TARGET_BELOW_RECEIVED", "목표는 이미 입고한 수량보다 작게 변경할 수 없습니다.")
			}
			_, e = tx.Exec(c.Context(), "UPDATE v2.work_orders SET target_quantity=$2,version=version+1 WHERE id=$1", id, body.Quantity)
		default:
			_, e = tx.Exec(c.Context(), "UPDATE v2.work_orders SET status=$2,version=version+1 WHERE id=$1", id, status)
		}
		if e != nil {
			return e
		}
		details := map[string]any{"reason": body.Reason, "quantity": body.Quantity, "acknowledge_variances": body.Acknowledge}
		if action == "hold" {
			details["previous_status"] = old
		}
		if e = auditEvent(c.Context(), tx, actor, "order_"+action, id, body.Reason, details); e != nil {
			return e
		}
		return saveReplay(c.Context(), tx, actor, "order:"+action+":"+id.String(), idempotency, body, result)
	})
	if e != nil {
		return respond(c, e)
	}
	if replayFound {
		return one(c, 200, result)
	}
	return one(c, 200, result)
}

func (s *Service) sessions(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner", "operator"); e != nil {
		return respond(c, e)
	}
	rows, e := s.Pool.Query(c.Context(), "SELECT id,order_id,user_id,status,started_at,ended_at FROM v2.work_sessions ORDER BY started_at DESC")
	if e != nil {
		return respond(c, e)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, order, user uuid.UUID
		var st string
		var start time.Time
		var end *time.Time
		if e = rows.Scan(&id, &order, &user, &st, &start, &end); e != nil {
			return respond(c, e)
		}
		out = append(out, map[string]any{"id": id, "order_id": order, "user_id": user, "status": st, "started_at": start, "ended_at": end})
	}
	return one(c, 200, out)
}
func (s *Service) startSession(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "operator"); e != nil {
		return respond(c, e)
	}
	var in struct {
		OrderID uuid.UUID `json:"order_id"`
	}
	if e = jsonBody(c, &in); e != nil {
		return respond(c, e)
	}
	idempotency, e := key(c)
	if e != nil {
		return respond(c, e)
	}
	id := uuid.New()
	result := map[string]any{"id": id, "order_id": in.OrderID, "user_id": actor.ID, "status": "active"}
	replayFound := false
	e = s.tx(c.Context(), func(tx pgx.Tx) error {
		previous, replayed, e := replay(c.Context(), tx, actor, "session:start", idempotency, in)
		if e != nil {
			return e
		}
		if replayed {
			if e = json.Unmarshal(previous, &result); e != nil {
				return e
			}
			replayFound = true
			return nil
		}
		var status string
		var assigned uuid.UUID
		if e := tx.QueryRow(c.Context(), "SELECT status,assigned_user_id FROM v2.work_orders WHERE id=$1 FOR UPDATE", in.OrderID).Scan(&status, &assigned); e != nil {
			return e
		}
		if assigned != actor.ID {
			return Forbidden()
		}
		if e = requireState(status, "issued", "in_progress"); e != nil {
			return e
		}
		if _, e = tx.Exec(c.Context(), "INSERT INTO v2.work_sessions(id,order_id,user_id) VALUES($1,$2,$3)", id, in.OrderID, actor.ID); e != nil {
			return e
		}
		if status == "issued" {
			if _, e = tx.Exec(c.Context(), "UPDATE v2.work_orders SET status='in_progress',version=version+1 WHERE id=$1", in.OrderID); e != nil {
				return e
			}
		}
		if e = auditEvent(c.Context(), tx, actor, "session_start", id, "", in); e != nil {
			return e
		}
		return saveReplay(c.Context(), tx, actor, "session:start", idempotency, in, result)
	})
	if e != nil {
		return respond(c, e)
	}
	if replayFound {
		return one(c, 200, result)
	}
	return one(c, 201, result)
}
func (s *Service) endSession(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "operator"); e != nil {
		return respond(c, e)
	}
	id, e := uuid.Parse(c.Params("id"))
	if e != nil {
		return respond(c, e)
	}
	idempotency, e := key(c)
	if e != nil {
		return respond(c, e)
	}
	result := map[string]any{"id": id, "status": "ended"}
	e = s.tx(c.Context(), func(tx pgx.Tx) error {
		previous, replayed, e := replay(c.Context(), tx, actor, "session:end:"+id.String(), idempotency, map[string]any{"id": id})
		if e != nil {
			return e
		}
		if replayed {
			return json.Unmarshal(previous, &result)
		}
		tag, e := tx.Exec(c.Context(), "UPDATE v2.work_sessions SET status='ended',ended_at=now() WHERE id=$1 AND user_id=$2 AND status='active'", id, actor.ID)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return conflict("SESSION_NOT_ACTIVE", "활성 세션이 없습니다.")
		}
		if e = auditEvent(c.Context(), tx, actor, "session_end", id, "", map[string]any{}); e != nil {
			return e
		}
		return saveReplay(c.Context(), tx, actor, "session:end:"+id.String(), idempotency, map[string]any{"id": id}, result)
	})
	if e != nil {
		return respond(c, e)
	}
	return one(c, 200, result)
}

type inspectionResultInput struct {
	ItemCode string `json:"item_code"`
	Result   string `json:"result"`
	Note     string `json:"note"`
}
type documentInput struct {
	Kind             string     `json:"kind"`
	OrderID          *uuid.UUID `json:"order_id"`
	SourceDocumentID *uuid.UUID `json:"source_document_id"`
	WorkSessionID    *uuid.UUID `json:"work_session_id"`
	OutputLotCode    string     `json:"output_lot_code"`
	LocationID       *uuid.UUID `json:"location_id"`
	Quantity         int64      `json:"quantity"`
	AcceptedQuantity int64      `json:"accepted_quantity"`
	RejectedQuantity int64      `json:"rejected_quantity"`
	DefectReasonID   *uuid.UUID `json:"defect_reason_id"`
	Reason           string     `json:"reason"`
	Disposition      string     `json:"disposition"`
	OccurredAt       *time.Time `json:"occurred_at"`
	Lines            []struct {
		LotID      uuid.UUID `json:"lot_id"`
		LocationID uuid.UUID `json:"location_id"`
		Quantity   int64     `json:"quantity"`
	} `json:"lines"`
	InspectionResults []inspectionResultInput `json:"inspection_results"`
}

func documentRole(kind string) []string {
	return map[string][]string{"component_receipt": {"materials"}, "issue": {"materials"}, "return": {"materials"}, "material_loss": {"materials"}, "production": {"operator"}, "inspection": {"quality"}, "disposition": {"quality"}, "rework": {"operator"}, "goods_receipt": {"materials"}}[kind]
}
func (s *Service) createDocument(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	var in documentInput
	if e = jsonBody(c, &in); e != nil {
		return respond(c, e)
	}
	if len(documentRole(in.Kind)) == 0 {
		return respond(c, domain.Input("전표 종류가 올바르지 않습니다."))
	}
	for _, line := range in.Lines {
		if line.LotID == uuid.Nil || line.LocationID == uuid.Nil {
			return respond(c, domain.Input("자재 행에는 로트와 위치 UUID가 필요합니다."))
		}
	}
	switch in.Kind {
	case "component_receipt", "production", "disposition", "rework", "goods_receipt":
		if e = inputQty(in.Quantity, 1); e != nil {
			return respond(c, e)
		}
	case "inspection":
		if in.AcceptedQuantity < 0 || in.RejectedQuantity < 0 || in.AcceptedQuantity+in.RejectedQuantity < 1 || in.AcceptedQuantity+in.RejectedQuantity > 1_000_000 {
			return respond(c, domain.Input("검사 수량이 허용 범위를 벗어났습니다."))
		}
	}
	if e = requireRole(actor, documentRole(in.Kind)...); e != nil {
		return respond(c, e)
	}
	idempotency, e := key(c)
	if e != nil {
		return respond(c, e)
	}
	id := uuid.New()
	request := in
	result := map[string]any{"id": id, "kind": in.Kind, "status": "draft"}
	replayFound := false
	e = s.tx(c.Context(), func(tx pgx.Tx) error {
		previous, replayed, e := replay(c.Context(), tx, actor, "document:create", idempotency, request)
		if e != nil {
			return e
		}
		if replayed {
			if e = json.Unmarshal(previous, &result); e != nil {
				return e
			}
			replayFound = true
			return nil
		}
		if in.OccurredAt == nil {
			now := time.Now().UTC()
			in.OccurredAt = &now
		}
		if _, e = tx.Exec(c.Context(), "INSERT INTO v2.documents(id,kind,order_id,source_document_id,work_session_id,output_lot_code,location_id,quantity,accepted_quantity,rejected_quantity,defect_reason_id,reason,disposition,occurred_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)", id, in.Kind, in.OrderID, in.SourceDocumentID, in.WorkSessionID, in.OutputLotCode, in.LocationID, in.Quantity, in.AcceptedQuantity, in.RejectedQuantity, in.DefectReasonID, in.Reason, in.Disposition, *in.OccurredAt, actor.ID); e != nil {
			return e
		}
		for i, l := range in.Lines {
			if e = inputQty(l.Quantity, 1); e != nil {
				return e
			}
			if _, e = tx.Exec(c.Context(), "INSERT INTO v2.document_lines(document_id,line_number,lot_id,location_id,quantity) VALUES($1,$2,$3,$4,$5)", id, i+1, l.LotID, l.LocationID, l.Quantity); e != nil {
				return e
			}
		}
		for _, r := range in.InspectionResults {
			if _, e = tx.Exec(c.Context(), "INSERT INTO v2.inspection_results(document_id,item_code,result,note) VALUES($1,$2,$3,$4)", id, r.ItemCode, r.Result, r.Note); e != nil {
				return e
			}
		}
		if e = auditEvent(c.Context(), tx, actor, "document_create", id, in.Reason, map[string]any{"kind": in.Kind}); e != nil {
			return e
		}
		return saveReplay(c.Context(), tx, actor, "document:create", idempotency, request, result)
	})
	if e != nil {
		return respond(c, e)
	}
	if replayFound {
		return one(c, 200, result)
	}
	return one(c, 201, result)
}
func (s *Service) documents(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner", "materials", "operator", "quality"); e != nil {
		return respond(c, e)
	}
	rows, e := s.Pool.Query(c.Context(), "SELECT row_to_json(v) FROM v2.document_view v ORDER BY created_at DESC LIMIT 500")
	if e != nil {
		return respond(c, e)
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return respond(c, e)
		}
		var v any
		_ = json.Unmarshal(b, &v)
		out = append(out, v)
	}
	return one(c, 200, out)
}
func (s *Service) document(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner", "materials", "operator", "quality"); e != nil {
		return respond(c, e)
	}
	id, e := uuid.Parse(c.Params("id"))
	if e != nil {
		return respond(c, e)
	}
	var b []byte
	if e = s.Pool.QueryRow(c.Context(), "SELECT row_to_json(v) FROM v2.document_view v WHERE id=$1", id).Scan(&b); e != nil {
		return respond(c, e)
	}
	var v any
	_ = json.Unmarshal(b, &v)
	return one(c, 200, v)
}
func (s *Service) updateDocument(c fiber.Ctx) error {
	return respond(c, conflict("POSTED_IMMUTABLE", "확정 문서는 역분개 후 새 문서로 입력합니다."))
}

func requirePostedSource(ctx context.Context, tx pgx.Tx, source, order *uuid.UUID, kinds ...string) error {
	if source == nil || order == nil {
		return domain.Input("원문서와 작업 지시가 필요합니다.")
	}
	var sourceOrder *uuid.UUID
	var kind, status string
	if err := tx.QueryRow(ctx, "SELECT order_id,kind,status FROM v2.documents WHERE id=$1 FOR UPDATE", *source).Scan(&sourceOrder, &kind, &status); err != nil {
		return err
	}
	if sourceOrder == nil || *sourceOrder != *order {
		return conflict("SOURCE_ORDER_MISMATCH", "원문서는 같은 작업 지시에 속해야 합니다.")
	}
	if status != "posted" {
		return conflict("SOURCE_NOT_POSTED", "확정된 원문서만 참조할 수 있습니다.")
	}
	if !slices.Contains(kinds, kind) {
		return conflict("SOURCE_KIND_MISMATCH", "업무 단계에 맞는 원문서를 참조해야 합니다.")
	}
	return nil
}

func requireActiveWorkSession(ctx context.Context, tx pgx.Tx, actor Actor, order uuid.UUID, sessionID *uuid.UUID) error {
	if sessionID == nil || *sessionID == uuid.Nil {
		return conflict("WORK_SESSION_REQUIRED", "생산·재작업 확정에는 활성 작업 세션이 필요합니다.")
	}
	var sessionOrder, user uuid.UUID
	var status string
	if err := tx.QueryRow(ctx, "SELECT order_id,user_id,status FROM v2.work_sessions WHERE id=$1 FOR UPDATE", *sessionID).Scan(&sessionOrder, &user, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return conflict("WORK_SESSION_REQUIRED", "활성 작업 세션이 필요합니다.")
		}
		return err
	}
	if sessionOrder != order || user != actor.ID {
		return Forbidden()
	}
	if status != "active" {
		return conflict("WORK_SESSION_NOT_ACTIVE", "종료된 작업 세션으로 생산·재작업을 확정할 수 없습니다.")
	}
	return nil
}

func (s *Service) postDocument(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	id, e := uuid.Parse(c.Params("id"))
	if e != nil {
		return respond(c, e)
	}
	idempotency, e := key(c)
	if e != nil {
		return respond(c, e)
	}
	var out any
	e = s.tx(c.Context(), func(tx pgx.Tx) error {
		previous, found, err := replay(c.Context(), tx, actor, "document:post:"+id.String(), idempotency, map[string]any{"document_id": id})
		if err != nil {
			return err
		}
		if found {
			if err = json.Unmarshal(previous, &out); err != nil {
				return err
			}
			return nil
		}
		var kind, status string
		var order *uuid.UUID
		var qty, accepted, rejected int64
		var src, loc, defect, workSession *uuid.UUID
		var reason, disposition string
		var createdBy uuid.UUID
		if e = tx.QueryRow(c.Context(), "SELECT kind,status,order_id,quantity,accepted_quantity,rejected_quantity,source_document_id,location_id,defect_reason_id,work_session_id,reason,disposition,created_by FROM v2.documents WHERE id=$1 FOR UPDATE", id).Scan(&kind, &status, &order, &qty, &accepted, &rejected, &src, &loc, &defect, &workSession, &reason, &disposition, &createdBy); e != nil {
			return e
		}
		if e = requireRole(actor, documentRole(kind)...); e != nil {
			return e
		}
		if createdBy != actor.ID {
			return Forbidden()
		}
		if status != "draft" {
			return conflict("INVALID_STATE", "초안 문서만 확정할 수 있습니다.")
		}
		switch kind {
		case "component_receipt":
			e = s.postReceipt(c.Context(), tx, id, loc, qty)
		case "issue", "return", "material_loss":
			e = s.postMaterial(c.Context(), tx, id, kind, order, loc, qty)
		case "production":
			e = s.postProduction(c.Context(), tx, actor, id, order, workSession, qty, reason)
		case "inspection":
			e = s.postInspection(c.Context(), tx, id, order, src, defect, accepted, rejected)
		case "disposition":
			e = s.postDisposition(c.Context(), tx, id, order, src, qty, reason, disposition)
		case "rework":
			e = s.postRework(c.Context(), tx, actor, id, order, src, workSession, qty)
		case "goods_receipt":
			e = s.postGoodsReceipt(c.Context(), tx, id, order, src, qty, loc)
		default:
			e = domain.Input("전표 종류가 올바르지 않습니다.")
		}
		if e != nil {
			return e
		}
		if src != nil {
			if _, e = tx.Exec(c.Context(), "INSERT INTO v2.document_dependencies(parent_id,child_id) VALUES($1,$2) ON CONFLICT DO NOTHING", *src, id); e != nil {
				return e
			}
		}
		if e = s.fail("after_document"); e != nil {
			return e
		}
		if _, e = tx.Exec(c.Context(), "UPDATE v2.documents SET status='posted',posted_by=$2,posted_at=now() WHERE id=$1", id, actor.ID); e != nil {
			return e
		}
		if e = auditEvent(c.Context(), tx, actor, "document_post", id, reason, map[string]any{"kind": kind}); e != nil {
			return e
		}
		out = map[string]any{"id": id, "status": "posted", "kind": kind}
		if e = saveReplay(c.Context(), tx, actor, "document:post:"+id.String(), idempotency, map[string]any{"document_id": id}, map[string]any{"id": id, "status": "posted", "kind": kind}); e != nil {
			return e
		}
		return nil
	})
	if e != nil {
		return respond(c, e)
	}
	return one(c, 200, out)
}
func (s *Service) lines(ctx context.Context, tx pgx.Tx, id uuid.UUID) ([]struct {
	lot, location uuid.UUID
	q             int64
}, error) {
	rows, e := tx.Query(ctx, "SELECT lot_id,location_id,quantity FROM v2.document_lines WHERE document_id=$1 ORDER BY line_number", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []struct {
		lot, location uuid.UUID
		q             int64
	}{}
	for rows.Next() {
		var v struct {
			lot, location uuid.UUID
			q             int64
		}
		if e = rows.Scan(&v.lot, &v.location, &v.q); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) balance(ctx context.Context, tx pgx.Tx, item, lot, location, owner uuid.UUID) (int64, error) {
	var quantity int64
	err := tx.QueryRow(ctx, "SELECT quantity FROM v2.inventory_balances WHERE item_id=$1 AND lot_id=$2 AND location_id=$3 AND owner_id=$4 FOR UPDATE", item, lot, location, owner).Scan(&quantity)
	if err == nil {
		return quantity, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO v2.inventory_balances(item_id,lot_id,location_id,owner_id,quantity) VALUES($1,$2,$3,$4,0) ON CONFLICT(item_id,lot_id,location_id,owner_id) DO NOTHING", item, lot, location, owner); err != nil {
		return 0, err
	}
	if err = tx.QueryRow(ctx, "SELECT quantity FROM v2.inventory_balances WHERE item_id=$1 AND lot_id=$2 AND location_id=$3 AND owner_id=$4 FOR UPDATE", item, lot, location, owner).Scan(&quantity); err != nil {
		return 0, err
	}
	return quantity, nil
}
func ensureLocationKind(ctx context.Context, tx pgx.Tx, id uuid.UUID, kinds ...string) error {
	var kind string
	if err := tx.QueryRow(ctx, "SELECT kind FROM v2.locations WHERE id=$1 AND active", id).Scan(&kind); err != nil {
		return err
	}
	if !slices.Contains(kinds, kind) {
		return domain.Input("전표 위치 종류가 업무 흐름과 맞지 않습니다.")
	}
	return nil
}

func (s *Service) move(ctx context.Context, tx pgx.Tx, doc, item, lot, from, to, owner uuid.UUID, q int64, movement string) error {
	return s.moveOwned(ctx, tx, doc, item, lot, from, to, owner, owner, q, movement)
}
func (s *Service) moveOwned(ctx context.Context, tx pgx.Tx, doc, item, lot, from, to, fromOwner, toOwner uuid.UUID, q int64, movement string) error {
	if q <= 0 {
		return domain.Input("수량은 양수여야 합니다.")
	}
	available, e := s.balance(ctx, tx, item, lot, from, fromOwner)
	if e != nil {
		return e
	}
	if available < q {
		return conflict("INSUFFICIENT_STOCK", "가용 재고가 부족합니다.")
	}
	if _, e = tx.Exec(ctx, "UPDATE v2.inventory_balances SET quantity=quantity-$5 WHERE item_id=$1 AND lot_id=$2 AND location_id=$3 AND owner_id=$4", item, lot, from, fromOwner, q); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO v2.material_ledger(document_id,item_id,lot_id,location_id,owner_id,quantity,movement) VALUES($1,$2,$3,$4,$5,$6,$7)", doc, item, lot, from, fromOwner, -q, movement); e != nil {
		return e
	}
	if from == to && fromOwner == toOwner {
		return nil
	}
	if _, e = tx.Exec(ctx, "INSERT INTO v2.inventory_balances(item_id,lot_id,location_id,owner_id,quantity) VALUES($1,$2,$3,$4,$5) ON CONFLICT(item_id,lot_id,location_id,owner_id) DO UPDATE SET quantity=v2.inventory_balances.quantity+EXCLUDED.quantity", item, lot, to, toOwner, q); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO v2.material_ledger(document_id,item_id,lot_id,location_id,owner_id,quantity,movement) VALUES($1,$2,$3,$4,$5,$6,$7)", doc, item, lot, to, toOwner, q, movement)
	return e
}
func (s *Service) postReceipt(ctx context.Context, tx pgx.Tx, doc uuid.UUID, loc *uuid.UUID, q int64) error {
	if err := inputQty(q, 1); err != nil {
		return err
	}
	ls, err := s.lines(ctx, tx, doc)
	if err != nil {
		return err
	}
	if len(ls) == 0 {
		return domain.Input("입고 전표에는 한 줄 이상의 로트가 필요합니다.")
	}
	var total int64
	for _, l := range ls {
		total += l.q
		if total > 1_000_000 {
			return domain.Input("입고 수량 상한을 초과했습니다.")
		}
	}
	if total != q {
		return conflict("QUANTITY_MISMATCH", "입고 수량과 로트별 합계가 다릅니다.")
	}
	for _, l := range ls {
		var item uuid.UUID
		if err = tx.QueryRow(ctx, "SELECT item_id FROM v2.lots WHERE id=$1", l.lot).Scan(&item); err != nil {
			return fmt.Errorf("receipt lot: %w", err)
		}
		location := l.location
		if loc != nil {
			location = *loc
		}
		if err = ensureLocationKind(ctx, tx, location, "warehouse"); err != nil {
			return fmt.Errorf("receipt location: %w", err)
		}
		if _, err = s.balance(ctx, tx, item, l.lot, location, uuid.Nil); err != nil {
			return fmt.Errorf("receipt balance: %w", err)
		}
		if _, err = tx.Exec(ctx, "UPDATE v2.inventory_balances SET quantity=quantity+$5 WHERE item_id=$1 AND lot_id=$2 AND location_id=$3 AND owner_id=$4", item, l.lot, location, uuid.Nil, l.q); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v2.material_ledger(document_id,item_id,lot_id,location_id,owner_id,quantity,movement) VALUES($1,$2,$3,$4,$5,$6,'receipt')", doc, item, l.lot, location, uuid.Nil, l.q); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) postMaterial(ctx context.Context, tx pgx.Tx, doc uuid.UUID, kind string, order *uuid.UUID, loc *uuid.UUID, q int64) error {
	ls, e := s.lines(ctx, tx, doc)
	if e != nil {
		return e
	}
	if len(ls) == 0 {
		return domain.Input("자재 전표에는 한 줄 이상의 로트가 필요합니다.")
	}
	if order == nil {
		return domain.Input("자재 전표에는 작업 지시가 필요합니다.")
	}
	owner := *order
	var status string
	if e = tx.QueryRow(ctx, "SELECT status FROM v2.work_orders WHERE id=$1 FOR UPDATE", owner).Scan(&status); e != nil {
		return e
	}
	if kind == "issue" {
		if e = requireState(status, "issued", "in_progress"); e != nil {
			return e
		}
	} else if e = requireState(status, "issued", "in_progress", "held"); e != nil {
		return e
	}
	for _, l := range ls {
		var item uuid.UUID
		if e = tx.QueryRow(ctx, "SELECT item_id FROM v2.lots WHERE id=$1", l.lot).Scan(&item); e != nil {
			return e
		}
		switch kind {
		case "issue":
			if loc == nil {
				return domain.Input("불출에는 현장 위치가 필요합니다.")
			}
			if e = ensureLocationKind(ctx, tx, l.location, "warehouse"); e != nil {
				return e
			}
			if e = ensureLocationKind(ctx, tx, *loc, "floor"); e != nil {
				return e
			}
			if e = s.moveOwned(ctx, tx, doc, item, l.lot, l.location, *loc, uuid.Nil, owner, l.q, "issue"); e != nil {
				return e
			}
		case "return":
			if loc == nil {
				return domain.Input("반납에는 현장 위치가 필요합니다.")
			}
			if e = ensureLocationKind(ctx, tx, *loc, "floor"); e != nil {
				return e
			}
			if e = ensureLocationKind(ctx, tx, l.location, "warehouse"); e != nil {
				return e
			}
			if e = s.moveOwned(ctx, tx, doc, item, l.lot, *loc, l.location, owner, uuid.Nil, l.q, "return"); e != nil {
				return e
			}
		case "material_loss":
			if e = ensureLocationKind(ctx, tx, l.location, "floor"); e != nil {
				return e
			}
			if e = s.move(ctx, tx, doc, item, l.lot, l.location, l.location, owner, l.q, "material_loss"); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Service) postProduction(ctx context.Context, tx pgx.Tx, actor Actor, doc uuid.UUID, order, workSession *uuid.UUID, q int64, reason string) error {
	if order == nil {
		return domain.Input("생산 보고에는 지시와 양수가 필요합니다.")
	}
	if err := inputQty(q, 1); err != nil {
		return err
	}
	var status string
	var allowance int64
	var assigned uuid.UUID
	if e := tx.QueryRow(ctx, "SELECT status,start_allowance,assigned_user_id FROM v2.work_orders WHERE id=$1 FOR UPDATE", *order).Scan(&status, &allowance, &assigned); e != nil {
		return e
	}
	if assigned != actor.ID {
		return Forbidden()
	}
	if e := requireState(status, "in_progress"); e != nil {
		return e
	}
	if e := requireActiveWorkSession(ctx, tx, actor, *order, workSession); e != nil {
		return e
	}
	var produced int64
	if e := tx.QueryRow(ctx, "SELECT COALESCE(sum(o.new_output_quantity),0) FROM v2.output_lots o JOIN v2.documents d ON d.id=o.production_document_id WHERE o.order_id=$1 AND d.status='posted'", *order).Scan(&produced); e != nil {
		return e
	}
	if produced+q > allowance {
		return conflict("START_ALLOWANCE_EXCEEDED", "착수 허용량을 초과했습니다.")
	}
	ls, e := s.lines(ctx, tx, doc)
	if e != nil {
		return e
	}
	var snapshot []byte
	if e = tx.QueryRow(ctx, "SELECT bom_snapshot FROM v2.work_orders WHERE id=$1", *order).Scan(&snapshot); e != nil {
		return e
	}
	var expected []struct {
		ComponentItemID uuid.UUID `json:"component_item_id"`
		Quantity        int64     `json:"quantity"`
	}
	if e = json.Unmarshal(snapshot, &expected); e != nil {
		return e
	}
	actual := map[uuid.UUID]int64{}
	for _, l := range ls {
		var item uuid.UUID
		if e = tx.QueryRow(ctx, "SELECT item_id FROM v2.lots WHERE id=$1", l.lot).Scan(&item); e != nil {
			return e
		}
		if e = ensureLocationKind(ctx, tx, l.location, "floor"); e != nil {
			return e
		}
		actual[item] += l.q
		if e = s.move(ctx, tx, doc, item, l.lot, l.location, l.location, *order, l.q, "consumption"); e != nil {
			return e
		}
	}
	var variance bool
	for _, line := range expected {
		if actual[line.ComponentItemID] != line.Quantity*q {
			variance = true
			break
		}
		delete(actual, line.ComponentItemID)
	}
	if len(actual) > 0 {
		variance = true
	}
	if variance {
		if strings.TrimSpace(reason) == "" {
			return conflict("VARIANCE_REASON_REQUIRED", "BOM 대비 실제 소비 차이에는 사유가 필요합니다.")
		}
		if _, e = tx.Exec(ctx, "UPDATE v2.documents SET has_variance=true WHERE id=$1", doc); e != nil {
			return e
		}
	}
	var finished uuid.UUID
	if e = tx.QueryRow(ctx, "SELECT finished_item_id FROM v2.work_orders WHERE id=$1", *order).Scan(&finished); e != nil {
		return e
	}
	outID := uuid.New()
	code := fmt.Sprintf("OUT-%s", outID.String()[:8])
	if _, e = tx.Exec(ctx, "INSERT INTO v2.lots(id,item_id,code) VALUES($1,$2,$3)", outID, finished, code); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE v2.documents SET output_lot_id=$2,output_lot_code=$3,quantity=$4 WHERE id=$1", doc, outID, code, q); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO v2.output_lots(id,order_id,production_document_id,new_output_quantity) VALUES($1,$2,$3,$4)", outID, *order, doc, q); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO v2.output_balances(source_document_id,output_lot_id,bucket,quantity) VALUES($1,$2,'pending',$3)", doc, outID, q)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO v2.output_ledger(document_id,source_document_id,output_lot_id,bucket,quantity) VALUES($1,$2,$3,'pending',$4)", doc, doc, outID, q)
	return e
}
func (s *Service) outputMove(ctx context.Context, tx pgx.Tx, doc, src uuid.UUID, sourceBucket, destinationBucket string, q int64) error {
	if q <= 0 {
		return domain.Input("수량은 양수여야 합니다.")
	}
	var lot uuid.UUID
	var avail int64
	if e := tx.QueryRow(ctx, "SELECT output_lot_id,quantity FROM v2.output_balances WHERE source_document_id=$1 AND bucket=$2 FOR UPDATE", src, sourceBucket).Scan(&lot, &avail); e != nil {
		return e
	}
	if avail < q {
		return conflict("OUTPUT_POOL_EXCEEDED", "대상 산출 잔량을 초과했습니다.")
	}
	if _, e := tx.Exec(ctx, "UPDATE v2.output_balances SET quantity=quantity-$3 WHERE source_document_id=$1 AND bucket=$2", src, sourceBucket, q); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, "INSERT INTO v2.output_ledger(document_id,source_document_id,output_lot_id,bucket,quantity) VALUES($1,$2,$3,$4,$5)", doc, src, lot, sourceBucket, -q); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, "INSERT INTO v2.output_balances(source_document_id,output_lot_id,bucket,quantity) VALUES($1,$2,$3,$4) ON CONFLICT(source_document_id,bucket) DO UPDATE SET quantity=v2.output_balances.quantity+EXCLUDED.quantity", doc, lot, destinationBucket, q); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "INSERT INTO v2.output_ledger(document_id,source_document_id,output_lot_id,bucket,quantity) VALUES($1,$2,$3,$4,$5)", doc, doc, lot, destinationBucket, q)
	return e
}
func (s *Service) postInspection(ctx context.Context, tx pgx.Tx, doc uuid.UUID, order, src, defect *uuid.UUID, accepted, rejected int64) error {
	if order == nil || src == nil || accepted < 0 || rejected < 0 || accepted+rejected == 0 {
		return domain.Input("검사 결과와 원산출 문서가 필요합니다.")
	}
	if accepted+rejected > 1_000_000 {
		return domain.Input("검사 수량 상한을 초과했습니다.")
	}
	var orderStatus string
	if e := tx.QueryRow(ctx, "SELECT status FROM v2.work_orders WHERE id=$1 FOR UPDATE", *order).Scan(&orderStatus); e != nil {
		return e
	}
	if e := requireState(orderStatus, "issued", "in_progress", "held"); e != nil {
		return e
	}
	if e := requirePostedSource(ctx, tx, src, order, "production", "rework"); e != nil {
		return e
	}
	if rejected > 0 {
		if defect == nil || *defect == uuid.Nil {
			return conflict("DEFECT_REASON_REQUIRED", "부적합 수량에는 불량 사유가 필요합니다.")
		}
		var active bool
		if e := tx.QueryRow(ctx, "SELECT active FROM v2.defect_reasons WHERE id=$1", *defect).Scan(&active); e != nil {
			return e
		}
		if !active {
			return conflict("DEFECT_REASON_REQUIRED", "활성 불량 사유가 필요합니다.")
		}
	}
	var snapshot []byte
	if e := tx.QueryRow(ctx, "SELECT inspection_snapshot FROM v2.work_orders WHERE id=$1", *order).Scan(&snapshot); e != nil {
		return e
	}
	var criteria []struct {
		ItemCode string `json:"item_code"`
		Required bool   `json:"required"`
	}
	if e := json.Unmarshal(snapshot, &criteria); e != nil {
		return e
	}
	for _, criterion := range criteria {
		if !criterion.Required {
			continue
		}
		var result string
		if e := tx.QueryRow(ctx, "SELECT result FROM v2.inspection_results WHERE document_id=$1 AND item_code=$2", doc, criterion.ItemCode).Scan(&result); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return conflict("INSPECTION_CRITERIA_REQUIRED", "필수 검사 항목 결과가 누락되었습니다.")
			}
			return e
		}
		if accepted > 0 && result != "pass" {
			return conflict("INSPECTION_CRITERIA_FAILED", "전량 합격 처리에는 모든 필수 검사 항목의 합격 결과가 필요합니다.")
		}
	}
	if accepted > 0 {
		if e := s.outputMove(ctx, tx, doc, *src, "pending", "accepted", accepted); e != nil {
			return e
		}
	}
	if rejected > 0 {
		if e := s.outputMove(ctx, tx, doc, *src, "pending", "rejected", rejected); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) postDisposition(ctx context.Context, tx pgx.Tx, doc uuid.UUID, order, src *uuid.UUID, q int64, reason, disposition string) error {
	if order == nil || src == nil || strings.TrimSpace(reason) == "" || (disposition != "disposal" && disposition != "rework") {
		return domain.Input("부적합 처분에는 원검사·처분·사유가 필요합니다.")
	}
	var orderStatus string
	if e := tx.QueryRow(ctx, "SELECT status FROM v2.work_orders WHERE id=$1 FOR UPDATE", *order).Scan(&orderStatus); e != nil {
		return e
	}
	if e := requireState(orderStatus, "issued", "in_progress", "held"); e != nil {
		return e
	}
	if e := requirePostedSource(ctx, tx, src, order, "inspection"); e != nil {
		return e
	}
	bucket := map[string]string{"disposal": "disposed", "rework": "rework"}[disposition]
	return s.outputMove(ctx, tx, doc, *src, "rejected", bucket, q)
}
func (s *Service) postRework(ctx context.Context, tx pgx.Tx, actor Actor, doc uuid.UUID, order, src, workSession *uuid.UUID, q int64) error {
	if order == nil || src == nil {
		return domain.Input("재작업에는 작업 지시와 원 처분 문서가 필요합니다.")
	}
	if err := inputQty(q, 1); err != nil {
		return err
	}
	var status string
	var assigned uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT status,assigned_user_id FROM v2.work_orders WHERE id=$1 FOR UPDATE", *order).Scan(&status, &assigned); err != nil {
		return err
	}
	if assigned != actor.ID {
		return Forbidden()
	}
	if err := requireState(status, "in_progress"); err != nil {
		return err
	}
	if err := requireActiveWorkSession(ctx, tx, actor, *order, workSession); err != nil {
		return err
	}
	if err := requirePostedSource(ctx, tx, src, order, "disposition"); err != nil {
		return err
	}
	var sourceDisposition string
	if err := tx.QueryRow(ctx, "SELECT disposition FROM v2.documents WHERE id=$1", *src).Scan(&sourceDisposition); err != nil {
		return err
	}
	if sourceDisposition != "rework" {
		return conflict("SOURCE_KIND_MISMATCH", "재작업 처분으로 확정된 원문서가 필요합니다.")
	}
	if err := s.outputMove(ctx, tx, doc, *src, "rework", "pending", q); err != nil {
		return err
	}
	ls, err := s.lines(ctx, tx, doc)
	if err != nil {
		return err
	}
	for _, l := range ls {
		var item uuid.UUID
		if err = tx.QueryRow(ctx, "SELECT item_id FROM v2.lots WHERE id=$1", l.lot).Scan(&item); err != nil {
			return err
		}
		if err = ensureLocationKind(ctx, tx, l.location, "floor"); err != nil {
			return err
		}
		if err = s.move(ctx, tx, doc, item, l.lot, l.location, l.location, *order, l.q, "consumption"); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) postGoodsReceipt(ctx context.Context, tx pgx.Tx, doc uuid.UUID, order, src *uuid.UUID, q int64, loc *uuid.UUID) error {
	if src == nil || loc == nil || q <= 0 {
		return domain.Input("합격품 입고에는 합격 검사·위치·수량이 필요합니다.")
	}
	if order == nil {
		return domain.Input("완제품 입고에는 지시가 필요합니다.")
	}
	if err := ensureLocationKind(ctx, tx, *loc, "finished"); err != nil {
		return err
	}
	var status string
	var target, received int64
	if e := tx.QueryRow(ctx, "SELECT status,target_quantity,COALESCE((SELECT sum(quantity) FROM v2.output_ledger WHERE source_document_id IN (SELECT id FROM v2.documents WHERE order_id=$1) AND bucket='received'),0) FROM v2.work_orders WHERE id=$1 FOR UPDATE", *order).Scan(&status, &target, &received); e != nil {
		return e
	}
	if e := requireState(status, "issued", "in_progress"); e != nil {
		return e
	}
	if e := requirePostedSource(ctx, tx, src, order, "inspection"); e != nil {
		return e
	}
	if received+q > target {
		return conflict("TARGET_EXCEEDED", "목표 변경 승인 전에는 목표를 초과해 입고할 수 없습니다.")
	}
	if e := s.outputMove(ctx, tx, doc, *src, "accepted", "received", q); e != nil {
		return e
	}
	var lot, item uuid.UUID
	if e := tx.QueryRow(ctx, "SELECT output_lot_id FROM v2.output_balances WHERE source_document_id=$1 AND bucket='received'", doc).Scan(&lot); e != nil {
		return e
	}
	if e := tx.QueryRow(ctx, "SELECT item_id FROM v2.lots WHERE id=$1", lot).Scan(&item); e != nil {
		return e
	}
	if _, e := s.balance(ctx, tx, item, lot, *loc, uuid.Nil); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, "UPDATE v2.inventory_balances SET quantity=quantity+$4 WHERE item_id=$1 AND lot_id=$2 AND location_id=$3 AND owner_id=$5", item, lot, *loc, q, uuid.Nil); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "INSERT INTO v2.material_ledger(document_id,item_id,lot_id,location_id,owner_id,quantity,movement) VALUES($1,$2,$3,$4,$5,$6,'goods_receipt')", doc, item, lot, *loc, uuid.Nil, q)
	return e
}
func (s *Service) reverseDocument(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin"); e != nil {
		return respond(c, e)
	}
	id, e := uuid.Parse(c.Params("id"))
	if e != nil {
		return respond(c, e)
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if e = jsonBody(c, &in); e != nil || strings.TrimSpace(in.Reason) == "" {
		return respond(c, domain.Input("역분개 사유가 필요합니다."))
	}
	idempotency, e := key(c)
	if e != nil {
		return respond(c, e)
	}
	request := map[string]any{"id": id, "reason": in.Reason}
	result := map[string]any{"id": id, "status": "reversed"}
	replayFound := false
	e = s.tx(c.Context(), func(tx pgx.Tx) error {
		previous, replayed, e := replay(c.Context(), tx, actor, "document:reverse:"+id.String(), idempotency, request)
		if e != nil {
			return e
		}
		if replayed {
			if e = json.Unmarshal(previous, &result); e != nil {
				return e
			}
			replayFound = true
			return nil
		}
		var status string
		if e = tx.QueryRow(c.Context(), "SELECT status FROM v2.documents WHERE id=$1 FOR UPDATE", id).Scan(&status); e != nil {
			return e
		}
		if status != "posted" {
			return conflict("INVALID_STATE", "확정된 문서만 역분개할 수 있습니다.")
		}
		var dependent bool
		if e = tx.QueryRow(c.Context(), "SELECT EXISTS(SELECT 1 FROM v2.document_dependencies dep JOIN v2.documents child ON child.id=dep.child_id WHERE dep.parent_id=$1 AND child.status='posted')", id).Scan(&dependent); e != nil {
			return e
		}
		if dependent {
			return conflict("DEPENDENCY_EXISTS", "후속 문서를 먼저 역분개해야 합니다.")
		}
		reversalID := uuid.New()
		if _, e = tx.Exec(c.Context(), "INSERT INTO v2.reversals(id,document_id,reason,actor_id) VALUES($1,$2,$3,$4)", reversalID, id, in.Reason, actor.ID); e != nil {
			return e
		}
		if _, e = tx.Exec(c.Context(), "UPDATE v2.inventory_balances b SET quantity=b.quantity-x.quantity FROM (SELECT item_id,lot_id,location_id,owner_id,SUM(quantity) AS quantity FROM v2.material_ledger WHERE document_id=$1 GROUP BY item_id,lot_id,location_id,owner_id) x WHERE b.item_id=x.item_id AND b.lot_id=x.lot_id AND b.location_id=x.location_id AND b.owner_id=x.owner_id", id); e != nil {
			return e
		}
		if _, e = tx.Exec(c.Context(), "UPDATE v2.output_balances b SET quantity=b.quantity-x.quantity FROM (SELECT source_document_id,bucket,SUM(quantity) AS quantity FROM v2.output_ledger WHERE document_id=$1 GROUP BY source_document_id,bucket) x WHERE b.source_document_id=x.source_document_id AND b.bucket=x.bucket", id); e != nil {
			return e
		}
		if _, e = tx.Exec(c.Context(), "INSERT INTO v2.material_ledger(document_id,reversal_id,item_id,lot_id,location_id,owner_id,quantity,movement) SELECT $1,$2,item_id,lot_id,location_id,owner_id,-quantity,'reversal' FROM v2.material_ledger WHERE document_id=$1", id, reversalID); e != nil {
			return e
		}
		if _, e = tx.Exec(c.Context(), "INSERT INTO v2.output_ledger(document_id,reversal_id,source_document_id,output_lot_id,bucket,quantity) SELECT $1,$2,source_document_id,output_lot_id,bucket,-quantity FROM v2.output_ledger WHERE document_id=$1", id, reversalID); e != nil {
			return e
		}
		if _, e = tx.Exec(c.Context(), "UPDATE v2.documents SET status='reversed' WHERE id=$1", id); e != nil {
			return e
		}
		if e = auditEvent(c.Context(), tx, actor, "document_reverse", id, in.Reason, map[string]any{"reversal_id": reversalID}); e != nil {
			return e
		}
		return saveReplay(c.Context(), tx, actor, "document:reverse:"+id.String(), idempotency, request, result)
	})
	if e != nil {
		return respond(c, e)
	}
	if replayFound {
		return one(c, 200, result)
	}
	return one(c, 200, result)
}

func (s *Service) inventory(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner", "materials", "operator", "quality"); e != nil {
		return respond(c, e)
	}
	rows, e := s.Pool.Query(c.Context(), "SELECT row_to_json(v) FROM v2.inventory_view v WHERE quantity<>0 ORDER BY item_code,lot_code,location_code")
	if e != nil {
		return respond(c, e)
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return respond(c, e)
		}
		var v any
		_ = json.Unmarshal(b, &v)
		out = append(out, v)
	}
	return one(c, 200, out)
}
func (s *Service) outputLots(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner", "materials", "operator", "quality"); e != nil {
		return respond(c, e)
	}
	rows, e := s.Pool.Query(c.Context(), "SELECT row_to_json(v) FROM v2.output_lot_view v ORDER BY id")
	if e != nil {
		return respond(c, e)
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var b []byte
		_ = rows.Scan(&b)
		var v any
		_ = json.Unmarshal(b, &v)
		out = append(out, v)
	}
	return one(c, 200, out)
}
func (s *Service) ledger(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner", "materials", "operator", "quality"); e != nil {
		return respond(c, e)
	}
	rows, e := s.Pool.Query(c.Context(), "SELECT row_to_json(v) FROM v2.material_ledger_view v ORDER BY recorded_at DESC LIMIT 1000")
	if e != nil {
		return respond(c, e)
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var b []byte
		_ = rows.Scan(&b)
		var v any
		_ = json.Unmarshal(b, &v)
		out = append(out, v)
	}
	return one(c, 200, out)
}
func (s *Service) reconciliation(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner", "materials", "operator", "quality"); e != nil {
		return respond(c, e)
	}
	ctx := c.Context()
	var inventoryJSON, outputJSON, lotJSON []byte
	if e = s.Pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(row_to_json(x)), '[]'::jsonb) FROM (
		SELECT b.item_id,b.lot_id,b.location_id,b.owner_id,b.quantity AS balance,COALESCE(sum(l.quantity),0)::bigint AS ledger_quantity
		FROM v2.inventory_balances b LEFT JOIN v2.material_ledger l ON l.item_id=b.item_id AND l.lot_id=b.lot_id AND l.location_id=b.location_id AND l.owner_id=b.owner_id
		GROUP BY b.item_id,b.lot_id,b.location_id,b.owner_id,b.quantity
		HAVING b.quantity <> COALESCE(sum(l.quantity),0)
		UNION ALL
		SELECT l.item_id,l.lot_id,l.location_id,l.owner_id,0::bigint,sum(l.quantity)::bigint
		FROM v2.material_ledger l LEFT JOIN v2.inventory_balances b ON b.item_id=l.item_id AND b.lot_id=l.lot_id AND b.location_id=l.location_id AND b.owner_id=l.owner_id
		WHERE b.item_id IS NULL GROUP BY l.item_id,l.lot_id,l.location_id,l.owner_id
	) x`).Scan(&inventoryJSON); e != nil {
		return respond(c, e)
	}
	if e = s.Pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(row_to_json(x)), '[]'::jsonb) FROM (
		SELECT b.source_document_id,b.output_lot_id,b.bucket,b.quantity AS balance,COALESCE(sum(l.quantity),0)::bigint AS ledger_quantity
		FROM v2.output_balances b LEFT JOIN v2.output_ledger l ON l.source_document_id=b.source_document_id AND l.output_lot_id=b.output_lot_id AND l.bucket=b.bucket
		GROUP BY b.source_document_id,b.output_lot_id,b.bucket,b.quantity
		HAVING b.quantity <> COALESCE(sum(l.quantity),0)
		UNION ALL
		SELECT l.source_document_id,l.output_lot_id,l.bucket,0::bigint,sum(l.quantity)::bigint
		FROM v2.output_ledger l LEFT JOIN v2.output_balances b ON b.source_document_id=l.source_document_id AND b.output_lot_id=l.output_lot_id AND b.bucket=l.bucket
		WHERE b.source_document_id IS NULL GROUP BY l.source_document_id,l.output_lot_id,l.bucket
	) x`).Scan(&outputJSON); e != nil {
		return respond(c, e)
	}
	if e = s.Pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(row_to_json(x)), '[]'::jsonb) FROM (
		SELECT id,code,new_output_quantity,
			(pending_quantity+accepted_quantity+rejected_quantity+rework_quantity+disposed_quantity+received_quantity)::bigint AS state_quantity
		FROM v2.output_lot_view
		WHERE new_output_quantity <> (pending_quantity+accepted_quantity+rejected_quantity+rework_quantity+disposed_quantity+received_quantity)
	) x`).Scan(&lotJSON); e != nil {
		return respond(c, e)
	}
	var inv, output, lots []any
	_ = json.Unmarshal(inventoryJSON, &inv)
	_ = json.Unmarshal(outputJSON, &output)
	_ = json.Unmarshal(lotJSON, &lots)
	return one(c, 200, map[string]any{
		"balanced":             len(inv) == 0 && len(output) == 0 && len(lots) == 0,
		"inventory_mismatches": inv,
		"output_mismatches":    output,
		"lot_mismatches":       lots,
	})
}
func (s *Service) trace(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin", "planner", "materials", "operator", "quality"); e != nil {
		return respond(c, e)
	}
	lotCode := c.Query("lot_id")
	lotID, e := uuid.Parse(lotCode)
	if e != nil {
		return respond(c, domain.Input("lot_id UUID가 필요합니다."))
	}
	rows, e := s.Pool.Query(c.Context(), "SELECT row_to_json(v) FROM v2.document_view v WHERE v.output_lot_id=$1 OR EXISTS(SELECT 1 FROM v2.document_lines l WHERE l.document_id=v.id AND l.lot_id=$1) OR EXISTS(SELECT 1 FROM v2.document_dependencies d WHERE (d.parent_id=v.id OR d.child_id=v.id) AND (d.parent_id=$1 OR d.child_id=$1)) ORDER BY v.created_at", lotID)
	if e != nil {
		return respond(c, e)
	}
	docs := []any{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return respond(c, e)
		}
		var v any
		_ = json.Unmarshal(b, &v)
		docs = append(docs, v)
	}
	rows.Close()
	ledger := []any{}
	rows, e = s.Pool.Query(c.Context(), "SELECT row_to_json(l) FROM v2.material_ledger_view l WHERE l.lot_id=$1 ORDER BY recorded_at,id", lotID)
	if e != nil {
		return respond(c, e)
	}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return respond(c, e)
		}
		var v any
		_ = json.Unmarshal(b, &v)
		ledger = append(ledger, v)
	}
	rows.Close()
	outputLots := []any{}
	rows, e = s.Pool.Query(c.Context(), "SELECT row_to_json(v) FROM v2.output_lot_view v WHERE v.id=$1", lotID)
	if e != nil {
		return respond(c, e)
	}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return respond(c, e)
		}
		var v any
		_ = json.Unmarshal(b, &v)
		outputLots = append(outputLots, v)
	}
	rows.Close()
	audit := []any{}
	rows, e = s.Pool.Query(c.Context(), "SELECT row_to_json(a) FROM v2.audit_events a WHERE a.entity_id IN (SELECT id FROM v2.documents WHERE output_lot_id=$1 OR EXISTS(SELECT 1 FROM v2.document_lines l WHERE l.document_id=id AND l.lot_id=$1)) ORDER BY recorded_at", lotID)
	if e != nil {
		return respond(c, e)
	}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return respond(c, e)
		}
		var v any
		_ = json.Unmarshal(b, &v)
		audit = append(audit, v)
	}
	rows.Close()
	return one(c, 200, map[string]any{"lot_id": lotCode, "documents": docs, "ledger": ledger, "output_lots": outputLots, "audit": audit})
}
func (s *Service) audit(c fiber.Ctx) error {
	actor, e := CurrentActor(c)
	if e != nil {
		return respond(c, e)
	}
	if e = requireRole(actor, "admin"); e != nil {
		return respond(c, e)
	}
	rows, e := s.Pool.Query(c.Context(), "SELECT row_to_json(a) FROM v2.audit_events a ORDER BY recorded_at DESC LIMIT 1000")
	if e != nil {
		return respond(c, e)
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var b []byte
		_ = rows.Scan(&b)
		var v any
		_ = json.Unmarshal(b, &v)
		out = append(out, v)
	}
	return one(c, 200, out)
}

var _ = pgconn.PgError{}
