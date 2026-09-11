-- +goose Up
CREATE TABLE items (
    id uuid PRIMARY KEY,
    code text NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9_-]{1,40}$'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100 AND name = btrim(name)),
    kind text NOT NULL CHECK (kind IN ('component', 'finished_good')),
    unit text NOT NULL CHECK (char_length(unit) BETWEEN 1 AND 20 AND unit = btrim(unit)),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE boms (
    id uuid PRIMARY KEY,
    finished_item_id uuid NOT NULL UNIQUE REFERENCES items(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE bom_lines (
    bom_id uuid NOT NULL REFERENCES boms(id),
    component_item_id uuid NOT NULL REFERENCES items(id),
    quantity_per_unit bigint NOT NULL CHECK (quantity_per_unit BETWEEN 1 AND 1000000),
    PRIMARY KEY (bom_id, component_item_id)
);

CREATE TABLE inventory_balances (
    item_id uuid PRIMARY KEY REFERENCES items(id),
    quantity bigint NOT NULL DEFAULT 0 CHECK (quantity BETWEEN 0 AND 1000000000000),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE stock_receipts (
    id uuid PRIMARY KEY,
    item_id uuid NOT NULL REFERENCES items(id),
    quantity bigint NOT NULL CHECK (quantity BETWEEN 1 AND 1000000),
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE production_orders (
    id uuid PRIMARY KEY,
    finished_item_id uuid NOT NULL REFERENCES items(id),
    planned_quantity bigint NOT NULL CHECK (planned_quantity BETWEEN 1 AND 1000000),
    good_quantity bigint NOT NULL DEFAULT 0 CHECK (good_quantity >= 0),
    defective_quantity bigint NOT NULL DEFAULT 0 CHECK (defective_quantity >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (good_quantity + defective_quantity <= planned_quantity)
);

CREATE TABLE production_order_materials (
    order_id uuid NOT NULL REFERENCES production_orders(id),
    component_item_id uuid NOT NULL REFERENCES items(id),
    quantity_per_unit bigint NOT NULL CHECK (quantity_per_unit BETWEEN 1 AND 1000000),
    PRIMARY KEY (order_id, component_item_id)
);

CREATE TABLE production_results (
    id uuid PRIMARY KEY,
    order_id uuid NOT NULL REFERENCES production_orders(id),
    good_quantity bigint NOT NULL CHECK (good_quantity BETWEEN 0 AND 1000000),
    defective_quantity bigint NOT NULL CHECK (defective_quantity BETWEEN 0 AND 1000000),
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (good_quantity + defective_quantity BETWEEN 1 AND 1000000)
);

CREATE TABLE stock_movements (
    id uuid PRIMARY KEY,
    item_id uuid NOT NULL REFERENCES items(id),
    delta bigint NOT NULL CHECK (delta <> 0 AND delta BETWEEN -1000000000000 AND 1000000000000),
    movement_type text NOT NULL CHECK (movement_type IN ('manual_receipt', 'production_consumption', 'production_receipt')),
    receipt_id uuid REFERENCES stock_receipts(id),
    result_id uuid REFERENCES production_results(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (movement_type = 'manual_receipt' AND delta > 0 AND receipt_id IS NOT NULL AND result_id IS NULL)
        OR (movement_type = 'production_consumption' AND delta < 0 AND receipt_id IS NULL AND result_id IS NOT NULL)
        OR (movement_type = 'production_receipt' AND delta > 0 AND receipt_id IS NULL AND result_id IS NOT NULL)
    )
);
CREATE UNIQUE INDEX stock_movements_receipt_once ON stock_movements(receipt_id) WHERE receipt_id IS NOT NULL;
CREATE UNIQUE INDEX stock_movements_result_item_once ON stock_movements(result_id, item_id) WHERE result_id IS NOT NULL;
CREATE INDEX stock_movements_item_history ON stock_movements(item_id, created_at DESC, id DESC);
CREATE INDEX stock_movements_history ON stock_movements(created_at DESC, id DESC);
CREATE INDEX items_history ON items(created_at DESC, id DESC);
CREATE INDEX production_orders_history ON production_orders(created_at DESC, id DESC);
CREATE INDEX production_results_order_history ON production_results(order_id, created_at DESC, id DESC);

CREATE TABLE idempotency_requests (
    operation text NOT NULL CHECK (operation IN ('stock_receipt', 'production_order', 'production_result')),
    key uuid NOT NULL,
    request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    resource_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (operation, key)
);

-- +goose Down
DROP TABLE idempotency_requests;
DROP TABLE stock_movements;
DROP TABLE production_results;
DROP TABLE production_order_materials;
DROP TABLE production_orders;
DROP TABLE stock_receipts;
DROP TABLE inventory_balances;
DROP TABLE bom_lines;
DROP TABLE boms;
DROP TABLE items;
