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

ARG XRAY_VERSION=v25.1.30

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
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS daemon
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
        tini

COPY --from=daemon /out/zeroone          /usr/local/bin/zeroone
COPY --from=xray   /opt/xray/xray        /usr/local/bin/xray
COPY --from=xray   /opt/xray/geoip.dat   /usr/local/share/xray/geoip.dat
COPY --from=xray   /opt/xray/geosite.dat /usr/local/share/xray/geosite.dat
COPY --from=ui     /src/web/app/dist     /app/ui
COPY config/stack.example.json           /app/stack.example.json

# Xray looks for geo files in the same dir as its binary by default; the
# /usr/local/share/xray location is exposed via XRAY_LOCATION_ASSET so
# both this container and a host install can find them.
ENV XRAY_LOCATION_ASSET=/usr/local/share/xray \
    ZEROONE_UI_PATH=/app/ui \
    ZEROONE_STATE_DIR=/var/lib/zeroone \
    ZEROONE_XRAY_BINARY=/usr/local/bin/xray \
    ZEROONE_XRAY_CONFIG_PATH=/var/lib/zeroone/xray.json \
    ZEROONE_AUTO_INIT=1

WORKDIR /var/lib/zeroone
VOLUME ["/var/lib/zeroone"]

# Default healthcheck pokes the admin API (set ZEROONE_ADMIN_LISTEN_PORT
# to match docker-compose if you change the listen port).
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=15s \
    CMD curl -fsS "http://127.0.0.1:${ZEROONE_ADMIN_LISTEN_PORT:-8000}/api/health" || exit 1

EXPOSE 8000
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/zeroone"]
CMD ["-config", "/var/lib/zeroone/stack.json", "-allow-apply", "-manage-xray"]
