#!/usr/bin/env bash

set -euo pipefail

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: ./deploy/vercel/preview.sh [options] [-- <extra vercel deploy args>]

Stages a temporary Vercel build root that preserves the repo-root Go module,
verifies it with a local `go build ./cmd/server`, and deploys it as a preview.

Options:
  --stage-only          Create and verify the staging directory, but do not deploy.
  --keep-stage-dir      Leave the staging directory on disk after the script exits.
  --catalog-path PATH   Set AFS_CATALOG_PATH for the preview deployment.
                        Default: /tmp/afs.catalog.sqlite
  --scope TEAM          Vercel team slug to use if the project is not linked yet.
  --project NAME        Vercel project name to use if the project is not linked yet.
  -h, --help            Show this help text.

Examples:
  ./deploy/vercel/preview.sh
  ./deploy/vercel/preview.sh --keep-stage-dir
  ./deploy/vercel/preview.sh --scope bookjournal --project agent-filesystem
  ./deploy/vercel/preview.sh -- --meta actor=codex
EOF
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

stage_only=0
keep_stage_dir=0
catalog_path="/tmp/afs.catalog.sqlite"
scope=""
project=""
extra_deploy_args=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --stage-only)
      stage_only=1
      ;;
    --keep-stage-dir)
      keep_stage_dir=1
      ;;
    --catalog-path)
      [[ $# -ge 2 ]] || die "--catalog-path requires a value"
      catalog_path="$2"
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
    --prod)
      die "preview.sh only supports preview deployments"
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

if [[ "$stage_only" -eq 1 ]]; then
  keep_stage_dir=1
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

stage_dir="$(mktemp -d "${TMPDIR:-/tmp}/afs-vercel-preview.XXXXXX")"

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

rm -rf "$stage_dir/internal/uistatic/dist"
cp -r "$repo_root/ui/dist" "$stage_dir/internal/uistatic/dist"
touch "$stage_dir/internal/uistatic/dist/.keep"

if [[ -f "$script_dir/.vercel/project.json" ]]; then
  mkdir -p "$stage_dir/.vercel"
  cp "$script_dir/.vercel/project.json" "$stage_dir/.vercel/project.json"
fi

printf 'staged preview source at %s\n' "$stage_dir" >&2

(
  cd "$stage_dir"
  go build ./cmd/server
)

if [[ "$stage_only" -eq 1 ]]; then
  printf 'stage verification succeeded\n' >&2
  exit 0
fi

if [[ ! -f "$stage_dir/.vercel/project.json" ]]; then
  [[ -n "$scope" && -n "$project" ]] || die "no linked Vercel project found in deploy/vercel/.vercel/project.json; pass --scope and --project to link the staging directory"
  (
    cd "$stage_dir"
    npx --yes vercel@latest link --yes --scope "$scope" --project "$project"
  )
fi

(
  cd "$stage_dir"
  npx --yes vercel@latest deploy --yes --logs \
    -e "AFS_CATALOG_PATH=$catalog_path" \
    ${extra_deploy_args[@]+"${extra_deploy_args[@]}"}
)
