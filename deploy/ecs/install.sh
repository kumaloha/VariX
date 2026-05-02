#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PREFIX="${PREFIX:-/opt/varix}"

install -d "${PREFIX}/bin"
install -d "${PREFIX}/deploy/ecs/bin"
install -d "${PREFIX}/data/assets"
install -d /etc/varix
install -d /var/lib/varix

(cd "${ROOT_DIR}/varix" && go build -o "${PREFIX}/bin/varix" ./cmd/cli)

rm -rf "${PREFIX}/config" "${PREFIX}/prompts"
cp -R "${ROOT_DIR}/config" "${PREFIX}/config"
cp -R "${ROOT_DIR}/prompts" "${PREFIX}/prompts"
install -m 0755 "${ROOT_DIR}/deploy/ecs/bin/varix-maintenance" "${PREFIX}/deploy/ecs/bin/varix-maintenance"
install -m 0644 "${ROOT_DIR}/deploy/ecs/systemd/"*.service /etc/systemd/system/
install -m 0644 "${ROOT_DIR}/deploy/ecs/systemd/"*.timer /etc/systemd/system/

if [[ ! -f /etc/varix/varix.env ]]; then
  install -m 0600 "${ROOT_DIR}/deploy/ecs/varix.env.example" /etc/varix/varix.env
  echo "Created /etc/varix/varix.env; edit secrets before starting services." >&2
fi

if id varix >/dev/null 2>&1; then
  chown -R varix:varix /var/lib/varix "${PREFIX}"
else
  echo "User 'varix' does not exist yet. Create it with: useradd --system --home /var/lib/varix --shell /usr/sbin/nologin varix" >&2
fi

systemctl daemon-reload
echo "Install complete. Edit /etc/varix/varix.env, then enable services:"
echo "  systemctl enable --now varix-api.service varix-poll.service varix-maintenance.timer"
