# ── Stage 1: Build the UI ──────────────────────────────────────────
FROM node:20-slim AS ui-builder

WORKDIR /build/ui

# Private @redislabsdev packages require a GitHub Packages token.
ARG NPM_AUTH_TOKEN
ENV NPM_AUTH_TOKEN=${NPM_AUTH_TOKEN}

COPY ui/package.json ui/package-lock.json ui/.npmrc ui/check-private-registry.mjs ./
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

# ── Stage 3: Runtime ───────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=go-builder /out/afs-control-plane /usr/local/bin/
COPY --from=go-builder /out/afs /usr/local/bin/

EXPOSE 8091

ENTRYPOINT ["afs-control-plane"]
CMD ["--listen", "0.0.0.0:8091"]
