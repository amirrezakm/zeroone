#!/usr/bin/env bash
# zeroone installer + CLI wrapper.
#
# One-line install:
#   sudo bash -c "$(curl -sSL https://raw.githubusercontent.com/amirrezakm/zeroone/main/scripts/install.sh)" @ install
#
# After install, the script self-copies to /usr/local/bin/zeroone and is
# invoked as `zeroone <subcommand>`. See `zeroone help` for the full list.

set -Eeuo pipefail

# ----- configuration knobs ----------------------------------------------------

INSTALL_DIR=${ZEROONE_INSTALL_DIR:-/opt/zeroone}
DATA_DIR=${ZEROONE_DATA_DIR:-/var/lib/zeroone}
CLI_PATH=${ZEROONE_CLI_PATH:-/usr/local/bin/zeroone}
REPO_RAW=${ZEROONE_REPO_RAW:-https://raw.githubusercontent.com/amirrezakm/zeroone/${ZEROONE_REF:-main}}
IMAGE_DEFAULT=ghcr.io/amirrezakm/zeroone:${ZEROONE_VERSION:-latest}

COMPOSE_FILE="${INSTALL_DIR}/docker-compose.yml"
ENV_FILE="${INSTALL_DIR}/.env"
STACK_FILE="${DATA_DIR}/stack.json"

SCRIPT_VERSION="0.1.0"

# ----- helpers ----------------------------------------------------------------

if [ -t 1 ]; then
    C_RED='\033[1;31m'
    C_GREEN='\033[1;32m'
    C_YELLOW='\033[1;33m'
    C_BLUE='\033[1;34m'
    C_RESET='\033[0m'
else
    C_RED=''
    C_GREEN=''
    C_YELLOW=''
    C_BLUE=''
    C_RESET=''
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
    case "${OS_ID}" in
        debian|ubuntu) PKG=apt ;;
        centos|rhel|fedora|almalinux|rocky) PKG=dnf ;;
        alpine) PKG=apk ;;
        *)
            case "${OS_LIKE}" in
                *debian*) PKG=apt ;;
                *rhel*|*fedora*) PKG=dnf ;;
                *) die "unsupported OS: ${OS_ID} (${OS_LIKE:-no ID_LIKE})" ;;
            esac
            ;;
    esac
}

ensure_curl() {
    if command -v curl >/dev/null 2>&1; then return; fi
    log "installing curl"
    case "${PKG}" in
        apt) apt-get update -qq && apt-get install -y -qq curl ;;
        dnf) dnf install -y -q curl ;;
        apk) apk add --no-cache curl ;;
    esac
}

ensure_docker() {
    if command -v docker >/dev/null 2>&1; then
        ok "docker present: $(docker --version)"
    else
        log "installing docker via get.docker.com"
        curl -fsSL https://get.docker.com | sh
    fi
    # Compose v2 ships as a docker plugin since Docker 20.10.13+
    if ! docker compose version >/dev/null 2>&1; then
        die "docker compose plugin missing. On older systems run: apt-get install docker-compose-plugin"
    fi
    if ! systemctl is-active --quiet docker 2>/dev/null; then
        systemctl enable --now docker >/dev/null 2>&1 || \
            service docker start >/dev/null 2>&1 || \
            warn "could not start docker; continuing anyway"
    fi
}

fetch() {
    # fetch URL → dest, creating parent dir.
    local url="$1" dest="$2"
    mkdir -p "$(dirname "${dest}")"
    curl -fsSL "${url}" -o "${dest}"
}

dc() {
    # docker compose wrapper bound to our install dir.
    docker compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" "$@"
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
    ok "zeroone is installed."
    printf '   panel:     http://%s:%s/\n' "$(curl -fsS -m 3 https://ifconfig.me 2>/dev/null || echo 'YOUR_HOST_IP')" "${port}"
    printf '   listen:    %s\n' "${host:-0.0.0.0:${port}}"
    printf '   compose:   %s\n' "${COMPOSE_FILE}"
    printf '   state:     %s\n' "${DATA_DIR}"
    printf '   cli:       %s\n' "${CLI_PATH}"
    printf '\nTry: zeroone status\n\n'
}

prompt_admin() {
    if [ -n "${ZEROONE_ADMIN_USERNAME:-}" ] && [ -n "${ZEROONE_ADMIN_PASSWORD:-}" ]; then
        ADMIN_USER=${ZEROONE_ADMIN_USERNAME}
        ADMIN_PASS=${ZEROONE_ADMIN_PASSWORD}
        return
    fi
    if [ ! -t 0 ]; then
        die "interactive admin prompt requires a TTY. Either rerun with stdin attached, or export ZEROONE_ADMIN_USERNAME / ZEROONE_ADMIN_PASSWORD before installing. (You can also skip and add an admin later: zeroone cli admin add ...)"
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

# ----- subcommands ------------------------------------------------------------

cmd_install() {
    require_root
    detect_arch
    detect_os
    ensure_curl

    if [ -f "${COMPOSE_FILE}" ]; then
        warn "already installed at ${COMPOSE_FILE}"
        warn "to upgrade an existing install, run: zeroone update"
        warn "to wipe and reinstall, run: zeroone uninstall --purge && zeroone install"
        exit 0
    fi

    ensure_docker

    log "creating directories"
    mkdir -p "${INSTALL_DIR}" "${DATA_DIR}/logs" "${DATA_DIR}/snapshots"

    log "fetching docker-compose.yml + .env.example"
    fetch "${REPO_RAW}/docker/docker-compose.yml" "${COMPOSE_FILE}"
    fetch "${REPO_RAW}/docker/.env.example"       "${ENV_FILE}"

    # stack.json gets auto-created by the daemon (ZEROONE_AUTO_INIT=1)
    # on first start.

    prompt_admin

    log "self-installing CLI to ${CLI_PATH}"
    install -m 0755 "$0" "${CLI_PATH}"

    log "pulling image ${IMAGE_DEFAULT}"
    dc pull

    log "starting container"
    dc up -d

    wait_for_health 90 || true

    log "creating admin account"
    if ! docker exec zeroone xray-stackd admin add \
            -config /var/lib/zeroone/stack.json \
            -username "${ADMIN_USER}" \
            -password "${ADMIN_PASS}" >/dev/null; then
        warn "failed to add admin via 'xray-stackd admin add' — try manually:"
        warn "  zeroone cli admin add -config /var/lib/zeroone/stack.json -username ${ADMIN_USER} -password ..."
    else
        ok "admin '${ADMIN_USER}' created"
    fi

    print_summary
}

cmd_up()      { dc up -d; }
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
    [ -f "${COMPOSE_FILE}" ] || die "not installed (no ${COMPOSE_FILE}). Run: zeroone install"
    log "fetching latest docker-compose.yml"
    fetch "${REPO_RAW}/docker/docker-compose.yml" "${COMPOSE_FILE}.new"
    mv "${COMPOSE_FILE}.new" "${COMPOSE_FILE}"
    log "self-updating CLI"
    fetch "${REPO_RAW}/scripts/install.sh" "${CLI_PATH}.new"
    chmod +x "${CLI_PATH}.new"
    mv "${CLI_PATH}.new" "${CLI_PATH}"
    log "pulling image"
    dc pull
    log "restarting container"
    dc up -d
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
        docker exec -it zeroone /usr/local/bin/xray-stackd --help 2>&1 || true
        return
    fi
    if [ -t 0 ]; then
        docker exec -it zeroone /usr/local/bin/xray-stackd "$@"
    else
        docker exec -i zeroone /usr/local/bin/xray-stackd "$@"
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
    dc up -d
    ok "restored"
}

cmd_version() {
    printf 'zeroone CLI version: %s\n' "${SCRIPT_VERSION}"
    if [ -f "${ENV_FILE}" ]; then
        local v
        v=$(grep -E '^ZEROONE_VERSION=' "${ENV_FILE}" | tail -1 | cut -d= -f2 || true)
        printf 'image:               ghcr.io/amirrezakm/zeroone:%s\n' "${v:-latest}"
    fi
    docker exec zeroone xray-stackd --help 2>&1 | head -1 || true
}

cmd_help() {
    cat <<'EOF'
zeroone — control panel for Xray-core

usage: zeroone <subcommand> [args...]

subcommands:
  install                       install zeroone (interactive admin prompt)
  up                            start container
  down                          stop container
  restart                       restart container
  status                        show container + panel health
  logs [-f|...]                 follow daemon logs (default: -f --tail=200)
  update                        pull newest compose + image, restart
  uninstall [--purge]           stop; --purge also deletes state
  cli ARGS...                   run xray-stackd inside the container
                                (e.g. zeroone cli admin add -config /var/lib/zeroone/stack.json
                                                            -username U -password P)
  edit                          $EDITOR /var/lib/zeroone/stack.json; restart
  edit-env                      $EDITOR /opt/zeroone/.env; restart
  backup [-o FILE]              tar state + compose + .env
  restore -i FILE               untar over state + restart
  version                       print versions
  help                          this text

environment overrides (rarely needed):
  ZEROONE_INSTALL_DIR  (default /opt/zeroone)
  ZEROONE_DATA_DIR     (default /var/lib/zeroone)
  ZEROONE_REF          (default main)
  ZEROONE_VERSION      (default latest)
  ZEROONE_ADMIN_USERNAME / ZEROONE_ADMIN_PASSWORD  (skip interactive prompt)
EOF
}

# ----- dispatch ---------------------------------------------------------------

main() {
    local sub=${1:-help}
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
