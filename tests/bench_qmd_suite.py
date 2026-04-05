#!/usr/bin/env python3
"""Benchmark redis-fs (mount + direct Redis) vs local filesystem, with optional redis-qmd.

This suite:
1) Generates realistic markdown-heavy chat, session, incident, memory, and log data.
2) Writes identical corpora to a mounted redis-fs path and a temporary local path.
3) Benchmarks:
   - mounted filesystem IO
   - local filesystem IO
   - GNU grep over local and mounted filesystems
   - direct Redis commands over redis-fs inode HASH keys
   - redis-qmd search/query over the mounted corpus (when RediSearch is available)
4) Compares result sets and timing summaries across all search backends.

Example:
  python3 tests/bench_qmd_suite.py \
    --redis-key ttt \
    --redis-mount /Users/me/ttt \
    --local-root /tmp/rfs-local-bench \
    --files 2000 \
    --rounds 5
"""

from __future__ import annotations

import argparse
from collections import Counter
import random
import shutil
import statistics
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Iterable


@dataclass(frozen=True)
class Doc:
    relpath: str
    content: str


@dataclass(frozen=True)
class SearchCase:
    name: str
    description: str
    grep_pattern: str
    grep_scope: str
    qmd_mode: str
    qmd_query: str


@dataclass(frozen=True)
class SearchMeasurement:
    median_ms: float
    max_ms: float
    count: int
    paths: tuple[str, ...]


@dataclass(frozen=True)
class SearchComparison:
    case: SearchCase
    local_count: int
    mount_count: int
    qmd_count: int | None
    local_vs_mount_match: bool
    local_vs_qmd_match: bool | None
    missing_from_mount: tuple[str, ...]
    extra_in_mount: tuple[str, ...]
    missing_from_qmd: tuple[str, ...]
    extra_in_qmd: tuple[str, ...]


@dataclass(frozen=True)
class BenchFlushStatus:
    file_count: int
    missing_path_count: int


DOC_WORDS = (
    "agent",
    "memory",
    "checkpoint",
    "follow-up",
    "incident",
    "customer",
    "tool",
    "workspace",
    "transcript",
    "rollback",
    "queue",
    "timeout",
    "latency",
    "search",
    "index",
    "session",
    "analysis",
    "summary",
    "retry",
    "auth",
    "stream",
    "worker",
    "archive",
    "handoff",
    "response",
    "thread",
    "document",
    "sync",
    "mount",
    "redis",
)


def run(
    cmd: list[str],
    *,
    check: bool = True,
    capture: bool = True,
    text: bool = True,
    cwd: Path | None = None,
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        cmd,
        check=check,
        text=text,
        capture_output=capture,
        cwd=str(cwd) if cwd else None,
    )


def timed(fn: Callable[[], None], rounds: int) -> tuple[float, float]:
    samples_ms: list[float] = []
    for _ in range(rounds):
        t0 = time.perf_counter()
        fn()
        samples_ms.append((time.perf_counter() - t0) * 1000.0)
    return statistics.median(samples_ms), max(samples_ms)


def check_bin(name: str) -> None:
    if shutil.which(name) is None:
        raise SystemExit(f"required binary not found in PATH: {name}")


def _noise_line(rng: random.Random, *, words: int = 18) -> str:
    return " ".join(rng.choice(DOC_WORDS) for _ in range(words))


def _marker_enabled(i: int, modulo: int, guaranteed: tuple[int, ...]) -> bool:
    return i in guaranteed or i % modulo == 0


def _render_doc(kind: str, team: str, i: int, rng: random.Random) -> list[str]:
    day = (i % 28) + 1
    auth_marker = "auth token refresh failed after queue timeout"
    disk_marker = "disk full recovery note appended by agent"
    checkpoint_marker = "memory checkpoint persisted for follow up"

    if kind == "session":
        lines = [
            "---",
            f"title: Agent session {i}",
            "doc_type: agent-session",
            f"team: {team}",
            f"session_id: sess-{i:06d}",
            "model: gpt-5.4-mini",
            f"started_at: 2026-03-{day:02d}T10:{i % 60:02d}:00Z",
            "tags: redis-fs,qmd,session,benchmark",
            "---",
            "",
            "# Agent Session Log",
            "## Session Summary",
            f"- workspace: /srv/{team}",
            "- objective: investigate degraded search quality and capture follow-up tasks",
            "- operator: codex-worker",
            "- status: completed",
            "",
            "## Conversation",
        ]
        for turn in range(10):
            lines.extend(
                [
                    f"### Turn {turn + 1}",
                    f"User: {_noise_line(rng, words=14)}",
                    f"Assistant: {_noise_line(rng, words=20)}",
                    "```text",
                    f"tool[{turn}]: {_noise_line(rng, words=16)}",
                    "```",
                ]
            )
        if _marker_enabled(i, 173, (0, 1)):
            lines.append(auth_marker)
        if _marker_enabled(i, 257, (0, 6)):
            lines.append(checkpoint_marker)
        return lines

    if kind == "chat":
        lines = [
            "---",
            f"title: Support chat {i}",
            "doc_type: customer-chat",
            f"team: {team}",
            f"chat_id: chat-{i:06d}",
            "tags: redis-fs,qmd,chat,benchmark",
            "---",
            "",
            "# Support Chat Transcript",
            "## Conversation",
        ]
        for turn in range(12):
            speaker = "user" if turn % 2 == 0 else "assistant"
            lines.append(f"**{speaker}:** {_noise_line(rng, words=17)}")
        if _marker_enabled(i, 173, (2,)):
            lines.append(auth_marker)
        return lines

    if kind == "memory":
        lines = [
            "---",
            f"title: Memory snapshot {i}",
            "doc_type: memory-snapshot",
            f"team: {team}",
            f"memory_id: mem-{i:06d}",
            "tags: redis-fs,qmd,memory,benchmark",
            "---",
            "",
            "# Agent Memory Snapshot",
            "## Session Summary",
            f"- thread: memory-{i:06d}",
            "- retention: 30 days",
            "",
            "## Follow-ups",
        ]
        for step in range(8):
            lines.append(f"- action {step + 1}: {_noise_line(rng, words=15)}")
        lines.append("memory checkpoint recorded for review")
        return lines

    if kind == "incident":
        lines = [
            "---",
            f"title: Incident review {i}",
            "doc_type: incident-review",
            f"team: {team}",
            f"incident_id: inc-{i:06d}",
            "severity: SEV-2",
            "tags: redis-fs,qmd,incident,benchmark",
            "---",
            "",
            "# Incident Review",
            "## Incident Timeline",
        ]
        for step in range(10):
            lines.append(f"- 2026-03-{day:02d}T12:{step:02d}:00Z {_noise_line(rng, words=14)}")
        if _marker_enabled(i, 211, (4,)):
            lines.append(disk_marker)
        return lines

    lines = [
        f"2026-03-{day:02d}T09:00:00Z INFO worker={team} message=\"{_noise_line(rng, words=10)}\"",
        f"2026-03-{day:02d}T09:00:01Z INFO worker={team} message=\"{_noise_line(rng, words=11)}\"",
        f"2026-03-{day:02d}T09:00:02Z WARN worker={team} message=\"{_noise_line(rng, words=12)}\"",
        f"2026-03-{day:02d}T09:00:03Z ERROR worker={team} message=\"{_noise_line(rng, words=13)}\"",
    ]
    if _marker_enabled(i, 211, (5,)):
        lines.append(f"2026-03-{day:02d}T09:00:04Z ERROR worker={team} message=\"{disk_marker}\"")
    return lines


def _expand_doc(lines: list[str], target_size: int, kind: str, i: int, rng: random.Random) -> str:
    chunk = 0
    body = "\n".join(lines) + "\n"
    while len(body.encode("utf-8")) < target_size:
        chunk += 1
        if kind == "log":
            day = (i % 28) + 1
            for offset in range(8):
                lines.append(
                    f"2026-03-{day:02d}T10:{(chunk + offset) % 60:02d}:00Z INFO chunk={chunk} "
                    f"message=\"{_noise_line(rng, words=15)}\""
                )
        else:
            lines.extend(
                [
                    "",
                    f"### Transcript Chunk {chunk}",
                    f"- note: {_noise_line(rng, words=18)}",
                    f"- observation: {_noise_line(rng, words=16)}",
                    "```text",
                    _noise_line(rng, words=22),
                    _noise_line(rng, words=24),
                    "```",
                ]
            )
        body = "\n".join(lines) + "\n"
    return body


def generate_docs(files: int, target_bytes: int, seed: int) -> list[Doc]:
    rng = random.Random(seed)
    profiles = [
        ("sessions/agent", "session", ".md", 1.9),
        ("sessions/review", "session", ".md", 1.5),
        ("chat/support", "chat", ".md", 1.4),
        ("memories/agents", "memory", ".md", 1.7),
        ("incidents", "incident", ".md", 1.2),
        ("logs/worker", "log", ".log", 0.9),
    ]
    teams = ["api", "search", "worker", "billing", "infra", "agent"]
    docs: list[Doc] = []
    mean_file = max(4096, target_bytes // max(1, files))
    for i in range(files):
        subdir, kind, ext, size_factor = profiles[i % len(profiles)]
        team = teams[i % len(teams)]
        day = (i % 28) + 1
        rel = f"bench/{subdir}/2026-03-{day:02d}/doc-{i:06d}{ext}"
        target_size = max(2048, int(mean_file * size_factor))
        body = _expand_doc(_render_doc(kind, team, i, rng), target_size, kind, i, rng)
        docs.append(Doc(relpath=rel, content=body))
    return docs


def write_docs(root: Path, docs: Iterable[Doc]) -> int:
    total = 0
    for d in docs:
        p = root / d.relpath
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(d.content, encoding="utf-8")
        total += len(d.content.encode("utf-8"))
    return total


def read_docs(root: Path, docs: Iterable[Doc]) -> int:
    total = 0
    for d in docs:
        p = root / d.relpath
        total += len(p.read_bytes())
    return total


def redis_cli_base(addr: str, db: int, password: str) -> list[str]:
    cmd = ["redis-cli", "-h", addr.split(":")[0], "-p", addr.split(":")[1], "-n", str(db)]
    if password:
        cmd += ["-a", password]
    return cmd


def run_redis_cli(addr: str, db: int, password: str, args: list[str], check_ok: bool = True) -> subprocess.CompletedProcess[str]:
    return run(redis_cli_base(addr, db, password) + args, check=check_ok)


def qmd_available(qmd_bin: str, redis_key: str, addr: str, db: int) -> bool:
    cp = run(
        [qmd_bin, "--key", redis_key, "--addr", addr, "--db", str(db), "doctor"],
        check=False,
    )
    out = (cp.stdout or "") + "\n" + (cp.stderr or "")
    return cp.returncode == 0 and "redisearch: ok" in out


def build_pipe_file_for_hget(path: Path, redis_key: str, redis_prefix: str, docs: Iterable[Doc]) -> None:
    with path.open("w", encoding="utf-8") as f:
        for d in docs:
            inode = f"rfs:{{{redis_key}}}:inode:{redis_prefix}/{d.relpath}"
            f.write(f"HGET {inode} content\n")


def build_pipe_file_for_hset_touch(
    path: Path, redis_key: str, redis_prefix: str, docs: Iterable[Doc], mtime_ms: int
) -> None:
    with path.open("w", encoding="utf-8") as f:
        for d in docs:
            inode = f"rfs:{{{redis_key}}}:inode:{redis_prefix}/{d.relpath}"
            f.write(f"HSET {inode} mtime_ms {mtime_ms}\n")


def redis_pipe(addr: str, db: int, password: str, command_file: Path) -> None:
    cmd = redis_cli_base(addr, db, password) + ["--pipe"]
    with command_file.open("rb") as inp:
        cp = subprocess.run(cmd, stdin=inp, text=False, capture_output=True)
    if cp.returncode != 0:
        raise RuntimeError(f"redis-cli --pipe failed: {cp.stderr.decode('utf-8', errors='ignore')}")


def redis_eval_naive_grep(
    addr: str, db: int, password: str, redis_key: str, redis_prefix: str, needle: str
) -> int:
    script = r"""
local cursor = "0"
local total = 0
local pattern = ARGV[1]
local needle = string.lower(ARGV[2])
repeat
  local r = redis.call("SCAN", cursor, "MATCH", pattern, "COUNT", "500")
  cursor = r[1]
  local keys = r[2]
  for _,k in ipairs(keys) do
    if redis.call("HGET", k, "type") == "file" then
      local content = redis.call("HGET", k, "content") or ""
      content = string.lower(content)
      local s = 1
      while true do
        local i, j = string.find(content, needle, s, true)
        if i == nil then break end
        total = total + 1
        s = j + 1
      end
    end
  end
until cursor == "0"
return total
"""
    pattern = f"rfs:{{{redis_key}}}:inode:{redis_prefix}/bench/*"
    cp = run_redis_cli(addr, db, password, ["--raw", "EVAL", script, "0", pattern, needle], check_ok=True)
    return int((cp.stdout or "0").strip() or "0")


def redis_eval_bench_flush_status(
    addr: str, db: int, password: str, redis_key: str, redis_prefix: str
) -> BenchFlushStatus:
    script = r"""
local cursor = "0"
local files = 0
local missing = 0
local pattern = ARGV[1]
repeat
  local r = redis.call("SCAN", cursor, "MATCH", pattern, "COUNT", "500")
  cursor = r[1]
  local keys = r[2]
  for _,k in ipairs(keys) do
    if redis.call("HGET", k, "type") == "file" then
      files = files + 1
      local path = redis.call("HGET", k, "path")
      if path == false or path == nil or path == "" then
        missing = missing + 1
      end
    end
  end
until cursor == "0"
return {files, missing}
"""
    pattern = f"rfs:{{{redis_key}}}:inode:{redis_prefix}/bench/*"
    cp = run_redis_cli(addr, db, password, ["--raw", "EVAL", script, "0", pattern], check_ok=True)
    lines = [line.strip() for line in (cp.stdout or "").splitlines() if line.strip()]
    if len(lines) < 2:
        raise RuntimeError(f"unexpected flush-status response: {cp.stdout!r}")
    return BenchFlushStatus(file_count=int(lines[0]), missing_path_count=int(lines[1]))


def bench_files_visible(status: BenchFlushStatus, expected_files: int) -> bool:
    return status.file_count >= expected_files


def bench_paths_backfilled(status: BenchFlushStatus) -> bool:
    return status.missing_path_count == 0


def wait_for_bench_flush(
    addr: str,
    db: int,
    password: str,
    redis_key: str,
    redis_prefix: str,
    expected_files: int,
    timeout_s: float = 30.0,
    poll_s: float = 0.25,
) -> BenchFlushStatus:
    deadline = time.time() + timeout_s
    last = BenchFlushStatus(file_count=0, missing_path_count=expected_files)
    stable_polls = 0
    last_count = -1
    while time.time() < deadline:
        last = redis_eval_bench_flush_status(addr, db, password, redis_key, redis_prefix)
        if last.file_count == last_count:
            stable_polls += 1
        else:
            stable_polls = 0
            last_count = last.file_count
        if bench_files_visible(last, expected_files) and stable_polls >= 2:
            return last
        time.sleep(poll_s)
    raise RuntimeError(
        "mounted corpus did not stabilize in Redis before indexing "
        f"(files={last.file_count}, missing_path={last.missing_path_count}, expected={expected_files})"
    )


def benchmark_search_cmd(
    cmd: list[str],
    rounds: int,
    *,
    cwd: Path | None = None,
    ok_returncodes: tuple[int, ...] = (0,),
) -> tuple[float, float]:
    def fn() -> None:
        cp = subprocess.run(
            cmd,
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            text=True,
            cwd=str(cwd) if cwd else None,
        )
        if cp.returncode not in ok_returncodes:
            raise RuntimeError(f"command failed with rc={cp.returncode}: {' '.join(cmd)}")

    return timed(fn, rounds)


def build_search_cases(bench_id: str) -> list[SearchCase]:
    return [
        SearchCase(
            name="auth-timeout",
            description="rare auth timeout phrase across chats and sessions",
            grep_pattern="auth token refresh failed after queue timeout",
            grep_scope="bench",
            qmd_mode="query",
            qmd_query=f'"auth token refresh failed after queue timeout" path:/{bench_id}/bench',
        ),
        SearchCase(
            name="disk-full",
            description="rare disk-full incident phrase across incidents and logs",
            grep_pattern="disk full recovery note appended by agent",
            grep_scope="bench",
            qmd_mode="query",
            qmd_query=f'"disk full recovery note appended by agent" path:/{bench_id}/bench',
        ),
        SearchCase(
            name="checkpoint-sessions",
            description="path-scoped memory checkpoint phrase in agent sessions",
            grep_pattern="memory checkpoint persisted for follow up",
            grep_scope="bench/sessions/agent",
            qmd_mode="query",
            qmd_query=f'"memory checkpoint persisted for follow up" path:/{bench_id}/bench/sessions/agent',
        ),
    ]


def corpus_profile(docs: Iterable[Doc]) -> Counter[str]:
    counts: Counter[str] = Counter()
    for doc in docs:
        parts = Path(doc.relpath).parts
        label = "/".join(parts[1:3]) if len(parts) >= 3 else doc.relpath
        counts[label] += 1
    return counts


def grep_file_matches(root: Path, case: SearchCase) -> tuple[str, ...]:
    cp = run(
        ["grep", "-R", "-l", "-F", "--binary-files=without-match", case.grep_pattern, case.grep_scope],
        check=False,
        cwd=root,
    )
    if cp.returncode not in (0, 1):
        raise RuntimeError(cp.stderr or f"grep failed for case {case.name}")
    lines = [line.strip() for line in (cp.stdout or "").splitlines() if line.strip()]
    return tuple(sorted(set(lines)))


def measure_grep_case(root: Path, case: SearchCase, rounds: int) -> SearchMeasurement:
    cmd = ["grep", "-R", "-l", "-F", "--binary-files=without-match", case.grep_pattern, case.grep_scope]
    median_ms, max_ms = benchmark_search_cmd(cmd, rounds, cwd=root, ok_returncodes=(0, 1))
    paths = grep_file_matches(root, case)
    return SearchMeasurement(median_ms=median_ms, max_ms=max_ms, count=len(paths), paths=paths)


def normalize_qmd_path(path: str, bench_id: str) -> str:
    prefix = f"/{bench_id}/"
    if path.startswith(prefix):
        return path[len(prefix):]
    return path.lstrip("/")


def qmd_command(qmd_bin: str, redis_key: str, addr: str, db: int, case: SearchCase) -> list[str]:
    return [qmd_bin, "--key", redis_key, "--addr", addr, "--db", str(db), case.qmd_mode, case.qmd_query]


def qmd_file_matches(
    qmd_bin: str,
    redis_key: str,
    addr: str,
    db: int,
    case: SearchCase,
    bench_id: str,
) -> tuple[int, tuple[str, ...]]:
    cp = run(qmd_command(qmd_bin, redis_key, addr, db, case), check=False)
    if cp.returncode != 0:
        raise RuntimeError((cp.stderr or cp.stdout or "").strip() or f"redis-qmd failed for case {case.name}")

    total = 0
    paths: list[str] = []
    for raw_line in (cp.stdout or "").splitlines():
        line = raw_line.strip()
        if not line:
            continue
        if line.startswith("total:"):
            total = int(line.split(":", 1)[1].strip())
            continue
        parts = raw_line.split("  ", 2)
        if len(parts) >= 2:
            paths.append(normalize_qmd_path(parts[1].strip(), bench_id))

    unique_paths = tuple(sorted(set(paths)))
    if total > len(unique_paths):
        raise RuntimeError(
            f"redis-qmd output for case {case.name} appears truncated "
            f"(total={total}, returned={len(unique_paths)})"
        )
    return total, unique_paths


def measure_qmd_case(
    qmd_bin: str,
    redis_key: str,
    addr: str,
    db: int,
    case: SearchCase,
    rounds: int,
    bench_id: str,
) -> SearchMeasurement:
    cmd = qmd_command(qmd_bin, redis_key, addr, db, case)
    median_ms, max_ms = benchmark_search_cmd(cmd, rounds)
    total, paths = qmd_file_matches(qmd_bin, redis_key, addr, db, case, bench_id)
    return SearchMeasurement(median_ms=median_ms, max_ms=max_ms, count=total, paths=paths)


def compare_search_results(
    case: SearchCase,
    local_paths: Iterable[str],
    mount_paths: Iterable[str],
    qmd_paths: Iterable[str] | None,
) -> SearchComparison:
    local = tuple(sorted(set(local_paths)))
    mount = tuple(sorted(set(mount_paths)))
    qmd = None if qmd_paths is None else tuple(sorted(set(qmd_paths)))

    local_set = set(local)
    mount_set = set(mount)
    qmd_set = None if qmd is None else set(qmd)

    return SearchComparison(
        case=case,
        local_count=len(local),
        mount_count=len(mount),
        qmd_count=None if qmd is None else len(qmd),
        local_vs_mount_match=local_set == mount_set,
        local_vs_qmd_match=None if qmd_set is None else local_set == qmd_set,
        missing_from_mount=tuple(sorted(local_set - mount_set)),
        extra_in_mount=tuple(sorted(mount_set - local_set)),
        missing_from_qmd=tuple() if qmd_set is None else tuple(sorted(local_set - qmd_set)),
        extra_in_qmd=tuple() if qmd_set is None else tuple(sorted(qmd_set - local_set)),
    )


def preview_paths(paths: tuple[str, ...], limit: int = 4) -> str:
    if not paths:
        return "(none)"
    shown = ", ".join(paths[:limit])
    if len(paths) > limit:
        return f"{shown}, ..."
    return shown


def capabilities(qmd_ok: bool) -> list[tuple[str, str, str, str]]:
    return [
        ("literal text search", "yes", "yes", "yes"),
        ("exact phrase search", "yes", "yes", "yes"),
        ("path prefix filter", "yes (DSL path:)", "yes (search subdir)", "yes (MATCH prefix)"),
        ("size filter", "yes (DSL size>)", "yes (find -size + rg)", "yes (Lua HGET size)"),
        ("ranked BM25 results", "yes", "no", "no"),
        ("single-command advanced query", "yes", "partial", "partial"),
        ("requires RediSearch module", "yes", "no", "no"),
        ("works when FT.* unavailable", "no", "yes", "yes"),
    ]


def print_capabilities(qmd_ok: bool) -> None:
    print("\nCapabilities")
    print("------------")
    cols = ("feature", "redis-qmd", "local filesystem", "direct Redis commands")
    print(f"{cols[0]:34} | {cols[1]:22} | {cols[2]:20} | {cols[3]}")
    print("-" * 104)
    for row in capabilities(qmd_ok):
        if row[0].startswith("ranked") or row[0].startswith("single") or row[0].startswith("requires") or row[0].startswith("works"):
            if not qmd_ok and row[1].startswith("yes"):
                row = (row[0], "unavailable (FT.* missing)", row[2], row[3])
        print(f"{row[0]:34} | {row[1]:22} | {row[2]:20} | {row[3]}")


def main() -> None:
    p = argparse.ArgumentParser(description="redis-fs vs local filesystem benchmark suite")
    p.add_argument("--redis-key", required=True, help="redis-fs key name")
    p.add_argument("--redis-mount", required=True, help="mounted redis-fs path")
    p.add_argument("--local-root", default="/tmp/rfs-local-bench", help="root for local benchmark corpus")
    p.add_argument("--addr", default="127.0.0.1:6379", help="Redis host:port")
    p.add_argument("--db", type=int, default=0, help="Redis DB")
    p.add_argument("--password", default="", help="Redis password")
    p.add_argument("--qmd-bin", default="./redis-qmd", help="path to redis-qmd binary")
    p.add_argument("--files", type=int, default=2000, help="number of files to generate")
    p.add_argument("--total-mb", type=int, default=32, help="target total corpus size in MiB")
    p.add_argument("--rounds", type=int, default=5, help="benchmark rounds")
    p.add_argument("--seed", type=int, default=42, help="random seed for deterministic corpus")
    p.add_argument("--keep-data", action="store_true", help="do not delete generated data")
    p.add_argument(
        "--require-redisearch",
        action="store_true",
        help="fail fast unless RediSearch (FT.*) is available for redis-qmd tests",
    )
    args = p.parse_args()

    if ":" not in args.addr:
        raise SystemExit("--addr must be host:port")

    check_bin("redis-cli")
    check_bin("grep")
    redis_mount = Path(args.redis_mount).resolve()
    local_root = Path(args.local_root).resolve()
    if not redis_mount.exists():
        raise SystemExit(f"redis mount path does not exist: {redis_mount}")
    if not redis_mount.is_dir():
        raise SystemExit(f"redis mount path is not a directory: {redis_mount}")

    qmd_ok = Path(args.qmd_bin).exists() and qmd_available(args.qmd_bin, args.redis_key, args.addr, args.db)
    if args.require_redisearch and not qmd_ok:
        raise SystemExit(
            "RediSearch is required but not available on this Redis instance.\n"
            "Tip: run Redis Stack and point this suite at it, e.g.\n"
            "  docker run -d --name redis-stack-bench -p 6380:6379 redis/redis-stack-server:latest\n"
            "Then run with:\n"
            "  --addr 127.0.0.1:6380 --require-redisearch"
        )

    bench_id = time.strftime("bench-%Y%m%d-%H%M%S")
    search_cases = build_search_cases(bench_id)
    redis_prefix = f"/{bench_id}"
    redis_bench_root = redis_mount / bench_id
    local_bench_root = local_root / bench_id
    local_bench_root.parent.mkdir(parents=True, exist_ok=True)

    print("Preparing synthetic corpus...")
    total_bytes = args.total_mb * 1024 * 1024
    docs = generate_docs(files=args.files, target_bytes=total_bytes, seed=args.seed)
    print(f"- files: {len(docs)}")
    print(f"- target size: {args.total_mb} MiB")
    print(f"- redis bench root: {redis_bench_root}")
    print(f"- local bench root: {local_bench_root}")

    redis_bench_root.mkdir(parents=True, exist_ok=True)
    local_bench_root.mkdir(parents=True, exist_ok=True)

    t0 = time.perf_counter()
    bytes_local = write_docs(local_bench_root, docs)
    local_write_ms = (time.perf_counter() - t0) * 1000.0

    t0 = time.perf_counter()
    bytes_mount = write_docs(redis_bench_root, docs)
    mount_write_ms = (time.perf_counter() - t0) * 1000.0

    if bytes_local != bytes_mount:
        raise RuntimeError("generated corpus mismatch between local and mounted writes")

    flush_status = wait_for_bench_flush(
        args.addr,
        args.db,
        args.password,
        args.redis_key,
        redis_prefix,
        expected_files=len(docs),
    )

    print("\nData generation + ingest complete")
    print(f"- bytes written: {bytes_local:,}")
    print(f"- local write: {local_write_ms:.2f} ms")
    print(f"- mounted write: {mount_write_ms:.2f} ms")
    print(f"- redis-visible files: {flush_status.file_count}")
    print(f"- missing path fields: {flush_status.missing_path_count}")
    print("- corpus profile:")
    for label, count in corpus_profile(docs).most_common():
        print(f"  - {label}: {count} files")

    print("\nRunning read benchmarks...")
    local_read_med, local_read_max = timed(lambda: read_docs(local_bench_root, docs), args.rounds)
    mount_read_med, mount_read_max = timed(lambda: read_docs(redis_bench_root, docs), args.rounds)

    print("Running direct Redis command benchmarks...")
    with tempfile.TemporaryDirectory(prefix="rfs-bench-") as td:
        td_path = Path(td)
        hget_file = td_path / "hget.pipe"
        hset_file = td_path / "hset.pipe"
        build_pipe_file_for_hget(hget_file, args.redis_key, redis_prefix, docs)
        build_pipe_file_for_hset_touch(hset_file, args.redis_key, redis_prefix, docs, int(time.time() * 1000))

        redis_hget_med, redis_hget_max = timed(
            lambda: redis_pipe(args.addr, args.db, args.password, hget_file), args.rounds
        )
        redis_hset_med, redis_hset_max = timed(
            lambda: redis_pipe(args.addr, args.db, args.password, hset_file), args.rounds
        )
        redis_scan_grep_med, redis_scan_grep_max = timed(
            lambda: redis_eval_naive_grep(
                args.addr, args.db, args.password, args.redis_key, redis_prefix, search_cases[0].grep_pattern
            ),
            args.rounds,
        )

    qmd_index_ms = None
    post_index_status = None
    if qmd_ok:
        print("Preparing redis-qmd index...")
        t0 = time.perf_counter()
        run([args.qmd_bin, "--key", args.redis_key, "--addr", args.addr, "--db", str(args.db), "index", "rebuild"])
        qmd_index_ms = (time.perf_counter() - t0) * 1000.0
        post_index_status = redis_eval_bench_flush_status(
            args.addr,
            args.db,
            args.password,
            args.redis_key,
            redis_prefix,
        )
        if not bench_paths_backfilled(post_index_status):
            raise RuntimeError(
                "redis-qmd index rebuild completed but some benchmark files still lack path fields "
                f"(files={post_index_status.file_count}, missing_path={post_index_status.missing_path_count})"
            )
    else:
        print("Skipping redis-qmd timings: RediSearch is not available on this Redis instance.")

    print("\nRunning search comparisons...")
    search_results: list[tuple[SearchCase, SearchMeasurement, SearchMeasurement, SearchMeasurement | None, SearchComparison]] = []
    for case in search_cases:
        print(f"- {case.name}: {case.description}")
        local_search = measure_grep_case(local_bench_root, case, args.rounds)
        mount_search = measure_grep_case(redis_bench_root, case, args.rounds)
        qmd_search = (
            measure_qmd_case(args.qmd_bin, args.redis_key, args.addr, args.db, case, args.rounds, bench_id)
            if qmd_ok
            else None
        )
        comparison = compare_search_results(case, local_search.paths, mount_search.paths, None if qmd_search is None else qmd_search.paths)
        search_results.append((case, local_search, mount_search, qmd_search, comparison))

    print_capabilities(qmd_ok)

    print("\nSearch Result Comparison")
    print("------------------------")
    for case, local_search, mount_search, qmd_search, comparison in search_results:
        qmd_status = "skipped"
        if qmd_search is not None:
            qmd_status = f"{qmd_search.count} hits median={qmd_search.median_ms:.2f} max={qmd_search.max_ms:.2f}"
        print(f"- {case.name}: {case.description}")
        print(
            f"  local grep   -> {local_search.count} hits median={local_search.median_ms:.2f} "
            f"max={local_search.max_ms:.2f}"
        )
        print(
            f"  mounted grep -> {mount_search.count} hits median={mount_search.median_ms:.2f} "
            f"max={mount_search.max_ms:.2f}"
        )
        print(f"  redis-qmd    -> {qmd_status}")
        compare_parts = ["local=mounted" if comparison.local_vs_mount_match else "local!=mounted"]
        if comparison.local_vs_qmd_match is None:
            compare_parts.append("qmd skipped")
        else:
            compare_parts.append("local=qmd" if comparison.local_vs_qmd_match else "local!=qmd")
        print(f"  compare      -> {', '.join(compare_parts)}")
        if not comparison.local_vs_mount_match:
            print(f"  missing mount -> {preview_paths(comparison.missing_from_mount)}")
            print(f"  extra mount   -> {preview_paths(comparison.extra_in_mount)}")
        if comparison.local_vs_qmd_match is False:
            print(f"  missing qmd   -> {preview_paths(comparison.missing_from_qmd)}")
            print(f"  extra qmd     -> {preview_paths(comparison.extra_in_qmd)}")

    print("\nPerformance (ms)")
    print("---------------")
    print(f"local write corpus                 median={local_write_ms:.2f}  max={local_write_ms:.2f}")
    print(f"mounted redis-fs write corpus      median={mount_write_ms:.2f}  max={mount_write_ms:.2f}")
    print(f"local read corpus                  median={local_read_med:.2f}  max={local_read_max:.2f}")
    print(f"mounted redis-fs read corpus       median={mount_read_med:.2f}  max={mount_read_max:.2f}")
    print(f"direct Redis HGET batch            median={redis_hget_med:.2f}  max={redis_hget_max:.2f}")
    print(f"direct Redis HSET batch            median={redis_hset_med:.2f}  max={redis_hset_max:.2f}")
    print(f"direct Redis naive grep (Lua)      median={redis_scan_grep_med:.2f}  max={redis_scan_grep_max:.2f}")
    if qmd_ok and qmd_index_ms is not None:
        print(f"redis-qmd index rebuild            median={qmd_index_ms:.2f}  max={qmd_index_ms:.2f}")
    else:
        print("redis-qmd timings                  skipped (no RediSearch)")
    for case, local_search, mount_search, qmd_search, _comparison in search_results:
        print(f"grep local {case.name:20} median={local_search.median_ms:.2f}  max={local_search.max_ms:.2f}")
        print(f"grep mount {case.name:20} median={mount_search.median_ms:.2f}  max={mount_search.max_ms:.2f}")
        if qmd_search is not None:
            print(f"qmd {case.name:27} median={qmd_search.median_ms:.2f}  max={qmd_search.max_ms:.2f}")
        else:
            print(f"qmd {case.name:27} skipped (no RediSearch)")

    if args.keep_data:
        print("\nGenerated data kept:")
        print(f"- {local_bench_root}")
        print(f"- {redis_bench_root}")
    else:
        shutil.rmtree(local_bench_root, ignore_errors=True)
        shutil.rmtree(redis_bench_root, ignore_errors=True)
        print("\nGenerated data cleaned up.")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        sys.exit(130)
