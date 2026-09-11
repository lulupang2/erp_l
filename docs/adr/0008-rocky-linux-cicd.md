# ADR-0008 — Rocky Linux Docker Compose와 GitHub Actions CI/CD

- 상태: 채택 / production 검증 대기
- 날짜: 2026-09-11
- 출처: MVP/Neon 구현 후 사용자가 보유 Rocky Linux 서버와 `erp.jisung.lol` 도메인으로 배포를 요청함
- 관련 요구사항: PRD 후속 포트폴리오 배포 범위, NFR-05·06

## 배경

애플리케이션은 SvelteKit, Go Fiber, Neon PostgreSQL로 구성되어 있고 로컬 MVP 검증은 완료했다.
사용자가 Rocky Linux 서버, `deploy` 계정, Docker, GitHub 저장소와 Actions SSH secrets를 준비했다.
제품 내부 로그인/권한은 아직 범위에 없으므로 API를 인터넷에 직접 노출해서는 안 된다.

## 결정

- GitHub Actions가 PR/main에서 기존 로컬 격리 테스트를 CI로 실행한다.
- 성공한 main commit만 API/Web Docker 이미지로 만들고 commit SHA 태그로 GHCR에 저장한다.
- CD는 SSH로 Rocky Linux의 `deploy` 계정에 접속해 Docker Compose를 실행한다.
- SvelteKit은 production에서 `adapter-node`를 사용한다.
- Linux host network를 사용하되 API는 `127.0.0.1:8080`, Web은 `127.0.0.1:3000`에만 바인딩한다.
- Caddy만 80/443을 수신하고 자동 TLS, 보안 헤더, Basic Auth를 적용한 뒤 같은 출처 `/api`와 Web으로 역프록시한다.
- API는 기본적으로 loopback Host만 허용하고, production에서 명시한 정확한 `PUBLIC_APP_HOST` 하나만 추가 허용한다.
- Neon 비밀값은 Rocky Linux의 `.env.production`에만 두며 GHCR pull에는 job 동안만 유효한 GitHub token을 사용한다.
- migration은 배포마다 one-shot 컨테이너로 `up`만 실행하고 seed/reset/down은 자동화하지 않는다.

## 대안

- **호스트에 Node/Go 직접 설치 + systemd:** 컨테이너보다 구성 드리프트와 런타임 버전 관리가 늘어난다.
- **Kubernetes/Argo CD:** 단일 포트폴리오 서버 규모에 과도하다.
- **Nginx + Certbot:** 가능하지만 TLS 갱신과 프록시 구성이 Caddy보다 분리된다.
- **API/Web Docker bridge 공개 포트:** 단순하지만 애플리케이션 포트를 호스트 전체에 노출하기 쉽다.
- **비밀번호 SSH/sshpass:** 자동화 비밀값 범위와 공격면이 커져 전용 SSH 키를 사용한다.

## 결과와 단점

배포 단위가 commit SHA로 고정되고 CI 통과 후에만 자동 배포되며 이전 commit을 수동 재배포할 수 있다.
반면 단일 Rocky Linux 서버는 고가용성이 없고 host network는 Linux 전용이다. Caddy Basic Auth는 애플리케이션의
사용자/권한 모델을 대신하지 않으므로 실제 다중 사용자 서비스로 확장할 때 제품 인증을 별도로 도입해야 한다.

## 검증 및 재검토

로컬에서 두 production Docker image build, Compose config, Caddy config와 기존 테스트를 검증한다.
이 ADR의 채택은 배포 방식의 결정만 의미하며 실제 production 배포 성공을 의미하지 않는다. 실제 서버 배포 완료는 GitHub Actions와 `https://erp.jisung.lol` TLS/401 게이트가 성공한 뒤 PROGRESS에 기록한다.
다중 서버, 무중단 배포, 앱 사용자 계정 또는 공개 체험 쓰기 기능이 필요해지면 이 결정을 재검토한다.
