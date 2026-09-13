-- name: V2SchemaVersion :one
SELECT 2::bigint AS version;

-- name: LockItem :one
SELECT * FROM v2.items WHERE id = $1;

-- name: LockBOMRevision :one
SELECT * FROM v2.bom_revisions WHERE id = $1 AND status = 'approved';

-- name: LockOrder :one
SELECT * FROM v2.work_orders WHERE id = $1 FOR UPDATE;

-- name: LockSessionActive :one
SELECT * FROM v2.work_sessions WHERE id = $1 AND status = 'active' FOR UPDATE;

-- name: LockDocument :one
SELECT * FROM v2.documents WHERE id = $1 FOR UPDATE;

-- name: ListInventory :many
SELECT * FROM v2.inventory_view WHERE quantity <> 0 ORDER BY item_code, lot_code, location_code;

-- name: ListOutputLots :many
SELECT * FROM v2.output_lot_view ORDER BY code;

-- name: ListOrders :many
SELECT * FROM v2.work_order_view ORDER BY created_at DESC;

-- name: ListDocuments :many
SELECT * FROM v2.document_view ORDER BY created_at DESC;

-- name: ListMaterialLedger :many
SELECT * FROM v2.material_ledger_view ORDER BY recorded_at, id;

-- name: InsertWorkOrder :one
INSERT INTO v2.work_orders (id,finished_item_id,target_quantity,start_allowance,bom_revision_id,inspection_revision_id,assigned_user_id,planned_date,bom_snapshot,inspection_snapshot,created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING *;

-- name: InsertDocument :one
INSERT INTO v2.documents (id,kind,order_id,source_document_id,work_session_id,output_lot_code,location_id,quantity,accepted_quantity,rejected_quantity,defect_reason_id,reason,disposition,occurred_at,created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING *;

-- name: InsertDocumentLine :exec
INSERT INTO v2.document_lines(document_id,line_number,lot_id,location_id,quantity) VALUES($1,$2,$3,$4,$5);

-- name: InsertInspectionResult :exec
INSERT INTO v2.inspection_results(document_id,item_code,result,note) VALUES($1,$2,$3,$4);

-- name: InsertMaterialLedger :exec
INSERT INTO v2.material_ledger(document_id,reversal_id,item_id,lot_id,location_id,owner_id,quantity,movement) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: InsertOutputLedger :exec
INSERT INTO v2.output_ledger(document_id,reversal_id,source_document_id,output_lot_id,bucket,quantity) VALUES($1,$2,$3,$4,$5,$6);

-- name: UpsertInventoryBalance :exec
INSERT INTO v2.inventory_balances(item_id,lot_id,location_id,owner_id,quantity) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(item_id,lot_id,location_id,owner_id) DO UPDATE SET quantity=v2.inventory_balances.quantity+EXCLUDED.quantity;

-- name: UpsertOutputBalance :exec
INSERT INTO v2.output_balances(source_document_id,output_lot_id,bucket,quantity) VALUES($1,$2,$3,$4)
ON CONFLICT(source_document_id,bucket) DO UPDATE SET quantity=v2.output_balances.quantity+EXCLUDED.quantity;

-- name: ExistingReplay :one
SELECT * FROM v2.command_replays WHERE account_id=$1 AND operation=$2 AND request_key=$3 FOR UPDATE;
