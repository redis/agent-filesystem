#!/bin/bash
set -e

echo "=== Redis-FS Sandbox Starting ==="
echo "Redis: ${REDIS_ADDR}"
echo "Key: ${REDIS_KEY}"
echo "Mount: ${MOUNT_POINT}"

# Parse host and port from REDIS_ADDR
REDIS_HOST=$(echo "${REDIS_ADDR}" | cut -d: -f1)
REDIS_PORT=$(echo "${REDIS_ADDR}" | cut -d: -f2)

# Wait for Redis to be available using simple TCP check
echo "Waiting for Redis at ${REDIS_HOST}:${REDIS_PORT}..."
for i in {1..30}; do
    if timeout 2 bash -c "</dev/tcp/${REDIS_HOST}/${REDIS_PORT}" 2>/dev/null; then
        echo "Redis is available!"
        break
    fi
    echo "Attempt $i: Redis not ready, waiting..."
    sleep 2
done

# Mount the Redis FS in background
echo "Mounting Redis FS key '${REDIS_KEY}' at ${MOUNT_POINT}..."
redis-fs-mount --redis "${REDIS_ADDR}" --allow-other "${REDIS_KEY}" "${MOUNT_POINT}" &
MOUNT_PID=$!

# Wait for mount to be ready - check /proc/mounts for FUSE
echo "Waiting for mount to be ready..."
MOUNT_READY=0
for i in {1..30}; do
    # Check if FUSE mount appears in /proc/mounts
    if grep -q "${MOUNT_POINT}" /proc/mounts 2>/dev/null; then
        echo "Redis FS mounted successfully! (detected in /proc/mounts)"
        MOUNT_READY=1
        break
    fi
    echo "Attempt $i: waiting for mount..."
    sleep 1
done

if [ "$MOUNT_READY" -ne 1 ]; then
    echo "ERROR: Failed to mount Redis FS after 30 seconds"
    echo "Mount process:"
    ps aux | grep redis-fs-mount || true
    echo "Current mounts:"
    cat /proc/mounts | grep -E "fuse|workspace" || echo "No relevant mounts found"
    exit 1
fi

# Handle shutdown
cleanup() {
    echo "Shutting down..."
    kill $MOUNT_PID 2>/dev/null || true
    fusermount3 -u "${MOUNT_POINT}" 2>/dev/null || true
    exit 0
}
trap cleanup SIGTERM SIGINT

# Run the sandbox server
echo "Starting sandbox server on port ${SANDBOX_PORT}..."
exec sandbox --port "${SANDBOX_PORT}" --workspace "${MOUNT_POINT}"

