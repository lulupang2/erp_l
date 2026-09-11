import { writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

// Source for the reproducible OpenAPI document. YAML accepts this JSON notation.
const root = dirname(dirname(fileURLToPath(import.meta.url)));
const ref = name => ({ $ref: `#/components/schemas/${name}` });
const id = { type: 'string', format: 'uuid', description: 'Canonical, non-nil UUID.' };
const timestamp = { type: 'string', format: 'date-time', description: 'UTC RFC3339; displayed in Asia/Seoul.' };
const quantity = (minimum = 0, maximum = 1_000_000) => ({ type: 'integer', format: 'int64', minimum, maximum });
const object = (properties, required = Object.keys(properties), strict = false) => ({ type: 'object', required, properties, ...(strict ? { additionalProperties: false } : {}) });
const array = items => ({ type: 'array', items });
const text = (maxLength, minLength = 0) => ({ type: 'string', minLength, maxLength });
const kind = { type: 'string', enum: ['component', 'finished_good'] };
const status = { type: 'string', enum: ['pending', 'in_progress', 'completed'] };
const data = schema => object({ data: schema });
const note = { ...text(500), default: '' };
const schemas = {
  Item: object({ id, code: { type: 'string', pattern: '^[A-Z0-9_-]{1,40}$' }, name: text(100, 1), kind, unit: text(20, 1), created_at: timestamp }),
  ItemInput: object({ code: { ...text(40, 1), description: 'Whitespace removed; converted to uppercase; then [A-Z0-9_-]{1,40}.' }, name: text(100, 1), kind, unit: text(20, 1) }, undefined, true),
  Material: object({ item_id: id, code: text(40, 1), name: text(100, 1), unit: text(20, 1), quantity_per_unit: quantity(1) }),
  Bom: object({ id, finished_item_id: id, updated_at: timestamp, components: { ...array(ref('Material')), minItems: 1 } }),
  BomInput: object({ components: { ...array(object({ item_id: id, quantity_per_unit: quantity(1) }, undefined, true)), minItems: 1, description: 'Whole replacement; component IDs must be distinct and refer to component items.' } }, undefined, true),
  Inventory: object({ item_id: id, code: text(40, 1), name: text(100, 1), kind, unit: text(20, 1), quantity: quantity(0, 1_000_000_000_000), updated_at: timestamp }),
  Receipt: object({ id, item_id: id, quantity: quantity(1), note, created_at: timestamp }),
  ReceiptInput: object({ item_id: id, quantity: quantity(1), note }, ['item_id', 'quantity'], true),
  Movement: object({ id, item_id: id, delta: { type: 'integer', format: 'int64', minimum: -1_000_000_000_000, maximum: 1_000_000_000_000, not: { const: 0 } },
    movement_type: { type: 'string', enum: ['manual_receipt', 'production_consumption', 'production_receipt'] },
    receipt_id: { ...id, type: ['string', 'null'] }, result_id: { ...id, type: ['string', 'null'] }, created_at: timestamp }),
  Order: object({ id, finished_item_id: id, planned_quantity: quantity(1), good_quantity: quantity(), defective_quantity: quantity(), remaining_quantity: quantity(), status, created_at: timestamp }),
  OrderDetail: { allOf: [ref('Order'), object({ materials: { ...array(ref('Material')), minItems: 1 } })] },
  OrderInput: object({ finished_item_id: id, planned_quantity: quantity(1) }, undefined, true),
  Result: object({ id, order_id: id, good_quantity: quantity(), defective_quantity: quantity(), note, created_at: timestamp }),
  ResultInput: { ...object({ good_quantity: quantity(), defective_quantity: quantity(), note }, ['good_quantity', 'defective_quantity'], true), description: 'good + defective >= 1 and <= order remaining. Defects consume materials but add no finished stock.' },
  Pagination: object({ page: quantity(1, 1_000_000_000), page_size: quantity(1, 100), total: { type: 'integer', format: 'int64', minimum: 0 } }),
  Shortage: object({ item_id: id, required_quantity: quantity(1, 1_000_000_000_000), available_quantity: quantity(0, 1_000_000_000_000) }),
  Error: object({ error: object({ code: { type: 'string' }, message: { type: 'string' }, details: object({ shortages: array(ref('Shortage')) }, []) }, ['code', 'message']) }),
};
const json = schema => ({ 'application/json': { schema } });
const response = (description, schema) => ({ description, content: json(schema) });
const errors = codes => Object.fromEntries(codes.map(code => [code, { $ref: `#/components/responses/Error${code}` }]));
const pathId = { name: 'id', in: 'path', required: true, schema: id };
const key = { name: 'Idempotency-Key', in: 'header', required: true, schema: id, description: 'Keep the exact same key and normalized input after network/503 ambiguity. Successful replay is 200; altered input or route target is 409.' };
const pageParameters = [
  { name: 'page', in: 'query', schema: { ...quantity(1, 1_000_000_000), default: 1 } },
  { name: 'page_size', in: 'query', schema: { ...quantity(1, 100), default: 20 } },
];
const filters = [
  { name: 'kind', in: 'query', schema: kind },
  { name: 'q', in: 'query', schema: text(1000), description: 'Case-insensitive literal substring of item code or name.' },
];
const get = (operationId, summary, schema, parameters = []) => ({ operationId, summary, parameters, responses: { 200: response('Successful authoritative read.', schema), ...errors([400, 404, 500, 503]) } });
const list = (operationId, summary, schema, extra = []) => get(operationId, summary, object({ data: array(ref(schema)), pagination: ref('Pagination') }), [...pageParameters, ...extra]);
const write = (operationId, summary, input, output, parameters = [], replacement = false) => ({
  operationId, summary, parameters,
  requestBody: { required: true, content: json(ref(input)) },
  responses: {
    ...(replacement ? { 200: response('Whole BOM replacement committed.', data(ref(output))) } : {
      201: response('New resource committed atomically.', data(ref(output))),
      ...(parameters.includes(key) ? { 200: response('Previously committed resource replayed without duplicate writes.', data(ref(output))) } : {}),
    }), ...errors([400, 404, 409, 500, 503]),
  },
});
const document = {
  openapi: '3.1.0',
  info: { title: '조립 제조 ERP · Local Demo API', version: '0.1.0', description: 'Single organization and stock location, no authentication or public deployment. Local same-origin UI proxy only. Write transactions use READ COMMITTED. All quantities are bounded integers; stock cannot be negative. No edit/delete for posted inventory or production records.' },
  servers: [{ url: '/api/v1', description: 'Same-origin SvelteKit proxy to the loopback Go API.' }],
  security: [],
  paths: {
    '/health': { get: get('getHealth', 'Check actual database connectivity', data(object({ status: { const: 'ok' } }))) },
    '/items': { get: list('listItems', 'List items newest first, then UUID descending', 'Item', filters), post: write('createItem', 'Create an item and its zero inventory balance', 'ItemInput', 'Item') },
    '/items/{id}': { get: get('getItem', 'Get one item', data(ref('Item')), [pathId]) },
    '/items/{id}/bom': { get: get('getBom', 'Get a finished item BOM', data(ref('Bom')), [pathId]), put: write('replaceBom', 'Replace the complete BOM; existing order snapshots remain unchanged', 'BomInput', 'Bom', [pathId], true) },
    '/stock-receipts': { post: write('createReceipt', 'Receive components and append inventory history atomically', 'ReceiptInput', 'Receipt', [key]) },
    '/stock-receipts/{id}': { get: get('getReceipt', 'Get the receipt that caused a stock movement', data(ref('Receipt')), [pathId]) },
    '/inventory': { get: list('listInventory', 'List current inventory by item code ascending', 'Inventory', filters) },
    '/stock-movements': { get: list('listMovements', 'List immutable stock movements newest first', 'Movement', [{ name: 'item_id', in: 'query', schema: id }]) },
    '/production-orders': { get: list('listOrders', 'List production orders newest first', 'Order', [{ name: 'status', in: 'query', schema: status }]), post: write('createOrder', 'Create an order with an immutable BOM snapshot; do not reserve stock', 'OrderInput', 'OrderDetail', [key]) },
    '/production-orders/{id}': { get: get('getOrder', 'Get order totals, derived status and its material snapshot', data(ref('OrderDetail')), [pathId]) },
    '/production-orders/{id}/results': { get: list('listResults', 'List results for the order newest first', 'Result', [pathId]), post: write('recordResult', 'Record good/defective production; consume all materials and receive only good output', 'ResultInput', 'Result', [pathId, key]) },
    '/production-results/{id}': { get: get('getResult', 'Get the result that caused production stock movements', data(ref('Result')), [pathId]) },
  },
  components: {
    schemas,
    responses: Object.fromEntries(Object.entries({
      400: 'Invalid JSON, integer/range/type/UUID, missing key, or unsafe local request origin.',
      404: 'Referenced resource was not found.',
      409: 'Business conflict: duplicate code, missing BOM, shortage, plan/completed state, stock limit, or idempotency input mismatch.',
      500: 'Sanitized unexpected failure. Do not assume the write failed; retain the original key.',
      503: 'Database/lock/connectivity failure. Retain the exact original idempotency key and payload when retrying.',
    }).map(([code, description]) => [`Error${code}`, response(description, ref('Error'))])),
  },
};

writeFileSync(join(root, 'contracts/openapi.yaml'), '# Generated by scripts/generate-openapi.mjs.\n' + JSON.stringify(document, null, 2) + '\n');
console.log('Generated contracts/openapi.yaml from the versioned API schema.');
