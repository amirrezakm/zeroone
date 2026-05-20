# Changelog

## [Unreleased]

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
