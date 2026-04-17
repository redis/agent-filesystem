#!/usr/bin/env bash

set -euo pipefail

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: ./deploy/vercel/smoke.sh <deployment-url>
       ./deploy/vercel/smoke.sh --deployment <deployment-url>

Runs a small protected-preview smoke check against the current AFS Vercel shape:

- GET /
- GET /healthz
- GET /v1/catalog/health

The script uses `vercel curl` so it can reach protected preview deployments.
EOF
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

deployment_url=""
scope=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --deployment)
      [[ $# -ge 2 ]] || die "--deployment requires a value"
      deployment_url="$2"
      shift
      ;;
    --scope)
      [[ $# -ge 2 ]] || die "--scope requires a value"
      scope="$2"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      if [[ -z "$deployment_url" ]]; then
        deployment_url="$1"
      else
        die "unexpected argument: $1"
      fi
      ;;
  esac
  shift
done

[[ -n "$deployment_url" ]] || die "deployment URL is required"

require_command curl
require_command mktemp
require_command npx

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/afs-vercel-smoke.XXXXXX")"
trap 'rm -rf "$tmpdir"' EXIT

run_check() {
  local path="$1"
  local label="$2"
  local expect="$3"
  local body_file="$tmpdir/body"
  local -a curl_args=()

  if [[ -n "$scope" ]]; then
    curl_args+=(--scope "$scope")
  fi

  local status
  if [[ ${#curl_args[@]} -gt 0 ]]; then
    status="$(
      npx --yes vercel@latest \
        --cwd "$script_dir" \
        "${curl_args[@]}" \
        curl "$path" \
        --deployment "$deployment_url" \
        -- \
        -sS \
        -o "$body_file" \
        -w '%{http_code}'
    )"
  else
    status="$(
      npx --yes vercel@latest \
        --cwd "$script_dir" \
        curl "$path" \
        --deployment "$deployment_url" \
        -- \
        -sS \
        -o "$body_file" \
        -w '%{http_code}'
    )"
  fi

  if [[ "$status" != "200" ]]; then
    printf 'FAIL  %s (%s) status=%s\n' "$label" "$path" "$status" >&2
    cat "$body_file" >&2
    return 1
  fi

  if ! grep -Fq "$expect" "$body_file"; then
    printf 'FAIL  %s (%s) missing expected content: %s\n' "$label" "$path" "$expect" >&2
    cat "$body_file" >&2
    return 1
  fi

  printf 'PASS  %s (%s)\n' "$label" "$path"
}

run_check "/" "ui root" "<!doctype html>"
run_check "/healthz" "health check" '"ok":true'
run_check "/v1/catalog/health" "catalog health" '"generated_at"'

printf 'Smoke check passed for %s\n' "$deployment_url"
