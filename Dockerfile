# Redis-FS Docker image
# Includes: Redis with fs.so module + Python library + MCP server

FROM python:3.12-slim AS python-deps

WORKDIR /app
COPY pyproject.toml .
COPY redis_fs/ redis_fs/
COPY mcp_server/ mcp_server/

RUN pip install --no-cache-dir ".[mcp]"


FROM redis:7-bookworm AS final

# Install build tools for compiling the Redis module
RUN apt-get update && apt-get install -y \
    build-essential \
    python3 \
    python3-pip \
    python3-venv \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy and build the Redis module
COPY fs.c fs.h path.c path.h redismodule.h Makefile ./
RUN make

# Copy Python package
COPY --from=python-deps /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/dist-packages
COPY --from=python-deps /usr/local/bin/redis-fs /usr/local/bin/
COPY --from=python-deps /usr/local/bin/redis-fs-mcp /usr/local/bin/
COPY redis_fs/ /app/redis_fs/
COPY mcp_server/ /app/mcp_server/

# Create startup script
RUN echo '#!/bin/bash\n\
redis-server --loadmodule /app/fs.so --daemonize yes\n\
sleep 1\n\
echo "Redis started with fs.so module"\n\
exec "$@"' > /app/start.sh && chmod +x /app/start.sh

ENV REDIS_URL=redis://localhost:6379/0
ENV PYTHONPATH=/app

EXPOSE 6379

# Default: start Redis and run MCP server
ENTRYPOINT ["/app/start.sh"]
CMD ["redis-fs-mcp"]

