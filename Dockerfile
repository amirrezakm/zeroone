# Runflare deployment Dockerfile for the edge-relay.
# This lives at the repo root because runflare requires `Dockerfile` at the
# project root. The provider-agnostic version is at cmd/edge-relay/Dockerfile;
# the only difference here is that ORIGIN_HOST and friends are baked in so
# the runflare dashboard env vars don't need to be configured.

# syntax=docker/dockerfile:1.7

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
# vendor/ is committed so the build needs no network. Go ≥1.14 auto-uses
# `-mod=vendor` when vendor/ is present. This matters for runflare's
# Iranian build host, which can't reach proxy.golang.org.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -mod=vendor -o /out/edge-relay ./cmd/edge-relay

# scratch = empty image, built-in to Docker. No network fetch, no distro,
# no shell. The Go binary is fully static (CGO_ENABLED=0) and makes only
# HTTP (not HTTPS) calls to origin, so it doesn't need ca-certificates or
# anything else from a base image. Runflare's builder previously failed
# to fetch gcr.io/distroless/static-debian12:nonroot when the cache
# expired — scratch sidesteps that entirely.
FROM scratch
COPY --from=build /out/edge-relay /edge-relay

# Defaults baked in for this stack. Runflare dashboard env vars (if set)
# still override these at runtime — same precedence as any Docker host.
ENV ORIGIN_HOST=185.128.139.68 \
    WS_PATH=/vless \
    XHTTP_PATH=/api/v1/events \
    LOG_LEVEL=info \
    LANDING_BODY="OK" \
    PORT=80
# Don't pin GOMAXPROCS. Go ≥1.25 auto-detects the cgroup CPU limit (runflare
# gives us 0.75 cores → Go uses 1 P with extra threads available for I/O).
# Pinning to 1 serializes all parallel xhttp substreams and kills throughput
# under multi-stream xmux load.

# Runflare's Docker item exposes container port 80 by default. The binary
# reads PORT and listens on :$PORT. scratch has no /etc/passwd, so the
# process runs as UID 0 implicitly — no USER directive needed.
EXPOSE 80
ENTRYPOINT ["/edge-relay"]
