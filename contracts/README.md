# v1 API 계약과 v2 전환

아래 DTO 설명은 구현된 v1 계약이며 상위 기준은 [SSOT-V1 7절](../docs/SSOT-V1.md#7-api-계약)이다. 경로는 `/api/v1`, JSON 속성은 snake_case, ID는 UUID, 시간은 UTC RFC3339다.
v2 전체 구현은 진행 중이다. 목표 계약은 [SSOT v2](../docs/SSOT.md)와 [전환 계획](../docs/FACTORY-PLAN.md)을 따르며, 작성 중인 DDL·OpenAPI·서버·클라이언트의 일치 및 인수 완료는 최종 통합 담당자의 [실제 증거](../docs/PROGRESS.md)로 판정한다.
[`openapi.yaml`](openapi.yaml)은 생성되는 상세 계약이다. 아래 v1 DTO를 v2에 그대로 재사용하거나 기존 생산 실적의 의미를 덮어쓰지 않는다.

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

## v2 계약 인수 항목

별도 `/api/v2`에서 로그인/로그아웃/현재 사용자, 역할·승인 기준정보, 지시 상태 전이, 수불·작업·검사·재작업·합격품 입고·정정을 명시한다. 계정·작업 세션 소유권, 권한/상태 거절, 보류 중 입고, 초과 합격 처리와 수량 경계는 [FACTORY-PLAN 7절](../docs/FACTORY-PLAN.md#7-구현-인수-경계와-결정-게이트)을 따른다.
요청 키는 계정+작업 종류+키에 귀속하고 대상·정규화 입력을 해시한다. 재응답 전 현재 권한 검사, 같은 계정 재로그인 재확인, 다른 계정 결과 노출 방지와 문서 자체의 중복 확정 차단을 계약/서버에서 함께 검증한다. 수량 상한은 [SSOT](../docs/SSOT.md#31-수량-한도와-계산-기본값)과 일치시킨다.
proxy Basic Auth는 요구하지 않는다. v1 익명 읽기/쓰기에 v2 인증이 소급 적용된다고 기술하지 않으며, v2 업무 API의 사용자 세션·역할 권한은 별도 필수 계약이다.
