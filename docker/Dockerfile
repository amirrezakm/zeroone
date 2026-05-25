# syntax=docker/dockerfile:1.7
#
# Production image for the zeroone control panel.
#
# Build args:
#   VERSION       — version stamp injected into the binary (e.g. v0.1.0).
#   XRAY_VERSION  — Xray-core release tag bundled into the image.
#
# Multi-arch: amd64 and arm64. The xray-core download URL switches on
# TARGETARCH (set automatically by `docker buildx`).

ARG XRAY_VERSION=v26.3.27

# ---------- 1. UI build ----------
# Arch-independent JS — always build natively to avoid QEMU emulation
# (npm install + vite build under arm64 QEMU on an amd64 runner takes
# tens of minutes and routinely makes the release job look hung).
FROM --platform=$BUILDPLATFORM node:20-alpine AS ui
WORKDIR /src/web/app
COPY web/app/package.json web/app/package-lock.json* ./
RUN if [ -f package-lock.json ]; then \
        npm ci --no-audit --no-fund; \
    else \
        npm install --no-audit --no-fund; \
    fi
COPY web/app/ ./
RUN npm run build

# ---------- 2. Go daemon ----------
# Build on $BUILDPLATFORM and cross-compile to $TARGETARCH; Go's
# cross-compiler is native, so this stays fast for both amd64 and arm64.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS daemon
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY cmd ./cmd
COPY internal ./internal
COPY config ./config
ARG VERSION=dev
ARG TARGETARCH
ENV CGO_ENABLED=0 GOOS=linux
RUN GOARCH=${TARGETARCH} go build \
        -mod=vendor -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/zeroone ./cmd/zeroone

# ---------- 3. Xray binary ----------
# Just curls a prebuilt binary keyed off TARGETARCH — no need to emulate
# the target architecture for that.
FROM --platform=$BUILDPLATFORM alpine:3.23 AS xray
ARG XRAY_VERSION
ARG TARGETARCH
RUN apk add --no-cache curl unzip ca-certificates && \
    case "${TARGETARCH}" in \
      amd64) X="Xray-linux-64.zip" ;; \
      arm64) X="Xray-linux-arm64-v8a.zip" ;; \
      *)     echo "unsupported arch: ${TARGETARCH}"; exit 1 ;; \
    esac && \
    curl -fsSL -o /tmp/xray.zip \
      "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/${X}" && \
    mkdir -p /opt/xray && \
    unzip /tmp/xray.zip -d /opt/xray && \
    chmod +x /opt/xray/xray && \
    rm /tmp/xray.zip

# ---------- 3b. byedpi (SNI-spoof desync SOCKS5 proxy) ----------
# Prebuilt release tarball; the binary inside is named `ciadpi`. The file
# version (17.3) differs from the release tag (v0.17.3).
FROM --platform=$BUILDPLATFORM alpine:3.23 AS byedpi
ARG BYEDPI_VERSION=v0.17.3
ARG BYEDPI_FILE_VERSION=17.3
ARG TARGETARCH
RUN apk add --no-cache curl tar ca-certificates && \
    case "${TARGETARCH}" in \
      amd64) B="byedpi-${BYEDPI_FILE_VERSION}-x86_64.tar.gz" ;; \
      arm64) B="byedpi-${BYEDPI_FILE_VERSION}-aarch64.tar.gz" ;; \
      *)     echo "unsupported arch: ${TARGETARCH}"; exit 1 ;; \
    esac && \
    curl -fsSL -o /tmp/byedpi.tgz \
      "https://github.com/hufrea/byedpi/releases/download/${BYEDPI_VERSION}/${B}" && \
    mkdir -p /opt/byedpi && tar -xzf /tmp/byedpi.tgz -C /opt/byedpi && \
    bin="$(find /opt/byedpi -type f -name 'ciadpi*' | head -1)" && \
    cp "$bin" /opt/byedpi/byedpi && chmod +x /opt/byedpi/byedpi && rm /tmp/byedpi.tgz

# ---------- 3c. tun2socks (real tun device backed by byedpi's SOCKS5) ----------
FROM --platform=$BUILDPLATFORM alpine:3.23 AS tun2socks
ARG TUN2SOCKS_VERSION=v2.6.0
ARG TARGETARCH
RUN apk add --no-cache curl unzip ca-certificates && \
    case "${TARGETARCH}" in \
      amd64) T="tun2socks-linux-amd64.zip" ;; \
      arm64) T="tun2socks-linux-arm64.zip" ;; \
      *)     echo "unsupported arch: ${TARGETARCH}"; exit 1 ;; \
    esac && \
    curl -fsSL -o /tmp/t2s.zip \
      "https://github.com/xjasonlyu/tun2socks/releases/download/${TUN2SOCKS_VERSION}/${T}" && \
    mkdir -p /opt/tun2socks && unzip /tmp/t2s.zip -d /opt/tun2socks && \
    bin="$(find /opt/tun2socks -type f -name 'tun2socks*' | head -1)" && \
    cp "$bin" /opt/tun2socks/tun2socks && chmod +x /opt/tun2socks/tun2socks && rm /tmp/t2s.zip

# ---------- 4. Runtime ----------
FROM alpine:3.23
ARG VERSION=dev
ARG XRAY_VERSION
LABEL org.opencontainers.image.source="https://github.com/amirrezakm/zeroone" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later" \
      org.opencontainers.image.title="zeroone" \
      org.opencontainers.image.description="Go control panel for Xray-core" \
      org.opencontainers.image.version="${VERSION}" \
      org.zeroone.xray-version="${XRAY_VERSION}"

RUN apk add --no-cache \
        iproute2 \
        ca-certificates \
        tzdata \
        curl \
        tini \
        libcap \
    && addgroup -S -g 65532 zeroone \
    && adduser -S -D -H -u 65532 -G zeroone zeroone

COPY --from=daemon    /out/zeroone             /usr/local/bin/zeroone
COPY --from=xray      /opt/xray/xray           /usr/local/bin/xray
COPY --from=xray      /opt/xray/geoip.dat      /usr/local/share/xray/geoip.dat
COPY --from=xray      /opt/xray/geosite.dat    /usr/local/share/xray/geosite.dat
COPY --from=byedpi    /opt/byedpi/byedpi       /usr/local/bin/byedpi
COPY --from=tun2socks /opt/tun2socks/tun2socks /usr/local/bin/tun2socks
COPY --from=ui        /src/web/app/dist        /app/ui
COPY config/stack.example.json                 /app/stack.example.json

# Run unprivileged (see USER below). Two things the non-root user still needs:
#   1. Xray binds inbound ports — often privileged ones like 443 under the
#      host-network compose. Grant the bundled binary the NET_BIND_SERVICE
#      file capability; it is part of Docker's default capability set, so no
#      --privileged or extra `docker run` flag is required for the common case.
#   2. The state dir and /app must be writable by uid 65532.
# Caveats for operators: an xray binary later installed via the panel (override
# tree under /var/lib/zeroone/xray) will NOT carry this file cap, and a *bind-
# mounted* state dir must be chowned to 65532 on the host (named/anonymous
# volumes inherit the ownership set here). Host-level features
# (-manage-failover/-manage-vpn/bandwidth shaping, and -manage-snispoof which
# needs CAP_NET_ADMIN + /dev/net/tun for byedpi/tun2socks + policy routing)
# need their own capabilities/devices granted at runtime.
RUN setcap cap_net_bind_service=+ep /usr/local/bin/xray \
    && mkdir -p /var/lib/zeroone /app \
    && chown -R zeroone:zeroone /var/lib/zeroone /app

# Xray looks for geo files in the same dir as its binary by default; the
# /usr/local/share/xray location is exposed via XRAY_LOCATION_ASSET so
# both this container and a host install can find them.
ENV XRAY_LOCATION_ASSET=/usr/local/share/xray \
    ZEROONE_UI_PATH=/app/ui \
    ZEROONE_STATE_DIR=/var/lib/zeroone \
    ZEROONE_XRAY_BINARY=/usr/local/bin/xray \
    ZEROONE_XRAY_CONFIG_PATH=/var/lib/zeroone/xray.json \
    ZEROONE_XRAY_INSTALL_DIR=/var/lib/zeroone/xray \
    ZEROONE_XRAY_IMAGE_BINARY=/usr/local/bin/xray \
    ZEROONE_XRAY_IMAGE_ASSETS=/usr/local/share/xray \
    ZEROONE_XRAY_IMAGE_VERSION="${XRAY_VERSION}" \
    ZEROONE_AUTO_INIT=1

WORKDIR /var/lib/zeroone
VOLUME ["/var/lib/zeroone"]

# Default healthcheck pokes the admin API (set ZEROONE_ADMIN_LISTEN_PORT
# to match docker-compose if you change the listen port).
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=15s \
    CMD curl -fsS "http://127.0.0.1:${ZEROONE_ADMIN_LISTEN_PORT:-8000}/api/health" || exit 1

EXPOSE 8000
# Drop to the unprivileged user for the running process. This removes the
# default root baseline (Trivy DS-0002); see the setcap/chown note above for
# the port-binding and writable-state-dir requirements this implies.
USER zeroone:zeroone
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/zeroone"]
CMD ["-config", "/var/lib/zeroone/stack.json", "-allow-apply", "-manage-xray"]
