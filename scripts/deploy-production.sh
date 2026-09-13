#!/usr/bin/env bash
# Installed beside compose.prod.yml; no database reset or seed.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
exec 9>.deploy.lock
flock -n 9 || { echo "Another deployment is active" >&2; exit 1; }
if [ ! -f .env.production ]; then
  echo 'Missing ~/assembly-erp/.env.production. See docs/DEPLOYMENT.md.' >&2
  exit 2
fi
stage=preflight
trap 'code=$?; if [ "$code" -ne 0 ]; then printf "Deployment failed at %s (exit %s)\n" "$stage" "$code" >&2; fi' EXIT
dc='docker compose --env-file .env.production --env-file .release.env -f compose.prod.yml'
docker compose version
$dc config --quiet
test -f Caddyfile
stage=caddy-preflight
sudo -n install -m 0644 -o root -g root Caddyfile /etc/caddy/Caddyfile
sudo -n caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile || { echo "Caddy config validation failed"; exit 1; }
stage=pull
$dc pull api web
stage=migrate-v1
$dc run --rm -T --interactive=false migrate </dev/null

stage=migrate-v2
$dc run --rm -T --interactive=false migrate-v2 </dev/null
stage=restart
$dc up -d --wait --wait-timeout 120 --remove-orphans api web </dev/null
stage=verify
# Liveness: DB-free health check (R4)
$dc exec -T --interactive=false api curl --fail --silent --show-error http://127.0.0.1:8080/api/v1/health >/dev/null
# Readiness: DB-dependent readiness check (R4)
$dc exec -T --interactive=false api curl --fail --silent --show-error http://127.0.0.1:8080/api/v1/ready >/dev/null
$dc exec -T --interactive=false web node -e "fetch('http://127.0.0.1:3000/items').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"
sudo -n systemctl reload-or-restart caddy || { echo "Caddy reload failed"; exit 1; }
# Verify actual running images rather than accepting an old healthy deployment.
source .release.env
for service in api web; do
  container_id=$($dc ps -q "$service")
  test -n "$container_id"
  if [ "$service" = api ]; then expected="$API_IMAGE:$IMAGE_TAG"; else expected="$WEB_IMAGE:$IMAGE_TAG"; fi
  actual=$(docker inspect --format '{{.Config.Image}}' "$container_id")
  test "$actual" = "$expected"
  test "$(docker inspect --format '{{.Image}}' "$container_id")" = "$(docker image inspect --format '{{.Id}}' "$expected")"
done
$dc exec -T --interactive=false api curl --fail --silent --show-error http://127.0.0.1:8080/api/v2/reference >/dev/null
$dc ps
cp .release.env .deployed.env.tmp
mv .deployed.env.tmp .deployed.env
printf 'DEPLOYMENT_COMPLETE=%s\n' "$IMAGE_TAG"
