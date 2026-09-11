-- Read-only end-to-end reconciliation of the isolated local test database.
-- A statement failure makes psql exit nonzero with ON_ERROR_STOP=1.
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;

DO $$
DECLARE failures bigint;
BEGIN
  SELECT count(*) INTO failures
  FROM items i LEFT JOIN inventory_balances b ON b.item_id = i.id
  WHERE b.item_id IS NULL OR b.quantity < 0 OR b.quantity > 1000000000000;
  IF failures <> 0 THEN RAISE EXCEPTION 'Missing or invalid inventory balances: %', failures; END IF;

  SELECT count(*) INTO failures
  FROM inventory_balances b
  LEFT JOIN (SELECT item_id, sum(delta) AS quantity FROM stock_movements GROUP BY item_id) m USING (item_id)
  WHERE b.quantity <> coalesce(m.quantity, 0);
  IF failures <> 0 THEN RAISE EXCEPTION 'Inventory/ledger mismatches: %', failures; END IF;

  SELECT count(*) INTO failures
  FROM production_orders o
  LEFT JOIN (SELECT order_id, sum(good_quantity) AS good, sum(defective_quantity) AS defective
             FROM production_results GROUP BY order_id) r ON r.order_id = o.id
  WHERE o.good_quantity <> coalesce(r.good, 0)
     OR o.defective_quantity <> coalesce(r.defective, 0)
     OR o.good_quantity + o.defective_quantity > o.planned_quantity;
  IF failures <> 0 THEN RAISE EXCEPTION 'Order/result aggregate mismatches: %', failures; END IF;

  SELECT count(*) INTO failures FROM production_orders o
  WHERE NOT EXISTS (SELECT 1 FROM production_order_materials m WHERE m.order_id = o.id);
  IF failures <> 0 THEN RAISE EXCEPTION 'Orders without material snapshots: %', failures; END IF;

  SELECT count(*) INTO failures FROM stock_movements
  WHERE delta = 0
     OR (movement_type = 'manual_receipt' AND (delta < 0 OR receipt_id IS NULL OR result_id IS NOT NULL))
     OR (movement_type = 'production_consumption' AND (delta > 0 OR result_id IS NULL OR receipt_id IS NOT NULL))
     OR (movement_type = 'production_receipt' AND (delta < 0 OR result_id IS NULL OR receipt_id IS NOT NULL));
  IF failures <> 0 THEN RAISE EXCEPTION 'Invalid movement types, signs or references: %', failures; END IF;

  SELECT count(*) INTO failures
  FROM stock_receipts r LEFT JOIN stock_movements m ON m.receipt_id = r.id
  WHERE m.id IS NULL OR m.item_id <> r.item_id OR m.delta <> r.quantity OR m.movement_type <> 'manual_receipt';
  IF failures <> 0 THEN RAISE EXCEPTION 'Receipt/movement mismatches: %', failures; END IF;

  SELECT count(*) INTO failures
  FROM production_results r
  JOIN production_order_materials bom ON bom.order_id = r.order_id
  LEFT JOIN stock_movements m ON m.result_id = r.id AND m.item_id = bom.component_item_id
  WHERE m.id IS NULL OR m.movement_type <> 'production_consumption'
     OR m.delta <> -(r.good_quantity + r.defective_quantity) * bom.quantity_per_unit;
  IF failures <> 0 THEN RAISE EXCEPTION 'Result/material consumption mismatches: %', failures; END IF;

  SELECT count(*) INTO failures
  FROM production_results r JOIN production_orders o ON o.id = r.order_id
  LEFT JOIN stock_movements m ON m.result_id = r.id AND m.item_id = o.finished_item_id
  WHERE (r.good_quantity = 0 AND m.id IS NOT NULL)
     OR (r.good_quantity > 0 AND (m.id IS NULL OR m.delta <> r.good_quantity OR m.movement_type <> 'production_receipt'));
  IF failures <> 0 THEN RAISE EXCEPTION 'Finished-goods result/movement mismatches: %', failures; END IF;

  SELECT count(*) INTO failures FROM stock_movements m
  JOIN items i ON i.id = m.item_id
  WHERE (m.movement_type IN ('manual_receipt', 'production_consumption') AND i.kind <> 'component')
     OR (m.movement_type = 'production_receipt' AND i.kind <> 'finished_good');
  IF failures <> 0 THEN RAISE EXCEPTION 'Movement/item-kind mismatches: %', failures; END IF;

  SELECT count(*) INTO failures FROM (
    SELECT receipt_id FROM stock_movements WHERE receipt_id IS NOT NULL GROUP BY receipt_id HAVING count(*) > 1
    UNION ALL
    SELECT result_id FROM stock_movements WHERE result_id IS NOT NULL GROUP BY result_id, item_id HAVING count(*) > 1
  ) duplicates;
  IF failures <> 0 THEN RAISE EXCEPTION 'Duplicate source movements: %', failures; END IF;
END $$;

SELECT 'items' AS entity, count(*) AS records FROM items
UNION ALL SELECT 'receipts', count(*) FROM stock_receipts
UNION ALL SELECT 'orders', count(*) FROM production_orders
UNION ALL SELECT 'results', count(*) FROM production_results
UNION ALL SELECT 'movements', count(*) FROM stock_movements;
COMMIT;
