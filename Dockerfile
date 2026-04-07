# Agent Filesystem all-in-one Docker image
# Includes: Redis with fs.so module + Python library + MCP server

FROM python:3.12-slim AS python-deps

WORKDIR /app
COPY pyproject.toml .
COPY agent_filesystem/ agent_filesystem/
COPY mcp_server/ mcp_server/

RUN pip install --no-cache-dir ".[mcp]"


FROM python:3.12-slim AS final

RUN apt-get update && apt-get install -y \
    build-essential \
    redis-server \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy and build the Redis module
COPY module/ /app/module/
RUN make -C /app/module clean && make -C /app/module

# Copy Python package and server source
COPY --from=python-deps /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=python-deps /usr/local/bin/agent-filesystem /usr/local/bin/
COPY --from=python-deps /usr/local/bin/agent-filesystem-mcp /usr/local/bin/
COPY agent_filesystem/ /app/agent_filesystem/
COPY mcp_server/ /app/mcp_server/

# Start Redis first, then hand off to the configured command.
RUN printf '%s\n' \
    '#!/bin/sh' \
    'set -eu' \
    'redis-server --loadmodule /app/module/fs.so --daemonize yes' \
    'sleep 1' \
    'exec "$@"' > /app/start.sh \
    && chmod +x /app/start.sh

ENV REDIS_URL=redis://localhost:6379/0
ENV MCP_PORT=8089
ENV PYTHONPATH=/app

EXPOSE 6379 8089

ENTRYPOINT ["/app/start.sh"]
CMD ["sh", "-lc", "agent-filesystem-mcp --transport http --port \"${MCP_PORT}\""]
