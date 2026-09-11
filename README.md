# 조립 제조 ERP

SvelteKit과 Go Fiber로 구현한 조립 제조 ERP 포트폴리오입니다. **품목 → BOM → 부품 입고 → 생산 지시 → 부분 생산·불량 → 재고 이력**을 연결합니다.
로그인 없는 단일 조직·단일 재고 위치의 **로컬 데모**이며 공개 배포는 지원하지 않습니다.

현재 구현·검증 결과와 남은 작업은 **[PROGRESS](docs/PROGRESS.md)**에서 관리합니다. 로컬 PostgreSQL MVP 검증에 이어 승인된 ERP 전용 Neon에서 실제 TLS 연결·마이그레이션·브라우저 생산 흐름·유휴 재개를 검증했습니다. 자동 명령 실행과 수동 제어 인계 범위는 진행 기록에 구분합니다.

## 기준 문서

| 문서 | 책임 |
| --- | --- |
| [PRD](docs/PRD.md) | 제품 목표·범위·업무 규칙·제품 인수 조건 |
| [ADR 목록](docs/adr/README.md) | 기술 결정의 배경·대안·트레이드오프 |
| [SSOT](docs/SSOT.md) | 수량 제한·데이터·API·트랜잭션·연결·검증 규칙 |
| [API 계약](contracts/openapi.yaml) / [DTO 개요](contracts/README.md) | 실제 요청·응답과 오류 스키마 |
| [ERD](docs/ERD.md) | 실제 마이그레이션의 테이블·관계·제약 |
| [데모 절차](docs/DEMO.md) | 키보드 조립 및 실패 시나리오 시연 |
| [프론트 디자인](docs/UI-DESIGN.md) | NASEEJ 참고 방향, 라이트/다크 테마와 화면 동작 |
| [Neon 체크리스트](docs/NEON-CHECKLIST.md) | 실제 Neon 연결·업무 흐름·유휴 재개 검증 |

## 실행 환경과 고정 버전

| 구성 | 버전 |
| --- | --- |
| Node.js / pnpm | 26.8.2 / 10.11.0 |
| Svelte / SvelteKit / TypeScript | 5.57.0 / 2.70.3 / 6.0.3 |
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

## 구조와 핵심 설계

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

이 프로젝트는 인증·권한·다중 창고·재고 조정·원가·판매·구매·공개 배포를 포함하지 않습니다. 로컬 접근 제한은 인증을 대신하는 운영 보안 설계가 아니므로 인터넷에 노출하지 않습니다.
