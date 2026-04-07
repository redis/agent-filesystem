#!/usr/bin/env python3
"""Benchmark mounted GNU grep against direct `afs grep`.

The comparison intentionally uses literal substring matching:
- GNU grep: `grep -R -n -F --binary-files=without-match`
- AFS grep: `afs grep` default literal mode

Examples:
  python3 tests/bench_afs_grep.py "hello"
  python3 tests/bench_afs_grep.py --workspace repo --mount-root /tmp/mnt/repo "disk full"
  python3 tests/bench_afs_grep.py --path /logs -i "error"
"""

from __future__ import annotations

import argparse
import json
from dataclasses import dataclass
from pathlib import Path
import posixpath
import shutil
import shlex
import statistics
import subprocess
import time


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_AFS_BIN = REPO_ROOT / "afs"


@dataclass(frozen=True)
class CommandTarget:
    label: str
    kind: str
    cmd: tuple[str, ...]
    cwd: Path | None


@dataclass(frozen=True)
class OutputSnapshot:
    lines: tuple[str, ...]
    exit_code: int
    stderr: str


@dataclass(frozen=True)
class RunResult:
    elapsed_ms: float
    match_lines: int
    exit_code: int
    stderr: str


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Benchmark mounted GNU grep vs direct `afs grep`.")
    p.add_argument("pattern", help="literal substring to search for")
    p.add_argument("--afs-bin", default=str(DEFAULT_AFS_BIN), help="path to the afs binary")
    p.add_argument("--grep-bin", default="grep", help="path to the grep binary")
    p.add_argument("--workspace", default="", help="workspace name (defaults to current workspace from AFS config)")
    p.add_argument(
        "--mount-root",
        default="",
        help="mounted workspace root for GNU grep; defaults to the configured mountpoint or mountpoint/workspace",
    )
    p.add_argument("--path", default="/", help="AFS path scope to search inside the workspace")
    p.add_argument("--rounds", type=int, default=5, help="number of measured rounds")
    p.add_argument("--warmup", type=int, default=1, help="number of warmup rounds")
    p.add_argument("-i", "--ignore-case", action="store_true", help="case-insensitive matching")
    p.add_argument(
        "--sample-diff",
        type=int,
        default=10,
        help="number of mismatch lines to sample when outputs differ",
    )
    return p.parse_args()


def resolve_executable(raw: str, label: str) -> str:
    candidate = Path(raw).expanduser()
    if "/" in raw or raw.startswith("."):
        candidate = candidate if candidate.is_absolute() else (Path.cwd() / candidate)
        candidate = candidate.resolve()
        if not candidate.exists():
            raise SystemExit(f"{label} binary not found: {candidate}")
        return str(candidate)

    found = shutil.which(raw)
    if found is None:
        raise SystemExit(f"{label} binary not found in PATH: {raw}")
    return found


def load_afs_config(afs_bin: str) -> dict[str, object]:
    cp = subprocess.run(
        [afs_bin, "config", "show", "--json"],
        check=False,
        text=True,
        capture_output=True,
    )
    if cp.returncode != 0:
        stderr = cp.stderr.strip() or "(no stderr)"
        raise SystemExit(
            "failed to read AFS config via `afs config show --json`\n"
            f"command: {afs_bin} config show --json\n"
            f"stderr: {stderr}"
        )
    try:
        return json.loads(cp.stdout)
    except json.JSONDecodeError as exc:
        raise SystemExit(f"failed to parse AFS config JSON: {exc}") from exc


def normalize_afs_path(raw: str) -> str:
    raw = raw.strip() or "/"
    if not raw.startswith("/"):
        raw = "/" + raw
    clean = posixpath.normpath(raw)
    if clean == ".":
        return "/"
    if not clean.startswith("/"):
        clean = "/" + clean
    return clean


def resolve_workspace_and_mount(args: argparse.Namespace, afs_bin: str) -> tuple[str, Path]:
    cfg = load_afs_config(afs_bin)

    workspace = args.workspace.strip() or str(cfg.get("currentWorkspace") or "").strip()
    if not workspace:
        raise SystemExit("workspace is required; pass --workspace or select a current workspace in AFS config")

    if args.mount_root:
        mount_root = Path(args.mount_root).expanduser().resolve()
    else:
        mountpoint_raw = str(cfg.get("mountpoint") or "").strip()
        if not mountpoint_raw:
            raise SystemExit("mount root is required; pass --mount-root or configure a mountpoint in AFS")
        mountpoint = Path(mountpoint_raw).expanduser().resolve()
        workspace_candidate = mountpoint / workspace
        mount_root = workspace_candidate if workspace_candidate.exists() else mountpoint

    if not mount_root.exists():
        raise SystemExit(f"mount root does not exist: {mount_root}")
    if not mount_root.is_dir():
        raise SystemExit(f"mount root is not a directory: {mount_root}")
    return workspace, mount_root


def grep_scope_arg(afs_path: str) -> str:
    if afs_path == "/":
        return "."
    return "." + afs_path


def build_grep_target(grep_bin: str, mount_root: Path, afs_path: str, pattern: str, ignore_case: bool) -> CommandTarget:
    cmd = [
        grep_bin,
        "-R",
        "-n",
        "-F",
        "--binary-files=without-match",
    ]
    if ignore_case:
        cmd.append("-i")
    cmd.extend([pattern, grep_scope_arg(afs_path)])
    return CommandTarget(label="grep", kind="grep", cmd=tuple(cmd), cwd=mount_root)


def build_afs_target(afs_bin: str, workspace: str, afs_path: str, pattern: str, ignore_case: bool) -> CommandTarget:
    cmd = [
        afs_bin,
        "grep",
        "--workspace",
        workspace,
        "--path",
        afs_path,
    ]
    if ignore_case:
        cmd.append("-i")
    cmd.append(pattern)
    return CommandTarget(label="afs grep", kind="afs", cmd=tuple(cmd), cwd=None)


def normalize_output_line(kind: str, raw: str) -> str | None:
    line = raw.rstrip("\n")
    if not line:
        return None

    if kind == "afs":
        if line.endswith(":Binary file matches"):
            return None
        return line

    parts = line.split(":", 2)
    if len(parts) < 3:
        return line

    rel_path, line_no, content = parts
    if rel_path == ".":
        norm_path = "/"
    else:
        if rel_path.startswith("./"):
            rel_path = rel_path[1:]
        if not rel_path.startswith("/"):
            rel_path = "/" + rel_path.lstrip("/")
        norm_path = rel_path
    return f"{norm_path}:{line_no}:{content}"


def collect_output(target: CommandTarget) -> OutputSnapshot:
    cp = subprocess.run(
        list(target.cmd),
        check=False,
        text=True,
        capture_output=True,
        cwd=str(target.cwd) if target.cwd else None,
    )
    if cp.returncode not in (0, 1):
        stderr = cp.stderr.strip() or "(no stderr)"
        raise RuntimeError(f"{target.label} failed with exit code {cp.returncode}\n{stderr}")

    lines = []
    for raw in cp.stdout.splitlines():
        normalized = normalize_output_line(target.kind, raw)
        if normalized is not None:
            lines.append(normalized)

    return OutputSnapshot(
        lines=tuple(sorted(lines)),
        exit_code=cp.returncode,
        stderr=cp.stderr.strip(),
    )


def run_once(target: CommandTarget) -> RunResult:
    started = time.perf_counter()
    proc = subprocess.Popen(
        list(target.cmd),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        cwd=str(target.cwd) if target.cwd else None,
    )

    assert proc.stdout is not None
    match_lines = 0
    for raw in proc.stdout:
        normalized = normalize_output_line(target.kind, raw)
        if normalized is not None:
            match_lines += 1
    proc.stdout.close()

    stderr = ""
    if proc.stderr is not None:
        stderr = proc.stderr.read().strip()
        proc.stderr.close()

    exit_code = proc.wait()
    if exit_code not in (0, 1):
        raise RuntimeError(f"{target.label} failed with exit code {exit_code}\n{stderr or '(no stderr)'}")

    return RunResult(
        elapsed_ms=(time.perf_counter() - started) * 1000.0,
        match_lines=match_lines,
        exit_code=exit_code,
        stderr=stderr,
    )


def target_order(round_index: int, targets: list[CommandTarget]) -> list[CommandTarget]:
    if round_index % 2 == 0:
        return list(targets)
    return list(reversed(targets))


def summarize(results: list[RunResult]) -> tuple[float, float, float]:
    samples = [result.elapsed_ms for result in results]
    return statistics.median(samples), min(samples), max(samples)


def format_count_summary(results: list[RunResult]) -> str:
    counts = sorted({result.match_lines for result in results})
    if len(counts) == 1:
        return str(counts[0])
    return f"{counts[0]}..{counts[-1]}"


def print_mismatch_report(
    grep_snapshot: OutputSnapshot,
    afs_snapshot: OutputSnapshot,
    sample_diff: int,
) -> None:
    grep_only = sorted(set(grep_snapshot.lines) - set(afs_snapshot.lines))
    afs_only = sorted(set(afs_snapshot.lines) - set(grep_snapshot.lines))

    print("Output comparison: DIFFERENT")
    print(f"- grep-only lines: {len(grep_only)}")
    print(f"- afs-only lines: {len(afs_only)}")

    if sample_diff > 0 and grep_only:
        print(f"- sample grep-only ({min(sample_diff, len(grep_only))}):")
        for line in grep_only[:sample_diff]:
            print(f"    {line}")
    if sample_diff > 0 and afs_only:
        print(f"- sample afs-only ({min(sample_diff, len(afs_only))}):")
        for line in afs_only[:sample_diff]:
            print(f"    {line}")


def main() -> None:
    args = parse_args()
    if args.rounds <= 0:
        raise SystemExit("--rounds must be > 0")
    if args.warmup < 0:
        raise SystemExit("--warmup must be >= 0")
    if args.sample_diff < 0:
        raise SystemExit("--sample-diff must be >= 0")

    afs_bin = resolve_executable(args.afs_bin, "afs")
    grep_bin = resolve_executable(args.grep_bin, "grep")

    workspace, mount_root = resolve_workspace_and_mount(args, afs_bin)
    afs_path = normalize_afs_path(args.path)
    mount_scope = (mount_root if afs_path == "/" else (mount_root / afs_path.lstrip("/"))).resolve()

    grep_target = build_grep_target(grep_bin, mount_root, afs_path, args.pattern, args.ignore_case)
    afs_target = build_afs_target(afs_bin, workspace, afs_path, args.pattern, args.ignore_case)
    targets = [grep_target, afs_target]

    print("Benchmark: grep vs afs grep")
    print(f"Pattern: {args.pattern!r}")
    print(f"Workspace: {workspace}")
    print(f"AFS path: {afs_path}")
    print(f"Mount root: {mount_root}")
    print(f"Mount scope: {mount_scope}")
    print(f"Rounds: {args.rounds} measured, {args.warmup} warmup")
    print()
    print("Commands:")
    print(f"- grep: {shlex.join(grep_target.cmd)}    (cwd={mount_root})")
    print(f"- afs grep: {shlex.join(afs_target.cmd)}")
    print()

    grep_snapshot = collect_output(grep_target)
    afs_snapshot = collect_output(afs_target)

    print("Validation:")
    print(f"- grep matches: {len(grep_snapshot.lines)} (exit {grep_snapshot.exit_code})")
    print(f"- afs grep matches: {len(afs_snapshot.lines)} (exit {afs_snapshot.exit_code})")
    if grep_snapshot.lines == afs_snapshot.lines:
        print("Output comparison: identical")
    else:
        print_mismatch_report(grep_snapshot, afs_snapshot, args.sample_diff)
    print()

    for warmup_round in range(args.warmup):
        for target in target_order(warmup_round, targets):
            run_once(target)

    results: dict[str, list[RunResult]] = {target.label: [] for target in targets}
    for round_index in range(args.rounds):
        print(f"Round {round_index + 1}:")
        for target in target_order(round_index, targets):
            result = run_once(target)
            results[target.label].append(result)
            print(f"  {target.label:<8} {result.elapsed_ms:9.2f} ms  matches={result.match_lines}")
        print()

    grep_summary = summarize(results["grep"])
    afs_summary = summarize(results["afs grep"])
    grep_median, grep_min, grep_max = grep_summary
    afs_median, afs_min, afs_max = afs_summary

    print("Summary:")
    print(
        f"- grep:     median={grep_median:.2f} ms  min={grep_min:.2f}  max={grep_max:.2f}  "
        f"matches={format_count_summary(results['grep'])}"
    )
    print(
        f"- afs grep: median={afs_median:.2f} ms  min={afs_min:.2f}  max={afs_max:.2f}  "
        f"matches={format_count_summary(results['afs grep'])}"
    )

    if grep_median > 0 and afs_median > 0:
        if grep_median > afs_median:
            print(f"- afs grep speedup: {grep_median / afs_median:.2f}x")
        else:
            print(f"- grep speedup: {afs_median / grep_median:.2f}x")


if __name__ == "__main__":
    main()
