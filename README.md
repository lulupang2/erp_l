# 조립 제조 ERP

> **2026-09-14 전환:** v2는 소규모 조립공장 흐름(작업 지시, 자재 불출·반납, 생산, 검사·불량, 완제품 입고)을 시연합니다. 포트폴리오 범위에서는 로그인·계정 관리·사용자별 권한을 제외하고 하나의 공용 데모 작업 공간으로 실행합니다.

SvelteKit과 Go Fiber로 구현한 조립 제조 ERP 포트폴리오입니다. **품목 → BOM → 부품 입고 → 생산 지시 → 부분 생산·불량 → 재고 이력**을 연결합니다.
v1과 v2 모두 포트폴리오 데모이며 로그인 기능을 포함하지 않습니다. 두 기능 영역은 Neon의 하나의 신뢰된 서버 연결을 공유하며, 운영 환경의 사용자 인증·권한 모델은 별도 요구사항으로 설계해야 합니다.

현재 구현·검증 결과와 남은 작업은 **[PROGRESS](docs/PROGRESS.md)**에서 관리합니다. 아래 로컬 PostgreSQL MVP 및 승인된 ERP 전용 Neon의 TLS 연결·마이그레이션·브라우저 생산 흐름·유휴 재개 결과는 v1의 날짜별 이력입니다. 이번 문서 정합화나 병렬 작업 시작은 재검증·production 배포·v2 통과 증거가 아닙니다.

## 기준 문서

| 문서 | 책임 |
| --- | --- |
| [PRD v2](docs/PRD.md) | 구현 중인 목표 제품의 범위·업무 규칙·인수 조건 |
| [ADR 목록](docs/adr/README.md) | 기술 결정의 배경·대안·트레이드오프 |
| [SSOT v2](docs/SSOT.md) | 목표 수량·권한·데이터·API·트랜잭션·검증 규칙; 구현 완료 명세가 아님 |
| [API 계약](contracts/openapi.yaml) / [DTO 개요](contracts/README.md) | v1 계약 기준과 v2 계약 전환의 구분 |
| [ERD](docs/ERD.md) | v1 마이그레이션의 테이블·관계·제약; v2 모델은 별도 검증 필요 |
| [v1 데모 절차](docs/DEMO.md) | 구현된 키보드 조립 및 실패 시나리오 시연 |
| [프론트 디자인](docs/UI-DESIGN.md) | v1 디자인·검증 이력과 v2 화면 요구 구분 |
| [Neon 체크리스트](docs/NEON-CHECKLIST.md) | 실제 Neon 연결·업무 흐름·유휴 재개 검증 |
| [Rocky Linux 배포](docs/DEPLOYMENT.md) | GitHub Actions CI/CD, GHCR, Docker Compose, Caddy/TLS와 서버 준비 |
| [2026-09-12 코드 리뷰](docs/CODE-REVIEW-2026-09-12.md) | v1 로컬 회귀 및 배포 지적 이력, 후속 결정과 검증 상태 |
| [공장 흐름 전환 계획](docs/FACTORY-PLAN.md) | v2 단계별 구현·인수 조건·데이터 이전 |
| [v1 PRD](docs/PRD-V1.md) / [v1 SSOT](docs/SSOT-V1.md) | 보존된 v1 코드·데이터의 업무 기준; v2 목표와 구분 |

## 실행 환경과 고정 버전

| 구성 | 버전 |
| --- | --- |
| Node.js / pnpm | 26.8.2 / 10.11.0 |
| Svelte / SvelteKit / TypeScript | 5.57.0 / 2.70.3 / 6.0.3 (`adapter-node` 5.5.7 production) |
| Vite | 8.3.0 |
| Go / Fiber | 1.26.7 / 3.5.0 |
| pgx / sqlc / Goose | 5.11.0 / 1.31.1 / 3.28.0 |
| 로컬 PostgreSQL | 17.10 (`postgres:17.10-bookworm`) |
| Playwright / Vitest | 1.63.0 / 5.0.0 |

패키지 잠금은 `pnpm-lock.yaml`, `apps/api/go.mod`, `apps/api/go.sum`을 따릅니다. Go 1.21 이상 런처가 있으면 루트 실행 스크립트가 Go 1.26.7 도구 체계를 사용합니다. 직접 `apps/api`에서 Go 명령을 실행할 때도 `GOTOOLCHAIN=go1.26.7`을 권장합니다.
Neon 프로젝트도 PostgreSQL 17로 생성해 로컬 테스트 DB와 메이저 버전을 맞췄습니다.

## 로컬 실행

Docker Desktop의 Linux 컨테이너 엔진, Go 런처, Node, pnpm 10.11.0이 필요합니다. PowerShell에서 저장소 루트로 이동합니다.

```powershell
Set-Location C:\work2\portfolio\erp
node --version
pnpm --version
docker compose version

pnpm install --frozen-lockfile
pnpm setup:local
pnpm db:up
pnpm db:migrate
pnpm demo:seed
pnpm dev
```

명령 하나가 실패하면 다음 단계로 넘어가지 말고 해당 오류를 확인합니다. `pnpm dev`는 API와 화면을 함께 실행하며, 종료는 해당 터미널에서 `Ctrl+C`를 누릅니다.

| 접속 대상 | 주소/범위 |
| --- | --- |
| 업무 화면 | `http://127.0.0.1:5173` |
| API | `http://127.0.0.1:8080/api/v1` |
| 개발 프록시 | 브라우저의 같은 출처 `/api` → 로컬 Go API |
| 데모 PostgreSQL | `127.0.0.1:55432`, DB `erp_demo` |
| 테스트 PostgreSQL | `127.0.0.1:55433`, DB `erp_test` |

`setup:local`은 무작위 로컬 전용 비밀값을 **Git 추적 제외된 `.env`에 한 번만** 생성합니다. 기존 `.env`가 있으면 덮어쓰지 않습니다. 실제 값은 출력하지 않으며 브라우저 실행 환경에서도 제거합니다. `.env.example`에는 값이 비어 있습니다.

별도 터미널 실행은 `pnpm dev:api`와 `pnpm dev:web`을 사용합니다. 서버 시작은 마이그레이션·시드·초기화를 자동 실행하지 않습니다.

### Windows에서 Node가 PATH에 없는 경우

프로젝트 전용 Node를 사용할 수 있습니다. 공식 배포본 체크섬을 검증하며 시스템 Node와 전역 PATH는 변경하지 않습니다.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/bootstrap-node.ps1
$env:PATH = "$PWD\.tools\node-v26.8.2-win-x64;$env:PATH"
node --version
```

이미 원하는 Node가 실행되는 터미널에서는 위 단계가 필요하지 않습니다. 브리지 터미널과 개인 터미널의 PATH가 다를 수 있습니다.

## 예제 데이터와 초기화

`pnpm demo:seed`는 케이스 10개, 스위치 800개, 키보드 품목과 케이스 1·스위치 80 BOM을 준비합니다. 생산 지시는 생성하지 않습니다.
같은 시드를 다시 실행하면 기존 입고 키를 재사용해 재고가 중복 증가하지 않습니다. 기존 예제 품목·BOM과 내용이 충돌하면 덮어쓰지 않고 실패합니다.

기본 시연은 계획 10개에 양품 4·불량 1, 이후 양품 3·불량 2를 등록합니다. 최종 재고는 케이스 0·스위치 0·키보드 7, 지시 누계는 양품 7·불량 3·잔여 0입니다. 자세한 절차는 [DEMO](docs/DEMO.md)를 따릅니다.

```powershell
# 명시적인 로컬 erp_demo:55432만 초기화합니다. 다른 DB는 거절합니다.
pnpm demo:reset --confirm ERP_DEMO_RESET
pnpm demo:seed

# 데이터 볼륨을 보존하고 ERP 컨테이너만 종료합니다.
pnpm db:down
```

초기화는 예제 데모 DB의 업무 데이터와 성공 요청 키를 제거합니다. 스키마 및 별도 테스트 DB는 보존합니다. 응답이 불확실한 요청을 화면에 남긴 상태에서 초기화했다면 기존 입력 대신 새 데모 작업을 시작하세요.

## 검증 명령

```powershell
pnpm check                 # Go vet + Svelte/TypeScript 검사
pnpm test:unit             # Go 단위 테스트 + 프론트엔드 + 환경 안전장치
pnpm test:integration      # 분리된 실제 PostgreSQL에서 잠금·롤백·제약 검증
pnpm exec playwright install chromium
pnpm test:e2e              # HTTP 인수 테스트 + 실제 Chromium 업무 흐름
pnpm check:invariants      # 테스트 DB 전체 잔액·이력·실적 집계 대조
pnpm contract:lint
pnpm build
pnpm check:secrets
```

통합/E2E 테스트는 로컬 `erp_test:55433`만 허용하며 `erp_demo`·외부 DB는 거절합니다. 테스트는 고유 코드를 사용하며 데모 DB를 자동 초기화하지 않습니다. 테스트 데이터는 테스트 DB에 남습니다.
`test:unit`에서는 실DB 통합 테스트를 명시적으로 건너뜁니다. 실DB 통과 근거는 별도의 `test:integration` 실행 결과입니다.
E2E는 관리되는 API `18080`, 화면 `5174` 포트를 사용하고 끝나면 해당 프로세스를 종료합니다. JSON 결과와 시연 스크린샷은 추적 제외된 `.local`에, 실패 추적 자료는 `test-results`에 생성됩니다.

### sqlc 재생성과 API 명세

Windows에서는 체크섬을 확인한 프로젝트 전용 sqlc를 준비합니다. sqlc 생성 코드는 포함하지만 수동 수정하지 않습니다.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/bootstrap-sqlc.ps1
pnpm db:generate
pnpm check:generated
pnpm contract:generate
pnpm contract:lint
```

SQL 원본은 `apps/api/db/queries`, 마이그레이션은 `apps/api/db/migrations`입니다. `check:generated`는 실제 sqlc로 재생성한 파일이 기존 파일과 같은지 확인합니다. API 명세 원본은 `scripts/generate-openapi.mjs`이며 생성 결과와 서버·클라이언트를 함께 유지합니다.

## v1 구조와 핵심 설계

```text
apps/
  api/
    cmd/                  server · migrate · demo
    db/migrations/        Goose SQL + embedded migrations
    db/queries/           sqlc 원본 SQL
    internal/
      catalog/            품목 · BOM
      inventory/          입고 · 현재고 · 재고 이력
      production/         지시 · BOM 복사본 · 부분 실적
      domain/             API DTO · 수량/입력/오류 정책
      store/              공통 트랜잭션 · 요청 키 · 재고 잠금
      db/                 sqlc 생성 코드
      httpapi/            Fiber HTTP 어댑터
      platform/           연결 · TLS · 로컬 실행 제한
      integration/        실제 PostgreSQL 검증
  web/src/                SvelteKit 업무 화면 · API 클라이언트
contracts/                OpenAPI · DTO 계약
scripts/                  로컬 환경 · 실행 · 검증 · 코드 생성
tests/browser/            HTTP 인수 및 실제 브라우저 테스트
docs/                     PRD · SSOT · ADR · ERD · 진행 기록
```

단일 Go 프로세스의 모듈형 모놀리스입니다. Fiber 타입은 HTTP 계층에만 두고 서비스는 표준 `context.Context`와 트랜잭션에 바인딩된 sqlc 쿼리를 사용합니다. ORM·캐시·메시지 브로커는 추가하지 않았습니다.

생산 실적은 READ COMMITTED 트랜잭션 안에서 지시 행, 이어서 UUID 순으로 모든 관련 재고 행을 잠급니다. 검증을 통과한 뒤 실적·집계·자재 차감·양품 입고·재고 이력·요청 키를 함께 커밋합니다. 중간 실패는 모두 롤백합니다.

BOM 교체와 지시 생성은 같은 완제품·BOM 헤더 잠금 순서를 사용해 최초 BOM 생성 및 교체 중에도 구성이 섞이지 않도록 합니다. 기존 지시는 자체 BOM 복사본을 사용합니다.

입고·지시·실적의 `Idempotency-Key`는 작업 종류와 함께 고유합니다. 실적 입력 해시에 경로의 지시 ID도 포함합니다. 동일 요청은 기존 결과를 반환하고 다른 입력에 같은 키를 쓰면 거절합니다. 화면은 응답이 유실되면 원래 키와 입력을 보존하며, 새로고침 후에도 같은 요청을 재확인합니다.

## Neon 상태와 운영 범위

승인된 `assembly-erp` 프로젝트를 싱가포르 리전, PostgreSQL 17, 0.25 CU 고정 크기로 생성했습니다. 기존 프로젝트와 로컬 DB는 보존했습니다. 실제 검증 결과는 [Neon 체크리스트](docs/NEON-CHECKLIST.md)와 [PROGRESS](docs/PROGRESS.md)를 따릅니다.

```powershell
# 기존 로컬 .env를 바꾸지 않고, 별도의 .env.neon을 선택합니다.
pnpm dev:neon

# 명시적 Neon 관리/진단 명령입니다. 자동 초기화하지 않습니다.
pnpm neon:status
pnpm neon:probe
pnpm neon:migrate
```

Neon 실행에는 DB 컨테이너가 필요하지 않습니다. 화면은 동일한 `http://127.0.0.1:5173`이며 `pnpm dev`와 동시에 같은 포트로 실행하지 않습니다. 일반 `pnpm dev`와 모든 자동 통합 테스트는 원래 로컬 설정을 계속 사용합니다.

`.env.neon`에는 백엔드 전용 `DATABASE_URL`, `MIGRATION_DATABASE_URL`과 대상 확인용 `NEON_PROJECT_ID`, `NEON_BRANCH_ID`, `NEON_HOST`, `NEON_DATABASE`를 둡니다. 이 파일은 Git에서 제외됩니다. 직접 연결·정확한 호스트/DB·`verify-full`이 일치하지 않으면 Neon 명령을 거절합니다. 실제 비밀값을 문서·커밋·브라우저 변수·애플리케이션 로그에 기록하지 않습니다.

`pnpm neon:verify`는 실제 클라우드 데모 데이터를 추가하고 운영자가 유휴 상태를 확인하는 별도 검증입니다. 일상적인 로컬 테스트에 포함하지 않으며, 사용법과 결과 위치는 체크리스트를 참조하세요. 원격 데모 초기화는 구현하지 않았습니다.
외부 PostgreSQL에는 `sslmode=verify-full`을 요구하고 연결 풀은 MaxConns 5 / MinConns 0, 연결 제한 15초 / DB 작업 제한 20초입니다. 로컬 테스트 결과를 실제 Neon TLS·유휴 재개·복구 검증으로 간주하지 않습니다.

## Rocky Linux production 배포

`erp.jisung.lol`의 목표 production 구성은 `adapter-node` Web과 Go API를 Docker로 실행하고 둘 다 loopback에만 바인딩하며, Caddy만 80/443을 수신하는 방식입니다. **Caddy는 TLS와 역프록시를 담당하며 Basic Auth를 요구하지 않습니다.** 이는 사용자의 배포 결정이며 v1의 익명 조회·쓰기 위험을 없애지 않습니다.

2026-09-12 리뷰 당시 Basic Auth 주석 처리, 수동 배포 CI 검증, Caddy reload, 주기 DB health check가 지적되었습니다. Basic Auth 복구 요구는 이후 사용자 결정으로 대체되었습니다. 나머지 수정과 no-proxy-auth 설정 적용·배포 검증 여부는 [리뷰 문서](docs/CODE-REVIEW-2026-09-12.md)와 [PROGRESS](docs/PROGRESS.md)의 실제 후속 증거를 따릅니다. 이 문서 수정만으로 해결을 선언하지 않습니다.

실제 서버 준비, DNS, `.env.production`, GitHub secrets, 배포 확인과 롤백은 [DEPLOYMENT](docs/DEPLOYMENT.md)를 따릅니다. 문서 존재나 작업자 시작을 production 완료로 간주하지 않습니다. v1에 실제 업무·민감 데이터를 넣지 않는 데모 운영은 위험 제한 권고이지 서버 접근 통제가 아닙니다.

v1에는 애플리케이션 사용자 계정·권한이 없습니다. **v2에는 사용자 세션·역할별 서버 권한·감사가 필수**이며 proxy 인증 제거와 별개의 제품 요구입니다. v2도 수주·구매·출하·회계·원가·다중 공장 전체 ERP로 확대하지 않습니다. v1의 단일 재고 위치와 달리 v2는 창고·지시별 현장·품질 상태의 논리 위치를 포함합니다.
