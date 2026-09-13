# 코드 리뷰 — 2026-09-12

대상: `b53c8356b11a4bcecc787f8e7abf17638a996448`와 시작 시 존재한 Caddyfile 작업 트리 변경.
범위: 제조 업무 서비스·SQL·HTTP, 화면 데이터/저장 흐름, 기존 테스트, CI/CD와 문서의 일치 여부.
이번 작업은 코드 리뷰와 문서 수정이다. 애플리케이션·워크플로·Caddyfile을 수정하거나 배포하지 않았다.

## 판정

리뷰 당시 로컬 회귀 검증은 통과했고 배포 경로 4건을 지적했다. 아래 근거와 실행 결과는 당시 이력이다. 이후 2026-09-12 사용자 결정으로 R1은 대체되었으며, R2/R3/R4는 구현·검증 증거가 연결되기 전까지 미해결이다. 과거 통과 결과를 현재 production 완료로 해석하지 않는다.
이 결과는 전체 결함 부재나 production 정상 동작을 보장하지 않는다.
R1은 현재 미해결 결함 목록의 수리 완료 항목이 아니다. 사용자 정책으로 Basic Auth 및 익명 401 조건을 폐기한 **대체(superseded)** 항목이며, v1 업무 API의 익명 읽기·쓰기 위험은 의도된 정책으로 남는다. v2 앱 내부 세션·역할 권한은 별도 요구사항이다.
아래 재현 조건은 소스에서 확인한 경로이며 원격 서버에서 실행한 결과와 구분한다.

## R1 — [P1] 작업 트리의 Basic Auth 제거로 업무 API가 인증 없이 공개됨

**현재 상태 — 2026-09-12 사용자 결정으로 대체(superseded), 보안 수정 완료가 아님.** 사용자가 reverse-proxy Basic Auth를 명시적으로 거절했다. 이를 복원하지 않으며 아래의 종전 수정 방향·401 완료 조건은 더 이상 현행 기준이 아니다. v1에는 앱 인증이 없으므로 익명 호출자는 v1 데모 데이터를 읽고 쓸 수 있다. 이 공개 업무 API 위험을 숨기거나 데이터 격리·읽기 전용 보호가 있다고 표현하지 않는다. Host·Origin 검사는 인증을 대신하지 않는다. v2 앱 계정·세션·역할 권한은 별도 승인 요구사항으로 유지한다.

### 최초 리뷰 근거와 종전 권고 (2026-09-12 이력)

- 위치: [Caddyfile](../Caddyfile) 6~8행, [HTTP 계층](../apps/api/internal/httpapi/app.go).
- 적용 범위: 커밋된 HEAD에서는 인증이 활성화되어 있으나, 리뷰 시작 시 작업 트리에서는 주석 처리되어 있었다.
- 조건: 현재 Caddyfile로 새로 기동하거나 설정을 다시 적용하면 모든 요청이 인증 없이 프록시로 전달된다.
- 영향: 앱 자체 인증이 없으므로 입고·BOM 교체·생산 실적 같은 쓰기를 누구나 실행할 수 있다. Host·Origin 검사는 사용자 인증을 대신하지 않는다. CD의 마지막 401 검사도 실패하지만 이미 반영한 설정을 자동 복구하지 않는다.
- 수정 방향: 공개 배포에 Basic Auth를 유지한다. 인증 없는 체험을 원한다면 별도 데이터 격리·쓰기 범위 정책을 PRD/ADR에 먼저 정의한다. 기존 변경을 이번 리뷰에서 임의로 되돌리지는 않았다.
- 완료 검증: 인증 없는 화면과 API가 401, 올바른 인증 후 읽기 요청이 200. 인증 없는 쓰기는 DB 호출 전 차단되는지 격리된 환경에서 확인한다.

현행 배포 인수 기준은 [DEPLOYMENT](DEPLOYMENT.md)를 따른다. Basic Auth 없는 v1 화면·읽기 API의 정상 응답을 확인하며 종전 401 게이트는 수정 대상이다. 정책 변경만으로 실제 설정 적용이나 배포 성공을 주장하지 않는다. production DB 쓰기 검증은 별도 명시 승인 없이 수행하지 않는다.

## R2 — [P2] 수동 배포는 선택한 커밋의 CI 성공 여부를 확인하지 않음

- 위치: [deploy.yml](../.github/workflows/deploy.yml) 24~39행.
- 조건: workflow_dispatch로 CI 실패 또는 미실행 SHA/ref를 지정한다. 기본값 main도 실행 시점에 미검증 커밋을 가리킬 수 있다.
- 근거: 수동 이벤트는 job 조건을 바로 통과하고 inputs.ref를 checkout한다. 이후 해당 SHA의 CI 결과를 검증하거나 테스트하는 단계가 없다.
- 영향: 빌드 가능한 테스트 실패 커밋도 이미지 push·migration·production 갱신까지 진행된다. DEPLOYMENT와 ADR-0008의 ‘성공한 main commit만 배포’ 기준이 수동 경로에는 적용되지 않는다.
- 수정 방향: ref를 불변 SHA로 해석한 뒤 그 SHA에 대한 신뢰할 수 있는 main push CI 성공을 검증하고, 성공이 없으면 원격 변경 전에 중단한다. 롤백도 검증된 SHA만 허용한다.
- 완료 검증: 미실행/실패 SHA는 배포 전 거절, 성공 SHA는 허용, 이동 가능한 브랜치 이름도 실제 checkout SHA 기준으로 검증한다.

## R3 — [P2] Caddyfile만 바뀐 배포에서 실행 중인 설정이 갱신되지 않음

- 위치: [deploy.yml](../.github/workflows/deploy.yml) 99~100행·120행, [compose.prod.yml](../compose.prod.yml) 45~60행.
- 조건: Caddy 이미지·서비스 환경은 동일하고 Caddyfile 내용만 변경한 뒤 같은 배포 절차를 실행한다.
- 근거: 파일을 업로드하고 compose up만 실행한다. Caddyfile은 bind mount이고 명시적인 validate/reload 단계가 없다. Compose 서비스 정의나 이미지가 바뀌지 않으면 Caddy 컨테이너 재생성을 기대할 수 없다.
- 영향: 디스크의 파일과 실행 중인 라우팅·인증·헤더가 달라진다. 예전 설정도 401을 반환하면 현재 외부 확인만으로 이 누락을 발견하지 못한다.
- 수정 방향: 새 Caddyfile을 검증한 뒤 기존 컨테이너에 명시적으로 reload하고 실패 시 배포를 실패 처리한다. 최초 기동과 재배포 경로를 모두 다룬다.
- 완료 검증: 테스트 환경에서 헤더나 라우팅만 바꾸어 같은 이미지를 재배포하고 실제 응답의 변경을 확인한다. 문법 오류 설정은 활성화하지 않는다.

Caddy는 설정 변경 후 [명시적 reload](https://caddyserver.com/docs/running)를 안내한다. Compose의 재생성 조건은 [공식 up 문서](https://docs.docker.com/reference/cli/docker/compose/up/)를 참고했다. 원격 재배포 재현은 이번에 수행하지 않았다.

## R4 — [P2] 15초 주기 DB health check가 유휴 중단을 방해함

- 위치: [compose.prod.yml](../compose.prod.yml) 23~28행, [app.go](../apps/api/internal/httpapi/app.go) 100~105행. API Dockerfile에도 같은 DB health endpoint 검사가 있다.
- 조건: production Compose를 계속 실행하며 사용자가 접속하지 않는 경우.
- 근거: Docker가 15초마다 /api/v1/health를 호출하고 핸들러는 매번 Pool.Ping으로 DB에 접근한다.
- 영향: 요청이 없는 동안에도 DB 쿼리가 계속 발생하여 SSOT의 ‘주기적 쿼리로 DB를 깨우지 않음’과 유휴 중단 설계에 어긋난다. 로컬 앱으로 수행한 과거 유휴 재개 검증은 이 production 구성에 대한 증거가 아니다. 실제 비용은 측정하지 않았다.
- 수정 방향: 주기적인 프로세스 생존 검사와 DB 준비 상태 검사를 분리한다. 컨테이너 health는 DB를 호출하지 않는 경로를 사용하고, DB 연결 검증은 기동·배포 검증과 실제 업무 요청에서 수행한다.
- 완료 검증: 주기 health 요청이 DB 쿼리를 발생시키지 않는지 로컬에서 확인하고, 변경된 배포 환경의 실제 idle→첫 업무 요청을 별도 검증한다.

## 최초 리뷰 실행 결과 (2026-09-12 이력)

| 명령 | 관측 결과 |
| --- | --- |
| pnpm check | Go vet 통과, Svelte 검사 오류 0·경고 0 |
| pnpm test:unit | Go 테스트 통과, 프론트 36개·스크립트 11개 통과. 이 단계의 실DB 테스트는 건너뜀 |
| pnpm test:integration | loopback 55433의 erp_test에 마이그레이션 v1 확인 후 실제 통합 테스트 통과 |
| pnpm test:e2e | Chromium HTTP/UI/디자인/타이포그래피 19개 통과, 43.3초 |
| pnpm check:invariants | 테스트 DB의 잔액·이력·실적 대조 통과 |
| pnpm build | Go 빌드와 SvelteKit adapter-node 빌드 통과 |
| pnpm contract:lint | OpenAPI lint 통과 |

DB 쓰기와 초기화는 기존 안전장치를 거친 격리된 로컬 erp_test에서만 수행했다.
E2E 이후 대조 시점의 테스트 레코드는 품목 250·입고 141·지시 148·실적 102·변동 381건이었다.
현재 테스트는 개발 서버와 로컬 PostgreSQL 경로를 검증한다. 통과 결과로 Caddy 인증이나 CI 배포 경로까지 검증되었다고 판단하지 않는다.

## 미실행과 후속 작업

- Neon 재검증, 실제 SSH 접속, GitHub Actions 실행, 이미지 push, Rocky Linux 배포와 외부 서비스 호출은 수행하지 않았다.
- production Docker 이미지 빌드/기동, Caddy reload 재현, race detector, Firefox/WebKit은 이번에 실행하지 않았다.
- sqlc 재생성 검사는 생성 파일을 다시 쓰는 명령이므로 코드 보존 범위에서 이번에는 실행하지 않았다. 기존 SQL과 생성 코드의 동작은 통합 테스트로 확인했으나 생성 일치 여부는 별도다.
- 리뷰 당시 CI에는 sqlc 생성 일치 검사와 인증 후 실제 화면/API 확인이 자동화되어 있지 않았다. 현행 후속 검증은 sqlc 생성 일치 및 Basic Auth 없는 v1 화면/읽기 API 정상 응답 확인이다.
- R1은 사용자 결정으로 대체되었으며 보안 결함을 수정한 것으로 집계하지 않는다. R2/R3/R4는 미해결로 유지한다. 각 항목의 구현 및 완료 검증 증거를 이 문서 또는 후속 리뷰에 연결한 뒤에만 상태를 변경한다.

## 최초 리뷰의 문서 정리 이력

README에 이 리뷰를 연결하고, PROGRESS 상단에 최신 검증 요약을 추가했다.
이전 PROGRESS의 ‘커밋·푸시 미수행’, ‘adapter-auto’ 설명은 당시 이력으로 보존하며 현재 상태로 해석하지 않도록 구분했다.
DEPLOYMENT는 요구하는 운영 기준과 현재 자동화의 실제 차이를 구분했다. 제품 범위와 ADR의 결정을 바꾸지는 않았다.

## 후속 인증 정책 정정 (2026-09-12)

사용자의 Basic Auth 거절을 DEPLOYMENT, PRD-V1, SSOT-V1, ADR-0008에 반영했다. 최초 리뷰의 R1 근거·종전 권고와 실행 결과는 위에 이력으로 보존한다. 이 정정은 문서 변경이며 Caddyfile·워크플로 수정, 신규 테스트 실행 또는 production 검증을 의미하지 않는다.
