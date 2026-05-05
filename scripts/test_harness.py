#!/usr/bin/env python3
"""Interactive catalog and runner for repo tests, benchmarks, and helpers."""

from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import subprocess
import sys
from collections import Counter
from dataclasses import dataclass
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]

GO_TEST_RE = re.compile(r"^func\s+(Test\w+)\s*\(\s*\w+\s+\*testing\.T\s*\)", re.MULTILINE)
GO_BENCH_RE = re.compile(r"^func\s+(Benchmark\w+)\s*\(\s*\w+\s+\*testing\.B\s*\)", re.MULTILINE)
PY_TEST_RE = re.compile(r"^\s*def\s+(test_\w+)\s*\(", re.MULTILINE)
JS_TEST_RE = re.compile(r"\b(?:test|it)\s*\(")
MODULE_NAME_RE = re.compile(r"^\s*module\s+(\S+)\s*$", re.MULTILINE)

CATEGORY_LABELS = {
    "suite": "Suites",
    "go-test": "Go Package Tests",
    "go-benchmark": "Go Benchmarks",
    "ui": "UI Tests",
    "sdk": "SDK Tests",
    "script": "Scripts and Smoke Checks",
}

CATEGORY_ORDER = ["suite", "go-test", "go-benchmark", "ui", "sdk", "script"]
CATEGORY_BADGES = {
    "suite": "ALL",
    "go-test": "GO",
    "go-benchmark": "BENCH",
    "ui": "UI",
    "sdk": "SDK",
    "script": "SCRIPT",
}
CATEGORY_COLORS = {
    "suite": "34",
    "go-test": "32",
    "go-benchmark": "33",
    "ui": "35",
    "sdk": "36",
    "script": "94",
}


@dataclass(frozen=True)
class AnsiTheme:
    enabled: bool = False

    def apply(self, text: str, *codes: str) -> str:
        if not self.enabled or not codes:
            return text
        return f"\033[{';'.join(codes)}m{text}\033[0m"

    def bold(self, text: str) -> str:
        return self.apply(text, "1")

    def dim(self, text: str) -> str:
        return self.apply(text, "2")

    def red(self, text: str) -> str:
        return self.apply(text, "31")

    def green(self, text: str) -> str:
        return self.apply(text, "32")

    def yellow(self, text: str) -> str:
        return self.apply(text, "33")

    def blue(self, text: str) -> str:
        return self.apply(text, "34")

    def magenta(self, text: str) -> str:
        return self.apply(text, "35")

    def cyan(self, text: str) -> str:
        return self.apply(text, "36")

    def bright_black(self, text: str) -> str:
        return self.apply(text, "90")

    def bright_blue(self, text: str) -> str:
        return self.apply(text, "94")

    def heading(self, text: str) -> str:
        return self.apply(text, "1", "96")

    def section(self, text: str) -> str:
        return self.apply(text, "1", "95")

    def title(self, text: str) -> str:
        return self.bold(text)

    def subtitle(self, text: str) -> str:
        return self.bright_black(text)

    def label(self, text: str) -> str:
        return self.apply(text, "1", "36")

    def key(self, text: str) -> str:
        return self.apply(text, "1", "33")

    def command(self, text: str) -> str:
        return self.apply(text, "1", "32")

    def prompt(self, text: str) -> str:
        return self.apply(text, "1", "94")

    def category_badge(self, category: str) -> str:
        badge = CATEGORY_BADGES.get(category, category.upper())
        color = CATEGORY_COLORS.get(category, "37")
        return self.apply(f"[{badge}]", "1", color)


STYLE = AnsiTheme(False)


@dataclass(frozen=True)
class ParamDef:
    key: str
    label: str
    kind: str
    flag: str | None = None
    default: Any = ""
    required: bool = False
    help: str = ""
    choices: tuple[str, ...] = ()


@dataclass(frozen=True)
class Entry:
    id: str
    category: str
    name: str
    description: str
    cwd: Path
    base_cmd: tuple[str, ...]
    source: str
    surface: str
    params: tuple[ParamDef, ...] = ()
    arg_separator: tuple[str, ...] = ()
    allow_extra_args: bool = True
    notes: str = ""
    tags: tuple[str, ...] = ()


@dataclass(frozen=True)
class GoModule:
    root: Path
    relpath: str
    module_name: str


@dataclass(frozen=True)
class GoPackage:
    module: GoModule
    repo_path: str
    target: str
    tests: tuple[str, ...]
    benchmarks: tuple[str, ...]


def text_param(
    key: str,
    label: str,
    *,
    flag: str | None = None,
    default: str = "",
    required: bool = False,
    help: str = "",
) -> ParamDef:
    kind = "positional" if flag is None else "text"
    return ParamDef(key=key, label=label, kind=kind, flag=flag, default=default, required=required, help=help)


def choice_param(
    key: str,
    label: str,
    *,
    flag: str | None,
    choices: tuple[str, ...],
    default: str = "",
    required: bool = False,
    help: str = "",
) -> ParamDef:
    kind = "positional-choice" if flag is None else "choice"
    return ParamDef(
        key=key,
        label=label,
        kind=kind,
        flag=flag,
        default=default,
        required=required,
        help=help,
        choices=choices,
    )


def bool_param(key: str, label: str, *, flag: str, default: bool = False, help: str = "") -> ParamDef:
    return ParamDef(key=key, label=label, kind="bool", flag=flag, default=default, help=help)


def repo_rel(path: Path) -> str:
    return path.relative_to(REPO_ROOT).as_posix()


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8", errors="ignore")


def pluralize(count: int, singular: str, plural: str | None = None) -> str:
    word = singular if count == 1 else (plural or f"{singular}s")
    return f"{count} {word}"


def pretty_case_summary(name: str) -> str:
    match = re.search(r"\((\d+) cases?\)$", name)
    if not match:
        return ""
    count = int(match.group(1))
    return pluralize(count, "case")


def title_from_relpath(relpath: str) -> str:
    basename = Path(relpath).name
    stem = basename
    for suffix in (".test.tsx", ".test.ts", ".test.mjs", ".py", ".sh", ".go"):
        if stem.endswith(suffix):
            stem = stem[: -len(suffix)]
            break
    stem = stem.lstrip("-_")
    parts = [part for part in re.split(r"[-_.]+", stem) if part]
    if not parts:
        return basename
    return " ".join(parts)


def looks_like_path(value: str) -> bool:
    return "/" in value or value.endswith((".sh", ".py", ".go", ".ts", ".tsx", ".mjs"))


def color_policy_enabled(mode: str) -> bool:
    if mode == "always":
        return True
    if mode == "never":
        return False
    if os.environ.get("NO_COLOR"):
        return False
    term = os.environ.get("TERM", "")
    if term.lower() == "dumb":
        return False
    return sys.stdout.isatty()


def set_style(mode: str) -> None:
    global STYLE
    STYLE = AnsiTheme(color_policy_enabled(mode))


def human_summary(entry: Entry) -> tuple[str, str]:
    if entry.category == "ui":
        summary = pretty_case_summary(entry.name)
        return title_from_relpath(entry.source), f"UI test · {entry.source}{(' · ' + summary) if summary else ''}"
    if entry.category == "sdk":
        summary = pretty_case_summary(entry.name)
        return title_from_relpath(entry.source), f"SDK test · {entry.source}{(' · ' + summary) if summary else ''}"
    if entry.category == "script":
        title = title_from_relpath(entry.source) if looks_like_path(entry.name) else entry.name
        return title, f"{entry.source} · {entry.description}"
    if entry.category == "go-test":
        return entry.source, f"Go package test · {entry.description}"
    if entry.category == "go-benchmark":
        return entry.name, f"Go benchmark · {entry.description}"
    if entry.category == "suite":
        return entry.name, entry.description
    return entry.name, entry.description


def parse_module_name(go_mod: Path) -> str:
    match = MODULE_NAME_RE.search(read_text(go_mod))
    if match:
        return match.group(1)
    return go_mod.parent.name


def discover_go_modules() -> list[GoModule]:
    modules: list[GoModule] = []
    for go_mod in sorted(REPO_ROOT.rglob("go.mod")):
        rel = repo_rel(go_mod)
        if rel.startswith(".git/") or "/node_modules/" in f"/{rel}/":
            continue
        module_root = go_mod.parent
        rel_root = "." if module_root == REPO_ROOT else repo_rel(module_root)
        modules.append(GoModule(root=module_root, relpath=rel_root, module_name=parse_module_name(go_mod)))
    modules.sort(key=lambda item: (0 if item.relpath == "." else item.relpath.count("/"), item.relpath))
    return modules


def scan_go_packages(module: GoModule) -> list[GoPackage]:
    packages: list[GoPackage] = []
    skip_dir_names = {".git", "node_modules", "dist"}
    for dirpath, dirnames, filenames in os.walk(module.root):
        current = Path(dirpath)
        if current != module.root and (current / "go.mod").exists():
            dirnames[:] = []
            continue
        dirnames[:] = [
            name
            for name in dirnames
            if name not in skip_dir_names and not (current / name / "go.mod").exists()
        ]
        test_files = sorted(name for name in filenames if name.endswith("_test.go"))
        if not test_files:
            continue
        tests: list[str] = []
        benchmarks: list[str] = []
        for filename in test_files:
            text = read_text(current / filename)
            tests.extend(GO_TEST_RE.findall(text))
            benchmarks.extend(GO_BENCH_RE.findall(text))
        if not tests and not benchmarks:
            continue
        rel_dir = current.relative_to(module.root)
        target = "." if str(rel_dir) == "." else f"./{rel_dir.as_posix()}"
        repo_path = repo_rel(current)
        packages.append(
            GoPackage(
                module=module,
                repo_path=repo_path,
                target=target,
                tests=tuple(sorted(dict.fromkeys(tests))),
                benchmarks=tuple(sorted(dict.fromkeys(benchmarks))),
            )
        )
    packages.sort(key=lambda item: item.repo_path)
    return packages


def module_key(module: GoModule) -> str:
    return "root" if module.relpath == "." else module.relpath


def discover_go_entries(modules: list[GoModule]) -> list[Entry]:
    entries: list[Entry] = []
    for module in modules:
        packages = scan_go_packages(module)
        if not packages:
            continue
        package_count = sum(1 for pkg in packages if pkg.tests)
        benchmark_count = sum(len(pkg.benchmarks) for pkg in packages)
        suite_desc = f"{package_count} package test target(s)"
        if benchmark_count:
            suite_desc += f", {benchmark_count} benchmark(s) available separately"
        suite_name = "Go suite: repo root" if module.relpath == "." else f"Go suite: {module.relpath}"
        entries.append(
            Entry(
                id=f"suite:go:{module_key(module)}",
                category="suite",
                name=suite_name,
                description=suite_desc,
                cwd=module.root,
                base_cmd=("go", "test", "./..."),
                source=f"{module.relpath}/go.mod" if module.relpath != "." else "go.mod",
                surface="go",
                params=(
                    text_param("run", "go test -run regex", flag="-run", help="Blank runs the whole suite."),
                    text_param("count", "go test -count", flag="-count", default="1", help="Use 1 to bypass the test cache."),
                    bool_param("verbose", "Verbose output", flag="-v"),
                    bool_param("race", "Enable the race detector", flag="-race"),
                    bool_param("short", "Use -short", flag="-short"),
                ),
                tags=(module.relpath, module.module_name, "go", "suite"),
            )
        )
        for pkg in packages:
            if pkg.tests:
                entries.append(
                    Entry(
                        id=f"go:test:{module_key(module)}:{pkg.repo_path}",
                        category="go-test",
                        name=f"{pkg.repo_path} ({len(pkg.tests)} test{'s' if len(pkg.tests) != 1 else ''})",
                        description=f"Run package tests in {pkg.repo_path}",
                        cwd=module.root,
                        base_cmd=("go", "test", pkg.target),
                        source=pkg.repo_path,
                        surface="go",
                        params=(
                            text_param("run", "go test -run regex", flag="-run", help="Example: ^TestHTTPBrowseAndRestore$"),
                            text_param("count", "go test -count", flag="-count", default="1"),
                            bool_param("verbose", "Verbose output", flag="-v"),
                            bool_param("race", "Enable the race detector", flag="-race"),
                            bool_param("short", "Use -short", flag="-short"),
                        ),
                        tags=(pkg.repo_path, module.relpath, "go", "test"),
                    )
                )
            if pkg.benchmarks:
                entries.append(
                    Entry(
                        id=f"go:bench:all:{module_key(module)}:{pkg.repo_path}",
                        category="go-benchmark",
                        name=f"{pkg.repo_path} (all {len(pkg.benchmarks)} benchmark{'s' if len(pkg.benchmarks) != 1 else ''})",
                        description=f"Run every Go benchmark in {pkg.repo_path}",
                        cwd=module.root,
                        base_cmd=("go", "test", pkg.target, "-run", "^$", "-bench", ".", "-benchmem"),
                        source=pkg.repo_path,
                        surface="go",
                        params=(
                            text_param("count", "go test -count", flag="-count", default="1"),
                            text_param("benchtime", "go test -benchtime", flag="-benchtime", help="Example: 2s or 10x"),
                            text_param("cpu", "go test -cpu", flag="-cpu", help="Example: 1,2,4"),
                        ),
                        tags=(pkg.repo_path, module.relpath, "go", "benchmark"),
                    )
                )
                for bench in pkg.benchmarks:
                    entries.append(
                        Entry(
                            id=f"go:bench:{module_key(module)}:{pkg.repo_path}:{bench}",
                            category="go-benchmark",
                            name=f"{pkg.repo_path} :: {bench}",
                            description=f"Run only {bench}",
                            cwd=module.root,
                            base_cmd=("go", "test", pkg.target, "-run", "^$", "-bench", f"^{bench}$", "-benchmem"),
                            source=pkg.repo_path,
                            surface="go",
                            params=(
                                text_param("count", "go test -count", flag="-count", default="1"),
                                text_param("benchtime", "go test -benchtime", flag="-benchtime", help="Example: 2s or 10x"),
                                text_param("cpu", "go test -cpu", flag="-cpu", help="Example: 1,2,4"),
                            ),
                            tags=(pkg.repo_path, module.relpath, bench, "go", "benchmark"),
                        )
                    )
    return entries


def count_js_cases(path: Path) -> int:
    return len(JS_TEST_RE.findall(read_text(path)))


def discover_ui_entries() -> list[Entry]:
    ui_root = REPO_ROOT / "ui"
    if not ui_root.exists():
        return []
    files = sorted(ui_root.glob("src/**/*.test.ts")) + sorted(ui_root.glob("src/**/*.test.tsx"))
    unique_files = sorted(dict.fromkeys(files))
    if not unique_files:
        return []
    total_cases = sum(count_js_cases(path) for path in unique_files)
    entries: list[Entry] = [
        Entry(
            id="suite:ui:vitest",
            category="suite",
            name="UI Vitest suite",
            description=f"{len(unique_files)} test file(s), about {total_cases} test case(s)",
            cwd=ui_root,
            base_cmd=("npm", "run", "test"),
            source="ui/package.json",
            surface="ui",
            params=(
                text_param("pattern", "Vitest path or name filter", help="Blank runs the whole UI suite."),
            ),
            arg_separator=("--",),
            tags=("ui", "vitest", "react"),
        )
    ]
    for path in unique_files:
        rel = path.relative_to(ui_root).as_posix()
        case_count = count_js_cases(path)
        entries.append(
            Entry(
                id=f"ui:file:{rel}",
                category="ui",
                name=f"{rel} ({case_count} case{'s' if case_count != 1 else ''})",
                description=f"Run only {rel}",
                cwd=ui_root,
                base_cmd=("npm", "run", "test", "--", rel),
                source=f"ui/{rel}",
                surface="ui",
                tags=(rel, "ui", "vitest"),
            )
        )
    return entries


def count_py_cases(path: Path) -> int:
    return len(PY_TEST_RE.findall(read_text(path)))


def discover_sdk_entries() -> list[Entry]:
    entries: list[Entry] = []

    py_root = REPO_ROOT / "sdk" / "python"
    py_tests = sorted(py_root.glob("tests/test_*.py"))
    if py_tests:
        total_cases = sum(count_py_cases(path) for path in py_tests)
        entries.append(
            Entry(
                id="suite:sdk:python",
                category="suite",
                name="Python SDK pytest suite",
                description=f"{len(py_tests)} test file(s), {total_cases} pytest case(s)",
                cwd=py_root,
                base_cmd=("python3", "-m", "pytest", "tests"),
                source="sdk/python/pyproject.toml",
                surface="sdk-python",
                params=(
                    text_param("keyword", "pytest -k expression", flag="-k"),
                    bool_param("verbose", "Verbose output", flag="-vv", default=True),
                ),
                tags=("sdk", "python", "pytest"),
            )
        )
        for path in py_tests:
            rel = path.relative_to(py_root).as_posix()
            case_count = count_py_cases(path)
            entries.append(
                Entry(
                    id=f"sdk:python:{rel}",
                    category="sdk",
                    name=f"Python SDK :: {rel} ({case_count} case{'s' if case_count != 1 else ''})",
                    description=f"Run only {rel}",
                    cwd=py_root,
                    base_cmd=("python3", "-m", "pytest", rel),
                    source=f"sdk/python/{rel}",
                    surface="sdk-python",
                    params=(bool_param("verbose", "Verbose output", flag="-vv", default=True),),
                    tags=(rel, "sdk", "python", "pytest"),
                )
            )

    ts_root = REPO_ROOT / "sdk" / "typescript"
    ts_tests = sorted(ts_root.glob("test/*.test.mjs"))
    if ts_tests:
        total_cases = sum(count_js_cases(path) for path in ts_tests)
        entries.append(
            Entry(
                id="suite:sdk:typescript",
                category="suite",
                name="TypeScript SDK test suite",
                description=f"{len(ts_tests)} test file(s), about {total_cases} test case(s)",
                cwd=ts_root,
                base_cmd=("npm", "run", "test"),
                source="sdk/typescript/package.json",
                surface="sdk-typescript",
                tags=("sdk", "typescript", "node-test"),
            )
        )

    return entries


def extract_script_summary(path: Path) -> str:
    lines = read_text(path).splitlines()
    for line in lines[:16]:
        stripped = line.strip()
        if not stripped or stripped.startswith("#!"):
            continue
        if stripped.startswith("#"):
            candidate = stripped.lstrip("#").strip()
            lowered = candidate.lower()
            if lowered.endswith(".sh") or lowered.endswith(".py") or lowered.startswith("usage:"):
                continue
            return candidate.rstrip(".")
        if stripped.startswith(('"""', "'''")):
            candidate = stripped.strip('"\' ').strip()
            if candidate:
                return candidate.rstrip(".")
            continue
    return "Runnable helper"


def detect_script_command(relpath: str, full_path: Path) -> tuple[str, ...] | None:
    if full_path.suffix == ".py":
        return ("python3", relpath)
    if full_path.suffix == ".sh":
        return ("bash", relpath)
    first_line = read_text(full_path).splitlines()[:1]
    if not first_line:
        return None
    line = first_line[0]
    if "python" in line:
        return ("python3", relpath)
    if "bash" in line or "sh" in line:
        return ("bash", relpath)
    return None


def curated_script_entries() -> list[Entry]:
    entries: list[Entry] = []

    def add(entry: Entry) -> None:
        entries.append(entry)

    add(
        Entry(
            id="suite:make:test",
            category="suite",
            name="make test",
            description="Repo-level Go unit test target from the Makefile",
            cwd=REPO_ROOT,
            base_cmd=("make", "test"),
            source="Makefile",
            surface="make",
            tags=("make", "go", "suite"),
        )
    )

    add(
        Entry(
            id="script:tests:bench",
            category="script",
            name="Synthetic local vs mounted filesystem benchmark",
            description="Build a synthetic corpus and compare local filesystem ops against a mounted AFS tree",
            cwd=REPO_ROOT / "tests" / "bench",
            base_cmd=("go", "run", "."),
            source="tests/bench/main.go",
            surface="go-program",
            params=(
                text_param("mount", "--mount path", flag="--mount", required=True, help="Path to an active mounted workspace."),
                text_param("rounds", "--rounds", flag="--rounds", default="10"),
                bool_param("keep", "--keep", flag="--keep"),
            ),
            notes="Requires a writable mounted workspace path.",
            tags=("bench", "mount", "filesystem"),
        )
    )

    add(
        Entry(
            id="script:tests:bench-md-workloads",
            category="script",
            name="Markdown workload benchmark",
            description="Markdown-heavy benchmark harness with temporary Redis, corpus generation, grep, and agent-style read workloads",
            cwd=REPO_ROOT / "tests" / "bench_md_workloads",
            base_cmd=("go", "run", "."),
            source="tests/bench_md_workloads/main.go",
            surface="go-program",
            params=(
                text_param("afs-bin", "--afs-bin", flag="--afs-bin"),
                text_param("workspace", "--workspace", flag="--workspace", default="bench-md"),
                text_param("markdown-files", "--markdown-files", flag="--markdown-files", default="4000"),
                text_param("target-bytes", "--target-bytes", flag="--target-bytes", default="8192"),
                text_param("hot-read-files", "--hot-read-files", flag="--hot-read-files", default="96"),
                text_param("rounds", "--rounds", flag="--rounds", default="5"),
                text_param("warmup", "--warmup", flag="--warmup", default="1"),
                text_param("output-dir", "--output-dir", flag="--output-dir", help="Use /tmp/... for durable artifacts."),
                bool_param("keep", "--keep", flag="--keep"),
            ),
            notes="Will start or build temporary dependencies as needed and can write artifacts to /tmp.",
            tags=("bench", "markdown", "redis", "grep"),
        )
    )

    add(
        Entry(
            id="script:tests:bench-afs-grep",
            category="script",
            name="Mounted grep vs afs grep benchmark",
            description="Compare mounted GNU grep against direct afs grep",
            cwd=REPO_ROOT,
            base_cmd=("python3", "tests/bench_afs_grep.py"),
            source="tests/bench_afs_grep.py",
            surface="python-script",
            params=(
                text_param("pattern", "pattern", required=True),
                text_param("workspace", "--workspace", flag="--workspace"),
                text_param("mount-root", "--mount-root", flag="--mount-root"),
                text_param("path", "--path", flag="--path", default="/"),
                text_param("rounds", "--rounds", flag="--rounds", default="5"),
                text_param("warmup", "--warmup", flag="--warmup", default="1"),
                bool_param("ignore-case", "--ignore-case", flag="--ignore-case"),
            ),
            notes="Needs a working afs binary and an accessible mounted workspace root for the grep side of the comparison.",
            tags=("bench", "grep", "afs"),
        )
    )

    add(
        Entry(
            id="script:scripts:bench-compare",
            category="script",
            name="Claude filesystem cold vs warm benchmark",
            description="End-to-end cold AFS, warm AFS, and local mirror comparison",
            cwd=REPO_ROOT,
            base_cmd=("bash", "scripts/bench_compare.sh"),
            source="scripts/bench_compare.sh",
            surface="shell-script",
            params=(
                text_param("rounds", "rounds", default="5"),
                text_param("out-dir", "out-dir", help="Blank lets the script choose /tmp/afs-perf-run-<timestamp>."),
            ),
            notes="Assumes ~/.claude is mounted via AFS and stages /tmp/claude-local as a local mirror.",
            tags=("bench", "afs", "claude"),
        )
    )

    add(
        Entry(
            id="script:scripts:bench-claude-fs",
            category="script",
            name="Claude filesystem micro-benchmark",
            description="CSV benchmark of filesystem operations against a target directory tree",
            cwd=REPO_ROOT,
            base_cmd=("bash", "scripts/bench_claude_fs.sh"),
            source="scripts/bench_claude_fs.sh",
            surface="shell-script",
            params=(
                text_param("target-dir", "target-dir", required=True),
                text_param("rounds", "rounds", default="5"),
                text_param("label", "label", default="bench"),
            ),
            tags=("bench", "filesystem", "csv"),
        )
    )

    add(
        Entry(
            id="script:scripts:compare-grep-times",
            category="script",
            name="Mounted vs archive grep timing",
            description="Compare recursive grep or ripgrep timings for mounted and archive Claude directories",
            cwd=REPO_ROOT,
            base_cmd=("python3", "scripts/compare_grep_times.py"),
            source="scripts/compare_grep_times.py",
            surface="python-script",
            params=(
                text_param("pattern", "pattern", required=True),
                choice_param("tool", "--tool", flag="--tool", choices=("grep", "rg"), default="grep"),
                text_param("rounds", "--rounds", flag="--rounds", default="5"),
                text_param("warmup", "--warmup", flag="--warmup", default="1"),
                bool_param("ignore-case", "--ignore-case", flag="--ignore-case"),
                bool_param("fixed-strings", "--fixed-strings", flag="--fixed-strings"),
            ),
            tags=("bench", "grep", "ripgrep"),
        )
    )

    add(
        Entry(
            id="script:scripts:test-mount-unmount-delete",
            category="script",
            name="Workspace lifecycle exercise",
            description="Interactive workspace lifecycle exerciser for the UI and mount/session behavior",
            cwd=REPO_ROOT,
            base_cmd=("bash", "scripts/test-mount-unmount-delete.sh"),
            source="scripts/test-mount-unmount-delete.sh",
            surface="shell-script",
            params=(
                text_param("count", "workspace count", required=True),
                text_param("prefix", "workspace prefix"),
            ),
            notes="Pauses between phases so you can watch the control-plane UI react.",
            tags=("test", "mount", "workspace", "ui"),
        )
    )

    add(
        Entry(
            id="script:scripts:capture-claude-fs-patterns",
            category="script",
            name="Claude filesystem access capture",
            description="Capture Claude Code filesystem access patterns under ~/.claude using fs_usage",
            cwd=REPO_ROOT,
            base_cmd=("bash", "scripts/capture_claude_fs_patterns.sh"),
            source="scripts/capture_claude_fs_patterns.sh",
            surface="shell-script",
            params=(
                text_param("out-dir", "out-dir", help="Blank uses /tmp/afs-perf-capture-<timestamp>."),
                text_param("prompt", "prompt", help="Optional canned Claude prompt to replay."),
            ),
            notes="macOS-only and effectively requires sudo because fs_usage needs elevated access.",
            tags=("capture", "claude", "filesystem"),
        )
    )

    add(
        Entry(
            id="script:deploy:vercel-smoke",
            category="script",
            name="Vercel preview smoke check",
            description="Protected-preview smoke check for a Vercel deployment",
            cwd=REPO_ROOT,
            base_cmd=("bash", "deploy/vercel/smoke.sh"),
            source="deploy/vercel/smoke.sh",
            surface="shell-script",
            params=(
                text_param("deployment", "deployment URL", required=True),
                text_param("scope", "--scope", flag="--scope"),
            ),
            tags=("smoke", "vercel", "deploy"),
        )
    )

    return entries


def generic_script_entries(curated_ids: set[str], curated_sources: set[str]) -> list[Entry]:
    entries: list[Entry] = []
    for root_name in ("scripts", "tests", "deploy"):
        root = REPO_ROOT / root_name
        if not root.exists():
            continue
        for path in sorted(root.rglob("*")):
            if not path.is_file():
                continue
            rel = repo_rel(path)
            if rel in curated_sources or rel == "scripts/test_harness.py":
                continue
            if path.suffix in {".go", ".ts", ".tsx", ".md", ".json"}:
                continue
            if not re.search(r"(?:^|[-_/])(bench|test|smoke|capture)", rel):
                continue
            command = detect_script_command(rel, path)
            if command is None:
                continue
            entry_id = f"script:auto:{rel}"
            if entry_id in curated_ids:
                continue
            entries.append(
                Entry(
                    id=entry_id,
                    category="script",
                    name=rel,
                    description=extract_script_summary(path),
                    cwd=REPO_ROOT,
                    base_cmd=command,
                    source=rel,
                    surface="script",
                    tags=(rel, "auto-script"),
                )
            )
    return entries


def discover_entries() -> list[Entry]:
    modules = discover_go_modules()
    entries = []
    entries.extend(curated_script_entries())
    entries.extend(discover_go_entries(modules))
    entries.extend(discover_ui_entries())
    entries.extend(discover_sdk_entries())
    curated_ids = {entry.id for entry in entries}
    curated_sources = {entry.source for entry in entries}
    entries.extend(generic_script_entries(curated_ids, curated_sources))
    return sort_entries(dedupe_entries(entries))


def dedupe_entries(entries: list[Entry]) -> list[Entry]:
    seen: dict[str, Entry] = {}
    for entry in entries:
        seen[entry.id] = entry
    return list(seen.values())


def sort_entries(entries: list[Entry]) -> list[Entry]:
    order = {name: index for index, name in enumerate(CATEGORY_ORDER)}
    return sorted(entries, key=lambda entry: (order.get(entry.category, 999), entry.name.lower(), entry.id))


def parse_bool(value: Any) -> bool:
    if isinstance(value, bool):
        return value
    text = str(value).strip().lower()
    if text in {"1", "true", "t", "yes", "y", "on"}:
        return True
    if text in {"0", "false", "f", "no", "n", "off", ""}:
        return False
    raise ValueError(f"cannot parse boolean value: {value!r}")


def build_dynamic_args(entry: Entry, values: dict[str, Any]) -> list[str]:
    args: list[str] = []
    for param in entry.params:
        value = values.get(param.key, param.default)
        if param.kind == "bool":
            if parse_bool(value):
                args.append(param.flag or "")
            continue
        text = "" if value is None else str(value).strip()
        if not text:
            if param.required:
                raise ValueError(f"missing required parameter: {param.key}")
            continue
        if param.choices and text not in param.choices:
            raise ValueError(f"{param.key} must be one of: {', '.join(param.choices)}")
        if param.flag:
            args.extend([param.flag, text])
        else:
            args.append(text)
    return [arg for arg in args if arg]


def build_command(entry: Entry, values: dict[str, Any], extra_args: str = "") -> list[str]:
    dynamic_args = build_dynamic_args(entry, values)
    extras = shlex.split(extra_args) if extra_args.strip() else []
    command = list(entry.base_cmd)
    if entry.arg_separator and (dynamic_args or extras):
        command.extend(entry.arg_separator)
    command.extend(dynamic_args)
    command.extend(extras)
    return command


def shell_join(command: list[str]) -> str:
    return shlex.join(command)


def format_default(value: Any) -> str:
    if isinstance(value, bool):
        return "yes" if value else "no"
    return str(value)


def prompt_for_values(entry: Entry) -> tuple[dict[str, Any], str]:
    values: dict[str, Any] = {}
    print()
    print(STYLE.heading(f"Selected: {entry.name}"))
    print(f"{STYLE.label('Run key:')} {STYLE.key(entry.id)}")
    print(f"{STYLE.label('Category:')} {STYLE.category_badge(entry.category)} {CATEGORY_LABELS.get(entry.category, entry.category)}")
    print(f"{STYLE.label('Source:')} {entry.source}")
    print(f"{STYLE.label('CWD:')} {repo_rel(entry.cwd) if entry.cwd != REPO_ROOT else '.'}")
    print(f"{STYLE.label('Description:')} {entry.description}")
    if entry.notes:
        print(f"{STYLE.label('Notes:')} {STYLE.subtitle(entry.notes)}")
    print(f"{STYLE.label('Base command:')} {STYLE.command(shell_join(list(entry.base_cmd)))}")
    for param in entry.params:
        if param.help:
            print(f"{STYLE.dim('-')} {STYLE.label(param.label + ':')} {STYLE.subtitle(param.help)}")
        if param.kind == "bool":
            default = bool(param.default)
            raw = input(STYLE.prompt(f"{param.label}? [{'Y/n' if default else 'y/N'}] ")).strip()
            values[param.key] = default if raw == "" else parse_bool(raw)
            continue
        default_text = ""
        if param.default not in ("", None):
            default_text = f" [{format_default(param.default)}]"
        raw = input(STYLE.prompt(f"{param.label}{default_text}: ")).strip()
        if raw == "":
            raw = "" if param.default is None else str(param.default)
        values[param.key] = raw
    extra = ""
    if entry.allow_extra_args:
        extra = input(STYLE.prompt("Extra raw args to append (optional): ")).strip()
    return values, extra


def run_command(entry: Entry, command: list[str]) -> int:
    print()
    print(f"{STYLE.label('CWD:')} {repo_rel(entry.cwd) if entry.cwd != REPO_ROOT else '.'}")
    print(f"{STYLE.label('Running:')} {STYLE.command(shell_join(command))}")
    print()
    proc = subprocess.Popen(command, cwd=str(entry.cwd))
    return proc.wait()


def counts_by_category(entries: list[Entry]) -> Counter[str]:
    return Counter(entry.category for entry in entries)


def print_catalog(entries: list[Entry]) -> None:
    counts = counts_by_category(entries)
    print(STYLE.heading(f"Indexed {len(entries)} runnable entries"))
    for category in CATEGORY_ORDER:
        count = counts.get(category, 0)
        if count:
            print(f"{STYLE.dim('-')} {STYLE.category_badge(category)} {CATEGORY_LABELS[category]}: {STYLE.bold(str(count))}")
    print()
    grouped = {category: [entry for entry in entries if entry.category == category] for category in CATEGORY_ORDER}
    for category in CATEGORY_ORDER:
        category_entries = grouped.get(category, [])
        if not category_entries:
            continue
        print(STYLE.section(CATEGORY_LABELS[category]))
        for entry in category_entries:
            title, subtitle = human_summary(entry)
            print(f"{STYLE.dim('-')} {STYLE.category_badge(entry.category)} {STYLE.title(title)}")
            print(f"  {STYLE.subtitle(subtitle)}")
            print(f"  {STYLE.label('run key:')} {STYLE.key(entry.id)}")
        print()


def entry_to_json(entry: Entry) -> dict[str, Any]:
    defaults = {param.key: param.default for param in entry.params}
    default_command: list[str] | None
    try:
        default_command = build_command(entry, defaults)
    except ValueError:
        default_command = None
    return {
        "id": entry.id,
        "category": entry.category,
        "category_label": CATEGORY_LABELS.get(entry.category, entry.category),
        "name": entry.name,
        "description": entry.description,
        "cwd": str(entry.cwd),
        "source": entry.source,
        "surface": entry.surface,
        "notes": entry.notes,
        "base_command": list(entry.base_cmd),
        "default_command": default_command,
        "params": [
            {
                "key": param.key,
                "label": param.label,
                "kind": param.kind,
                "flag": param.flag,
                "default": param.default,
                "required": param.required,
                "help": param.help,
                "choices": list(param.choices),
            }
            for param in entry.params
        ],
        "tags": list(entry.tags),
    }


def search_entries(entries: list[Entry], term: str) -> list[Entry]:
    needle = term.strip().lower()
    if not needle:
        return entries
    return [
        entry
        for entry in entries
        if needle in entry.id.lower()
        or needle in entry.name.lower()
        or needle in entry.description.lower()
        or needle in entry.source.lower()
        or any(needle in tag.lower() for tag in entry.tags)
    ]


def print_entry_list(entries: list[Entry], title: str) -> None:
    print()
    print(STYLE.section(title))
    print(STYLE.subtitle(f"Showing {len(entries)} entry(s)"))
    for index, entry in enumerate(entries, start=1):
        heading, subtitle = human_summary(entry)
        print(f"{STYLE.key(f'{index:>3}.')} {STYLE.category_badge(entry.category)} {STYLE.title(heading)}")
        print(f"     {STYLE.subtitle(subtitle)}")


def browse_entries(entries: list[Entry], title: str) -> str | None:
    current = entries
    while True:
        print_entry_list(current, title)
        raw = input(STYLE.prompt("Choose a number, /search text, b back, or q quit: ")).strip()
        if raw == "b":
            return None
        if raw == "q":
            return "quit"
        if raw.startswith("/"):
            term = raw[1:].strip()
            current = search_entries(entries, term)
            continue
        if not raw.isdigit():
            print(STYLE.red("Please enter a number, /search text, b, or q."))
            continue
        index = int(raw)
        if index < 1 or index > len(current):
            print(STYLE.red("That selection is out of range."))
            continue
        entry = current[index - 1]
        try:
            values, extra = prompt_for_values(entry)
            command = build_command(entry, values, extra)
        except ValueError as exc:
            print(STYLE.red(f"error: {exc}"))
            continue
        print()
        print(f"{STYLE.label('Resolved command:')} {STYLE.command(shell_join(command))}")
        confirm = input(STYLE.prompt("Run it? [Y/n] ")).strip().lower()
        if confirm in {"", "y", "yes"}:
            rc = run_command(entry, command)
            print()
            exit_text = STYLE.green(str(rc)) if rc == 0 else STYLE.red(str(rc))
            print(f"{STYLE.label('Exit code:')} {exit_text}")
            input(STYLE.prompt("Press Enter to return to the list..."))


def interactive(entries: list[Entry]) -> int:
    current_entries = entries
    while True:
        counts = counts_by_category(current_entries)
        print()
        print(STYLE.heading(f"Indexed {len(current_entries)} runnable entries"))
        for index, category in enumerate(CATEGORY_ORDER, start=1):
            count = counts.get(category, 0)
            print(
                f"{STYLE.key(f'{index}.')} "
                f"{STYLE.category_badge(category)} "
                f"{STYLE.title(CATEGORY_LABELS[category])} "
                f"{STYLE.subtitle(f'({count})')}"
            )
        print(f"{STYLE.key('a.')} {STYLE.title('All entries')}")
        print(f"{STYLE.key('/')} {STYLE.subtitle('text search across the full catalog')}")
        print(f"{STYLE.key('r.')} {STYLE.subtitle('Refresh the catalog')}")
        print(f"{STYLE.key('q.')} {STYLE.subtitle('Quit')}")
        raw = input(STYLE.prompt("Choose a category or command: ")).strip()
        if raw == "q":
            return 0
        if raw == "r":
            current_entries = discover_entries()
            continue
        if raw == "a":
            result = browse_entries(current_entries, "All entries")
            if result == "quit":
                return 0
            continue
        if raw.startswith("/"):
            result = browse_entries(search_entries(current_entries, raw[1:]), f"Search results for {raw[1:].strip()!r}")
            if result == "quit":
                return 0
            continue
        if not raw.isdigit():
            print(STYLE.red("Please enter a category number, a, /search text, r, or q."))
            continue
        choice = int(raw)
        if choice < 1 or choice > len(CATEGORY_ORDER):
            print(STYLE.red("That category number is out of range."))
            continue
        category = CATEGORY_ORDER[choice - 1]
        filtered = [entry for entry in current_entries if entry.category == category]
        if not filtered:
            print(STYLE.red("That category is currently empty."))
            continue
        result = browse_entries(filtered, CATEGORY_LABELS[category])
        if result == "quit":
            return 0


def find_entry(entries: list[Entry], selector: str) -> Entry:
    exact = [entry for entry in entries if entry.id == selector]
    if len(exact) == 1:
        return exact[0]
    matches = search_entries(entries, selector)
    if not matches:
        raise ValueError(f"no entry matches {selector!r}")
    if len(matches) > 1:
        names = ", ".join(entry.id for entry in matches[:5])
        extra = "" if len(matches) <= 5 else f", and {len(matches) - 5} more"
        raise ValueError(f"{selector!r} matched multiple entries: {names}{extra}")
    return matches[0]


def parse_set_args(raw_sets: list[str]) -> dict[str, Any]:
    values: dict[str, Any] = {}
    for raw in raw_sets:
        if "=" not in raw:
            raise ValueError(f"--set expects KEY=VALUE, got {raw!r}")
        key, value = raw.split("=", 1)
        key = key.strip()
        if not key:
            raise ValueError(f"empty parameter name in {raw!r}")
        values[key] = value
    return values


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Interactive test and benchmark catalog for this repo.")
    parser.add_argument("--list", action="store_true", help="print the catalog in a human-readable form and exit")
    parser.add_argument("--json", action="store_true", help="print the catalog as JSON and exit")
    parser.add_argument("--run", metavar="ENTRY", help="run an entry by id or unique search term")
    parser.add_argument("--set", action="append", default=[], metavar="KEY=VALUE", help="parameter override for --run")
    parser.add_argument("--extra", default="", help="extra raw args to append when using --run")
    parser.add_argument("--dry-run", action="store_true", help="print the resolved command for --run without executing it")
    parser.add_argument("--color", choices=("auto", "always", "never"), default="auto", help="ANSI color output policy")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    set_style(args.color)
    entries = discover_entries()

    if args.list and args.json:
        print(STYLE.red("error: choose either --list or --json, not both"), file=sys.stderr)
        return 2

    if args.json:
        json.dump([entry_to_json(entry) for entry in entries], sys.stdout, indent=2)
        sys.stdout.write("\n")
        return 0

    if args.list:
        print_catalog(entries)
        return 0

    if args.run:
        try:
            entry = find_entry(entries, args.run)
            values = parse_set_args(args.set)
            command = build_command(entry, values, args.extra)
        except ValueError as exc:
            print(STYLE.red(f"error: {exc}"), file=sys.stderr)
            return 2
        print(STYLE.heading(f"Selected: {entry.name}"))
        print(f"{STYLE.label('Run key:')} {STYLE.key(entry.id)}")
        print(f"{STYLE.label('CWD:')} {repo_rel(entry.cwd) if entry.cwd != REPO_ROOT else '.'}")
        print(f"{STYLE.label('Command:')} {STYLE.command(shell_join(command))}")
        if args.dry_run:
            return 0
        return run_command(entry, command)

    try:
        return interactive(entries)
    except KeyboardInterrupt:
        print()
        return 130


if __name__ == "__main__":
    raise SystemExit(main())
