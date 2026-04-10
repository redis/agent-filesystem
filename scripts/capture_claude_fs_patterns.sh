#!/usr/bin/env bash
# capture_claude_fs_patterns.sh
#
# Capture the filesystem access pattern Claude Code makes against ~/.claude
# during a short canned non-interactive session.
#
# Uses macOS fs_usage (requires sudo). Runs claude -p with a fixed prompt,
# tees fs_usage output to a file, then post-processes to extract:
#   - op mix (stat/open/read/readdir)
#   - top N most-touched paths
#   - total op count
#
# Usage:
#   sudo scripts/capture_claude_fs_patterns.sh [out-dir] [prompt]
#
# NOTE: Must be run with sudo (fs_usage needs it on macOS).

set -euo pipefail

OUT_DIR="${1:-$(pwd)/tasks/perf-capture-$(date +%Y%m%d-%H%M%S)}"
PROMPT="${2:-list the files in your ~/.claude/plans directory and tell me how many there are}"
mkdir -p "$OUT_DIR"

RAW="$OUT_DIR/fs_usage.raw"
FILTERED="$OUT_DIR/fs_usage.claude.log"
CLAUDE_BIN="/Users/rowantrollope/Library/Application Support/Claude/claude-code/2.1.92/claude.app/Contents/MacOS/claude"

if [[ $EUID -ne 0 ]]; then
  echo "re-exec under sudo (fs_usage requires root)..." >&2
  exec sudo -E "$0" "$@"
fi

# Preserve the invoking user's identity for running claude (so it reads the
# right ~/.claude and credentials).
REAL_USER="${SUDO_USER:-$USER}"
REAL_HOME=$(eval echo "~$REAL_USER")

echo "  out_dir = $OUT_DIR"
echo "  prompt  = $PROMPT"
echo "  user    = $REAL_USER"

# ---- Start fs_usage in background -----------------------------------------
# -w  wide output (full paths)
# -f filesys  filesystem events only
echo "starting fs_usage..."
fs_usage -w -f filesys > "$RAW" 2>&1 &
FS_PID=$!
# give it a moment to start
sleep 0.5

cleanup() {
  if kill -0 "$FS_PID" 2>/dev/null; then
    kill "$FS_PID" 2>/dev/null || true
    wait "$FS_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# ---- Run canned claude session --------------------------------------------
echo "running claude -p ..."
# drop privileges; use the same HOME as the real user so ~/.claude is right
T0=$(date +%s.%N 2>/dev/null || date +%s)
sudo -u "$REAL_USER" -H HOME="$REAL_HOME" \
  "$CLAUDE_BIN" -p "$PROMPT" \
  --dangerously-skip-permissions \
  --model haiku \
  > "$OUT_DIR/claude-stdout.log" 2> "$OUT_DIR/claude-stderr.log" || true
T1=$(date +%s.%N 2>/dev/null || date +%s)
echo "claude finished in $(awk -v a="$T0" -v b="$T1" 'BEGIN{printf "%.2fs", b-a}')"

# ---- Stop capture ----------------------------------------------------------
sleep 0.5
cleanup

echo "raw capture size: $(wc -l < "$RAW") lines"

# ---- Filter to .claude paths only -----------------------------------------
grep -E '/\.claude(/|$)' "$RAW" > "$FILTERED" || true
echo ".claude-scoped lines: $(wc -l < "$FILTERED")"

# ---- Post-process: op mix --------------------------------------------------
# fs_usage format varies; each line looks roughly like:
#   HH:MM:SS.fraction  OP  [args...]  W  process.pid
# The OP is the 2nd whitespace field.
echo
echo "==== op mix (top 20) ===="
awk '{ print $2 }' "$FILTERED" | sort | uniq -c | sort -rn | head -20 > "$OUT_DIR/op_mix.txt"
cat "$OUT_DIR/op_mix.txt"

# ---- Post-process: top paths ---------------------------------------------
# Extract any token that contains /.claude/
echo
echo "==== top 25 paths under ~/.claude ===="
grep -oE '/[^ ]*/\.claude/[^ ]*' "$FILTERED" \
  | sed 's/[,;]$//' \
  | sort | uniq -c | sort -rn | head -25 > "$OUT_DIR/top_paths.txt" || true
cat "$OUT_DIR/top_paths.txt"

# ---- Summary -------------------------------------------------------------
{
  echo "# Claude Code fs access capture"
  echo
  echo "- prompt: \`$PROMPT\`"
  echo "- total fs_usage lines: $(wc -l < "$RAW")"
  echo "- .claude-scoped lines: $(wc -l < "$FILTERED")"
  echo
  echo "## Op mix (top 20)"
  echo '```'
  cat "$OUT_DIR/op_mix.txt"
  echo '```'
  echo
  echo "## Top 25 paths"
  echo '```'
  cat "$OUT_DIR/top_paths.txt"
  echo '```'
} > "$OUT_DIR/summary.md"

echo
echo "wrote $OUT_DIR/summary.md"
