# ADR-0008 — Rocky Linux Docker Compose와 GitHub Actions CI/CD

- 상태: 채택 / 2026-09-12 Basic Auth 결정 대체 / production 검증 대기
- 날짜: 2026-09-11
- 출처: MVP/Neon 구현 후 사용자가 보유 Rocky Linux 서버와 `erp.jisung.lol` 도메인으로 배포를 요청함
- 관련 요구사항: PRD 후속 포트폴리오 배포 범위, NFR-05·06

## 배경

애플리케이션은 SvelteKit, Go Fiber, Neon PostgreSQL로 구성되어 있고 로컬 MVP 검증은 2026-09-11~12 실행 기록에서 통과했다. 이 결과는 당시 로컬·Neon 검증의 역사이며 현재 production 배포·운영 완료를 의미하지 않는다.
사용자가 Rocky Linux 서버, `deploy` 계정, Docker, GitHub 저장소와 Actions SSH secrets를 준비했다.
v1 제품 내부 로그인/권한은 범위에 없다. 최초 2026-09-11 결정은 Caddy Basic Auth로 공개 접근을 보호하는 것이었으나, 2026-09-12 사용자가 이를 명시적으로 거절했다. API/Web의 외부 직접 포트 노출 금지는 유지한다.

## 결정

- GitHub Actions가 PR/main에서 기존 로컬 격리 테스트를 CI로 실행한다.
- 성공한 main commit만 API/Web Docker 이미지로 만들고 commit SHA 태그로 GHCR에 저장한다.
- CD는 SSH로 Rocky Linux의 `deploy` 계정에 접속해 Docker Compose를 실행한다.
- SvelteKit은 production에서 `adapter-node`를 사용한다.
- Linux host network를 사용하되 API는 `127.0.0.1:8080`, Web은 `127.0.0.1:3000`에만 바인딩한다.
- Caddy만 80/443을 수신하고 자동 TLS와 보안 헤더를 적용한 뒤 같은 출처 `/api`와 Web으로 역프록시한다. 사용자 결정에 따라 reverse-proxy Basic Auth를 사용하거나 복원하지 않는다.
- API는 기본적으로 loopback Host만 허용하고, production에서 명시한 정확한 `PUBLIC_APP_HOST` 하나만 추가 허용한다.
- Neon 비밀값은 Rocky Linux의 `.env.production`에만 두며 GHCR pull에는 job 동안만 유효한 GitHub token을 사용한다.
- migration은 배포마다 one-shot 컨테이너로 `up`만 실행하고 seed/reset/down은 자동화하지 않는다.
- v1에는 앱 인증이 없으므로 익명 호출자는 v1 데모 데이터를 읽고 쓸 수 있다. 데이터 격리나 읽기 전용 보호를 제공한다는 뜻이 아니며 Host·Origin 검사와 loopback 바인딩은 사용자 인증을 대신하지 않는다.
- v2 앱 계정·서버 세션·역할 권한은 별도 승인 요구사항으로 유지한다. Basic Auth 거절을 v2 앱 인증 제거로 확대하지 않는다.

## 대안

- **호스트에 Node/Go 직접 설치 + systemd:** 컨테이너보다 구성 드리프트와 런타임 버전 관리가 늘어난다.
- **Kubernetes/Argo CD:** 단일 포트폴리오 서버 규모에 과도하다.
- **Nginx + Certbot:** 가능하지만 TLS 갱신과 프록시 구성이 Caddy보다 분리된다.
- **API/Web Docker bridge 공개 포트:** 단순하지만 애플리케이션 포트를 호스트 전체에 노출하기 쉽다.
- **비밀번호 SSH/sshpass:** 자동화 비밀값 범위와 공격면이 커져 전용 SSH 키를 사용한다.

## 결과와 단점

배포 단위가 commit SHA로 고정되고 CI 통과 후에만 자동 배포되며 이전 commit을 수동 재배포할 수 있다.
반면 단일 Rocky Linux 서버는 고가용성이 없고 host network는 Linux 전용이다. Basic Auth를 사용하지 않는 v1 공개 데모에서는 익명 사용자가 데이터를 조회·변경할 수 있다. 이 노출을 보안 수정 완료로 표현하지 않는다. v2의 사용자/권한 모델은 별도로 구현·검증해야 한다.

## 검증 및 재검토

로컬에서 두 production Docker image build, Compose config, Caddy config와 기존 테스트를 검증한다.
이 ADR의 채택은 배포 방식의 결정만 의미하며 실제 production 배포 성공을 의미하지 않는다. 실제 서버 배포 완료는 GitHub Actions, TLS, Basic Auth 없는 v1 화면·읽기 API·health 정상 응답 등 [DEPLOYMENT](../DEPLOYMENT.md)의 완료 조건을 충족한 뒤 PROGRESS에 기록한다. v1의 익명 401 또는 Basic Auth 복원은 더 이상 게이트가 아니다. v2 보호 경로는 앱 세션·역할 계약으로 별도 검증한다.
다중 서버, 무중단 배포 또는 공개 데모 정책 변경이 필요해지면 이 결정을 재검토한다. production DB 쓰기 검증은 별도 명시 승인 없이 수행하지 않는다.

## 결정 변경 이력과 미해결 사항

- 2026-09-11: Caddy Basic Auth를 포함한 보호된 배포 방식을 채택했다. 당시 결정과 과거 검증 결과는 이력이며 현재 정책으로 해석하지 않는다.
- 2026-09-12: 사용자가 reverse-proxy Basic Auth를 거절하여 해당 요구와 401 게이트를 대체했다. [코드 리뷰 R1](../CODE-REVIEW-2026-09-12.md)은 정책 변경으로 대체(superseded)되었으며 보안 수정 완료가 아니다.
- 리뷰의 R2(수동 배포 CI 검증), R3(Caddy validate/reload), R4(DB를 깨우는 주기 health)는 구현·검증 증거가 연결되기 전까지 미해결이다. 문서 변경이나 종전 로컬 테스트 통과만으로 해소하지 않는다.
