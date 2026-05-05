#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/test-mount-unmount-delete.sh <N> [prefix]

Creates N temporary workspaces, mounts them, writes test files, unmounts them,
and deletes them. The script pauses after each phase so you can watch the UI.

Options:
  N       Positive number of workspaces to create.
  prefix  Optional workspace name prefix. Defaults to afs-watch-<timestamp>.

Environment:
  AFS_BIN  Path to the afs binary. Defaults to ./afs from the repo root.
USAGE
}

die() {
  echo "error: $*" >&2
  exit 1
}

pause() {
  local message="$1"
  echo
  echo "$message"
  read -r -p "Press Enter to continue..." _
}

run() {
  echo "+ $*"
  "$@"
}

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
afs_bin="${AFS_BIN:-$repo_root/afs}"

if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage
  exit 2
fi

count="$1"
prefix="${2:-afs-watch-$(date +%Y%m%d-%H%M%S)}"

if ! [[ "$count" =~ ^[1-9][0-9]*$ ]]; then
  die "N must be a positive integer"
fi

if [[ ! -x "$afs_bin" ]]; then
  die "AFS binary is not executable at $afs_bin. Build it with 'make commands' or set AFS_BIN=/path/to/afs"
fi

mount_root="${TMPDIR:-/tmp}/afs-mount-unmount-delete-${prefix}"
mkdir -p "$mount_root"

workspaces=()
mounts=()
for ((i = 1; i <= count; i += 1)); do
  workspace="$(printf "%s-%02d" "$prefix" "$i")"
  mount_dir="$mount_root/$workspace"
  workspaces+=("$workspace")
  mounts+=("$mount_dir")
done

echo "AFS binary: $afs_bin"
echo "Mount root: $mount_root"
echo "Workspaces:"
for workspace in "${workspaces[@]}"; do
  echo "  - $workspace"
done

pause "Ready to create ${count} workspace(s)."

for workspace in "${workspaces[@]}"; do
  run "$afs_bin" ws create "$workspace"
done

pause "Created workspace(s). Watch them appear, then continue to mount."

for i in "${!workspaces[@]}"; do
  mkdir -p "${mounts[$i]}"
  run "$afs_bin" ws mount "${workspaces[$i]}" "${mounts[$i]}"
done

pause "Mounted workspace(s). Watch agent/topology updates, then continue to write files."

for i in "${!workspaces[@]}"; do
  workspace="${workspaces[$i]}"
  mount_dir="${mounts[$i]}"
  mkdir -p "$mount_dir/docs" "$mount_dir/data" "$mount_dir/nested/one/two"
  printf "# %s\n\nCreated by %s at %s.\n" "$workspace" "$(basename "$0")" "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" > "$mount_dir/README.md"
  printf "workspace=%s\nmount=%s\n" "$workspace" "$mount_dir" > "$mount_dir/data/info.txt"
  printf "alpha\nbeta\ngamma\n" > "$mount_dir/docs/list.txt"
  printf '{"workspace":"%s","createdAt":"%s"}\n' "$workspace" "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" > "$mount_dir/nested/one/two/meta.json"
done

pause "Wrote test files. Wait for file/change activity if you want, then continue to unmount."

for workspace in "${workspaces[@]}"; do
  run "$afs_bin" ws unmount "$workspace"
done

pause "Unmounted workspace(s). Watch agents disappear, then continue to delete workspaces."

for workspace in "${workspaces[@]}"; do
  run "$afs_bin" ws delete --no-confirmation "$workspace"
done

pause "Deleted workspace(s). Watch them pop out, then press Enter to finish."

echo
echo "Done."
echo "Local mount root left in place for inspection: $mount_root"
