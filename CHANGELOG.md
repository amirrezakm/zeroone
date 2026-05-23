# Changelog

## v0.3.1 — 2026-05-23

### Bug Fixes

- abort edit mode when live config refetch fails (2fb3f43)
- seed xray config edit buffer from live config, not stack render (1ba4287)
- seed live xray.json on boot + editable config editor (457f514)

## v0.3.0 — 2026-05-23

### Features

- auto-discover and extract bundle on `zeroone update` (b510cd3)

### Bug Fixes

- verify bundle by digest, not sidecar filename (f0a84e3)
- use if-block in temp cleanup to satisfy shellcheck (SC2015) (263b402)

## v1.4.1 — 2026-05-23

### Bug Fixes

- upgrade Go toolchain to 1.25.10 and golang.org/x/net to v0.55.0 (8124c7b)

### Documentation

- add AGENTS.md with Cursor Cloud development instructions (f6c361d)

## v0.2.2 — 2026-05-23

### Bug Fixes

- upgrade Go toolchain to 1.25.10 and golang.org/x/net to v0.55.0 (8124c7b)

### Documentation

- add AGENTS.md with Cursor Cloud development instructions (f6c361d)

## v0.2.1 — 2026-05-23

### Bug Fixes

- restore table layouts broken by Tailwind v4 migration (23b2c25)

## v1.3.2 — 2026-05-23

### Bug Fixes

- restore table layouts broken by Tailwind v4 migration (23b2c25)

## v0.2.0 — 2026-05-21

### Features

- xray runtime updates with mirror + offline upload (a3c4410)
- titled snapshots + Xray Config panel page (30dfa31)

### Bug Fixes

- clean upload staging dir on success too (832ea88)
- address Codex review findings from PRs #7–#9 (008f780)
- use regexp sanitiser CodeQL recognises (67986bc)
- lint findings + path-traversal hardening (449a575)
- require login after install; close bootstrap-open hole (7c3963a)

## v0.1.0 — 2026-05-21

### ⚠ Breaking Changes

- rebrand xray-stack/xray-stackd to zeroone (4c97ed9)

### Features

- add air-gapped install flow for Iranian deployments (fbbb3cb)
- one-tap "Add to app" deeplinks in user portal (56a1726)
- add ESLint + Prettier and format the whole tree (ec022ca)
- automate semver releases on merge to main (17666d8)
- let forks install from their own GHCR via ZEROONE_REPO (3359981)
- dockerize + one-line installer + CI (6354edd)

### Bug Fixes

- handle deb822 apt sources; recover image tag from docker-load output (d3efd03)
- pass --pull never to compose; mkdir before tar -C in docs (74f8994)
- pin ZEROONE_VERSION from bundle; restore apt sources on failure (7dd09f8)
- propagate close error from copyFile writer (de31444)
- fall back to recursive copy on cross-device legacy state (1220150)
- merge legacy state into pre-created /var/lib/zeroone (00303c7)
- pin builder stages to $BUILDPLATFORM to avoid QEMU stalls (132e32e)
- bump golangci-lint-action to v7 and Trivy to master (5f8d2fe)
- copy CLI from disk path instead of $0 (86a4a48)
- honor ZEROONE_ADMIN_LISTEN and ZEROONE_STATE_DIR for container template (4b6cc3b)
- gofmt + race in xrayproc supervisor (6bd988f)

### Documentation

- add emojis and cross-links to install guides (9e3950b)
- elevate offline install with Iran-specific rationale (8b75f69)
- simplify with 3-step quick install + Advanced section (384bbee)

### CI

- publish prebuilt offline bundles as release assets (57adf31)
- cover offline install scripts; fix two warnings (25d0e71)

## v1.1.0 — 2026-05-20

### Features

- add air-gapped install flow for Iranian deployments (fbbb3cb)

### Bug Fixes

- handle deb822 apt sources; recover image tag from docker-load output (d3efd03)
- pass --pull never to compose; mkdir before tar -C in docs (74f8994)
- pin ZEROONE_VERSION from bundle; restore apt sources on failure (7dd09f8)

### Documentation

- add emojis and cross-links to install guides (9e3950b)
- elevate offline install with Iran-specific rationale (8b75f69)
- simplify with 3-step quick install + Advanced section (384bbee)

### CI

- publish prebuilt offline bundles as release assets (57adf31)
- cover offline install scripts; fix two warnings (25d0e71)

## v1.0.0 — 2026-05-20

### ⚠ Breaking Changes

- rebrand xray-stack/xray-stackd to zeroone (4c97ed9)

### Bug Fixes

- propagate close error from copyFile writer (de31444)
- fall back to recursive copy on cross-device legacy state (1220150)
- merge legacy state into pre-created /var/lib/zeroone (00303c7)

## [Unreleased]

### Features

- titled snapshots: every snapshot now carries a title, source
  (manual/auto) and action tag. The panel prompts the operator for a
  title on manual capture, Xray apply, and rollback. Important stack
  mutations (xray apply, rollback, failover mode, client endpoints,
  SOCKS, quota/bandwidth apply, direct domains) capture an auto-titled
  snapshot before mutating. Manual snapshots are kept forever; auto
  snapshots are pruned to a 50-entry cap.
- new "Xray Config" panel page: single place to view the rendered
  `xray.json`, see whether changes are pending, and run a titled
  Apply.
- offline install path for air-gapped Iranian deployments via
  `scripts/offline-bundle.sh` + `scripts/install-offline.sh`, with the
  full flow documented in `docs/OFFLINE-INSTALL.md`. Pulls the image
  through the Runflare Docker mirror, packages compose+env+installer
  into a single tarball, and loads everything on the destination via
  `docker load` — no GHCR access required.

### Breaking Changes

- rebrand: rename daemon binary `xray-stackd` → `zeroone`, npm package
  `xray-stack-ui` → `zeroone-ui`, systemd unit
  `xray-stackd.service` → `zeroone.service`, host paths
  `/usr/local/etc/xray-stack` → `/usr/local/etc/zeroone`,
  `/usr/local/share/xray-stack-ui` → `/usr/local/share/zeroone-ui`,
  `/var/lib/xray-stack` → `/var/lib/zeroone`,
  `/etc/default/xray-stackd` → `/etc/default/zeroone`, env var
  `XRAY_STACKD_FLAGS` → `ZEROONE_FLAGS`, session cookie
  `xray_stack_session` → `zeroone_session`, and HTML/UI strings
  to `Zeroone`. The daemon auto-migrates `/var/lib/xray-stack` →
  `/var/lib/zeroone` on first start when the new directory does not
  yet exist; existing admins must log in again after the cookie name
  change.

## v0.2.1 — 2026-05-20

### Bug Fixes

- pin builder stages to $BUILDPLATFORM to avoid QEMU stalls (132e32e)

## v0.2.0 — 2026-05-20

### Features

- one-tap "Add to app" deeplinks in user portal (56a1726)

## v0.1.0 — 2026-05-20

### Features

- add ESLint + Prettier and format the whole tree (ec022ca)
- automate semver releases on merge to main (17666d8)
- let forks install from their own GHCR via ZEROONE_REPO (3359981)
- dockerize + one-line installer + CI (6354edd)

### Bug Fixes

- bump golangci-lint-action to v7 and Trivy to master (5f8d2fe)
- copy CLI from disk path instead of $0 (86a4a48)
- honor ZEROONE_ADMIN_LISTEN and ZEROONE_STATE_DIR for container template (4b6cc3b)
- gofmt + race in xrayproc supervisor (6bd988f)

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Public release preparation: AGPL-3.0 license, CONTRIBUTING / SECURITY /
  CODE_OF_CONDUCT, issue and PR templates.
- Sanitized example configs (`config/stack.example.json`,
  `config/examples/config.example.json`) using RFC-5737 TEST-NET addresses
  and `example.com` hostnames.
- `deploy/skeleton/xray.service` — reference systemd unit for host installs.

### Changed

- Go module path: `github.com/sakhtar/xray-stack-zeroone` →
  `github.com/amirrezakm/zeroone`.
- Repo-root `Dockerfile` (edge-relay for Runflare) moved to
  `cmd/edge-relay/Dockerfile.runflare`.

### Removed

- `rootfs/` — live-host snapshot. Not appropriate for a public repo.
- `scripts/sync-from-server.sh`, `scripts/import-live-stack.py`,
  `scripts/install-local-layout.sh` — internal tooling.
- `docs/migration-cutover.md`, `docs/local-file-inventory.txt`,
  `docs/rewrite-plan.md` — internal history.

## [0.1.0] — TBD

First public release.

[Unreleased]: https://github.com/amirrezakm/zeroone/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/amirrezakm/zeroone/releases/tag/v0.1.0
