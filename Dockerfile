# ── Stage 1: Build the UI ──────────────────────────────────────────
FROM node:20-slim AS ui-builder

WORKDIR /build/ui

COPY ui/package.json ui/package-lock.json ./
RUN npm ci

COPY ui/ ./
RUN npm run build

# ── Stage 2: Build Go binaries ─────────────────────────────────────
FROM golang:1.22-bookworm AS go-builder

WORKDIR /build

# Cache Go modules first.
COPY go.mod go.sum ./
COPY mount/go.mod mount/go.sum ./mount/
RUN go mod download

# Copy full source.
COPY . .

# Place compiled UI assets where go:embed expects them.
COPY --from=ui-builder /build/ui/dist ./internal/uistatic/dist

# Build both binaries.
RUN go build -o /out/afs-control-plane ./cmd/afs-control-plane && \
    go build -o /out/afs ./cmd/afs

# Build downloadable CLI binaries for host machines that may differ from the
# control plane runtime environment.
RUN mkdir -p /out/cli/darwin-amd64 /out/cli/darwin-arm64 /out/cli/linux-amd64 /out/cli/linux-arm64 && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o /out/cli/darwin-amd64/afs ./cmd/afs && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /out/cli/darwin-arm64/afs ./cmd/afs && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/cli/linux-amd64/afs ./cmd/afs && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/cli/linux-arm64/afs ./cmd/afs

# ── Stage 3: Runtime ───────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=go-builder /out/afs-control-plane /usr/local/bin/
COPY --from=go-builder /out/afs /usr/local/bin/
COPY --from=go-builder /out/cli /opt/afs-cli

EXPOSE 8091

ENV AFS_CLI_ARTIFACT_DIR=/opt/afs-cli

ENTRYPOINT ["afs-control-plane"]
CMD ["--listen", "0.0.0.0:8091"]
