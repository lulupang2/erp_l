# Rocky Linux 배포

대상은 `https://erp.jisung.lol`이다. GitHub Actions에서 CI가 성공한 main 커밋의 API/Web 이미지를 GHCR에 올리고, Rocky 서버에서 `deploy-production.sh`를 실행한다. API/Web은 Docker Compose로 실행하고, Caddy는 Rocky Linux의 systemd 서비스로 직접 운영한다.

## 환경 변수 위치

| 위치 | 이름 | 용도 |
| --- | --- | --- |
| GitHub Repository secrets | DEPLOY_HOST | Rocky 서버 SSH 주소 |
| GitHub Repository secrets | DEPLOY_USER | Docker 실행 권한이 있는 서버 사용자 |
| GitHub Repository secrets | DEPLOY_SSH_KEY | 해당 사용자의 SSH 개인 키 |
| GitHub 자동 제공 | GITHUB_TOKEN | 실행 중 GHCR 이미지 push/pull 및 CI 조회 |
| 서버 .env.production | DATABASE_URL | Neon direct 연결, sslmode=verify-full |
| 서버 .env.production | MIGRATION_DATABASE_URL | 같은 대상 DB의 마이그레이션 권한 연결 |
| 서버 .env.production | PUBLIC_APP_HOST | erp.jisung.lol |

현재 등록된 SSH 시크릿 3개를 유지한다. 자동 배포에 별도 PAT는 필요하지 않다. DB 비밀값은 Git에 넣지 않고 `~/assembly-erp/.env.production`에 저장하며 파일 권한은 600으로 설정한다. v1·v2는 서버 연결을 공유한다. 로그인 없는 합성 데이터 데모다.

## 자동 배포 절차

1. main push → CI 성공 → Deploy production 실행.
2. checkout된 정확한 SHA에 성공한 main push CI가 있는지 Actions API로 확인한다. 수동 실행에도 동일하게 적용한다.
3. SHA 태그로 API/Web 이미지 빌드·push.
4. Compose, Caddyfile, 배포 스크립트, `.release.env`를 서버로 전송.
5. 실행용 GitHub 토큰으로 서버 GHCR 로그인.
6. 서버의 배포 스크립트 실행: 설정 검증 → 호스트 Caddyfile 설치·validate → pull → v1/v2 migration up → API/Web Compose up 및 health 대기 → Caddy reload → 실행 이미지 태그·ID 확인 → v2 API 확인.
7. 로컬 검증 성공 시 `.deployed.env` 기록과 커밋별 완료 표식 출력. 워크플로는 완료 표식과 배포 SHA의 일치를 검사한다.
8. 외부 HTTPS `/`, `/v2/orders`, `/api/v2/reference` 응답을 확인한다. 이 단계까지 성공해야 GitHub 배포 성공이다.

`.release.env`는 배포 요청 버전이며 `.deployed.env`는 서버 내부 검증을 통과한 버전이다. 외부 확인에 실패하면 내부 완료 기록이 있어도 Actions는 실패한다. 업로드 직후 실패할 수 있으므로 release 파일만 보고 배포 성공으로 판단하지 않는다.

## 서버 준비

- deploy 계정에서 `docker ps`가 sudo 없이 동작해야 한다.
- deploy 계정에서 아래 명령을 password 없이 sudo로 실행할 수 있어야 한다.
  - `/usr/bin/install -m 0644 -o root -g root Caddyfile /etc/caddy/Caddyfile`
  - `/usr/bin/caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile`
  - `/usr/bin/systemctl reload-or-restart caddy`
- Docker Compose v2가 `--wait`, `--wait-timeout`, `--interactive=false`를 지원해야 한다.
- Bash와 flock(util-linux)이 있어야 한다.
- DNS가 서버를 가리키고 80/443 포트를 사용할 수 있어야 한다.
- API/Web은 loopback에서만 수신하고 Caddy가 외부 요청을 전달한다.

## Rocky Caddy 최초 설치

Rocky Linux 서버에서 root 또는 sudo 가능한 계정으로 한 번만 실행한다.

```bash
cd ~/assembly-erp
sudo bash setup-rocky-caddy.sh deploy ~/assembly-erp
```

스크립트 없이 직접 실행하려면 아래 명령을 사용한다.

```bash
sudo dnf install -y 'dnf-command(copr)'
sudo dnf copr enable -y @caddy/caddy
sudo dnf install -y caddy
sudo systemctl enable caddy
```

기존 Compose Caddy 컨테이너가 있으면 80/443 포트 충돌을 피하기 위해 중지한다.

```bash
docker rm -f assembly-erp-prod-caddy-1 2>/dev/null || true
sudo systemctl start caddy
```

배포 계정이 `deploy`라면 sudoers 파일을 만든다. 계정명이 다르면 `deploy`를 실제 `DEPLOY_USER` 값으로 바꾼다. 배포 스크립트는 `sudo -n`을 사용하므로 이 권한이 없으면 대기하지 않고 실패한다.

```bash
sudo visudo -f /etc/sudoers.d/assembly-erp-deploy
```

```sudoers
deploy ALL=(root) NOPASSWD: /usr/bin/install -m 0644 -o root -g root Caddyfile /etc/caddy/Caddyfile
deploy ALL=(root) NOPASSWD: /usr/bin/caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
deploy ALL=(root) NOPASSWD: /usr/bin/systemctl reload-or-restart caddy
```

서버에 현재 Caddyfile을 처음 적용한다.

```bash
cd ~/assembly-erp
sudo install -m 0644 -o root -g root Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
sudo systemctl reload-or-restart caddy
sudo systemctl status caddy --no-pager
```

방화벽을 사용 중이면 HTTP/HTTPS를 열어 둔다.

```bash
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
```

## 수동 재시도

GitHub Actions의 Deploy production → Run workflow에서 CI를 통과한 SHA/태그/main을 지정하는 방식이 기본이다.
서버에 최신 파일이 이미 전달된 상태라면 다음 명령으로 같은 절차를 실행할 수 있다.

```bash
cd ~/assembly-erp
# Private GHCR 이미지 pull 권한이 없을 때만 로그인한다.
docker login ghcr.io -u lulupang2
bash --noprofile --norc ./deploy-production.sh
```

수동 로그인 비밀번호에는 read:packages 권한을 가진 토큰을 입력한다. 자동 배포는 자체 토큰을 사용하고 종료 시 로그아웃하므로 수동 로그인 상태가 유지되지 않을 수 있다.

## 상태 확인

```bash
cd ~/assembly-erp
cat .deployed.env
docker compose --env-file .env.production --env-file .release.env -f compose.prod.yml ps
systemctl status caddy --no-pager
curl --fail http://127.0.0.1:8080/api/v2/reference
```

실패 시 로그의 `Deployment failed at ...` 단계와 Actions 실패 항목을 확인한다. 현재 실행 컨테이너가 중간 단계에서 교체됐을 수 있으므로 실패를 자동 원복으로 해석하지 않는다. 자동 seed/reset/down은 실행하지 않는다. 롤백은 이전 CI 성공 SHA로 수동 배포하며, DB 스키마 하향은 수행하지 않는다. 이전 이미지의 스키마 호환성은 먼저 확인한다.

## 2026-09-14 점검 결과

이전 SSH 표준입력 기반 배포는 마이그레이션이 나머지 입력을 소모해 재시작이 누락됐다. 이제 서버의 스크립트 파일을 명시적인 Bash로 실행하고 컨테이너 명령의 입력을 차단한다.
이후 실행 34770843017에서는 마이그레이션·컨테이너 교체가 실제 완료됐지만 `caddy reload`의 설정 경로 누락으로 실패했다. Caddy를 Compose 컨테이너에서 분리해 Rocky Linux systemd 서비스로 운영하고, 배포 스크립트가 `/etc/caddy/Caddyfile` 설치·검증·reload를 명시적으로 수행하도록 수정했다.
이후 실행 34771737067에서는 API/Web 교체와 마이그레이션은 성공했지만 호스트 sudoers가 준비되지 않아 `sudo -n install ...`에서 실패했다. 기존 Caddy 컨테이너가 제거된 뒤 실패하지 않도록 Caddy sudo/install/validate 검사를 컨테이너 교체 전에 실행하도록 조정했다.
