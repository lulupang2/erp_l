-- name: CreateItem :one
INSERT INTO items (id, code, name, kind, unit) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetItem :one
SELECT * FROM items WHERE id = $1;

-- name: LockItem :one
SELECT * FROM items WHERE id = $1 FOR UPDATE;

-- name: ListItems :many
SELECT * FROM items
WHERE (sqlc.arg(kind_filter)::text = '' OR kind = sqlc.arg(kind_filter))
AND (sqlc.arg(search)::text = '' OR position(lower(sqlc.arg(search)) IN lower(code || ' ' || name)) > 0)
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(row_limit)::bigint OFFSET sqlc.arg(row_offset)::bigint;

-- name: CountItems :one
SELECT count(*) FROM items
WHERE (sqlc.arg(kind_filter)::text = '' OR kind = sqlc.arg(kind_filter))
AND (sqlc.arg(search)::text = '' OR position(lower(sqlc.arg(search)) IN lower(code || ' ' || name)) > 0);

-- name: CreateBOM :one
INSERT INTO boms (id, finished_item_id) VALUES ($1, $2) RETURNING *;

-- name: GetBOM :one
SELECT * FROM boms WHERE finished_item_id = $1;

-- name: LockBOM :one
SELECT * FROM boms WHERE finished_item_id = $1 FOR UPDATE;

-- name: TouchBOM :one
UPDATE boms SET updated_at = now() WHERE id = $1 RETURNING *;

-- name: DeleteBOMLines :exec
DELETE FROM bom_lines WHERE bom_id = $1;

-- name: CreateBOMLine :exec
INSERT INTO bom_lines (bom_id, component_item_id, quantity_per_unit) VALUES ($1, $2, $3);

-- name: ListBOMLines :many
SELECT i.id AS item_id, i.code, i.name, i.unit, l.quantity_per_unit
FROM bom_lines l JOIN items i ON i.id = l.component_item_id
WHERE l.bom_id = $1 ORDER BY i.id;
