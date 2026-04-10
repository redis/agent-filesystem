#!/usr/bin/env bash
# bench_claude_fs.sh
#
# Benchmark a set of fs operations against a target directory tree.
# Designed to compare AFS-mounted ~/.claude vs a local rsync'd copy.
#
# Usage:
#   scripts/bench_claude_fs.sh <target-dir> [rounds] [label]
#
# Output: CSV with header then one row per op:
#   label,op,rounds,median_ms,min_ms,max_ms,stddev_ms,result

set -euo pipefail

TARGET="${1:?usage: bench_claude_fs.sh <target-dir> [rounds] [label]}"
ROUNDS="${2:-5}"
LABEL="${3:-bench}"

if [[ ! -d "$TARGET" ]]; then
  echo "error: target is not a directory: $TARGET" >&2
  exit 1
fi

export TARGET

# ---- timing harness ---------------------------------------------------------
#
# Single perl invocation per measurement. Perl startup happens BEFORE the
# monotonic clock reading, so only the command's wall time is captured.
# Emits "ms\n" to stdout, exit code of the command is preserved.

time_ms() {
  # Redirect the measured command's stdout to /dev/null so it does not
  # pollute the numeric timing line we emit.
  perl -MTime::HiRes=clock_gettime,CLOCK_MONOTONIC -e '
    open(STDOUT_SAVE, ">&", \*STDOUT) or die;
    open(STDOUT, ">", "/dev/null") or die;
    $t0 = clock_gettime(CLOCK_MONOTONIC);
    $rc = system("bash", "-c", $ARGV[0]);
    $elapsed = (clock_gettime(CLOCK_MONOTONIC) - $t0) * 1000.0;
    open(STDOUT, ">&", \*STDOUT_SAVE) or die;
    printf "%.3f\n", $elapsed;
    exit($rc >> 8);
  ' -- "$1" 2>/dev/null
}

# stats: reads one ms value per line, emits "median,min,max,stddev"
stats() {
  awk '
    { vals[NR]=$1; sum+=$1; if (NR==1||$1<min) min=$1; if (NR==1||$1>max) max=$1 }
    END {
      n=NR
      for (i=1;i<=n;i++) for (j=i+1;j<=n;j++) if (vals[i]>vals[j]) { t=vals[i]; vals[i]=vals[j]; vals[j]=t }
      if (n%2) med=vals[(n+1)/2]; else med=(vals[n/2]+vals[n/2+1])/2
      mean=sum/n
      ss=0; for (i=1;i<=n;i++) { d=vals[i]-mean; ss+=d*d }
      std=(n>1)?sqrt(ss/(n-1)):0
      printf "%.3f,%.3f,%.3f,%.3f", med, min, max, std
    }
  '
}

# run_op <name> <shell-command>
run_op() {
  local name="$1"; shift
  local cmd="$1"
  local raw=""
  local result=""
  for ((i=1; i<=ROUNDS; i++)); do
    raw+=$(time_ms "$cmd")
    raw+=$'\n'
  done
  local s
  s=$(printf '%s' "$raw" | stats)
  # capture a single result sample (not timed, just for sanity)
  result=$(bash -c "$cmd" 2>/dev/null | head -1 | tr '\n' ' ' | sed 's/,/;/g' | awk '{$1=$1};1')
  printf '%s,%s,%d,%s,%s\n' "$LABEL" "$name" "$ROUNDS" "$s" "$result"
}

# ---- build deterministic sample file (~100 files) ---------------------------

SAMPLE_FILE="$(mktemp -t bench_sample.XXXXXX)"
trap 'rm -f "$SAMPLE_FILE"' EXIT
find "$TARGET" -type f 2>/dev/null | LC_ALL=C sort | awk 'NR%13==0' | head -100 > "$SAMPLE_FILE"
export SAMPLE_FILE

# ---- op definitions ---------------------------------------------------------
# Each op is a single-line shell command that produces a short stdout result.

OP_stat_root='stat "$TARGET" >/dev/null && echo ok'
OP_readdir_root='ls "$TARGET" | wc -l | tr -d " "'
OP_tree_walk='find "$TARGET" -type f | wc -l | tr -d " "'
OP_tree_walk_dirs='find "$TARGET" -type d | wc -l | tr -d " "'
OP_ls_recursive='ls -laR "$TARGET" >/dev/null && echo ok'
OP_du='du -sh "$TARGET" | awk "{print \$1}"'
OP_grep_text='grep -rIl --include="*.md" --include="*.json" claude "$TARGET" 2>/dev/null | wc -l | tr -d " "'
OP_glob_md='find "$TARGET" -name "*.md" | wc -l | tr -d " "'
# Random stat/read are measured in a single perl process so fork/exec cost
# doesn't dominate the measurement (raw stat() / read() in-process).
OP_random_stat='perl -e '"'"'open F,"<",$ENV{SAMPLE_FILE} or die; while(<F>){chomp; $n++ if stat $_} close F; print "$n\n"'"'"
OP_random_read='perl -e '"'"'open F,"<",$ENV{SAMPLE_FILE} or die; while(<F>){chomp; if(open my $fh,"<",$_){local $/; my $c=<$fh>; $b+=length($c//""); close $fh; $n++}} close F; print "$n"."f/$b"."b\n"'"'"
OP_head_of_tree='n=0; while IFS= read -r f; do cat "$f" >/dev/null 2>&1 && n=$((n+1)) || true; done < <(find "$TARGET" -maxdepth 2 \( -name CLAUDE.md -o -name settings.json -o -name memory.md -o -name MEMORY.md -o -name .afsignore \) 2>/dev/null); echo "$n"'

# Write ops: all scoped to a per-run scratch dir inside the target.
# Each op handles its own setup/teardown so rounds are reproducible.
SCRATCH_REL="__bench_scratch_$$"
OP_write_new_small='d="$TARGET/'"$SCRATCH_REL"'/new"; rm -rf "$d"; mkdir -p "$d"; for i in $(seq 1 50); do echo "hello world $i" > "$d/f_$i.txt"; done; echo 50'
OP_write_overwrite='d="$TARGET/'"$SCRATCH_REL"'/ov"; mkdir -p "$d"; f="$d/x.txt"; : > "$f"; for i in $(seq 1 50); do printf "line %d\n" "$i" > "$f"; done; echo 50'
# Simulates claude jsonl append pattern: open, seek-end, append one line, close, 100 times.
OP_append_jsonl='d="$TARGET/'"$SCRATCH_REL"'/ap"; mkdir -p "$d"; f="$d/session.jsonl"; : > "$f"; perl -e '"'"'$f=$ARGV[0]; for $i (1..100) { open my $fh,">>",$f or die; print $fh qq({"i":$i,"role":"user","content":"hello"}\n); close $fh; } print "100\n"'"'"' "$f"'
OP_cleanup_scratch='rm -rf "$TARGET/'"$SCRATCH_REL"'"; echo ok'

# ---- run --------------------------------------------------------------------

echo "label,op,rounds,median_ms,min_ms,max_ms,stddev_ms,result"
# Harness overhead baseline: no-op shell. Subtract from fast ops for true cost.
run_op _harness_noop  ':'
run_op stat_root      "$OP_stat_root"
run_op readdir_root   "$OP_readdir_root"
run_op tree_walk      "$OP_tree_walk"
run_op tree_walk_dirs "$OP_tree_walk_dirs"
run_op ls_recursive   "$OP_ls_recursive"
run_op du             "$OP_du"
run_op grep_text      "$OP_grep_text"
run_op glob_md        "$OP_glob_md"
run_op random_stat    "$OP_random_stat"
run_op random_read    "$OP_random_read"
run_op head_of_tree   "$OP_head_of_tree"
run_op write_new_small  "$OP_write_new_small"
run_op write_overwrite  "$OP_write_overwrite"
run_op append_jsonl     "$OP_append_jsonl"
# final cleanup (not timed as a real measurement; emits a row for trace)
run_op _cleanup         "$OP_cleanup_scratch"
