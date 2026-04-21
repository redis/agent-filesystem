#!/usr/bin/env bash

set -euo pipefail

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: ./deploy/vercel/prod.sh [options] [-- <extra vercel deploy args>]

Stages a temporary Vercel build root that preserves the repo-root Go module,
verifies it with a local `go build ./cmd/server`, and deploys it to production.

Options:
  --keep-stage-dir      Leave the staging directory on disk after the script exits.
  --alias DOMAIN        Attach a production alias after deploy (best effort).
  --scope TEAM          Vercel team slug to use if the project is not linked yet.
  --project NAME        Vercel project name to use if the project is not linked yet.
  -h, --help            Show this help text.

Examples:
  ./deploy/vercel/prod.sh
  ./deploy/vercel/prod.sh --alias agent-filesystem.vercel.app
  ./deploy/vercel/prod.sh --scope bookjournal --project agent-filesystem
EOF
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

keep_stage_dir=0
scope=""
project=""
alias_domain=""
extra_deploy_args=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep-stage-dir)
      keep_stage_dir=1
      ;;
    --alias)
      [[ $# -ge 2 ]] || die "--alias requires a value"
      alias_domain="$2"
      shift
      ;;
    --scope)
      [[ $# -ge 2 ]] || die "--scope requires a value"
      scope="$2"
      shift
      ;;
    --project)
      [[ $# -ge 2 ]] || die "--project requires a value"
      project="$2"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      extra_deploy_args+=("$@")
      break
      ;;
    *)
      extra_deploy_args+=("$1")
      ;;
  esac
  shift
done

if [[ -n "$scope" || -n "$project" ]]; then
  [[ -n "$scope" && -n "$project" ]] || die "--scope and --project must be provided together"
fi

require_command go
require_command mktemp
require_command npx
require_command rsync
require_command npm

printf 'building ui bundle for embedded deploy\n' >&2
(
  cd "$repo_root/ui"
  npm run build >/dev/null
)

stage_dir="$(mktemp -d "${TMPDIR:-/tmp}/afs-vercel-prod.XXXXXX")"

cleanup() {
  if [[ "$keep_stage_dir" -eq 0 ]]; then
    rm -rf "$stage_dir"
    return
  fi
  printf 'staging directory kept at %s\n' "$stage_dir" >&2
}

trap cleanup EXIT

mkdir -p "$stage_dir/cmd/server"

rsync -a --delete \
  --exclude '.git' \
  --exclude '.DS_Store' \
  --exclude 'ui/node_modules' \
  --exclude 'afs' \
  --exclude 'afs-control-plane' \
  --exclude 'mount/agent-filesystem-mount' \
  --exclude 'mount/agent-filesystem-nfs' \
  --exclude 'vercel' \
  --exclude 'afs.catalog.sqlite*' \
  --exclude 'afs.config.json' \
  --exclude 'afs.databases.json' \
  "$repo_root/go.mod" \
  "$repo_root/go.sum" \
  "$repo_root/internal" \
  "$repo_root/mount" \
  "$stage_dir/"

cp "$script_dir/main.go" "$stage_dir/cmd/server/main.go"
cp "$script_dir/cli_embed.go" "$stage_dir/cmd/server/cli_embed.go"

# Include cmd/afs so the control plane can fall back to a local build if the
# prebuilt artifact for an OS/arch is missing (prebuilts are generated below).
rsync -a --delete "$repo_root/cmd/afs" "$stage_dir/cmd/"

rm -rf "$stage_dir/internal/uistatic/dist"
cp -r "$repo_root/ui/dist" "$stage_dir/internal/uistatic/dist"
touch "$stage_dir/internal/uistatic/dist/.keep"

# Cross-compile the AFS CLI for the platforms we expect to serve from
# /v1/cli. On Vercel the running function has no Go toolchain and no source
# tree, so the control plane can only hand back prebuilt artifacts. We bake
# them into cmd/server/cli/<os>-<arch>/afs so the //go:embed directive in
# cli_embed.go packages them into the server binary, and extractCLIBundle
# unpacks them at startup into AFS_CLI_ARTIFACT_DIR.
printf 'cross-compiling afs cli for prod\n' >&2
cli_targets=(
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
  "linux arm64"
)
for target in "${cli_targets[@]}"; do
  os="${target%% *}"
  arch="${target##* }"
  out_dir="$stage_dir/cmd/server/cli/${os}-${arch}"
  mkdir -p "$out_dir"
  (
    cd "$repo_root"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -trimpath -ldflags="-s -w" \
        -o "$out_dir/afs" \
        ./cmd/afs
  )
done

if [[ -f "$script_dir/.vercel/project.json" ]]; then
  mkdir -p "$stage_dir/.vercel"
  cp "$script_dir/.vercel/project.json" "$stage_dir/.vercel/project.json"
fi

printf 'staged production source at %s\n' "$stage_dir" >&2

build_probe="$(mktemp "${TMPDIR:-/tmp}/afs-vercel-server.XXXXXX")"
trap 'rm -f "$build_probe"; cleanup' EXIT

(
  cd "$stage_dir"
  go build -o "$build_probe" ./cmd/server
)

if [[ ! -f "$stage_dir/.vercel/project.json" ]]; then
  [[ -n "$scope" && -n "$project" ]] || die "no linked Vercel project found in deploy/vercel/.vercel/project.json; pass --scope and --project to link the staging directory"
  (
    cd "$stage_dir"
    npx --yes vercel@latest link --yes --scope "$scope" --project "$project"
  )
fi

deploy_cmd=(npx --yes vercel@latest deploy --prod --yes --archive=tgz)
if [[ ${#extra_deploy_args[@]} -gt 0 ]]; then
  deploy_cmd+=("${extra_deploy_args[@]}")
fi

deploy_output="$(
  cd "$stage_dir"
  "${deploy_cmd[@]}" 2>&1
)"

printf '%s\n' "$deploy_output"

deployment_url="$(printf '%s\n' "$deploy_output" | sed -nE 's/^Production: (https:\/\/[^ ]+).*/\1/p' | tail -n1)"
inspect_url="$(printf '%s\n' "$deploy_output" | sed -nE 's/^Inspect: (https:\/\/[^ ]+).*/\1/p' | tail -n1)"

if [[ -z "$deployment_url" ]]; then
  die "production deploy did not return a deployment url"
fi

printf 'production deployment ready: %s\n' "$deployment_url"
if [[ -n "$inspect_url" ]]; then
  printf 'inspect: %s\n' "$inspect_url"
fi

if [[ -n "$alias_domain" ]]; then
  if printf '%s\n' "$deploy_output" | grep -Fq "Aliased: https://$alias_domain"; then
    printf 'alias ready: https://%s\n' "$alias_domain"
    exit 0
  fi
  printf 'attaching alias %s\n' "$alias_domain" >&2
  alias_cmd=(npx --yes vercel@latest alias set "$deployment_url" "$alias_domain")
  if [[ -n "$scope" ]]; then
    alias_cmd+=(--scope "$scope")
  fi
  "${alias_cmd[@]}"
  printf 'alias ready: https://%s\n' "$alias_domain"
fi
