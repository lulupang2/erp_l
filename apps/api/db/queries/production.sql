-- name: CreateOrder :one
INSERT INTO production_orders (id, finished_item_id, planned_quantity) VALUES ($1, $2, $3) RETURNING *;

-- name: GetOrder :one
SELECT * FROM production_orders WHERE id = $1;

-- name: LockOrder :one
SELECT * FROM production_orders WHERE id = $1 FOR UPDATE;

-- name: AddOrderTotals :one
UPDATE production_orders SET good_quantity = good_quantity + $2, defective_quantity = defective_quantity + $3
WHERE id = $1 RETURNING *;

-- name: ListOrders :many
SELECT * FROM production_orders
WHERE (sqlc.arg(status_filter)::text = '' OR sqlc.arg(status_filter) =
  CASE WHEN good_quantity + defective_quantity = 0 THEN 'pending'
       WHEN good_quantity + defective_quantity = planned_quantity THEN 'completed' ELSE 'in_progress' END)
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(row_limit)::bigint OFFSET sqlc.arg(row_offset)::bigint;

-- name: CountOrders :one
SELECT count(*) FROM production_orders
WHERE (sqlc.arg(status_filter)::text = '' OR sqlc.arg(status_filter) =
  CASE WHEN good_quantity + defective_quantity = 0 THEN 'pending'
       WHEN good_quantity + defective_quantity = planned_quantity THEN 'completed' ELSE 'in_progress' END);

-- name: CreateOrderMaterial :exec
INSERT INTO production_order_materials (order_id, component_item_id, quantity_per_unit) VALUES ($1, $2, $3);

-- name: ListOrderMaterials :many
SELECT i.id AS item_id, i.code, i.name, i.unit, m.quantity_per_unit
FROM production_order_materials m JOIN items i ON i.id = m.component_item_id
WHERE m.order_id = $1 ORDER BY i.id;

-- name: CreateResult :one
INSERT INTO production_results (id, order_id, good_quantity, defective_quantity, note) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetResult :one
SELECT * FROM production_results WHERE id = $1;

-- name: ListResults :many
SELECT * FROM production_results WHERE order_id = $1
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(row_limit)::bigint OFFSET sqlc.arg(row_offset)::bigint;

-- name: CountResults :one
SELECT count(*) FROM production_results WHERE order_id = $1;

-- name: ClaimRequest :one
INSERT INTO idempotency_requests (operation, key, request_hash, resource_id) VALUES ($1, $2, $3, $4)
ON CONFLICT (operation, key) DO NOTHING RETURNING resource_id;

-- name: GetRequest :one
SELECT * FROM idempotency_requests WHERE operation = $1 AND key = $2;
