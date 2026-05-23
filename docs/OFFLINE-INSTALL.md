# 🇮🇷 Offline Install (Iran / Air-Gapped)

> 🌐 **Have full internet on your server?** You probably want the
> [standard one-line install](INSTALL.md) instead. This guide is for
> servers that **can't reach `ghcr.io`** — typically VPS hosts inside
> Iran or other air-gapped networks.

For Iranian servers — or any server that can't reach `ghcr.io`,
Docker Hub, or GitHub. You'll need a laptop (or any machine with
internet) plus SFTP access to your target server. ⏱️ Total time:
about 3 minutes once the bundle is downloaded.

## ✨ Quick install — 3 steps

### 📦 1. Download the bundle

Go to the **[latest release page](https://github.com/amirrezakm/zeroone/releases/latest)**
and download the file matching your server's CPU:

- 💻 `zeroone-offline-vX.Y.Z-amd64.tar.gz` — for Intel / AMD servers (most servers)
- 📱 `zeroone-offline-vX.Y.Z-arm64.tar.gz` — for ARM servers (Raspberry Pi, Ampere, AWS Graviton)

> 🤔 Not sure which arch? On the target server run `uname -m`.
> `x86_64` means amd64; `aarch64` means arm64.

### 📤 2. Upload to your server via SFTP

```bash
sftp root@YOUR_SERVER_IP
sftp> put zeroone-offline-latest-amd64.tar.gz
sftp> bye
```

(Or use `scp`, `rsync`, FileZilla — anything that gets the file there.)

### ▶️ 3. Install

SSH into the server, then:

```bash
tar -xzf zeroone-offline-*.tar.gz
sudo bash install-offline.sh
```

It'll ask for an admin username and password. After ~30 seconds,
open `http://YOUR_SERVER_IP:8000/` in your browser and log in.

🎉 **That's it.**

## ✅ Day-to-day

```bash
zeroone status       # is the panel up?
zeroone logs -f      # follow logs
zeroone restart      # restart the container
zeroone backup       # tar up state to /root/zeroone-backup-*.tgz
```

## ⬆️ Upgrading

🆕 A new version came out? Just SFTP the new bundle to the server (same
as step 2) and run one command:

```bash
sudo zeroone update
```

It finds the newest `zeroone-offline-*.tar.gz` you uploaded (it checks
your home dir, the current dir, `/tmp`, and the install dirs), verifies
its `.sha256` if present, extracts it, and swaps in the new image — no
manual `tar` needed.

🛡️ Your admins, users, and config are preserved.

> Want to point at a specific file or a directory you already extracted?
> `sudo zeroone update -a /path/to/bundle.tar.gz` or
> `sudo zeroone update -b /path/to/extracted-dir`.

---

## 🛠 Advanced

Everything below is optional — only read this if you hit a problem
or have a non-standard setup.

### 🤖 Skip the interactive admin prompt

Useful for automation. Set both env vars before running the installer:

```bash
sudo ZEROONE_ADMIN_USERNAME=admin \
     ZEROONE_ADMIN_PASSWORD='choose-a-long-one' \
     bash install-offline.sh
```

### 🐳 When Docker is missing on the server

If the server has no Docker installed, `install-offline.sh` will
try to install it for you using the Runflare apt mirror
(`mirror-linux.runflare.com`). This works on Debian / Ubuntu and
requires the server to reach Runflare's mirror network — almost
always true on Iranian VPS hosts.

🛟 The script backs up `/etc/apt/sources.list` (and any Deb822
`*.sources` defaults on modern Ubuntu / Debian) before touching
them, and restores them on **any** exit path — success or
failure — via an `EXIT` trap. Your apt config is never left in an
inconsistent state.

On RHEL / AlmaLinux / Rocky / Alpine, the script doesn't try to
auto-install Docker. Install it yourself (any version with `docker
compose v2` works) and re-run. Example for Debian Bookworm via the
Runflare mirror:

```bash
cat > /etc/apt/sources.list <<'EOF'
deb http://mirror-linux.runflare.com/debian bookworm main
deb http://mirror-linux.runflare.com/debian-security bookworm-security main
EOF
apt-get update
apt-get install -y docker.io docker-compose-v2
```

🎛 Override knobs:

- `ZEROONE_LINUX_MIRROR=...` — point at a different apt mirror.
- `ZEROONE_KEEP_MIRROR=1` — keep the Runflare mirror in
  `sources.list` after the install (default is to restore the
  original).
- `ZEROONE_SKIP_DOCKER_INSTALL=1` — refuse to touch `sources.list`
  at all; fail loudly instead. Use this when you've pre-installed
  Docker yourself.

### 🔨 Building your own bundle

The prebuilt bundles cover the upstream `amirrezakm/zeroone` image.
Build your own when you're packaging a fork, an unreleased build,
or a custom registry source.

On a machine with Docker and access to either `ghcr.io` or
`mirror-docker.runflare.com`:

```bash
cd zeroone
bash scripts/offline-bundle.sh
# → dist/zeroone-offline-latest-amd64.tar.gz + .sha256
```

🎛 Knobs:

| Variable | Default | Notes |
| --- | --- | --- |
| `ZEROONE_VERSION` | `latest` | Image tag to bundle (e.g. `v1.0.0`). |
| `ZEROONE_ARCH` | `amd64` | Target arch — `amd64` or `arm64`. Must match `uname -m` on the destination. |
| `ZEROONE_REPO` | `amirrezakm/zeroone` | Source repo on the registry. |
| `ZEROONE_IMAGE_SRC` | `mirror-docker.runflare.com/$ZEROONE_REPO` | Where to pull from. Set to `ghcr.io/$ZEROONE_REPO` if your builder has direct GHCR access. |
| `OUT_DIR` | `./dist` | Where to write the tarball. |

### 🪞 Direct Runflare mirror (skip SFTP entirely)

If your destination server itself can reach
`mirror-docker.runflare.com` (most Iranian VPSes can), skip the
bundle entirely. Add the mirror to Docker's daemon config:

```bash
mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'EOF'
{ "registry-mirrors": ["https://mirror-docker.runflare.com"] }
EOF
systemctl restart docker
```

Then run the standard online installer. This still needs
`raw.githubusercontent.com` to fetch `docker-compose.yml`, so if
GitHub is blocked too, use the bundle flow above.

### 🔐 Verify the SHA-256 of a downloaded bundle

Each bundle on the Releases page ships with a `.sha256` sidecar.
Verify before installing:

```bash
sha256sum -c zeroone-offline-vX.Y.Z-amd64.tar.gz.sha256
```

### 🔧 Troubleshooting

**❗ "manifest unknown" or "image not found" when the container starts.**
The bundle is built for a different CPU arch than the server. Check
with `uname -m` and download the matching `amd64` / `arm64` bundle.

**❗ `apt-get install docker.io` fails.**
The Runflare apt mirror is unreachable from your server. Install
Docker manually (see "When Docker is missing" above) and re-run
`install-offline.sh`.

**❗ `docker compose version` says command not found.**
You have the legacy Python v1 `docker-compose` installed. Install
the v2 plugin: `apt-get install docker-compose-v2` (Debian / Ubuntu)
or `dnf install docker-compose-plugin` (RHEL family).

**❗ SHA-256 doesn't match after SFTP.**
The transfer corrupted the tarball. Retry with `rsync -avP
--checksum` or re-`sftp put`.

**❗ "admin add failed" at the end of install.**
The container started but the panel hadn't finished initializing
`/var/lib/zeroone/stack.json` yet. Wait a few seconds, then run:

```bash
zeroone cli admin add -config /var/lib/zeroone/stack.json \
    -username admin -password 'choose-a-long-one'
```

### 🚩 Useful `install-offline.sh` flags

```bash
sudo bash install-offline.sh --bundle /path/to/extracted/dir  # explicit bundle dir
sudo bash install-offline.sh --force                          # overwrite existing .env

sudo zeroone update                       # auto-discover the newest uploaded bundle
sudo zeroone update -a bundle.tar.gz      # update from a specific archive
sudo zeroone update -b /path/to/extracted # update from an already-extracted dir
```

### 🔗 Related docs

- 📘 [`INSTALL.md`](INSTALL.md) — the standard online install path.
- 🖥 [`HOST-INSTALL.md`](HOST-INSTALL.md) — systemd install (no Docker)
  for OpenVPN failover and bandwidth shaping.
- ☁️ [`runflare-edge-deploy.md`](runflare-edge-deploy.md) — running a
  Runflare PaaS edge in front of an origin server.
