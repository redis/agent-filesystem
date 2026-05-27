#!/usr/bin/env python3
"""Exercise the LiveSkills CLI as an isolated end-to-end surface test."""

from __future__ import annotations

import argparse
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile
import textwrap
import time
from dataclasses import dataclass
from typing import Iterable


class HarnessFailure(Exception):
    pass


@dataclass
class CommandResult:
    args: list[str]
    cwd: pathlib.Path
    status: int
    stdout: str
    stderr: str
    elapsed: float


class UX:
    def __init__(self, verbose: bool) -> None:
        self.verbose = verbose
        self.color = sys.stdout.isatty() and os.environ.get("NO_COLOR", "") == ""
        self.section_count = 0
        self.check_count = 0

    def paint(self, code: str, value: str) -> str:
        if not self.color:
            return value
        return f"\033[{code}m{value}\033[0m"

    def section(self, name: str) -> None:
        self.section_count += 1
        print()
        print(self.paint("1;36", f"== {name} =="))

    def ok(self, message: str) -> None:
        self.check_count += 1
        print(f"  {self.paint('32', '[ok]')} {message}")

    def info(self, message: str) -> None:
        print(f"  {self.paint('2', '--')} {message}")

    def command(self, cmd: Iterable[str], cwd: pathlib.Path) -> None:
        if self.verbose:
            rendered = " ".join(shell_quote(part) for part in cmd)
            print(self.paint("2", f"     cwd={cwd} {rendered}"))

    def fail(self, message: str) -> None:
        print(f"  {self.paint('31', '[fail]')} {message}", file=sys.stderr)


class LiveSkillsHarness:
    def __init__(self, args: argparse.Namespace) -> None:
        self.args = args
        self.repo = pathlib.Path(__file__).resolve().parents[1]
        self.ux = UX(args.verbose)
        self.root = pathlib.Path(tempfile.mkdtemp(prefix="liveskills-surface-")).resolve()
        self.project = self.root / "project"
        self.home = self.root / "home"
        self.store = self.root / "store"
        self.sources = self.root / "sources"
        self.downloads = self.root / "downloads"
        self.bin_dir = self.root / "bin"
        self.binary = pathlib.Path(args.binary).resolve() if args.binary else self.bin_dir / "liveskills"
        self.commands: list[CommandResult] = []
        self.failed = False
        for path in [self.project, self.home, self.store, self.sources, self.downloads, self.bin_dir]:
            path.mkdir(parents=True, exist_ok=True)

    def env(self, extra: dict[str, str] | None = None) -> dict[str, str]:
        env = os.environ.copy()
        env.update(
            {
                "HOME": str(self.home),
                "LIVESKILLS_HOME": str(self.store),
                "LIVESKILLS_AFS_MODE": "local",
                "NO_COLOR": "1",
                "TERM": "dumb",
                "USER": "liveskills-e2e",
            }
        )
        if extra:
            env.update(extra)
        return env

    def external(
        self,
        args: list[str],
        cwd: pathlib.Path | None = None,
        expect: int | None = 0,
        timeout: float = 120,
        env_extra: dict[str, str] | None = None,
    ) -> CommandResult:
        cwd = cwd or self.repo
        self.ux.command(args, cwd)
        started = time.monotonic()
        proc = subprocess.run(
            args,
            cwd=str(cwd),
            env=self.env(env_extra),
            text=True,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=timeout,
        )
        result = CommandResult(
            args=args,
            cwd=cwd,
            status=proc.returncode,
            stdout=proc.stdout,
            stderr=proc.stderr,
            elapsed=time.monotonic() - started,
        )
        self.commands.append(result)
        if expect is not None and result.status != expect:
            raise HarnessFailure(format_command_failure(result, expect))
        return result

    def cli(
        self,
        *args: str,
        cwd: pathlib.Path | None = None,
        expect: int | None = 0,
        timeout: float = 45,
        env_extra: dict[str, str] | None = None,
    ) -> CommandResult:
        return self.external([str(self.binary), *args], cwd or self.project, expect, timeout, env_extra)

    def cli_json(
        self,
        *args: str,
        cwd: pathlib.Path | None = None,
        expect: int = 0,
        env_extra: dict[str, str] | None = None,
    ) -> object:
        result = self.cli(*args, "--json", cwd=cwd, expect=expect, env_extra=env_extra)
        try:
            return json.loads(result.stdout)
        except json.JSONDecodeError as exc:
            raise HarnessFailure(f"JSON decode failed for {' '.join(args)}: {exc}\n{result.stdout}") from exc

    def require(self, condition: bool, message: str) -> None:
        if not condition:
            raise HarnessFailure(message)

    def require_contains(self, haystack: str, needle: str, context: str) -> None:
        self.require(needle in haystack, f"expected {context} to contain {needle!r}\n{haystack}")

    def target_for(self, payload: dict[str, object], agent: str) -> dict[str, object]:
        for target in payload.get("targets", []):
            if target.get("agent") == agent:
                return target
        raise HarnessFailure(f"missing {agent} target: {payload}")

    def run(self) -> None:
        try:
            self.build_and_unit_test()
            self.help_auth_and_empty_state()
            self.publish_find_show_download()
            self.project_install_update_remove()
            self.global_install_remove()
            self.source_discovery_and_errors()
            self.scan_inventory()
            self.stress_cli_surface()
            self.final_registry_checks()
        except Exception:
            self.failed = True
            raise

    def build_and_unit_test(self) -> None:
        self.ux.section("Build")
        if not self.args.skip_go_test:
            self.external(["go", "test", "./..."], timeout=180)
            self.ux.ok("go test ./...")
        if self.args.binary:
            self.require(self.binary.exists(), f"binary does not exist: {self.binary}")
            self.ux.ok(f"using binary {self.binary}")
            return
        if not self.args.skip_build:
            self.external(["go", "build", "-o", str(self.binary), "."], timeout=180)
        self.require(self.binary.exists(), f"build did not create {self.binary}")
        self.ux.ok(f"built isolated binary at {self.binary}")

    def help_auth_and_empty_state(self) -> None:
        self.ux.section("Help, Auth, Empty State")
        for args in [(), ("help",), ("--help",), ("-h",)]:
            result = self.cli(*args)
            self.require_contains(result.stdout, "LiveSkills", "help")
            self.require_contains(result.stdout, "Usage: liveskills", "help")
            self.require_contains(result.stdout, "Commands:", "help")
        self.ux.ok("help aliases return the compact command screen")

        result = self.cli("list")
        self.require_contains(result.stdout, "No project skills found.", "empty project list")
        self.require_contains(result.stdout, "Try listing global skills with -g", "empty project list")
        self.cli("list", "-g")
        self.ux.ok("empty project/global lists are readable")

        auth = self.cli_json("auth", "login", "--endpoint", "https://registry.test", "--token", "token-123")
        self.require(auth["endpoint"] == "https://registry.test", f"unexpected auth payload: {auth}")
        registry = read_json(self.store / "registry.json")
        self.require(registry["config"]["auth"]["token"] == "token-123", "auth token was not persisted")
        env_auth = self.cli_json(
            "auth",
            "login",
            "--endpoint",
            "https://env-token.test",
            env_extra={"LIVESKILLS_TOKEN": "env-token-456"},
        )
        self.require(env_auth["endpoint"] == "https://env-token.test", f"unexpected env auth payload: {env_auth}")
        registry = read_json(self.store / "registry.json")
        self.require(registry["config"]["auth"]["token"] == "env-token-456", "auth did not use LIVESKILLS_TOKEN fallback")
        self.ux.ok("auth login persists explicit tokens and env-token fallback")

        unknown = self.cli("definitely-not-a-command", expect=1)
        self.require_contains(unknown.stderr, "Unknown command", "unknown-command error")
        self.ux.ok("unknown commands fail cleanly")

    def publish_find_show_download(self) -> None:
        self.ux.section("Publish, Find, Show, Download")
        source = self.write_skill(self.sources / "review", "Review Skill", version="0.1.0")
        payload = self.cli_json("publish", str(source), "--owner", "acme", "--visibility", "public")
        self.require(payload["name"] == "acme/review-skill", f"unexpected publish payload: {payload}")
        self.require(payload["version"] == "0.1.0", f"unexpected version: {payload}")
        self.require(payload["scripts"] == ["scripts/check.js"], f"unexpected scripts: {payload}")
        self.ux.ok("publish registers source metadata, scripts, volume, and checkpoint")

        duplicate = self.cli("publish", str(source), "--owner", "acme", expect=1)
        self.require_contains(duplicate.stderr, "already exists", "duplicate publish error")
        self.ux.ok("duplicate publish is rejected without --allowExisting")

        rows = self.cli_json("find")
        self.require(any(row["name"] == "acme/review-skill" for row in rows), f"find missing skill: {rows}")
        text = self.cli("find", "review")
        self.require_contains(text.stdout, "Ref: acme/review-skill", "find text")
        self.require_contains(text.stdout, "Install: liveskills add acme/review-skill", "find text")
        self.ux.ok("find supports JSON and copyable text output")

        detail = self.cli_json("show", "acme/review-skill")
        self.require(detail["visibility"] == "public", f"unexpected show payload: {detail}")
        self.require(len(detail["versions"]) == 1, f"unexpected show versions: {detail}")
        self.require(detail["volume"] == "skill_acme_review-skill", f"unexpected volume: {detail}")
        self.ux.ok("show exposes registry details")

        output = self.downloads / "review"
        downloaded = self.cli_json("download", "acme/review-skill", "--output", str(output))
        self.require(downloaded["output"] == str(output), f"unexpected download payload: {downloaded}")
        self.require((output / "SKILL.md").exists(), "download did not write SKILL.md")
        self.require(read_text(output / "references" / "notes.md") == "Reference notes for Review Skill.\n", "download missed reference file")
        self.ux.ok("download exports the pinned snapshot")

        self.write_skill(source, "Review Skill", version="0.1.1", overwrite=True)
        next_payload = self.cli_json("publish", str(source), "--owner", "acme", "--version", "0.1.1")
        self.require(next_payload["version"] == "0.1.1", f"unexpected second version: {next_payload}")
        self.ux.ok("second version publish works for update coverage")

    def project_install_update_remove(self) -> None:
        self.ux.section("Project Add, List, Update, Remove")
        manual = self.project / ".agents" / "skills" / "manual-skill"
        self.write_local_skill(manual, "Manual Skill")

        installed = self.cli_json(
            "add",
            "acme/review-skill",
            "--version",
            "0.1.0",
            "--workspace",
            "demo",
            "--mount",
            str(self.project),
        )
        self.require(installed["scope"] == "project", f"unexpected install scope: {installed}")
        self.require(installed["workspace"] == "demo", f"unexpected workspace: {installed}")
        self.require(installed["path"] == "skills/review-skill", f"unexpected canonical path: {installed}")
        target = self.target_for(installed, "Codex")
        self.require(
            target["path"] == str(self.project / ".agents" / "skills" / "review-skill"),
            f"unexpected codex target: {installed}",
        )
        self.require(target["mode"] == "symlink", f"expected symlink target: {installed}")
        self.require((self.project / ".agents" / "skills" / "review-skill" / "SKILL.md").exists(), "project install missing SKILL.md")
        self.require((self.project / ".agents" / "skills" / "review-skill").is_symlink(), "project install should be a symlink")
        self.require((self.project / ".liveskills" / "mount" / "skills" / "review-skill" / "SKILL.md").exists(), "canonical workspace install missing SKILL.md")
        manifest = read_json(self.project / ".liveskills" / "manifest.json")
        self.require(manifest["skills"][0]["version"] == "0.1.0", f"unexpected manifest: {manifest}")
        self.require(manifest["skills"][0]["targets"][0]["agent"] == "codex", f"manifest missing target: {manifest}")
        self.ux.ok("project add installs one skill folder and writes manifest")

        repeat = self.cli_json(
            "add",
            "acme/review-skill",
            "--version",
            "0.1.0",
            "--workspace",
            "demo",
            "--mount",
            str(self.project),
        )
        self.require(repeat["status"] == "unchanged", f"repeat add should be unchanged: {repeat}")
        self.ux.ok("project add is idempotent for the same pinned version")

        rows = self.cli_json("list")
        names = {row["name"] for row in rows}
        self.require("acme/review-skill" in names, f"project list missing live row: {rows}")
        self.require("Manual Skill" in names, f"project list missing neighboring local skill: {rows}")
        text = self.cli("list")
        self.require_contains(text.stdout, "Live Skills (managed by LiveSkills)", "project list text")
        self.require_contains(text.stdout, "Local Skills (not managed by LiveSkills)", "project list text")
        alias_rows = self.cli_json("ls")
        self.require(alias_rows == rows, f"ls alias drifted from list: {alias_rows} != {rows}")
        self.ux.ok("project list separates LiveSkills-managed/local skills and ls aliases list")

        rejected = self.cli("add", "acme/review-skill", "--workspace", "demo", "--mount", str(self.project), expect=1)
        self.require_contains(rejected.stderr, "Use liveskills update", "add-newer-version rejection")
        self.ux.ok("add refuses to replace an installed version")

        updated = self.cli_json("update", "acme/review-skill", "--workspace", "demo", "--version", "0.1.1")
        self.require(updated["version"] == "0.1.1", f"unexpected update payload: {updated}")
        manifest = read_json(self.project / ".liveskills" / "manifest.json")
        self.require(manifest["skills"][0]["version"] == "0.1.1", f"manifest did not update: {manifest}")
        self.ux.ok("update moves an existing install to the requested version")

        removed = self.cli_json("remove", "acme/review-skill", "--workspace", "demo")
        self.require(removed["name"] == "acme/review-skill", f"unexpected remove payload: {removed}")
        self.require(not (self.project / ".agents" / "skills" / "review-skill").exists(), "remove left skill directory behind")
        self.require((manual / "SKILL.md").exists(), "remove touched neighboring local skill")
        manifest = read_json(self.project / ".liveskills" / "manifest.json")
        self.require(manifest["skills"] == [], f"manifest should be empty after remove: {manifest}")
        self.ux.ok("remove cleans the managed skill and preserves neighbors")

        missing = self.cli("remove", "acme/review-skill", "--workspace", "demo", expect=1)
        self.require_contains(missing.stderr, "not installed", "repeat remove error")
        self.ux.ok("remove reports missing installs")

        agent_root = self.root / "agent-project"
        agent_root.mkdir(parents=True, exist_ok=True)
        claude_source = self.write_skill(self.sources / "claude-path", "Claude Path Skill")
        cursor_source = self.write_skill(self.sources / "cursor-path", "Cursor Path Skill")
        self.cli_json("publish", str(claude_source), "--owner", "paths")
        self.cli_json("publish", str(cursor_source), "--owner", "paths")
        claude = self.cli_json("add", "paths/claude-path-skill", "--agent", "claude-code", "--mount", str(agent_root))
        self.require(claude["workspace"] == "agent-project", f"--mount should default workspace to basename: {claude}")
        self.require(claude["path"] == "skills/claude-path-skill", f"unexpected claude canonical path: {claude}")
        claude_target = self.target_for(claude, "Claude Code")
        self.require(
            claude_target["path"] == str(agent_root / ".claude" / "skills" / "claude-path-skill"),
            f"unexpected claude target path: {claude}",
        )
        self.require((agent_root / ".claude" / "skills" / "claude-path-skill" / "SKILL.md").exists(), "project claude-code install missing")
        cursor = self.cli_json("add", "paths/cursor-path-skill", "--agent", "cursor", "--mount", str(agent_root))
        self.require(cursor["path"] == "skills/cursor-path-skill", f"unexpected cursor canonical path: {cursor}")
        cursor_target = self.target_for(cursor, "Cursor")
        self.require(
            cursor_target["path"] == str(agent_root / ".agents" / "skills" / "cursor-path-skill"),
            f"unexpected cursor target path: {cursor}",
        )
        self.require((agent_root / ".agents" / "skills" / "cursor-path-skill" / "SKILL.md").exists(), "project cursor install missing")
        self.cli_json("rm", "paths/claude-path-skill", "--agent", "claude-code", "--workspace", "agent-project", "--mount", str(agent_root))
        self.cli_json("rm", "paths/cursor-path-skill", "--agent", "cursor", "--workspace", "agent-project", "--mount", str(agent_root))
        self.ux.ok("project --agent paths, --mount workspace default, and rm alias work")

    def global_install_remove(self) -> None:
        self.ux.section("Global Installs")
        source = self.write_skill(self.sources / "global", "Global Skill")
        self.cli_json("publish", str(source), "--owner", "acme")

        codex = self.cli_json("add", "-g", "acme/global-skill")
        self.require(codex["scope"] == "global", f"unexpected global add payload: {codex}")
        self.require(codex["listCommand"] == "liveskills list -g", f"unexpected list command: {codex}")
        self.require((self.home / ".codex" / "skills" / "global-skill" / "SKILL.md").exists(), "global codex install missing")
        self.ux.ok("global add defaults to Codex")

        claude = self.cli_json("add", "-g", "acme/global-skill", "--agent", "claude-code")
        self.require(claude["scope"] == "global", f"unexpected claude global payload: {claude}")
        self.require((self.home / ".claude" / "skills" / "global-skill" / "SKILL.md").exists(), "global claude-code install missing")
        cursor = self.cli_json("add", "acme/global-skill", "--global", "--agent", "cursor")
        self.require(cursor["scope"] == "global", f"unexpected cursor global payload: {cursor}")
        self.require((self.home / ".cursor" / "skills" / "global-skill" / "SKILL.md").exists(), "global cursor install missing")
        universal = self.cli_json("add", "acme/global-skill", "--global", "--agent", "universal")
        self.require(universal["scope"] == "global", f"unexpected universal global payload: {universal}")
        self.require((self.home / ".config" / "agents" / "skills" / "global-skill" / "SKILL.md").exists(), "global universal install missing")
        self.ux.ok("global add supports -g, --global, and agent-specific install roots")

        rows = self.cli_json("list", "-g")
        live_rows = [row for row in rows if row["name"] == "acme/global-skill" and row["live"]]
        self.require(len(live_rows) == 1, f"expected one aggregated global live row: {rows}")
        agents = set(live_rows[0]["agents"])
        for expected in ["Codex", "Claude Code", "Cursor", "Universal"]:
            self.require(expected in agents, f"global list missing {expected}: {rows}")
        text = self.cli("list", "-g")
        self.require_contains(text.stdout, "Global Live Skills (managed by LiveSkills)", "global list text")
        self.require_contains(text.stdout, "Scope: global", "global list text")
        self.ux.ok("global list reports aggregated live managed installs")

        self.cli_json("remove", "-g", "acme/global-skill", "--agent", "codex")
        self.require(not (self.home / ".codex" / "skills" / "global-skill").exists(), "codex global remove left files behind")
        self.require((self.home / ".claude" / "skills" / "global-skill").exists(), "codex remove should not remove claude target")
        self.cli_json("remove", "-g", "acme/global-skill", "--agent", "claude-code")
        self.cli_json("rm", "acme/global-skill", "--global", "--agent", "cursor")
        self.cli_json("rm", "acme/global-skill", "--global", "--agent", "universal")
        self.require(not (self.home / ".claude" / "skills" / "global-skill").exists(), "claude global remove left files behind")
        self.require(not (self.home / ".cursor" / "skills" / "global-skill").exists(), "cursor global remove left files behind")
        self.require(not (self.home / ".config" / "agents" / "skills" / "global-skill").exists(), "universal global remove left files behind")
        self.ux.ok("global remove can target individual agents")

    def source_discovery_and_errors(self) -> None:
        self.ux.section("Source Discovery And Errors")
        local_source = self.write_skill(self.sources / "source-only", "Source Only")
        added_source = self.cli_json("add", str(local_source))
        self.require(added_source["name"] == "local/source-only", f"unexpected source add payload: {added_source}")
        self.require((self.project / ".agents" / "skills" / "source-only" / "SKILL.md").exists(), "source add did not install skill")
        self.ux.ok("add local source registers and installs in one step")

        multi = self.sources / "multi"
        self.write_skill(multi / "alpha", "Alpha Source")
        self.write_skill(multi / "beta", "Beta Source")
        source_rows = self.cli_json("add", str(multi), "--list")
        self.require([row["slug"] for row in source_rows] == ["alpha-source", "beta-source"], f"unexpected source list: {source_rows}")
        ambiguous = self.cli("publish", str(multi), "--owner", "team", expect=1)
        self.require_contains(ambiguous.stderr, "Multiple skills", "ambiguous source error")
        published = self.cli_json("publish", str(multi), "--skill", "alpha", "--owner", "team")
        self.require(published["name"] == "team/alpha-source", f"unexpected selected publish: {published}")
        self.ux.ok("multi-skill sources list and require --skill")

        invalid = self.sources / "invalid"
        invalid.mkdir(parents=True, exist_ok=True)
        write_text(invalid / "SKILL.md", "# Missing description\n")
        result = self.cli("publish", str(invalid), "--owner", "team", expect=1)
        self.require_contains(result.stderr, "description", "invalid metadata error")
        self.ux.ok("publish validates required metadata")

        symlink_source = self.write_skill(self.sources / "symlinked", "Symlink Skill")
        try:
            os.symlink(symlink_source / "SKILL.md", symlink_source / "linked.md")
        except (OSError, NotImplementedError):
            self.ux.info("skipping symlink rejection on this filesystem")
        else:
            symlinked = self.cli("publish", str(symlink_source), "--owner", "team", expect=1)
            self.require_contains(symlinked.stderr, "Symlinks are not allowed", "symlink rejection")
            self.ux.ok("publish rejects symlinks inside skill sources")

        unsupported = self.cli("add", "team/alpha-source", "--agent", "unknown-agent", expect=1)
        self.require_contains(unsupported.stderr, "Unsupported agent", "unsupported agent error")
        bad_ref = self.cli("show", "not-a-ref", expect=1)
        self.require_contains(bad_ref.stderr, "Expected skill reference", "bad-ref error")
        bad_visibility = self.cli("publish", str(local_source), "--owner", "team", "--visibility", "friends", expect=1)
        self.require_contains(bad_visibility.stderr, "Visibility must be", "bad-visibility error")
        bad_version = self.cli("download", "team/alpha-source", "--version", "9.9.9", "--output", str(self.downloads / "missing"), expect=1)
        self.require_contains(bad_version.stderr, "version 9.9.9 not found", "bad-version error")
        self.ux.ok("common error paths are actionable")

    def scan_inventory(self) -> None:
        self.ux.section("Scan Inventory")
        self.write_local_skill(self.project / ".claude" / "skills" / "claude-project", "Claude Project")
        self.write_local_skill(self.home / ".codex" / "skills" / "codex-global", "Codex Global")
        self.write_local_skill(self.home / ".cursor" / "skills" / "cursor-global", "Cursor Global")
        self.write_local_skill(self.home / ".config" / "opencode" / "skills" / "open-global", "Open Global")
        self.write_local_skill(self.project / "skills" / ".experimental" / "experimental-project", "Experimental Project")

        rows = self.cli_json("scan")
        names = {row["name"] for row in rows}
        for expected in ["Manual Skill", "Claude Project", "Codex Global", "Cursor Global", "Open Global", "Experimental Project"]:
            self.require(expected in names, f"scan missing {expected}: {rows}")
        self.ux.ok("scan finds project, global, agent, and standard skill folders")

        project_rows = self.cli_json("scan", "-p")
        self.require(all(row["scope"] == "project" for row in project_rows), f"scan -p leaked global rows: {project_rows}")
        global_rows = self.cli_json("scan", "-g")
        self.require(all(row["scope"] == "global" for row in global_rows), f"scan -g leaked project rows: {global_rows}")
        codex_rows = self.cli_json("scan", "--agent", "codex")
        self.require(any(row["name"] == "Codex Global" for row in codex_rows), f"codex scan missing global row: {codex_rows}")
        cursor_rows = self.cli_json("scan", "--agent", "cursor")
        self.require(any(row["name"] == "Cursor Global" for row in cursor_rows), f"cursor scan missing global row: {cursor_rows}")
        self.ux.ok("scan scope and agent filters work")

        text = self.cli("scan")
        self.require_contains(text.stdout, "Summary", "scan text")
        self.require_contains(text.stdout, "Local Skills", "scan text")
        self.require_contains(text.stdout, "Agents:", "scan text")
        self.ux.ok("scan text output includes summary and agent attribution")

    def stress_cli_surface(self) -> None:
        self.ux.section("Stress")
        count = self.args.stress
        self.require(count >= 1, "--stress must be at least 1")
        self.ux.info(f"publishing {count} generated skills and installing {min(count, 12)}")
        refs: list[str] = []
        for index in range(count):
            name = f"Stress Skill {index:03d}"
            source = self.write_skill(self.sources / f"stress-{index:03d}", name)
            if index % 2 == 0:
                payload = self.cli_json("publish", str(source), "--owner=stress", "--visibility=team")
            else:
                payload = self.cli_json("publish", str(source), "--owner", "stress", "--name", name)
            refs.append(payload["name"])

        found = self.cli_json("find", "stress")
        found_refs = {row["name"] for row in found}
        missing = sorted(set(refs) - found_refs)
        self.require(not missing, f"stress find missing refs: {missing}")
        self.ux.ok("bulk publish remains discoverable through find")

        install_count = min(count, 12)
        for ref in refs[:install_count]:
            self.cli_json("add", ref)
        rows = self.cli_json("list")
        installed_refs = {row["name"] for row in rows if row.get("live")}
        missing_installs = sorted(set(refs[:install_count]) - installed_refs)
        self.require(not missing_installs, f"stress installs missing from list: {missing_installs}")
        self.ux.ok("bulk project installs are listed as live")

        repeat = self.cli_json("add", refs[0])
        self.require(repeat["status"] == "unchanged", f"stress repeat add should be unchanged: {repeat}")
        self.ux.ok("bulk install path remains idempotent")

        for ref in refs[:install_count]:
            self.cli_json("remove", ref)
        rows_after_remove = self.cli_json("list")
        remaining = {row["name"] for row in rows_after_remove if row.get("live") and row["name"].startswith("stress/")}
        self.require(not remaining, f"stress removes left live rows: {remaining}")
        self.ux.ok("bulk removes clean up live installs")

        bad_commands = [
            ("publish",),
            ("add",),
            ("show",),
            ("download", "stress/nope"),
            ("remove", "stress/nope"),
            ("update", "stress/nope"),
            ("list", "--all"),
            ("find", "--interactive"),
        ]
        for command in bad_commands:
            self.cli(*command, expect=1)
        self.ux.ok("bad command shapes fail instead of hanging or mutating state")

    def final_registry_checks(self) -> None:
        self.ux.section("Registry And Cleanup")
        registry_path = self.store / "registry.json"
        data = registry_path.read_bytes()
        self.require(data.endswith(b"\n"), "registry.json should end with a newline")
        registry = json.loads(data)
        self.require(registry["version"] == 1, f"unexpected registry version: {registry['version']}")
        self.require(len(registry["skills"]) >= self.args.stress + 4, "registry has fewer skills than expected")
        self.ux.ok("registry JSON stays readable and newline-terminated")
        self.ux.ok(f"{len(self.commands)} subprocess commands completed")

    def write_skill(
        self,
        directory: pathlib.Path,
        name: str,
        version: str = "0.1.0",
        overwrite: bool = False,
    ) -> pathlib.Path:
        if overwrite and directory.exists():
            shutil.rmtree(directory)
        directory.mkdir(parents=True, exist_ok=True)
        (directory / "scripts").mkdir(parents=True, exist_ok=True)
        (directory / "references").mkdir(parents=True, exist_ok=True)
        write_text(
            directory / "SKILL.md",
            textwrap.dedent(
                f"""\
                ---
                name: {name}
                description: {name} description.
                version: {version}
                ---

                # {name}

                {name} body.
                """
            ),
        )
        write_text(directory / "scripts" / "check.js", f"console.log({name!r})\n")
        write_text(directory / "references" / "notes.md", f"Reference notes for {name}.\n")
        return directory

    def write_local_skill(self, directory: pathlib.Path, name: str) -> pathlib.Path:
        directory.mkdir(parents=True, exist_ok=True)
        write_text(
            directory / "SKILL.md",
            textwrap.dedent(
                f"""\
                ---
                name: {name}
                description: {name} description.
                ---

                # {name}
                """
            ),
        )
        return directory

    def cleanup(self) -> None:
        if self.args.keep_temp or self.failed:
            self.ux.info(f"kept temp dir: {self.root}")
            return
        shutil.rmtree(self.root, ignore_errors=True)


def write_text(path: pathlib.Path, value: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(value, encoding="utf-8")


def read_text(path: pathlib.Path) -> str:
    return path.read_text(encoding="utf-8")


def read_json(path: pathlib.Path) -> object:
    return json.loads(read_text(path))


def shell_quote(value: str) -> str:
    if re.fullmatch(r"[A-Za-z0-9_./:=+-]+", value):
        return value
    return "'" + value.replace("'", "'\\''") + "'"


def format_command_failure(result: CommandResult, expected: int | None) -> str:
    rendered = " ".join(shell_quote(part) for part in result.args)
    return textwrap.dedent(
        f"""\
        command failed
          expected: {expected}
          status:   {result.status}
          cwd:      {result.cwd}
          command:  {rendered}
          elapsed:  {result.elapsed:.2f}s
        stdout:
        {indent_block(result.stdout)}
        stderr:
        {indent_block(result.stderr)}
        """
    ).rstrip()


def indent_block(value: str) -> str:
    if value == "":
        return "  <empty>"
    return "\n".join("  " + line for line in value.rstrip("\n").splitlines())


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run an isolated full-surface LiveSkills CLI test.",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    parser.add_argument("--binary", help="Use an existing liveskills binary instead of building one.")
    parser.add_argument("--skip-build", action="store_true", help="Skip go build. Requires --binary or an existing temp binary.")
    parser.add_argument("--skip-go-test", action="store_true", help="Do not run go test ./... before the surface test.")
    parser.add_argument("--stress", type=int, default=16, help="Number of generated skills for stress coverage.")
    parser.add_argument("--keep-temp", action="store_true", help="Keep the isolated temp HOME/project/store after success.")
    parser.add_argument("--verbose", action="store_true", help="Print every command before it runs.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    harness = LiveSkillsHarness(args)
    print("LiveSkills surface test")
    print(f"repo: {harness.repo}")
    print(f"temp: {harness.root}")
    try:
        harness.run()
    except subprocess.TimeoutExpired as exc:
        harness.failed = True
        harness.ux.fail(f"timed out: {exc.cmd}")
        return 1
    except HarnessFailure as exc:
        harness.failed = True
        harness.ux.fail(str(exc))
        return 1
    except Exception as exc:
        harness.failed = True
        harness.ux.fail(f"unexpected error: {exc}")
        return 1
    finally:
        harness.cleanup()

    print()
    print(f"Done: {harness.ux.check_count} checks across {harness.ux.section_count} sections.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
