# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
# vendor/ is committed so this builds offline.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -mod=vendor -o /out/edge-relay ./cmd/edge-relay

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/edge-relay /edge-relay
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/edge-relay"]
# Provider-agnostic image: listens on $PORT (default 8080). Non-privileged
# user can't bind <1024 — runs as nonroot for safer defaults. The runflare
# variant at /Dockerfile (repo root) overrides to root + port 80 because
# runflare's Docker item is hardcoded to expose container port 80.
