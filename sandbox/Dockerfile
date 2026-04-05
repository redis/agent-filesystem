# Redis-FS Sandbox - Code execution environment with FUSE-mounted Redis FS
# Requires --privileged or --cap-add SYS_ADMIN --device /dev/fuse to run

FROM golang:1.21-bookworm AS builder

# Build redis-fs-mount
WORKDIR /build/mount
COPY mount/go.mod mount/go.sum ./
RUN go mod download
COPY mount/ ./
RUN CGO_ENABLED=0 go build -o /redis-fs-mount ./cmd/redis-fs-mount

# Build sandbox server
WORKDIR /build/sandbox
COPY sandbox/go.mod sandbox/go.sum ./
RUN go mod download
COPY sandbox/ ./
RUN CGO_ENABLED=0 go build -o /sandbox ./cmd/sandbox
RUN CGO_ENABLED=0 go build -o /sandbox-cli ./cmd/sandbox-cli

# Runtime image
FROM debian:bookworm-slim

# Install FUSE3 and common dev tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    fuse3 \
    ca-certificates \
    curl \
    git \
    python3 \
    python3-pip \
    nodejs \
    npm \
    bash \
    jq \
    procps \
    && rm -rf /var/lib/apt/lists/*

# Allow non-root FUSE mounts
RUN echo "user_allow_other" >> /etc/fuse.conf

# Copy binaries
COPY --from=builder /redis-fs-mount /usr/local/bin/
COPY --from=builder /sandbox /usr/local/bin/
COPY --from=builder /sandbox-cli /usr/local/bin/

# Create workspace directory for the FUSE mount
RUN mkdir -p /workspace

# Environment
ENV REDIS_ADDR=redis:6379
ENV REDIS_KEY=sandbox
ENV MOUNT_POINT=/workspace
ENV SANDBOX_PORT=8090

EXPOSE 8090

# Entry point mounts the FS then runs the sandbox server
COPY sandbox/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]

