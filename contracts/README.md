# API 구현 계약

상위 기준은 `docs/SSOT.md` 7절이다. 경로는 `/api/v1`, JSON 속성은 snake_case, ID는 UUID, 시간은 UTC RFC3339다.
`openapi.yaml`에 상세 계약을 유지한다. 아래는 화면과 API를 함께 구현하기 위한 응답 필드 기준이다.

- Item: `id, code, name, kind, unit, created_at`.
- BOM: `id, finished_item_id, updated_at, components: [{item_id, code, name, unit, quantity_per_unit}]`.
- Inventory: `item_id, code, name, kind, unit, quantity, updated_at`.
- Receipt: `id, item_id, quantity, note, created_at`.
- Movement: `id, item_id, delta, movement_type, receipt_id, result_id, created_at`. 없는 참조는 null.
- Order: `id, finished_item_id, planned_quantity, good_quantity, defective_quantity, remaining_quantity, status, created_at`.
- Order detail: Order에 `materials: [{item_id, code, name, unit, quantity_per_unit}]`를 추가한다.
- Result: `id, order_id, good_quantity, defective_quantity, note, created_at`.

단건은 `{data: ...}`, 목록은 `{data: [...], pagination: {page, page_size, total}}`다.
오류는 `{error: {code, message, details?}}`다. 부족 자재 details는 `{shortages: [{item_id, required_quantity, available_quantity}]}`다.
화면은 품목 목록을 이용해 ID를 품목명으로 표시한다. 목록에 없는 품목은 상세 조회로 보완한다.
수량은 JSON 정수다. 불확실한 재시도는 동일한 `Idempotency-Key`와 동일 입력으로 수행한다.
