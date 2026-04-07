#!/usr/bin/env python3
"""Compare recursive text-search timings for mounted and archive Claude directories.

Examples:
  python3 scripts/compare_grep_times.py "hello"
  python3 scripts/compare_grep_times.py --tool rg "hello"
  python3 scripts/compare_grep_times.py --rounds 7 --fixed-strings --ignore-case "hello"
  python3 scripts/compare_grep_times.py --exclude-dir projects --exclude '*.jsonl' "hello"
"""

from __future__ import annotations

import argparse
from dataclasses import dataclass
from pathlib import Path
import shutil
import statistics
import subprocess
import time


@dataclass(frozen=True)
class Target:
    label: str
    path: Path


@dataclass(frozen=True)
class RunResult:
    elapsed_ms: float
    match_lines: int
    exit_code: int
    stderr: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Compare recursive search timings for ~/.claude (mounted) and ~/.claude-archive."
    )
    parser.add_argument("pattern", help="search pattern")
    parser.add_argument("--mount-dir", default="~/.claude", help="mounted directory to benchmark")
    parser.add_argument("--archive-dir", default="~/.claude-archive", help="archive directory to benchmark")
    parser.add_argument(
        "--tool",
        choices=("grep", "rg"),
        default="grep",
        help="search tool to benchmark",
    )
    parser.add_argument("--rounds", type=int, default=5, help="number of measured rounds")
    parser.add_argument("--warmup", type=int, default=1, help="number of warmup rounds to discard")
    parser.add_argument("--tool-bin", default="", help="override the search binary path")
    parser.add_argument("--ignore-case", action="store_true", help="pass -i to the search tool")
    parser.add_argument("--fixed-strings", action="store_true", help="pass -F to the search tool")
    parser.add_argument("--extended-regexp", action="store_true", help="pass -E to the search tool")
    parser.add_argument("--word-regexp", action="store_true", help="pass -w to the search tool")
    parser.add_argument(
        "--hidden",
        action="store_true",
        help="for rg, include hidden files and directories",
    )
    parser.add_argument(
        "--no-ignore",
        action="store_true",
        help="for rg, disable ignore-file filtering",
    )
    parser.add_argument(
        "--exclude-dir",
        action="append",
        default=[],
        help="exclude a directory pattern; repeat as needed",
    )
    parser.add_argument(
        "--exclude",
        action="append",
        default=[],
        help="exclude a file pattern; repeat as needed",
    )
    parser.add_argument(
        "--include",
        action="append",
        default=[],
        help="include only matching file patterns; repeat as needed",
    )
    parser.add_argument(
        "--tool-arg",
        action="append",
        default=[],
        help="extra raw tool argument; repeat as needed",
    )
    return parser.parse_args()


def ensure_binary(name: str) -> None:
    if shutil.which(name) is None:
        raise SystemExit(f"required binary not found in PATH: {name}")


def ensure_dir(path_str: str, label: str) -> Path:
    path = Path(path_str).expanduser().resolve()
    if not path.exists():
        raise SystemExit(f"{label} path does not exist: {path}")
    if not path.is_dir():
        raise SystemExit(f"{label} path is not a directory: {path}")
    return path


def build_grep_cmd(args: argparse.Namespace, root: Path) -> list[str]:
    tool_bin = args.tool_bin or "grep"
    cmd = [tool_bin, "-R"]
    if args.ignore_case:
        cmd.append("-i")
    if args.fixed_strings:
        cmd.append("-F")
    if args.extended_regexp:
        cmd.append("-E")
    if args.word_regexp:
        cmd.append("-w")
    for pattern in args.exclude_dir:
        cmd.extend(["--exclude-dir", pattern])
    for pattern in args.exclude:
        cmd.extend(["--exclude", pattern])
    for pattern in args.include:
        cmd.extend(["--include", pattern])
    cmd.extend(args.tool_arg)
    cmd.extend([args.pattern, str(root)])
    return cmd


def build_rg_cmd(args: argparse.Namespace, root: Path) -> list[str]:
    tool_bin = args.tool_bin or "rg"
    cmd = [tool_bin, "-n"]
    if args.ignore_case:
        cmd.append("-i")
    if args.fixed_strings:
        cmd.append("-F")
    if args.extended_regexp:
        cmd.append("-E")
    if args.word_regexp:
        cmd.append("-w")

    # For apples-to-apples comparisons with recursive grep, include hidden files
    # and optionally disable ignore files when requested.
    if args.hidden or args.tool == "rg":
        cmd.append("--hidden")
    if args.no_ignore or args.tool == "rg":
        cmd.append("--no-ignore")

    for pattern in args.exclude_dir:
        normalized = pattern.rstrip("/")
        cmd.extend(["--glob", f"!{normalized}/**"])
    for pattern in args.exclude:
        cmd.extend(["--glob", f"!{pattern}"])
    for pattern in args.include:
        cmd.extend(["--glob", pattern])
    cmd.extend(args.tool_arg)
    cmd.extend([args.pattern, str(root)])
    return cmd


def build_search_cmd(args: argparse.Namespace, root: Path) -> list[str]:
    if args.tool == "grep":
        return build_grep_cmd(args, root)
    return build_rg_cmd(args, root)


def run_once(cmd: list[str]) -> RunResult:
    started = time.perf_counter()
    proc = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=False,
    )

    match_lines = 0
    assert proc.stdout is not None
    for _ in proc.stdout:
        match_lines += 1
    proc.stdout.close()

    stderr_bytes = b""
    if proc.stderr is not None:
        stderr_bytes = proc.stderr.read()
        proc.stderr.close()

    exit_code = proc.wait()
    elapsed_ms = (time.perf_counter() - started) * 1000.0
    stderr = stderr_bytes.decode("utf-8", errors="replace").strip()

    if exit_code not in (0, 1):
        raise RuntimeError(
            f"search tool failed with exit code {exit_code}: {' '.join(cmd)}\n{stderr or '(no stderr)'}"
        )

    return RunResult(
        elapsed_ms=elapsed_ms,
        match_lines=match_lines,
        exit_code=exit_code,
        stderr=stderr,
    )


def target_order(round_index: int, targets: list[Target]) -> list[Target]:
    if round_index % 2 == 0:
        return list(targets)
    return list(reversed(targets))


def summarize(results: list[RunResult]) -> tuple[float, float, float]:
    samples = [result.elapsed_ms for result in results]
    return statistics.median(samples), min(samples), max(samples)


def format_match_summary(results: list[RunResult]) -> str:
    counts = sorted({result.match_lines for result in results})
    if len(counts) == 1:
        return str(counts[0])
    return f"{counts[0]}..{counts[-1]}"


def format_exit_summary(results: list[RunResult]) -> str:
    codes = sorted({result.exit_code for result in results})
    if len(codes) == 1:
        return str(codes[0])
    return ",".join(str(code) for code in codes)


def main() -> None:
    args = parse_args()
    if args.rounds <= 0:
        raise SystemExit("--rounds must be > 0")
    if args.warmup < 0:
        raise SystemExit("--warmup must be >= 0")

    ensure_binary(args.tool_bin or args.tool)
    mount_dir = ensure_dir(args.mount_dir, "mount")
    archive_dir = ensure_dir(args.archive_dir, "archive")

    targets = [
        Target(label="mounted", path=mount_dir),
        Target(label="archive", path=archive_dir),
    ]
    cmds = {target.label: build_search_cmd(args, target.path) for target in targets}
    results: dict[str, list[RunResult]] = {target.label: [] for target in targets}

    print(f"Tool: {args.tool}")
    print(f"Pattern: {args.pattern!r}")
    print(f"Mounted: {mount_dir}")
    print(f"Archive: {archive_dir}")
    print(f"Rounds: {args.rounds} measured, {args.warmup} warmup")
    print()

    for warmup_round in range(args.warmup):
        for target in target_order(warmup_round, targets):
            run_once(cmds[target.label])

    for round_index in range(args.rounds):
        print(f"Round {round_index + 1}:")
        for target in target_order(round_index, targets):
            result = run_once(cmds[target.label])
            results[target.label].append(result)
            print(
                f"  {target.label:7} {result.elapsed_ms:8.2f} ms"
                f"  matches={result.match_lines}"
                f"  exit={result.exit_code}"
            )
        print()

    mounted_summary = summarize(results["mounted"])
    archive_summary = summarize(results["archive"])
    mounted_median = mounted_summary[0]
    archive_median = archive_summary[0]

    print("Summary:")
    print(
        "  mounted  "
        f"median={mounted_summary[0]:.2f} ms"
        f"  min={mounted_summary[1]:.2f}"
        f"  max={mounted_summary[2]:.2f}"
        f"  matches={format_match_summary(results['mounted'])}"
        f"  exits={format_exit_summary(results['mounted'])}"
    )
    print(
        "  archive  "
        f"median={archive_summary[0]:.2f} ms"
        f"  min={archive_summary[1]:.2f}"
        f"  max={archive_summary[2]:.2f}"
        f"  matches={format_match_summary(results['archive'])}"
        f"  exits={format_exit_summary(results['archive'])}"
    )

    if mounted_median == archive_median:
        print("  median result: tied")
    elif mounted_median == 0 or archive_median == 0:
        faster = "mounted" if mounted_median < archive_median else "archive"
        print(f"  median result: {faster} faster")
    elif mounted_median < archive_median:
        print(f"  median result: mounted faster by {archive_median / mounted_median:.2f}x")
    else:
        print(f"  median result: archive faster by {mounted_median / archive_median:.2f}x")

    mounted_counts = {result.match_lines for result in results["mounted"]}
    archive_counts = {result.match_lines for result in results["archive"]}
    if len(mounted_counts) == 1 and len(archive_counts) == 1:
        mounted_count = next(iter(mounted_counts))
        archive_count = next(iter(archive_counts))
        if mounted_count != archive_count:
            print("  warning: mounted and archive match counts differ")
    else:
        print("  warning: match counts changed across rounds; directories may be mutating")


if __name__ == "__main__":
    main()
