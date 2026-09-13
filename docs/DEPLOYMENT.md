# Rocky Linux 배포 및 CI/CD

대상: `https://erp.jisung.lol`  
구성: GitHub Actions → GHCR → Rocky Linux Docker Compose → Caddy → SvelteKit/Go Fiber → Neon PostgreSQL

> 현재 상태: **배포 코드·워크플로 작성 상태이며 production 배포 완료가 아닙니다.** 2026-09-12 리뷰의 로컬 회귀 결과와 배포 경로 지적은 당시 실행 이력입니다. 사용자 결정으로 Caddy Basic Auth 요구(R1)는 정책으로 대체되었고, R2/R3/R4는 구현·검증 증거가 연결될 때까지 미해결입니다. 실제 완료 여부는 [PROGRESS](PROGRESS.md)에 새 실행 증거가 기록된 경우에만 판단합니다.

## 현재 코드와 운영 기준의 차이 (2026-09-12)

아래 절차는 목표 운영 기준이다. 현재 구현이 모든 기준을 충족한 상태는 아니다. 구현 차이의 근거는 2026-09-12 리뷰 시점이며, 후속 코드 변경의 완료 여부는 별도 구현·검증 증거로 갱신한다.

| 항목 | 현재 코드 | 필요한 보완 |
| --- | --- | --- |
| Basic Auth | 리뷰 시작 시 Caddyfile 작업 트리에서 주석 처리 | 사용자 결정으로 사용하지 않음. R1은 정책 변경으로 대체되었으며 보안 수정 완료가 아님 |
| 수동 배포 | inputs.ref를 CI 성공 확인 없이 배포 | checkout SHA의 성공한 main push CI 검증 |
| Caddyfile 갱신 | 업로드 후 compose up, reload 없음 | validate와 명시적 reload 및 변경 응답 확인 |
| 유휴 DB | 15초마다 DB Ping health 호출 | 프로세스 생존 검사와 DB 준비 상태 검사 분리 |
| 외부 smoke test | 리뷰 당시 workflow는 인증 없는 401까지만 확인 | Basic Auth 없이 v1 화면·읽기 API 200 확인으로 변경 필요 |

세부 근거·수정 조건은 [코드 리뷰](CODE-REVIEW-2026-09-12.md)를 따른다. 이번 문서 갱신에서 서버 설정이나 workflow를 변경하지 않았다.

2026-09-12 사용자 결정으로 reverse-proxy Basic Auth를 사용하지 않으며 Caddyfile에 복원하지 않습니다. v1에는 앱 계정/권한이 없으므로 **익명 호출자는 v1 데모 데이터를 읽고 쓸 수 있습니다**. 이는 공개 업무 API의 읽기·쓰기 위험을 수용하는 정책이며 안전한 공개 운영이나 데이터 격리를 뜻하지 않습니다. Host·Origin 검사와 loopback 바인딩은 사용자 인증을 대신하지 않습니다. Go API와 SvelteKit 서버의 외부 직접 노출 금지는 유지합니다.

이 결정은 v2의 개별 앱 계정·세션·서버 역할 권한 요구사항을 제거하지 않습니다. 아래 익명 200 기준은 v1 데모에만 적용하며 v2 보호 경로의 앱 인증/권한 거절 응답과 구분합니다. v1 익명 쓰기 자체는 production smoke test에서 수행하지 않습니다.

## 1. 현재 준비 상태

사용자가 이미 준비한 항목:

- Git 저장소 초기화 및 GitHub `origin` 연결
- 첫 커밋 생성
- Rocky Linux 서버 보유
- `deploy` 사용자 존재
- Docker Engine 설치
- GitHub Actions repository secrets 등록: `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_SSH_KEY`
- production 도메인 확정: `erp.jisung.lol`

아직 실제 서버에서 별도 확인이 필요한 항목:

- `deploy` 사용자의 Docker 실행 권한
- DNS A/AAAA 레코드가 실제 Rocky Linux 서버를 가리키는지
- 80/443 방화벽 개방
- 서버의 `.env.production` 생성
- GitHub Actions CI/CD 실제 실행
- `https://erp.jisung.lol` TLS와 Basic Auth 없는 v1 화면/읽기 API/health 확인

## 2. 서버 1회 준비

`deploy` 사용자가 sudo 없이 Docker를 실행할 수 있어야 합니다.

```bash
sudo systemctl enable --now docker
sudo usermod -aG docker deploy
```

그룹 변경 후에는 `deploy` 계정으로 다시 로그인하고 확인합니다.

```bash
docker --version
docker compose version
groups
docker ps
mkdir -p ~/assembly-erp
chmod 700 ~/assembly-erp
```

`docker ps`가 sudo 없이 성공해야 GitHub Actions의 SSH 배포가 동작합니다.

Rocky Linux에서 firewalld를 사용한다면 80/443만 외부에 개방합니다.

```bash
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
```

SvelteKit 3000과 Go API 8080은 외부 방화벽에 공개하지 않습니다.

## 3. DNS

`erp.jisung.lol`의 A 레코드를 Rocky Linux 서버의 public IPv4로 지정합니다.
IPv6가 실제 서버까지 정상 라우팅되지 않는다면 잘못된 AAAA 레코드는 두지 않습니다.

```bash
getent ahosts erp.jisung.lol
```

Caddy가 80/443을 받을 수 있으면 최초 기동 시 TLS 인증서를 자동 발급합니다.

## 4. production 환경 파일

실제 비밀값은 Git에 저장하지 않습니다. 서버의 `deploy` 계정 홈 아래에만 둡니다.

```bash
cd ~/assembly-erp
umask 077
vi .env.production
chmod 600 .env.production
```

필수 항목:

```dotenv
DATABASE_URL='<Neon direct URL with sslmode=verify-full>'
MIGRATION_DATABASE_URL='<same direct Neon target with sslmode=verify-full>'
PUBLIC_APP_HOST=erp.jisung.lol
```

DB 연결은 ERP 전용 Neon direct endpoint를 사용합니다. Pooler, 다른 DB, `sslmode=require` 또는 `sslmode=disable`은 production 기준으로 허용하지 않습니다.

Basic Auth 사용자·비밀번호·해시는 이 정책의 필수 환경 변수가 아니며 생성하거나 설정할 필요가 없습니다.

## 5. GitHub Actions secrets

repository secrets:

```text
DEPLOY_HOST
DEPLOY_USER
DEPLOY_SSH_KEY
```

현재 이 3개는 사용자가 등록 완료했습니다. 서버 계정 비밀번호는 CI/CD에서 사용하지 않습니다.

## 6. CI 기준

PR과 `main` push에서 다음을 검증합니다.

1. Node/pnpm/Go 고정 버전 설치
2. lockfile 기반 의존성 설치
3. 격리된 PostgreSQL 테스트 DB 기동
4. Go vet 및 Svelte/TypeScript 검사
5. 단위 테스트
6. 실제 PostgreSQL 통합 테스트
7. Chromium HTTP/UI E2E
8. DB invariant 검사
9. OpenAPI lint
10. production build
11. secret scan

CI는 Neon production secret 없이 실행 가능해야 하며, 실패한 commit은 production으로 배포하지 않습니다.

## 7. CD 기준

운영 기준은 `main`의 CI가 성공한 commit만 배포하는 것입니다. 자동 workflow_run 경로는 이를 검사하지만 수동 workflow_dispatch 경로에는 아직 같은 검사가 없습니다.

1. 해당 commit SHA checkout
2. API/Web production Docker image build
3. GHCR에 commit SHA tag로 push
4. SSH로 Rocky Linux의 `deploy` 사용자에 접속
5. release 파일과 image tag 전달
6. GHCR 로그인 및 image pull
7. v1·v2 migration `up`을 각각 one-shot으로 1회 실행
8. API/Web 갱신 및 Caddy 설정 검증·적용(현재 workflow에 명시적인 reload 단계 추가 필요)
9. 내부 API/Web health check
10. 외부 `https://erp.jisung.lol`의 TLS와 Basic Auth 없는 v1 화면 HTTP 200 확인
11. 인증 정보 없이 v1 읽기 API와 health HTTP 200 및 정상 응답 확인(리뷰 당시 401 게이트는 이 기준으로 변경 필요)

배포 시 `seed`, `reset`, migration `down`은 자동 실행하지 않습니다.

## 8. 실행 구조

```text
Internet
   │
   ▼
Caddy :80/:443
   │
   ├── /api/* ──► Go Fiber 127.0.0.1:8080 ──► Neon PostgreSQL
   │
   └── /* ──────► SvelteKit 127.0.0.1:3000
```

Caddy만 외부 포트를 수신합니다. 애플리케이션 컨테이너의 3000/8080은 외부에서 직접 접근하지 않습니다.

## 9. 배포 후 확인

```bash
cd ~/assembly-erp
docker compose --env-file .env.production --env-file .release.env -f compose.prod.yml ps
docker compose --env-file .env.production --env-file .release.env -f compose.prod.yml logs --tail=100 api web caddy
```

외부에서 확인할 항목:

```text
https://erp.jisung.lol               → Basic Auth 없이 v1 화면 200
https://erp.jisung.lol/api/v1/items  → 인증 정보 없이 v1 읽기 응답 200
https://erp.jisung.lol/api/v1/health → 인증 정보 없이 정상 health 200
```

화면에서는 품목, 재고, 생산 지시 목록을 확인하고 브라우저 콘솔 오류가 없는지 확인합니다. production DB 쓰기 검증은 별도 명시 승인 없이 수행하지 않습니다.

## 10. 롤백

Docker image는 commit SHA로 식별합니다. 이전에 검증된 commit SHA로 `.release.env`를 되돌린 뒤 동일한 Compose 갱신 절차를 적용합니다.

DB migration은 forward-only `up`을 기본으로 합니다. 자동 `down` 롤백은 하지 않습니다. schema 호환성이 깨질 수 있는 변경은 expand/contract 방식으로 배포합니다.

## 11. 장애 확인 순서

- SSH 실패: `DEPLOY_HOST`, `DEPLOY_USER`, public key 등록, 서버 SSH 정책 확인
- Docker permission denied: `deploy`의 docker group 적용 및 재로그인 확인
- GHCR pull 실패: workflow package 권한 및 registry login 확인
- Caddy TLS 실패: DNS A/AAAA, 80/443 방화벽, 기존 포트 점유 확인
- API unhealthy: Neon direct URL, `sslmode=verify-full`, migration 상태 확인
- v1 화면/읽기 API에 Basic Auth challenge 또는 예상하지 않은 401: 실행 중인 Caddy 설정과 reload 적용 여부 확인. Basic Auth를 복원하지 않는다. v2 앱 인증에 따른 401/403은 별도 계약으로 판정한다.
- 502/503: API/Web 컨테이너 상태와 loopback health check부터 확인

## 12. 완료 판정

다음이 모두 확인된 뒤에만 `docs/PROGRESS.md`에 production 배포 완료로 기록합니다.

- CI 전 단계 통과
- API/Web image GHCR push 성공
- Rocky Linux pull 및 Compose 기동 성공
- migration `up` 성공
- 내부 API/Web health 정상
- Caddy TLS 인증서 정상
- Basic Auth challenge 없이 외부 v1 화면 200
- 인증 정보 없이 v1 읽기 API와 health 200 및 정상 응답
- production smoke test 중 DB/브라우저 오류 없음

이 문서의 체크리스트가 작성되어 있다는 사실만으로 실제 배포가 완료된 것으로 간주하지 않습니다.
