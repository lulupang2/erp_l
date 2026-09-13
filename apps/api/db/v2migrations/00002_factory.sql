-- +goose Up
CREATE TABLE v2.items (
 id uuid PRIMARY KEY, code text NOT NULL UNIQUE, name text NOT NULL,
 kind text NOT NULL CHECK(kind IN ('component','finished')), unit text NOT NULL DEFAULT 'ea' CHECK(unit='ea'), active boolean NOT NULL DEFAULT true
);
CREATE TABLE v2.locations (
 id uuid PRIMARY KEY, code text NOT NULL UNIQUE, name text NOT NULL,
 kind text NOT NULL CHECK(kind IN ('warehouse','floor','finished')), active boolean NOT NULL DEFAULT true
);
CREATE TABLE v2.lots (id uuid PRIMARY KEY, item_id uuid NOT NULL REFERENCES v2.items, code text NOT NULL UNIQUE, active boolean NOT NULL DEFAULT true);
CREATE TABLE v2.defect_reasons (id uuid PRIMARY KEY, code text NOT NULL UNIQUE, name text NOT NULL, active boolean NOT NULL DEFAULT true);
CREATE TABLE v2.bom_revisions (
 id uuid PRIMARY KEY, finished_item_id uuid NOT NULL REFERENCES v2.items, revision text NOT NULL,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','approved','retired')),
 created_by uuid NOT NULL REFERENCES v2.users, approved_by uuid REFERENCES v2.users, approved_at timestamptz,
 UNIQUE(finished_item_id,revision)
);
CREATE TABLE v2.bom_lines (
 revision_id uuid NOT NULL REFERENCES v2.bom_revisions, component_item_id uuid NOT NULL REFERENCES v2.items,
 quantity bigint NOT NULL CHECK(quantity BETWEEN 1 AND 1000000), PRIMARY KEY(revision_id,component_item_id)
);
CREATE TABLE v2.inspection_revisions (
 id uuid PRIMARY KEY, finished_item_id uuid NOT NULL REFERENCES v2.items, revision text NOT NULL,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','approved','retired')),
 created_by uuid NOT NULL REFERENCES v2.users, approved_by uuid REFERENCES v2.users, approved_at timestamptz,
 UNIQUE(finished_item_id,revision)
);
CREATE TABLE v2.inspection_items (
 revision_id uuid NOT NULL REFERENCES v2.inspection_revisions, item_code text NOT NULL, name text NOT NULL,
 required boolean NOT NULL, acceptance text NOT NULL, PRIMARY KEY(revision_id,item_code)
);
CREATE TABLE v2.work_orders (
 id uuid PRIMARY KEY, finished_item_id uuid NOT NULL REFERENCES v2.items,
 target_quantity bigint NOT NULL CHECK(target_quantity BETWEEN 1 AND 1000000000000),
 start_allowance bigint NOT NULL CHECK(start_allowance BETWEEN 1 AND 1000000000000),
 bom_revision_id uuid NOT NULL REFERENCES v2.bom_revisions, inspection_revision_id uuid NOT NULL REFERENCES v2.inspection_revisions,
 assigned_user_id uuid NOT NULL REFERENCES v2.users, planned_date text NOT NULL,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','issued','in_progress','held','closed','early_closed','cancelled')),
 version bigint NOT NULL DEFAULT 1, bom_snapshot jsonb NOT NULL DEFAULT '[]', inspection_snapshot jsonb NOT NULL DEFAULT '[]',
 created_by uuid NOT NULL REFERENCES v2.users, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE v2.work_sessions (
 id uuid PRIMARY KEY, order_id uuid NOT NULL REFERENCES v2.work_orders, user_id uuid NOT NULL REFERENCES v2.users,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','ended')),
 started_at timestamptz NOT NULL DEFAULT now(), ended_at timestamptz
);
CREATE UNIQUE INDEX work_sessions_one_active ON v2.work_sessions(order_id,user_id) WHERE status='active';
CREATE TABLE v2.documents (
 id uuid PRIMARY KEY, kind text NOT NULL CHECK(kind IN ('component_receipt','issue','return','material_loss','production','inspection','disposition','rework','goods_receipt')),
 order_id uuid REFERENCES v2.work_orders, source_document_id uuid REFERENCES v2.documents, work_session_id uuid REFERENCES v2.work_sessions,
 output_lot_id uuid REFERENCES v2.lots, output_lot_code text NOT NULL DEFAULT '', location_id uuid REFERENCES v2.locations,
 quantity bigint NOT NULL DEFAULT 0 CHECK(quantity BETWEEN 0 AND 1000000),
 accepted_quantity bigint NOT NULL DEFAULT 0 CHECK(accepted_quantity BETWEEN 0 AND 1000000),
 rejected_quantity bigint NOT NULL DEFAULT 0 CHECK(rejected_quantity BETWEEN 0 AND 1000000),
 defect_reason_id uuid REFERENCES v2.defect_reasons, reason text NOT NULL DEFAULT '', disposition text NOT NULL DEFAULT '' CHECK(disposition IN ('','disposal','rework')),
 occurred_at timestamptz NOT NULL, version bigint NOT NULL DEFAULT 1,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','posted','reversed')),
 has_variance boolean NOT NULL DEFAULT false, created_by uuid NOT NULL REFERENCES v2.users, created_at timestamptz NOT NULL DEFAULT now(),
 posted_by uuid REFERENCES v2.users, posted_at timestamptz,
 CHECK ((kind='component_receipt' AND order_id IS NULL) OR (kind<>'component_receipt' AND order_id IS NOT NULL))
);
CREATE INDEX documents_order ON v2.documents(order_id);
CREATE INDEX documents_source ON v2.documents(source_document_id);
CREATE TABLE v2.document_lines (
 document_id uuid NOT NULL REFERENCES v2.documents, line_number integer NOT NULL,
 lot_id uuid NOT NULL REFERENCES v2.lots, location_id uuid NOT NULL REFERENCES v2.locations,
 quantity bigint NOT NULL CHECK(quantity BETWEEN 1 AND 1000000), PRIMARY KEY(document_id,line_number), UNIQUE(document_id,lot_id,location_id)
);
CREATE TABLE v2.inspection_results (
 document_id uuid NOT NULL REFERENCES v2.documents, item_code text NOT NULL,
 result text NOT NULL CHECK(result IN ('pass','fail')), note text NOT NULL DEFAULT '', PRIMARY KEY(document_id,item_code)
);
CREATE TABLE v2.output_lots (
 id uuid PRIMARY KEY REFERENCES v2.lots, order_id uuid NOT NULL REFERENCES v2.work_orders,
 production_document_id uuid NOT NULL UNIQUE REFERENCES v2.documents,
 new_output_quantity bigint NOT NULL CHECK(new_output_quantity BETWEEN 1 AND 1000000)
);
CREATE TABLE v2.document_dependencies (
 parent_id uuid NOT NULL REFERENCES v2.documents, child_id uuid NOT NULL REFERENCES v2.documents,
 PRIMARY KEY(parent_id,child_id), CHECK(parent_id<>child_id)
);
CREATE TABLE v2.reversals (
 id uuid PRIMARY KEY, document_id uuid NOT NULL UNIQUE REFERENCES v2.documents,
 reason text NOT NULL CHECK(length(btrim(reason))>0), actor_id uuid NOT NULL REFERENCES v2.users, recorded_at timestamptz NOT NULL DEFAULT now()
);
-- The nil UUID is an internal, non-null warehouse owner, exposed as null in the API.
CREATE TABLE v2.inventory_balances (
 item_id uuid NOT NULL REFERENCES v2.items, lot_id uuid NOT NULL REFERENCES v2.lots,
 location_id uuid NOT NULL REFERENCES v2.locations, owner_id uuid NOT NULL,
 quantity bigint NOT NULL DEFAULT 0 CHECK(quantity BETWEEN 0 AND 1000000000000),
 PRIMARY KEY(item_id,lot_id,location_id,owner_id)
);
CREATE TABLE v2.material_ledger (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, document_id uuid NOT NULL REFERENCES v2.documents,
 reversal_id uuid REFERENCES v2.reversals, item_id uuid NOT NULL REFERENCES v2.items, lot_id uuid NOT NULL REFERENCES v2.lots,
 location_id uuid NOT NULL REFERENCES v2.locations, owner_id uuid NOT NULL,
 quantity bigint NOT NULL CHECK(quantity<>0 AND quantity BETWEEN -1000000000000 AND 1000000000000),
 movement text NOT NULL CHECK(movement IN ('receipt','issue','return','consumption','material_loss','goods_receipt','reversal')),
 recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX material_ledger_bucket ON v2.material_ledger(item_id,lot_id,location_id,owner_id);
CREATE INDEX material_ledger_document ON v2.material_ledger(document_id);
CREATE TABLE v2.output_balances (
 source_document_id uuid NOT NULL REFERENCES v2.documents, output_lot_id uuid NOT NULL REFERENCES v2.output_lots,
 bucket text NOT NULL CHECK(bucket IN ('pending','accepted','rejected','rework','disposed','received')),
 quantity bigint NOT NULL DEFAULT 0 CHECK(quantity BETWEEN 0 AND 1000000000000), PRIMARY KEY(source_document_id,bucket)
);
CREATE TABLE v2.output_ledger (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, document_id uuid NOT NULL REFERENCES v2.documents,
 reversal_id uuid REFERENCES v2.reversals, source_document_id uuid NOT NULL REFERENCES v2.documents,
 output_lot_id uuid NOT NULL REFERENCES v2.output_lots, bucket text NOT NULL CHECK(bucket IN ('pending','accepted','rejected','rework','disposed','received')),
 quantity bigint NOT NULL CHECK(quantity<>0 AND quantity BETWEEN -1000000000000 AND 1000000000000), recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX output_ledger_document ON v2.output_ledger(document_id);
CREATE TABLE v2.command_replays (
 account_id uuid NOT NULL REFERENCES v2.users, operation text NOT NULL, request_key uuid NOT NULL,
 request_hash text NOT NULL, response jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(account_id,operation,request_key)
);
CREATE TABLE v2.audit_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, actor_id uuid NOT NULL REFERENCES v2.users,
 actor_role text NOT NULL, operation text NOT NULL, entity_id uuid NOT NULL, reason text NOT NULL DEFAULT '',
 details jsonb NOT NULL DEFAULT '{}', recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_entity ON v2.audit_events(entity_id);

-- Enforce immutability even when a future caller bypasses the HTTP service.
-- +goose StatementBegin
CREATE FUNCTION v2.guard_document() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.status<>'draft' THEN RAISE EXCEPTION 'posted document is immutable'; END IF;
  RETURN OLD;
 END IF;
 IF OLD.status<>'draft' THEN
  IF OLD.status='posted' AND NEW.status='reversed' AND
    (to_jsonb(NEW)-'status')=(to_jsonb(OLD)-'status') AND
    EXISTS(SELECT 1 FROM v2.reversals WHERE document_id=OLD.id) THEN RETURN NEW; END IF;
  RAISE EXCEPTION 'posted document is immutable';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER immutable_document BEFORE UPDATE OR DELETE ON v2.documents FOR EACH ROW EXECUTE FUNCTION v2.guard_document();
-- +goose StatementBegin
CREATE FUNCTION v2.guard_document_child() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent uuid;
BEGIN
 IF TG_OP='DELETE' THEN parent:=OLD.document_id; ELSE parent:=NEW.document_id; END IF;
 IF EXISTS(SELECT 1 FROM v2.documents WHERE id=parent AND status<>'draft') THEN RAISE EXCEPTION 'posted document lines are immutable'; END IF;
 IF TG_OP='UPDATE' AND OLD.document_id<>NEW.document_id THEN RAISE EXCEPTION 'document parent is immutable'; END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER immutable_document_lines BEFORE INSERT OR UPDATE OR DELETE ON v2.document_lines FOR EACH ROW EXECUTE FUNCTION v2.guard_document_child();
CREATE TRIGGER immutable_inspection_results BEFORE INSERT OR UPDATE OR DELETE ON v2.inspection_results FOR EACH ROW EXECUTE FUNCTION v2.guard_document_child();
-- +goose StatementBegin
CREATE FUNCTION v2.guard_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'accounting history is append only'; END $$;
-- +goose StatementEnd
CREATE TRIGGER immutable_material_ledger BEFORE UPDATE OR DELETE ON v2.material_ledger FOR EACH ROW EXECUTE FUNCTION v2.guard_append_only();
CREATE TRIGGER immutable_output_ledger BEFORE UPDATE OR DELETE ON v2.output_ledger FOR EACH ROW EXECUTE FUNCTION v2.guard_append_only();
CREATE TRIGGER immutable_reversal BEFORE UPDATE OR DELETE ON v2.reversals FOR EACH ROW EXECUTE FUNCTION v2.guard_append_only();
CREATE TRIGGER immutable_audit BEFORE UPDATE OR DELETE ON v2.audit_events FOR EACH ROW EXECUTE FUNCTION v2.guard_append_only();
CREATE TRIGGER immutable_replay BEFORE UPDATE OR DELETE ON v2.command_replays FOR EACH ROW EXECUTE FUNCTION v2.guard_append_only();

CREATE VIEW v2.bom_revision_view AS SELECT r.*, COALESCE((SELECT jsonb_agg(jsonb_build_object('component_item_id',b.component_item_id,'quantity',b.quantity,'code',i.code,'name',i.name,'unit',i.unit) ORDER BY b.component_item_id) FROM v2.bom_lines b JOIN v2.items i ON i.id=b.component_item_id WHERE b.revision_id=r.id),'[]'::jsonb) AS lines FROM v2.bom_revisions r;
CREATE VIEW v2.inspection_revision_view AS SELECT r.*, COALESCE((SELECT jsonb_agg(jsonb_build_object('item_code',i.item_code,'name',i.name,'required',i.required,'acceptance',i.acceptance) ORDER BY i.item_code) FROM v2.inspection_items i WHERE i.revision_id=r.id),'[]'::jsonb) AS items FROM v2.inspection_revisions r;
CREATE VIEW v2.output_lot_view AS SELECT o.id,l.code,o.order_id,l.item_id,o.production_document_id,
 CASE WHEN d.status='posted' THEN o.new_output_quantity ELSE 0 END AS new_output_quantity,
 COALESCE(sum(b.quantity) FILTER(WHERE b.bucket='pending'),0)::bigint AS pending_quantity,
 COALESCE(sum(b.quantity) FILTER(WHERE b.bucket='accepted'),0)::bigint AS accepted_quantity,
 COALESCE(sum(b.quantity) FILTER(WHERE b.bucket='rejected'),0)::bigint AS rejected_quantity,
 COALESCE(sum(b.quantity) FILTER(WHERE b.bucket='rework'),0)::bigint AS rework_quantity,
 COALESCE(sum(b.quantity) FILTER(WHERE b.bucket='disposed'),0)::bigint AS disposed_quantity,
 COALESCE(sum(b.quantity) FILTER(WHERE b.bucket='received'),0)::bigint AS received_quantity
 FROM v2.output_lots o JOIN v2.lots l ON l.id=o.id JOIN v2.documents d ON d.id=o.production_document_id
 LEFT JOIN v2.output_balances b ON b.output_lot_id=o.id GROUP BY o.id,l.code,l.item_id,d.status;
CREATE VIEW v2.work_order_view AS SELECT o.*,
 COALESCE(s.new_output_quantity,0)::bigint AS new_output_quantity, COALESCE(s.pending_quantity,0)::bigint AS pending_quantity,
 COALESCE(s.accepted_quantity,0)::bigint AS accepted_quantity, COALESCE(s.rejected_quantity,0)::bigint AS rejected_quantity,
 COALESCE(s.rework_quantity,0)::bigint AS rework_quantity, COALESCE(s.disposed_quantity,0)::bigint AS disposed_quantity,
 COALESCE(s.received_quantity,0)::bigint AS received_quantity, GREATEST(o.target_quantity-COALESCE(s.received_quantity,0),0)::bigint AS remaining_quantity,
 (SELECT COALESCE(sum(quantity),0)::bigint FROM v2.inventory_balances b WHERE b.owner_id=o.id) AS floor_quantity,
 (SELECT count(*) FROM v2.work_sessions w WHERE w.order_id=o.id AND w.status='active') AS active_sessions,
 EXISTS(SELECT 1 FROM v2.documents d WHERE d.order_id=o.id AND d.status='posted' AND d.has_variance) AS has_variances
 FROM v2.work_orders o LEFT JOIN (SELECT order_id,sum(new_output_quantity) AS new_output_quantity,sum(pending_quantity) AS pending_quantity,sum(accepted_quantity) AS accepted_quantity,sum(rejected_quantity) AS rejected_quantity,sum(rework_quantity) AS rework_quantity,sum(disposed_quantity) AS disposed_quantity,sum(received_quantity) AS received_quantity FROM v2.output_lot_view GROUP BY order_id) s ON s.order_id=o.id;
CREATE VIEW v2.document_view AS SELECT d.*,r.id AS reversal_id,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('lot_id',l.lot_id,'location_id',l.location_id,'quantity',l.quantity) ORDER BY l.line_number) FROM v2.document_lines l WHERE l.document_id=d.id),'[]'::jsonb) AS lines,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('item_code',i.item_code,'result',i.result,'note',i.note) ORDER BY i.item_code) FROM v2.inspection_results i WHERE i.document_id=d.id),'[]'::jsonb) AS inspection_results,
 COALESCE((SELECT quantity FROM v2.output_balances b WHERE b.source_document_id=d.id AND b.bucket='pending'),0) AS available_pending_quantity,
 COALESCE((SELECT quantity FROM v2.output_balances b WHERE b.source_document_id=d.id AND b.bucket='accepted'),0) AS available_accepted_quantity,
 COALESCE((SELECT quantity FROM v2.output_balances b WHERE b.source_document_id=d.id AND b.bucket='rejected'),0) AS available_rejected_quantity,
 COALESCE((SELECT quantity FROM v2.output_balances b WHERE b.source_document_id=d.id AND b.bucket='rework'),0) AS available_rework_quantity
 FROM v2.documents d LEFT JOIN v2.reversals r ON r.document_id=d.id;
CREATE VIEW v2.inventory_view AS SELECT b.item_id,b.lot_id,b.location_id,NULLIF(b.owner_id,'00000000-0000-0000-0000-000000000000'::uuid) AS order_id,b.quantity,i.code AS item_code,i.name AS item_name,l.code AS lot_code,p.code AS location_code,p.kind AS location_kind FROM v2.inventory_balances b JOIN v2.items i ON i.id=b.item_id JOIN v2.lots l ON l.id=b.lot_id JOIN v2.locations p ON p.id=b.location_id;
CREATE VIEW v2.material_ledger_view AS SELECT id,document_id,reversal_id,item_id,lot_id,location_id,NULLIF(owner_id,'00000000-0000-0000-0000-000000000000'::uuid) AS order_id,quantity,movement,recorded_at FROM v2.material_ledger;

-- +goose Down
DROP VIEW v2.material_ledger_view,v2.inventory_view,v2.document_view,v2.work_order_view,v2.output_lot_view,v2.inspection_revision_view,v2.bom_revision_view;
DROP TABLE v2.audit_events,v2.command_replays,v2.output_ledger,v2.output_balances,v2.material_ledger,v2.inventory_balances,v2.reversals,v2.document_dependencies,v2.output_lots,v2.inspection_results,v2.document_lines,v2.documents,v2.work_sessions,v2.work_orders,v2.inspection_items,v2.inspection_revisions,v2.bom_lines,v2.bom_revisions,v2.defect_reasons,v2.lots,v2.locations,v2.items;
DROP FUNCTION v2.guard_document(),v2.guard_document_child(),v2.guard_append_only();
