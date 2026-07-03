# Multi-arch build: the builder runs natively on the host platform and
# cross-compiles the static Go binary for the requested target (no emulation),
# so `docker buildx --platform linux/amd64,linux/arm64` is fast and reliable.
FROM --platform=$BUILDPLATFORM golang:1.26 AS builder

# Provided by buildx (target platform) and the CI workflow (release version).
ARG TARGETOS
ARG TARGETARCH
ARG TAG=dev

WORKDIR /src
COPY . .

# GOWORK=off ignores the committed local-dev go.work (its sibling paths are not
# present in the build context); CGO is off so the binary is fully static.
ENV GOWORK=off CGO_ENABLED=0
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags "-s -w -X main.Version=${TAG} -X main.Built=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      -o /consensusdb .

FROM ubuntu:24.04
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /consensusdb /app/consensusdb

# 8441 http (health, metrics, welcome). The value-rpc data plane binds
# vrpc-server.bind-address (e.g. 8444) — publish it when enabled.
EXPOSE 8441 8444

ENTRYPOINT ["/app/consensusdb"]
CMD ["run"]
