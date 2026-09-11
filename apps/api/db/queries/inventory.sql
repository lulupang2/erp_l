-- name: CreateBalance :exec
INSERT INTO inventory_balances (item_id) VALUES ($1);

-- name: GetBalance :one
SELECT * FROM inventory_balances WHERE item_id = $1;

-- name: LockBalance :one
SELECT * FROM inventory_balances WHERE item_id = $1 FOR UPDATE;

-- name: SetBalance :exec
UPDATE inventory_balances SET quantity = $2, updated_at = now() WHERE item_id = $1;

-- name: ListInventory :many
SELECT i.id AS item_id, i.code, i.name, i.kind, i.unit, b.quantity, b.updated_at
FROM items i JOIN inventory_balances b ON b.item_id = i.id
WHERE (sqlc.arg(kind_filter)::text = '' OR i.kind = sqlc.arg(kind_filter))
AND (sqlc.arg(search)::text = '' OR position(lower(sqlc.arg(search)) IN lower(i.code || ' ' || i.name)) > 0)
ORDER BY i.code ASC LIMIT sqlc.arg(row_limit)::bigint OFFSET sqlc.arg(row_offset)::bigint;

-- name: CreateReceipt :one
INSERT INTO stock_receipts (id, item_id, quantity, note) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetReceipt :one
SELECT * FROM stock_receipts WHERE id = $1;

-- name: CreateMovement :exec
INSERT INTO stock_movements (id, item_id, delta, movement_type, receipt_id, result_id)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListMovements :many
SELECT * FROM stock_movements
WHERE (sqlc.narg(item_filter)::uuid IS NULL OR item_id = sqlc.narg(item_filter))
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(row_limit)::bigint OFFSET sqlc.arg(row_offset)::bigint;

-- name: CountMovements :one
SELECT count(*) FROM stock_movements
WHERE (sqlc.narg(item_filter)::uuid IS NULL OR item_id = sqlc.narg(item_filter));
