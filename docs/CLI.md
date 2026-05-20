# `zeroone` CLI reference

The `zeroone` command at `/usr/local/bin/zeroone` is a thin wrapper
around `docker compose` and `docker exec`. It's the same script you
installed with — running `zeroone update` refreshes both the CLI and the
compose file.

## Subcommands

### `zeroone install`

Bootstrap a fresh install. See [`INSTALL.md`](INSTALL.md).

Idempotent: refuses to overwrite an existing `/opt/zeroone/docker-compose.yml`.
Reruns are safe.

Env vars:

- `ZEROONE_ADMIN_USERNAME`, `ZEROONE_ADMIN_PASSWORD` — skip the
  interactive prompt (for scripted installs).
- `ZEROONE_REF` — branch / tag to fetch the compose file from
  (default `main`).
- `ZEROONE_VERSION` — image tag (default `latest`).
- `ZEROONE_INSTALL_DIR` (default `/opt/zeroone`) and
  `ZEROONE_DATA_DIR` (default `/var/lib/zeroone`).

### `zeroone up` / `down` / `restart`

Plain `docker compose up -d` / `down` / `restart`. State persists across
`down` + `up`.

### `zeroone status`

Prints `docker compose ps` and pokes `/api/health` over loopback.

### `zeroone logs [args...]`

`docker compose logs -f --tail=200 zeroone` by default. Extra args are
forwarded to `docker compose logs`.

```bash
zeroone logs --since 30m       # last 30 min
zeroone logs --tail 50         # last 50 lines, no follow
```

### `zeroone update`

Re-fetches `docker-compose.yml` and the installer script from `main`,
pulls the newest image, restarts the container, and runs
`docker image prune -f`.

Pin a specific version by editing `/opt/zeroone/.env`:

```
ZEROONE_VERSION=v0.1.0
```

### `zeroone uninstall [--purge]`

Without `--purge`: stops the container, keeps `/opt/zeroone` and
`/var/lib/zeroone` so you can `zeroone up` again later.

With `--purge`: deletes everything (`/opt/zeroone`, `/var/lib/zeroone`,
`/usr/local/bin/zeroone`). You will be prompted to type `yes`.

### `zeroone cli ARGS...`

Runs `zeroone` inside the container. Most common uses:

```bash
zeroone cli admin add -config /var/lib/zeroone/stack.json \
    -username bob -password 's3cret'
zeroone cli admin reset-password -config /var/lib/zeroone/stack.json \
    -username bob -password 'new-passw0rd'
zeroone cli admin list -config /var/lib/zeroone/stack.json
zeroone cli -print-xray -config /var/lib/zeroone/stack.json
```

### `zeroone edit` / `zeroone edit-env`

Opens `/var/lib/zeroone/stack.json` (the daemon config) or
`/opt/zeroone/.env` (compose env) in `$EDITOR`, then restarts the
container.

For day-to-day config — adding users, changing inbounds — use the panel
UI; `stack.json` is the source of truth but editing it by hand is meant
for one-off changes the UI doesn't expose.

### `zeroone backup [-o FILE]`

Tars `/var/lib/zeroone/`, `/opt/zeroone/.env`, and
`/opt/zeroone/docker-compose.yml` into a single archive.

```bash
zeroone backup                              # writes /root/zeroone-backup-<timestamp>.tgz
zeroone backup -o /tmp/zeroone-snap.tgz
```

The archive contains password hashes (PBKDF2) and the Xray UUIDs of
every user. Store it encrypted (`age`, `gpg`, S3 SSE, …).

### `zeroone restore -i FILE`

Stops the container, untars over the install + data dirs, and starts
the container again.

```bash
zeroone restore -i /root/zeroone-backup-2026-05-18.tgz
```

### `zeroone version`

Prints the CLI version, the image tag in use, and the daemon's flag
banner.

## Exit codes

- `0` — success.
- `1` — runtime error (no container, no compose file, healthcheck failed).
- `2` — usage error (missing required flag).
