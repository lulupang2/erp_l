#!/usr/bin/env bash
# One-time Rocky Linux setup for host-managed Caddy.
set -euo pipefail

deploy_user="${1:-deploy}"
project_dir="${2:-$HOME/assembly-erp}"

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root, for example: sudo bash scripts/setup-rocky-caddy.sh $deploy_user" >&2
  exit 2
fi

dnf install -y 'dnf-command(copr)'
dnf copr enable -y @caddy/caddy
dnf install -y caddy
systemctl enable caddy

docker rm -f assembly-erp-prod-caddy-1 2>/dev/null || true

cat >/etc/sudoers.d/assembly-erp-deploy <<EOF
$deploy_user ALL=(root) NOPASSWD: /usr/bin/install -m 0644 -o root -g root Caddyfile /etc/caddy/Caddyfile
$deploy_user ALL=(root) NOPASSWD: /usr/bin/caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
$deploy_user ALL=(root) NOPASSWD: /usr/bin/systemctl reload-or-restart caddy
EOF
chmod 0440 /etc/sudoers.d/assembly-erp-deploy
visudo -cf /etc/sudoers.d/assembly-erp-deploy

if [ -f "$project_dir/Caddyfile" ]; then
  install -m 0644 -o root -g root "$project_dir/Caddyfile" /etc/caddy/Caddyfile
  caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
fi

systemctl restart caddy
systemctl --no-pager --full status caddy
