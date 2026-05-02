# VariX ECS self-use deployment

This is the small single-user deployment path for one Alibaba Cloud ECS instance.
It assumes SQLite on a persistent disk, local API binding, Nginx as the public
entrypoint, and systemd for process supervision.

## 1. Server layout

Use these paths:

- `/opt/varix` for the checked-out release or copied artifact
- `/opt/varix/bin/varix` for the CLI binary
- `/opt/varix/prompts` and `/opt/varix/config` for runtime templates/config
- `/etc/varix/varix.env` for secrets and runtime config
- `/var/lib/varix/content.db` for SQLite data
- `/var/lib/varix/assets` for localized media/assets

Create a service user:

```bash
sudo useradd --system --home /var/lib/varix --shell /usr/sbin/nologin varix
```

## 2. Install

From the repository root on the ECS instance:

```bash
sudo ./deploy/ecs/install.sh
sudo editor /etc/varix/varix.env
sudo systemctl enable --now varix-api.service varix-poll.service varix-maintenance.timer
```

The API service listens on `127.0.0.1:8000`. Keep it local and expose it through
Nginx/Caddy with HTTPS.

The install script builds the binary and syncs the runtime `prompts/` and
`config/` directories into `/opt/varix`, so `VARIX_ROOT=/opt/varix` is stable
even when the source checkout lives elsewhere.

## 3. Nginx

Copy `deploy/ecs/nginx/varix.conf` to `/etc/nginx/conf.d/varix.conf`, replace
`server_name`, then reload Nginx:

```bash
sudo nginx -t
sudo systemctl reload nginx
```

Open only SSH and HTTP/HTTPS in the ECS security group. Do not expose port 8000
directly.

## 4. What runs automatically

- `varix-api.service`: memory API
- `varix-poll.service`: followed-source polling loop
- `varix-maintenance.timer`: provenance lookup, compile sweep, verify sweep, memory projection sweep

`compile sweep` compiles raw captures that do not yet have persisted compile
outputs. When `--user` is supplied, it also backfills those compiled outputs into
that user's content-memory graph so the projection sweep can refresh the API
surfaces.

Useful manual commands:

```bash
VARIX_ROOT=/opt/varix /opt/varix/bin/varix ingest list-follows
VARIX_ROOT=/opt/varix /opt/varix/bin/varix ingest fetch --follow-author <url>
VARIX_ROOT=/opt/varix /opt/varix/bin/varix compile sweep --user kuma --limit 20
VARIX_ROOT=/opt/varix /opt/varix/bin/varix compile run --platform <platform> --id <external_id>
VARIX_ROOT=/opt/varix /opt/varix/bin/varix verify run --platform <platform> --id <external_id>
VARIX_ROOT=/opt/varix /opt/varix/bin/varix memory project-all --user kuma
```

## 5. Backup

Back up `/var/lib/varix/content.db` and `/var/lib/varix/assets`. For SQLite, use
the online backup command when possible:

```bash
sqlite3 /var/lib/varix/content.db ".backup '/var/lib/varix/content.$(date +%Y%m%d%H%M%S).db'"
```

Also configure ECS disk snapshots for the data disk.

## 6. Minimum release check before copying to ECS

```bash
./tests/go-test.sh -count=1 ./...
(cd varix && go vet ./... && go build ./...)
(cd eval && go vet ./... && go build ./...)
git diff --check
```
