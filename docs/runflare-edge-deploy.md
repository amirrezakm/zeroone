# Runflare edge deployment

Runflare (`runflare.com`) is an Iranian PaaS. Apps deploy as Docker
containers on a managed subdomain with automatic TLS. Runflare does **not**
expose raw TCP or custom ports, so we add an Xray inbound by running a
small Go reverse proxy (`cmd/edge-relay`) on runflare that forwards
WebSocket and XHTTP requests back to the origin server. The origin IP
in the examples below is the RFC 5737 placeholder `203.0.113.1` — set
`ORIGIN_HOST` to your real origin at deploy time.

The same binary works on any PaaS that runs a single Docker container
behind HTTPS (liara, hamravesh, render, fly.io…) — set the env vars and
you're done.

---

## Architecture

```
client ── wss/https ──► your-app.runflare.run ── http ──► <origin>
                       (runflare terminates TLS)            ws:443 /vless
                                                            xhttp:80 /api/v1/events  (nginx → 127.0.0.1:3002)
```

Runflare's edge terminates TLS. The edge-relay container receives plain
HTTP and proxies to the origin:

| Client path             | Origin target                                       |
| ----------------------- | --------------------------------------------------- |
| `/vless` (ws)           | `http://203.0.113.1:443/vless`                   |
| `/api/v1/events` (xhttp)| `http://203.0.113.1:80/api/v1/events` (nginx)    |
| anything else           | benign `OK` landing page                            |

---

## Origin side — prerequisites

The origin host must already have:

- `xray` listening on `0.0.0.0:443` (VLESS-WS, plain — no TLS at origin).
- `xray` listening on `127.0.0.1:3002` (VLESS-XHTTP, plain).
- `nginx` listening on `0.0.0.0:80` forwarding `/api/v1/events` → `127.0.0.1:3002`.
- The firewall allows 80/tcp and 443/tcp from runflare's egress range
  (UFW `allow 80,443/tcp` covers any source).

If the production stack is already running, no origin changes are
required for this edge.

---

## Runflare side

The runflare CLI **only deploys**. It has no `apps create`, no `env set`,
no domain commands. You create the project + item in the web dashboard
first, then `runflare deploy` from this repo pushes the build.

### 1. Install the CLI (one time)

```sh
npm install -g runflare
```

Available commands: `login`, `logout`, `deploy`, `start`, `stop`, `restart`,
`status`, `log`, `event`, `reset`, `update`, `version`. Run `runflare help <cmd>`
for flags.

### 2. Create the project in the dashboard

1. Sign in at https://runflare.com.
2. Create a new **Project**.
3. Inside the project, create an **Item** (sometimes labelled "service")
   of type **Docker** / **Custom Dockerfile**.
4. Set the build to use this repo's Dockerfile path: `cmd/edge-relay/Dockerfile`
   with build context `.` (repo root).
5. Set the listening port to `8080`.
6. Set environment variables (see table below).
7. Note the assigned **Project ID** + **Item ID** (in the URL or item
   settings page) and the **public hostname** runflare gives the item
   (e.g. `xray-edge.runflare.run`).

### 3. Required env vars (set in the dashboard)

| Env var              | Value for this stack          | Purpose                                              |
| -------------------- | ----------------------------- | ---------------------------------------------------- |
| `ORIGIN_HOST`        | your origin IP / host         | Convenience: defaults WS_TARGET=:443, XHTTP_TARGET=:80 |
| `WS_PATH`            | `/vless`                      | Set empty to disable the WS route                    |
| `XHTTP_PATH`         | `/api/v1/events`              | Set empty to disable the XHTTP route                 |
| `LOG_LEVEL`          | `info`                        | `debug` / `info` / `warn` / `error`                  |

Optional knobs:

| Env var              | Default                        | Purpose                                              |
| -------------------- | ------------------------------ | ---------------------------------------------------- |
| `WS_TARGET`          | `http://ORIGIN_HOST:443`       | Full origin URL when ORIGIN_HOST isn't enough        |
| `WS_HOST`            | _(passthrough)_                | Override Host header to origin                       |
| `XHTTP_TARGET`       | `http://ORIGIN_HOST:80`        | Full origin URL when ORIGIN_HOST isn't enough        |
| `XHTTP_HOST`         | _(passthrough)_                | Override Host header to origin                       |
| `EDGE_EXTRA_ROUTES`  | _(empty)_                      | `label:/path=http://host[:port][,Host=override];...` |
| `LANDING_BODY`       | `OK`                           | Body served for unmatched paths                      |
| `PORT`               | `8080`                         | Runflare sets this automatically                     |

### 4. Log in from this machine

```sh
runflare login                # interactive prompt for email/totp
# or:
runflare login -e you@example.com -p 'pass' -t 123456
```

### 5. Deploy

From the repo root:

```sh
cd <repo root>
runflare deploy
```

First run prompts for project/item; pick the ones you created. Subsequent
deploys use the cached selection (`runflare deploy -y` to skip the
prompt). If you noted the IDs:

```sh
runflare deploy \
  --project-id <PID> --item-id <IID> \
  --project-name xray-edge --item-name relay
```

The deploy uploads the working tree (`.dockerignore` keeps `node_modules`,
`web/app/dist`, the local stack config, etc. out of the bundle), runflare
builds the Dockerfile, and starts the container.

### 6. Smoke test

```sh
curl -i https://<your-runflare-host>/healthz   # 200 ok
curl -i https://<your-runflare-host>/          # 200 OK landing
curl -sS -X POST https://<your-runflare-host>/api/v1/events | head
```

For WS, test via a VLESS client after wiring the panel — see step 8.

### 7. Tail logs

```sh
runflare log -y           # tails the cached item
# or with explicit IDs:
runflare log --project-id <PID> --item-id <IID>
```

You should see `route` lines on start and `proxy error` lines only on
backend failures.

---

## Wire the runflare endpoint into the panel

Two `client_endpoint` entries already exist in `config/stack.local.json`,
both **disabled** with placeholder host `your-app.runflare.run`:

- `runflare-ws`    — `:443/vless` (ws + tls)
- `runflare-xhttp` — `:443/api/v1/events` (xhttp + tls)

Once you know the real hostname runflare assigned:

1. On the server, edit `/etc/xray-stack/stack.json` (or whatever
   `xray-stackd` is loading) and update both `host` fields, then
   `enabled: true`.
2. `systemctl reload xray-stackd` (or `restart`) — or call
   `PUT /api/client-endpoints` from the panel for live update.

VLESS share links generated by the panel then include both runflare
variants alongside `pars-pack`.

---

## Reusing the binary on other PaaS

The Dockerfile is provider-agnostic. Examples:

**Liara**:
```sh
liara create --app xray-edge-relay --platform docker
liara env set --app xray-edge-relay ORIGIN_HOST=<origin> WS_PATH=/vless XHTTP_PATH=/api/v1/events
liara deploy --app xray-edge-relay --dockerfile cmd/edge-relay/Dockerfile
```

**Hamravesh / Darkube** — push the built image to their registry and
deploy as a single-replica workload with `PORT=8080` and the same env vars.

For each new edge, add a matching `client_endpoint` pair (one ws, one
xhttp) to `config/stack.local.json` and the panel picks them up.

---

## Troubleshooting

**`bad gateway` on every request** — origin host isn't reachable from
runflare's egress. The tailed log will show
`proxy error … dial tcp <origin>:443: connection refused`. Verify
reachability from any external box: `curl -v http://<origin>/api/v1/events`.

**WS 502 / no upgrade** — runflare's edge isn't passing `Connection:
upgrade` through. Confirm the dashboard hasn't enabled HTTP/2-only mode
on the item; WebSockets need HTTP/1.1.

**Origin sees wrong `Host` header** — set `WS_HOST=<origin>` and/or
`XHTTP_HOST=<origin>` to match what Xray's inbound expects. The
current `/vless` ws inbound has no Host check, so this is usually only
needed if you front a different upstream.

**xhttp stream-up disconnects at ~100s** — runflare's edge may close idle
streams. Mirror the `pars-pack` xmux tuning in the
`runflare-xhttp.extra` block (`c_max_lifetime_ms: 60000`) so the client
rotates first.
