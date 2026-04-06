#!/bin/sh

set -eu

server_bin=${AFS_WEB_SERVER_BIN:?AFS_WEB_SERVER_BIN is required}
server_addr=${AFS_WEB_SERVER_ADDR:-127.0.0.1:8091}
allow_origin=${AFS_WEB_ALLOW_ORIGIN:-*}
api_base_url=${AFS_WEB_API_BASE_URL:-http://127.0.0.1:8091}
ui_dir=${AFS_WEB_UI_DIR:?AFS_WEB_UI_DIR is required}
ui_host=${AFS_WEB_UI_HOST:-127.0.0.1}
ui_port=${AFS_WEB_UI_PORT:-5173}
npm_bin=${AFS_WEB_NPM_BIN:-npm}
repo_root=$(CDPATH= cd -- "$(dirname "$server_bin")" && pwd)
server_pid=

cleanup() {
	if [ -n "$server_pid" ] && kill -0 "$server_pid" 2>/dev/null; then
		kill "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
}

trap cleanup EXIT INT TERM HUP

if [ ! -x "$server_bin" ]; then
	echo "error: afs-server binary not found at $server_bin" >&2
	echo "Run 'make cli' first." >&2
	exit 1
fi

if [ ! -f "$repo_root/afs.config.json" ]; then
	echo "error: missing afs.config.json next to afs-server" >&2
	echo "Run './afs setup' from $repo_root, then rerun 'make web-dev'." >&2
	exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
	echo "error: curl is required for the web-dev health check" >&2
	exit 1
fi

echo "Starting AFS control plane on $server_addr"
"$server_bin" --listen "$server_addr" --allow-origin "$allow_origin" &
server_pid=$!

attempt=0
until curl -fsS "$api_base_url/healthz" >/dev/null 2>&1; do
	if ! kill -0 "$server_pid" 2>/dev/null; then
		wait "$server_pid" 2>/dev/null || true
		echo "error: afs-server exited before becoming ready" >&2
		exit 1
	fi
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 50 ]; then
		echo "error: timed out waiting for $api_base_url/healthz" >&2
		exit 1
	fi
	sleep 0.2
done

echo "AFS control plane ready at $api_base_url"
echo "Starting AFS Web UI at http://$ui_host:$ui_port"

cd "$ui_dir"
VITE_AFS_API_BASE_URL="$api_base_url" exec "$npm_bin" run dev -- --host "$ui_host" --port "$ui_port"
