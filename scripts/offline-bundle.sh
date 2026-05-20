#!/usr/bin/env bash
# offline-bundle.sh — produce a self-contained tarball for air-gapped
# zeroone installs, intended for Iranian operators whose destination
# servers cannot reach GHCR / Docker Hub / GitHub raw but can be
# SFTP'd to.
#
# Run this on any host with Docker and outbound access (or access to
# https://mirror-docker.runflare.com). Output:
#
#   dist/zeroone-offline-<version>-<arch>.tar.gz
#
# Transfer that single file to the destination, extract it, and run
# scripts/install-offline.sh on the destination. See
# docs/OFFLINE-INSTALL.md for the full flow.

set -Eeuo pipefail

# ----- configuration knobs ----------------------------------------------------

VERSION=${ZEROONE_VERSION:-latest}
REPO=${ZEROONE_REPO:-amirrezakm/zeroone}
ARCH=${ZEROONE_ARCH:-amd64}

# Source registry to pull from. Defaults to Runflare's Docker mirror,
# which proxies ghcr.io transparently. Override to pull from a private
# registry, a local mirror, or directly from ghcr.io when you have
# access.
IMAGE_SRC=${ZEROONE_IMAGE_SRC:-mirror-docker.runflare.com/${REPO}}

# Destination tag — what the bundled docker-compose.yml expects. Keep
# in sync with the default in docker/docker-compose.yml.
IMAGE_DST=${ZEROONE_IMAGE_DST:-ghcr.io/${REPO}}

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT_DIR=${OUT_DIR:-${REPO_ROOT}/dist}
STAGE_DIR="${OUT_DIR}/offline-stage-${VERSION}-${ARCH}"
BUNDLE_NAME="zeroone-offline-${VERSION}-${ARCH}.tar.gz"
BUNDLE_PATH="${OUT_DIR}/${BUNDLE_NAME}"

# ----- helpers ----------------------------------------------------------------

if [ -t 1 ]; then
    C_GREEN='\033[1;32m'
    C_YELLOW='\033[1;33m'
    C_BLUE='\033[1;34m'
    C_RED='\033[1;31m'
    C_RESET='\033[0m'
else
    C_GREEN=''; C_YELLOW=''; C_BLUE=''; C_RED=''; C_RESET=''
fi

log()  { printf '%b==>%b %s\n' "${C_BLUE}" "${C_RESET}" "$*"; }
ok()   { printf '%b ok%b %s\n' "${C_GREEN}" "${C_RESET}" "$*"; }
warn() { printf '%bwarn%b %s\n' "${C_YELLOW}" "${C_RESET}" "$*" >&2; }
die()  { printf '%berr%b %s\n' "${C_RED}" "${C_RESET}" "$*" >&2; exit 1; }

trap 'die "command failed at line $LINENO"' ERR

require() {
    command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1"
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

# ----- main -------------------------------------------------------------------

require docker
require tar

case "${ARCH}" in
    amd64|arm64) : ;;
    *) die "unsupported ARCH: ${ARCH} (expected amd64 or arm64)" ;;
esac

log "building offline bundle"
log "  source image: ${IMAGE_SRC}:${VERSION}"
log "  target tag:   ${IMAGE_DST}:${VERSION}"
log "  platform:     linux/${ARCH}"
log "  output:       ${BUNDLE_PATH}"

rm -rf "${STAGE_DIR}"
mkdir -p "${STAGE_DIR}"

log "pulling image"
docker pull --platform "linux/${ARCH}" "${IMAGE_SRC}:${VERSION}"

if [ "${IMAGE_SRC}" != "${IMAGE_DST}" ]; then
    log "retagging ${IMAGE_SRC}:${VERSION} → ${IMAGE_DST}:${VERSION}"
    docker tag "${IMAGE_SRC}:${VERSION}" "${IMAGE_DST}:${VERSION}"
fi

IMAGE_TAR="${STAGE_DIR}/zeroone-image-${VERSION}-${ARCH}.tar"
log "saving image to ${IMAGE_TAR}"
docker save -o "${IMAGE_TAR}" "${IMAGE_DST}:${VERSION}"

log "copying compose + env + installer"
cp "${REPO_ROOT}/docker/docker-compose.yml" "${STAGE_DIR}/docker-compose.yml"
cp "${REPO_ROOT}/docker/.env.example"       "${STAGE_DIR}/.env.example"
cp "${REPO_ROOT}/scripts/install-offline.sh" "${STAGE_DIR}/install-offline.sh"
chmod 0755 "${STAGE_DIR}/install-offline.sh"

# Capture checksums so the destination can verify nothing was mangled
# in transit. Written into BUNDLE-README.txt next to the artifacts.
IMG_SHA=$(sha256_of "${IMAGE_TAR}")
CMP_SHA=$(sha256_of "${STAGE_DIR}/docker-compose.yml")
ENV_SHA=$(sha256_of "${STAGE_DIR}/.env.example")
INS_SHA=$(sha256_of "${STAGE_DIR}/install-offline.sh")

cat > "${STAGE_DIR}/BUNDLE-README.txt" <<EOF
zeroone offline bundle
======================

version:   ${VERSION}
image tag: ${IMAGE_DST}:${VERSION}
arch:      linux/${ARCH}
built at:  $(date -u +%Y-%m-%dT%H:%M:%SZ)

Contents (SHA256):
  ${IMG_SHA}  $(basename "${IMAGE_TAR}")
  ${CMP_SHA}  docker-compose.yml
  ${ENV_SHA}  .env.example
  ${INS_SHA}  install-offline.sh

Install on the destination:

    tar -xzf ${BUNDLE_NAME}
    sudo bash install-offline.sh

See docs/OFFLINE-INSTALL.md in the zeroone repository for the full
guide, troubleshooting, and the upgrade flow.
EOF

log "packing tarball"
tar -czf "${BUNDLE_PATH}" -C "${STAGE_DIR}" .

BUNDLE_SHA=$(sha256_of "${BUNDLE_PATH}")
BUNDLE_SIZE=$(du -h "${BUNDLE_PATH}" | awk '{print $1}')

# Sidecar checksum file, matching the convention used by the daemon
# tarballs in .github/workflows/release.yml.
printf '%s  %s\n' "${BUNDLE_SHA}" "${BUNDLE_NAME}" > "${BUNDLE_PATH}.sha256"

rm -rf "${STAGE_DIR}"

ok "bundle ready"
printf '   path:    %s\n' "${BUNDLE_PATH}"
printf '   size:    %s\n' "${BUNDLE_SIZE}"
printf '   sha256:  %s\n' "${BUNDLE_SHA}"
printf '\n'
printf 'Transfer to the destination, then run:\n'
printf '   tar -xzf %s && sudo bash install-offline.sh\n' "${BUNDLE_NAME}"
