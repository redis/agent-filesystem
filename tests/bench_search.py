#!/usr/bin/env python3
"""Benchmark redis-qmd BM25 search against naive scan and GNU grep.

Usage:
  python tests/bench_search.py --key myfs --query "error" --mount /path/to/mount
"""

from __future__ import annotations

import argparse
import statistics
import subprocess
import time
from pathlib import Path

import redis


def naive_scan_grep(r: redis.Redis, key: str, needle: str, nocase: bool) -> int:
    pattern = f"rfs:{{{key}}}:inode:*"
    total = 0
    cursor = 0
    if nocase:
        needle = needle.lower()
    while True:
        cursor, keys = r.scan(cursor=cursor, match=pattern, count=500)
        for k in keys:
            h = r.hgetall(k)
            if h.get(b"type") != b"file":
                continue
            content = h.get(b"content", b"").decode("utf-8", errors="ignore")
            if nocase:
                content_cmp = content.lower()
            else:
                content_cmp = content
            for line in content.splitlines():
                test_line = line.lower() if nocase else line
                if needle in test_line:
                    total += 1
        if cursor == 0:
            break
    return total


def timed(fn, rounds: int) -> tuple[float, float]:
    samples = []
    for _ in range(rounds):
        t0 = time.perf_counter()
        fn()
        samples.append((time.perf_counter() - t0) * 1000.0)
    return statistics.median(samples), max(samples)


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--redis", default="127.0.0.1")
    p.add_argument("--port", type=int, default=6379)
    p.add_argument("--db", type=int, default=0)
    p.add_argument("--key", required=True)
    p.add_argument("--query", required=True)
    p.add_argument("--rounds", type=int, default=5)
    p.add_argument("--mount", default="", help="Mounted filesystem path for GNU grep baseline")
    p.add_argument("--nocase", action="store_true")
    args = p.parse_args()

    r = redis.Redis(host=args.redis, port=args.port, db=args.db)

    cmd = [
        "./redis-qmd",
        "--redis",
        f"{args.redis}:{args.port}",
        "--db",
        str(args.db),
        "--key",
        args.key,
        "--limit",
        "200",
        "grep",
        args.query,
    ]
    if args.nocase:
        cmd.insert(-2, "--nocase")

    bm25_median, bm25_pmax = timed(
        lambda: subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL),
        args.rounds,
    )

    naive_median, naive_pmax = timed(
        lambda: naive_scan_grep(r, args.key, args.query, args.nocase), args.rounds
    )

    print("Benchmark (ms)")
    print(f"- redis-qmd BM25: median={bm25_median:.2f} max={bm25_pmax:.2f}")
    print(f"- naive hash scan: median={naive_median:.2f} max={naive_pmax:.2f}")

    if args.mount:
        mount = Path(args.mount)
        if not mount.exists():
            raise SystemExit(f"mount path not found: {mount}")
        grep_cmd = ["grep", "-R", "-n", args.query, str(mount)]
        if args.nocase:
            grep_cmd.insert(2, "-i")
        gnu_median, gnu_pmax = timed(
            lambda: subprocess.run(grep_cmd, check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL),
            args.rounds,
        )
        print(f"- GNU grep -R: median={gnu_median:.2f} max={gnu_pmax:.2f}")


if __name__ == "__main__":
    main()
