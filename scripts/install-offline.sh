#!/usr/bin/env bash
# install-offline.sh — air-gapped zeroone installer + CLI wrapper.
#
# This is the counterpart to scripts/install.sh. It expects to find the
# Docker image tar, compose file, and env example in the same directory
# (or in a directory passed via --bundle), produced by
# scripts/offline-bundle.sh. It never reaches GHCR, Docker Hub, or
# GitHub raw.
#
# Typical usage on the destination:
#
#     tar -xzf zeroone-offline-*.tar.gz
#     sudo bash install-offline.sh
#
# After install, this script self-copies to /usr/local/bin/zeroone and
# is invoked as `zeroone <subcommand>`. The `install` and `update`
# subcommands of the online installer are replaced by an offline
# `update -b BUNDLE_DIR` that loads a new image tar from a fresh
# bundle.

set -Eeuo pipefail

# ----- configuration knobs ----------------------------------------------------

INSTALL_DIR=${ZEROONE_INSTALL_DIR:-/opt/zeroone}
DATA_DIR=${ZEROONE_DATA_DIR:-/var/lib/zeroone}
CLI_PATH=${ZEROONE_CLI_PATH:-/usr/local/bin/zeroone}

COMPOSE_FILE="${INSTALL_DIR}/docker-compose.yml"
ENV_FILE="${INSTALL_DIR}/.env"
STACK_FILE="${DATA_DIR}/stack.json"

SCRIPT_VERSION="0.1.0-offline"

# Runflare apt mirror — used only if Docker is missing on Debian/Ubuntu
# and the operator has not pre-installed it. Override
# ZEROONE_LINUX_MIRROR to point at a different mirror, or set
# ZEROONE_SKIP_DOCKER_INSTALL=1 to refuse to touch sources.list.
LINUX_MIRROR=${ZEROONE_LINUX_MIRROR:-http://mirror-linux.runflare.com}

# ----- helpers ----------------------------------------------------------------

if [ -t 1 ]; then
    C_RED='\033[1;31m'
    C_GREEN='\033[1;32m'
    C_YELLOW='\033[1;33m'
    C_BLUE='\033[1;34m'
    C_RESET='\033[0m'
else
    C_RED=''; C_GREEN=''; C_YELLOW=''; C_BLUE=''; C_RESET=''
fi

log()  { printf '%b==>%b %s\n' "${C_BLUE}" "${C_RESET}" "$*"; }
ok()   { printf '%b ok%b %s\n' "${C_GREEN}" "${C_RESET}" "$*"; }
warn() { printf '%bwarn%b %s\n' "${C_YELLOW}" "${C_RESET}" "$*" >&2; }
die()  { printf '%berr%b %s\n' "${C_RED}" "${C_RESET}" "$*" >&2; exit 1; }

trap 'die "command failed at line $LINENO"' ERR

require_root() {
    if [ "$(id -u)" -ne 0 ]; then
        die "must be run as root (try: sudo $0)"
    fi
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64|aarch64|arm64) : ;;
        *) die "unsupported architecture: $(uname -m). zeroone supports x86_64 and aarch64." ;;
    esac
}

detect_os() {
    if [ ! -r /etc/os-release ]; then
        die "/etc/os-release not found; cannot detect Linux distribution"
    fi
    # shellcheck source=/dev/null
    . /etc/os-release
    OS_ID=${ID:-unknown}
    OS_LIKE=${ID_LIKE:-}
    OS_CODENAME=${VERSION_CODENAME:-}
    case "${OS_ID}" in
        debian|ubuntu|centos|rhel|fedora|almalinux|rocky|alpine) : ;;
        *)
            case "${OS_LIKE}" in
                *debian*|*rhel*|*fedora*) : ;;
                *) die "unsupported OS: ${OS_ID} (${OS_LIKE:-no ID_LIKE})" ;;
            esac
            ;;
    esac
}

dc() {
    docker compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" "$@"
}

# Locate the bundle. Sets BUNDLE_DIR and BUNDLE_IMAGE_TAR.
#
#   --bundle DIR   explicit path
#   default:       directory of $0, then $PWD
find_bundle() {
    local explicit=${1:-}
    local candidates=()
    if [ -n "${explicit}" ]; then
        candidates+=("${explicit}")
    else
        local self_dir
        self_dir=$(cd "$(dirname "$0")" && pwd)
        candidates+=("${self_dir}" "${PWD}")
    fi
    local dir tar
    for dir in "${candidates[@]}"; do
        [ -d "${dir}" ] || continue
        # Match the bundler's output pattern. Glob expansion (not ls)
        # so filenames with spaces are handled correctly.
        tar=""
        for candidate in "${dir}"/zeroone-image*.tar; do
            if [ -f "${candidate}" ]; then
                tar="${candidate}"
                break
            fi
        done
        if [ -n "${tar}" ] \
            && [ -f "${dir}/docker-compose.yml" ] \
            && [ -f "${dir}/.env.example" ]; then
            BUNDLE_DIR="${dir}"
            BUNDLE_IMAGE_TAR="${tar}"
            return 0
        fi
    done
    die "could not locate bundle. Expected zeroone-image*.tar + docker-compose.yml + .env.example in one of: ${candidates[*]}. Pass --bundle DIR to point at the extracted bundle directory."
}

ensure_docker_via_runflare_mirror() {
    [ "${ZEROONE_SKIP_DOCKER_INSTALL:-0}" = "1" ] && \
        die "docker is missing and ZEROONE_SKIP_DOCKER_INSTALL=1 set. Install docker manually and re-run."

    case "${OS_ID}" in
        debian|ubuntu) : ;;
        *)
            cat >&2 <<EOF
Docker is not installed and automatic installation via the Runflare
mirror is only implemented for Debian / Ubuntu. On ${OS_ID}, install
Docker manually (the engine + the compose v2 plugin) and re-run this
script. See docs/OFFLINE-INSTALL.md for an example sources.list /
dnf.repos.d snippet pointing at ${LINUX_MIRROR}.
EOF
            die "manual docker install required on ${OS_ID}"
            ;;
    esac

    local codename=${OS_CODENAME}
    if [ -z "${codename}" ] && command -v lsb_release >/dev/null 2>&1; then
        codename=$(lsb_release -cs)
    fi
    [ -n "${codename}" ] || die "could not detect Debian/Ubuntu codename for the mirror sources.list"

    local mirror_path
    case "${OS_ID}" in
        debian) mirror_path="${LINUX_MIRROR}/debian" ;;
        ubuntu) mirror_path="${LINUX_MIRROR}/ubuntu" ;;
    esac

    log "rewriting apt sources to use ${mirror_path} (codename: ${codename})"

    # Arm the EXIT trap before touching any file so a failure between
    # here and the end of the function still restores everything.
    trap _restore_apt_sources_list EXIT

    # Modern Ubuntu (24.04+) and Debian (13+) keep their default repo
    # configuration in /etc/apt/sources.list.d/{ubuntu,debian}.sources
    # (Deb822 format) rather than /etc/apt/sources.list. If we only
    # rewrote sources.list, apt would still hit the original blocked
    # mirrors via the Deb822 file. Disable any Deb822 default by
    # renaming it aside; our sources.list takes over for the duration
    # of the install, and the rename is undone by the EXIT trap.
    _backup_apt_file /etc/apt/sources.list
    _disable_apt_file /etc/apt/sources.list.d/ubuntu.sources
    _disable_apt_file /etc/apt/sources.list.d/debian.sources

    case "${OS_ID}" in
        debian)
            cat > /etc/apt/sources.list <<EOF
deb ${mirror_path} ${codename} main contrib non-free non-free-firmware
deb ${LINUX_MIRROR}/debian-security ${codename}-security main contrib non-free non-free-firmware
EOF
            ;;
        ubuntu)
            cat > /etc/apt/sources.list <<EOF
deb ${mirror_path} ${codename} main restricted universe multiverse
deb ${mirror_path} ${codename}-updates main restricted universe multiverse
deb ${mirror_path} ${codename}-backports main restricted universe multiverse
deb ${mirror_path} ${codename}-security main restricted universe multiverse
EOF
            ;;
    esac

    log "apt-get update via ${mirror_path}"
    apt-get update -qq

    log "installing docker.io + docker-compose-v2"
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker.io docker-compose-v2
}

# Copy ${1} → ${1}.zeroone.bak so we can rewrite the file in place
# and restore it later. No-op if the file doesn't exist or a backup
# is already present.
_backup_apt_file() {
    local f=$1
    [ -f "${f}" ] || return 0
    [ -e "${f}.zeroone.bak" ] && return 0
    cp "${f}" "${f}.zeroone.bak"
}

# Rename ${1} → ${1}.zeroone.bak so apt stops reading it. Used for
# Deb822 default sources in /etc/apt/sources.list.d/ on modern
# Ubuntu / Debian — we want only our /etc/apt/sources.list active
# during the install.
_disable_apt_file() {
    local f=$1
    [ -f "${f}" ] || return 0
    [ -e "${f}.zeroone.bak" ] && return 0
    mv "${f}" "${f}.zeroone.bak"
}

# EXIT-trap cleanup: put every apt sources file back the way we
# found it. Safe to run multiple times; a no-op if there's no
# backup. Honors ZEROONE_KEEP_MIRROR=1 to skip restoration.
_restore_apt_sources_list() {
    [ "${ZEROONE_KEEP_MIRROR:-0}" = "1" ] && return 0
    local restored=0 f
    for f in /etc/apt/sources.list \
             /etc/apt/sources.list.d/ubuntu.sources \
             /etc/apt/sources.list.d/debian.sources; do
        [ -f "${f}.zeroone.bak" ] || continue
        mv -f "${f}.zeroone.bak" "${f}" 2>/dev/null || true
        restored=1
    done
    [ "${restored}" -eq 1 ] && \
        log "restored original apt sources (set ZEROONE_KEEP_MIRROR=1 next time to keep the Runflare mirror)"
    return 0
}

ensure_docker() {
    if command -v docker >/dev/null 2>&1; then
        ok "docker present: $(docker --version)"
    else
        log "docker not found; installing via Runflare mirror"
        ensure_docker_via_runflare_mirror
    fi
    if ! docker compose version >/dev/null 2>&1; then
        die "docker compose plugin missing. Install docker-compose-v2 (Debian/Ubuntu) or docker-compose-plugin (RHEL family) and re-run."
    fi
    if ! systemctl is-active --quiet docker 2>/dev/null; then
        systemctl enable --now docker >/dev/null 2>&1 || \
            service docker start >/dev/null 2>&1 || \
            warn "could not start docker; continuing anyway"
    fi
}

wait_for_health() {
    local timeout=${1:-60}
    local port
    port=$(grep -E '^ZEROONE_ADMIN_LISTEN_PORT=' "${ENV_FILE}" | tail -1 | cut -d= -f2 || true)
    port=${port:-8000}
    log "waiting for panel on http://127.0.0.1:${port}/api/health (${timeout}s)"
    local deadline=$((SECONDS + timeout))
    while [ "${SECONDS}" -lt "${deadline}" ]; do
        if curl -fsS "http://127.0.0.1:${port}/api/health" >/dev/null 2>&1; then
            ok "panel is up"
            return 0
        fi
        sleep 2
    done
    warn "panel did not respond on /api/health within ${timeout}s — check 'zeroone logs'"
    return 1
}

print_summary() {
    local host port
    host=$(grep -E '^ZEROONE_ADMIN_LISTEN=' "${ENV_FILE}" | tail -1 | cut -d= -f2 || true)
    port=$(grep -E '^ZEROONE_ADMIN_LISTEN_PORT=' "${ENV_FILE}" | tail -1 | cut -d= -f2 || true)
    port=${port:-8000}
    printf '\n'
    ok "zeroone is installed (offline)."
    printf '   panel:     http://%s:%s/   (substitute your server IP)\n' "YOUR_HOST_IP" "${port}"
    printf '   listen:    %s\n' "${host:-0.0.0.0:${port}}"
    printf '   compose:   %s\n' "${COMPOSE_FILE}"
    printf '   state:     %s\n' "${DATA_DIR}"
    printf '   cli:       %s\n' "${CLI_PATH}"
    printf '\nTry: zeroone status\n'
    printf 'For upgrades, build a fresh bundle on the connected host and run:\n'
    printf '   zeroone update -b /path/to/extracted/bundle\n\n'
}

prompt_admin() {
    if [ -n "${ZEROONE_ADMIN_USERNAME:-}" ] && [ -n "${ZEROONE_ADMIN_PASSWORD:-}" ]; then
        ADMIN_USER=${ZEROONE_ADMIN_USERNAME}
        ADMIN_PASS=${ZEROONE_ADMIN_PASSWORD}
        return
    fi
    if [ ! -t 0 ]; then
        die "interactive admin prompt requires a TTY. Either rerun with stdin attached, or export ZEROONE_ADMIN_USERNAME / ZEROONE_ADMIN_PASSWORD before installing."
    fi
    printf '\n--- create panel admin ---\n'
    local pass2
    while :; do
        read -rp "admin username: " ADMIN_USER
        ADMIN_USER=${ADMIN_USER//$'\r'/}
        [ -n "${ADMIN_USER}" ] && break
        echo "username cannot be empty"
    done
    while :; do
        read -rsp "admin password: " ADMIN_PASS; echo
        read -rsp "      (confirm): " pass2; echo
        if [ "${ADMIN_PASS}" != "${pass2}" ]; then
            echo "passwords do not match"
            continue
        fi
        if [ ${#ADMIN_PASS} -lt 8 ]; then
            echo "password must be at least 8 characters"
            continue
        fi
        break
    done
    printf '\n'
}

# Load image tar and capture the docker-load output so the tag can
# be recovered without relying on python3. Sets LOAD_OUTPUT as a
# script-global. The captured output is also printed to the user.
LOAD_OUTPUT=""
load_image() {
    local tar=$1
    log "loading image from $(basename "${tar}")"
    LOAD_OUTPUT=$(docker load -i "${tar}" 2>&1)
    printf '%s\n' "${LOAD_OUTPUT}"
}

# Parse `Loaded image: <tag>` from the last docker-load output.
# Works on every system; no python3 needed.
tag_from_load_output() {
    [ -n "${LOAD_OUTPUT}" ] || return 0
    printf '%s\n' "${LOAD_OUTPUT}" | sed -n 's/^Loaded image: //p' | head -n1
}

# Set KEY=VALUE in ${ENV_FILE}, replacing an existing active line or
# appending a new one with a header comment. Does not touch
# commented-out forms (#KEY=...).
set_env_var() {
    local key=$1 value=$2
    if grep -qE "^${key}=" "${ENV_FILE}" 2>/dev/null; then
        sed -i "s|^${key}=.*|${key}=${value}|" "${ENV_FILE}"
    else
        printf '\n# Pinned by install-offline.sh from the bundle image tag.\n%s=%s\n' "${key}" "${value}" >> "${ENV_FILE}"
    fi
}

# Pin ZEROONE_IMAGE + ZEROONE_VERSION in .env from the tag inside the
# bundled image tar, so `docker compose up` resolves to the image we
# just `docker load`-ed instead of whatever default (typically
# `latest`) the .env.example shipped with.
pin_image_from_tar() {
    local tar=$1 loaded_tag base ver
    # Prefer the tag printed by `docker load` (no python3 dependency,
    # works on minimal systems). Fall back to parsing the tar manifest
    # if for some reason docker-load output didn't include a tag.
    loaded_tag=$(tag_from_load_output)
    if [ -z "${loaded_tag}" ]; then
        loaded_tag=$(tag_from_tar "${tar}")
    fi
    if [ -z "${loaded_tag}" ]; then
        die "could not determine image tag from docker-load output or $(basename "${tar}") manifest. The bundle may be malformed. Check \`docker images\` for the loaded tag, set ZEROONE_VERSION + ZEROONE_IMAGE in ${ENV_FILE} manually, then run 'zeroone up'."
    fi
    base="${loaded_tag%:*}"
    ver="${loaded_tag##*:}"
    # Always pin the version, even when the base repo is the default —
    # otherwise a bundle built with ZEROONE_VERSION=v1.1.0 would run
    # nothing (image:latest isn't loaded).
    set_env_var ZEROONE_VERSION "${ver}"
    if [ "${base}" != "ghcr.io/amirrezakm/zeroone" ]; then
        set_env_var ZEROONE_IMAGE "${base}"
    fi
    log "pinned ZEROONE_IMAGE=${base} ZEROONE_VERSION=${ver} in ${ENV_FILE}"
}

# Extract the first image tag from a docker save tarball's manifest.
# Used to pin ZEROONE_IMAGE in .env when the bundle was built for a
# fork (e.g. ghcr.io/myfork/zeroone instead of the default).
tag_from_tar() {
    local tar=$1
    # `docker load` already printed the tag, but parsing the tar's
    # manifest.json is more reliable for non-interactive flows.
    if command -v python3 >/dev/null 2>&1; then
        python3 - "$tar" <<'PY' 2>/dev/null || true
import json, sys, tarfile
with tarfile.open(sys.argv[1]) as t:
    m = t.extractfile("manifest.json")
    if m is None: sys.exit(0)
    j = json.load(m)
    tags = j[0].get("RepoTags") or []
    if tags: print(tags[0])
PY
    fi
}

# ----- subcommands ------------------------------------------------------------

cmd_install() {
    require_root
    detect_arch
    detect_os

    local bundle_dir="" force=0
    while [ $# -gt 0 ]; do
        case "$1" in
            -b|--bundle) bundle_dir=$2; shift 2 ;;
            -f|--force)  force=1; shift ;;
            *) die "unknown flag for install: $1" ;;
        esac
    done

    if [ -f "${COMPOSE_FILE}" ] && [ "${force}" -ne 1 ]; then
        warn "already installed at ${COMPOSE_FILE}"
        warn "to upgrade with a fresh offline bundle: zeroone update -b BUNDLE_DIR"
        warn "to wipe and reinstall: zeroone uninstall --purge && bash install-offline.sh"
        exit 0
    fi

    find_bundle "${bundle_dir}"
    log "using bundle: ${BUNDLE_DIR}"

    ensure_docker

    log "creating directories"
    mkdir -p "${INSTALL_DIR}" "${DATA_DIR}/logs" "${DATA_DIR}/snapshots"

    log "installing docker-compose.yml + .env"
    cp "${BUNDLE_DIR}/docker-compose.yml" "${COMPOSE_FILE}"
    if [ -f "${ENV_FILE}" ] && [ "${force}" -ne 1 ]; then
        warn "${ENV_FILE} already exists; leaving it as-is (pass --force to overwrite)"
    else
        cp "${BUNDLE_DIR}/.env.example" "${ENV_FILE}"
    fi

    load_image "${BUNDLE_IMAGE_TAR}"
    pin_image_from_tar "${BUNDLE_IMAGE_TAR}"

    prompt_admin

    log "self-installing CLI to ${CLI_PATH}"
    install -m 0755 "$0" "${CLI_PATH}"

    log "starting container"
    dc up -d --pull never

    wait_for_health 90 || true

    log "creating admin account"
    if ! docker exec zeroone zeroone admin add \
            -config /var/lib/zeroone/stack.json \
            -username "${ADMIN_USER}" \
            -password "${ADMIN_PASS}" >/dev/null; then
        warn "failed to add admin via 'zeroone admin add' — try manually:"
        warn "  zeroone cli admin add -config /var/lib/zeroone/stack.json -username ${ADMIN_USER} -password ..."
    else
        ok "admin '${ADMIN_USER}' created"
    fi

    print_summary
}

cmd_up()      { dc up -d --pull never; }
cmd_down()    { dc down; }
cmd_restart() { dc restart zeroone; }

cmd_status() {
    dc ps
    printf '\n'
    local port
    port=$(grep -E '^ZEROONE_ADMIN_LISTEN_PORT=' "${ENV_FILE}" 2>/dev/null | tail -1 | cut -d= -f2 || true)
    port=${port:-8000}
    if curl -fsS "http://127.0.0.1:${port}/api/health" >/dev/null 2>&1; then
        ok "healthcheck: panel responding on :${port}"
    else
        warn "healthcheck: panel NOT responding on :${port}"
    fi
}

cmd_logs() {
    if [ $# -eq 0 ]; then
        dc logs -f --tail=200 zeroone
    else
        dc logs "$@"
    fi
}

cmd_update() {
    require_root
    [ -f "${COMPOSE_FILE}" ] || die "not installed (no ${COMPOSE_FILE}). Run install-offline.sh first."

    local bundle_dir=""
    while [ $# -gt 0 ]; do
        case "$1" in
            -b|--bundle) bundle_dir=$2; shift 2 ;;
            *) die "unknown flag for update: $1. Offline update requires -b BUNDLE_DIR." ;;
        esac
    done
    [ -n "${bundle_dir}" ] || die "offline update requires a fresh bundle: zeroone update -b /path/to/extracted/bundle"

    find_bundle "${bundle_dir}"
    log "using bundle: ${BUNDLE_DIR}"

    log "refreshing docker-compose.yml from bundle"
    cp "${BUNDLE_DIR}/docker-compose.yml" "${COMPOSE_FILE}"

    log "self-updating CLI"
    install -m 0755 "${BUNDLE_DIR}/install-offline.sh" "${CLI_PATH}"

    load_image "${BUNDLE_IMAGE_TAR}"
    pin_image_from_tar "${BUNDLE_IMAGE_TAR}"

    log "restarting container"
    dc up -d --pull never
    docker image prune -f >/dev/null 2>&1 || true
    ok "updated"
}

cmd_uninstall() {
    require_root
    local purge=0
    for a in "$@"; do
        case "$a" in
            --purge) purge=1 ;;
            *) die "unknown flag for uninstall: $a" ;;
        esac
    done
    if [ -f "${COMPOSE_FILE}" ]; then
        log "stopping container"
        dc down || true
    fi
    if [ "${purge}" -eq 1 ]; then
        printf '%bWARNING%b this will delete %s and %s.\n' "${C_RED}" "${C_RESET}" "${INSTALL_DIR}" "${DATA_DIR}"
        printf 'Type yes to confirm: '
        read -r confirm
        if [ "${confirm}" = "yes" ]; then
            rm -rf "${INSTALL_DIR}" "${DATA_DIR}"
            rm -f "${CLI_PATH}"
            ok "purged"
        else
            warn "aborted"
        fi
    else
        ok "stopped; state preserved at ${DATA_DIR}. Re-run 'zeroone up' to start again, or 'zeroone uninstall --purge' to wipe."
    fi
}

cmd_cli() {
    if [ $# -eq 0 ]; then
        docker exec -it zeroone /usr/local/bin/zeroone --help 2>&1 || true
        return
    fi
    if [ -t 0 ]; then
        docker exec -it zeroone /usr/local/bin/zeroone "$@"
    else
        docker exec -i zeroone /usr/local/bin/zeroone "$@"
    fi
}

cmd_edit() {
    require_root
    [ -f "${STACK_FILE}" ] || die "no ${STACK_FILE}"
    "${EDITOR:-vi}" "${STACK_FILE}"
    cmd_restart
}

cmd_edit_env() {
    require_root
    [ -f "${ENV_FILE}" ] || die "no ${ENV_FILE}"
    "${EDITOR:-vi}" "${ENV_FILE}"
    cmd_restart
}

cmd_backup() {
    require_root
    local out=""
    while [ $# -gt 0 ]; do
        case "$1" in
            -o|--output) out=$2; shift 2 ;;
            *) die "unknown flag: $1" ;;
        esac
    done
    out=${out:-"/root/zeroone-backup-$(date +%Y%m%d-%H%M%S).tgz"}
    log "creating backup ${out}"
    tar -czf "${out}" -C / \
        "${DATA_DIR#/}" \
        "${INSTALL_DIR#/}/.env" \
        "${INSTALL_DIR#/}/docker-compose.yml"
    ok "backup written: ${out}"
}

cmd_restore() {
    require_root
    local in=""
    while [ $# -gt 0 ]; do
        case "$1" in
            -i|--input) in=$2; shift 2 ;;
            *) die "unknown flag: $1" ;;
        esac
    done
    [ -n "${in}" ] || die "usage: zeroone restore -i FILE.tgz"
    [ -f "${in}" ] || die "file not found: ${in}"
    log "stopping container"
    dc down || true
    log "extracting ${in}"
    tar -xzf "${in}" -C /
    log "starting container"
    dc up -d --pull never
    ok "restored"
}

cmd_version() {
    printf 'zeroone offline CLI version: %s\n' "${SCRIPT_VERSION}"
    if [ -f "${ENV_FILE}" ]; then
        local v img
        v=$(grep -E '^ZEROONE_VERSION=' "${ENV_FILE}" | tail -1 | cut -d= -f2 || true)
        img=$(grep -E '^ZEROONE_IMAGE=' "${ENV_FILE}" | tail -1 | cut -d= -f2 || true)
        printf 'image:                       %s:%s\n' "${img:-ghcr.io/amirrezakm/zeroone}" "${v:-latest}"
    fi
    docker exec zeroone zeroone --help 2>&1 | head -1 || true
}

cmd_help() {
    cat <<'EOF'
zeroone (offline) — air-gapped control panel installer for Xray-core

usage: zeroone <subcommand> [args...]

install subcommands (run from the extracted bundle directory):
  install [-b DIR] [-f]         load image, write compose+env, start container
  update  -b DIR                load a new image tar, restart container

operational subcommands (after install):
  up                            start container
  down                          stop container
  restart                       restart container
  status                        show container + panel health
  logs [-f|...]                 follow daemon logs (default: -f --tail=200)
  uninstall [--purge]           stop; --purge also deletes state
  cli ARGS...                   run zeroone inside the container
  edit                          $EDITOR /var/lib/zeroone/stack.json; restart
  edit-env                      $EDITOR /opt/zeroone/.env; restart
  backup [-o FILE]              tar state + compose + .env
  restore -i FILE               untar over state + restart
  version                       print versions
  help                          this text

environment overrides:
  ZEROONE_INSTALL_DIR           (default /opt/zeroone)
  ZEROONE_DATA_DIR              (default /var/lib/zeroone)
  ZEROONE_LINUX_MIRROR          apt mirror used if docker is missing on
                                Debian/Ubuntu (default
                                http://mirror-linux.runflare.com)
  ZEROONE_KEEP_MIRROR=1         keep the Runflare apt mirror in
                                /etc/apt/sources.list after installing
                                docker (default: restore the original)
  ZEROONE_SKIP_DOCKER_INSTALL=1 refuse to touch sources.list / install
                                docker; fail loudly instead
  ZEROONE_ADMIN_USERNAME, ZEROONE_ADMIN_PASSWORD
                                skip the interactive admin prompt
EOF
}

# ----- dispatch ---------------------------------------------------------------

main() {
    local sub=${1:-install}
    if [ $# -gt 0 ]; then shift; fi
    case "${sub}" in
        install)   cmd_install   "$@" ;;
        up)        cmd_up        "$@" ;;
        down)      cmd_down      "$@" ;;
        restart)   cmd_restart   "$@" ;;
        status)    cmd_status    "$@" ;;
        logs)      cmd_logs      "$@" ;;
        update)    cmd_update    "$@" ;;
        uninstall) cmd_uninstall "$@" ;;
        cli)       cmd_cli       "$@" ;;
        edit)      cmd_edit      "$@" ;;
        edit-env)  cmd_edit_env  "$@" ;;
        backup)    cmd_backup    "$@" ;;
        restore)   cmd_restore   "$@" ;;
        version)   cmd_version   "$@" ;;
        help|-h|--help) cmd_help "$@" ;;
        *) die "unknown subcommand: ${sub}. Try: zeroone help" ;;
    esac
}

main "$@"
