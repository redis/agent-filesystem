package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTopLevelHelpDocumentsAIAgents(t *testing.T) {
	t.Helper()

	withArg0(t, "afs", func() {
		out := stripAnsi(captureStderrText(t, printUsage))
		for _, want := range []string{
			"skill              show or install the packaged AFS skill",
			"AI Agents:",
			"- Run `afs mcp` to expose the MCP server (stdio) to agents.",
			"- `afs skill install` installs the AFS skill into ./.agents/skills/afs.",
			"- Use `afs skill install --global` for ~/.agents/skills/afs.",
			"- `afs --skill` is kept as an alias for `afs skill show`.",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("top-level help missing %q:\n%s", want, out)
			}
		}
	})
}

func TestSkillHelpMatchesInstallContract(t *testing.T) {
	t.Helper()

	withArg0(t, "afs", func() {
		out, err := captureStdout(t, func() error {
			return cmdSkill([]string{"skill", "--help"})
		})
		if err != nil {
			t.Fatalf("cmdSkill(--help) returned error: %v", err)
		}
		for _, want := range []string{
			"Usage: afs skill <show|install> [options]",
			"show                 Print the packaged AFS skill",
			"install              Install into ./.agents/skills/afs",
			"--global             Install into ~/.agents/skills/afs",
			"--yes                Also create the .claude/skills/afs symlink",
			"-f, --force          Replace existing install or symlink",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("skill help missing %q:\n%s", want, out)
			}
		}
	})
}

func TestSkillShowPrintsEmbeddedSkill(t *testing.T) {
	t.Helper()

	out, err := captureStdout(t, func() error {
		return cmdSkill([]string{"skill", "show"})
	})
	if err != nil {
		t.Fatalf("cmdSkill(show) returned error: %v", err)
	}
	for _, want := range []string{
		"AFS Skill (embedded)",
		"name: agent-filesystem",
		"# Agent Filesystem",
		"Use `afs mcp` when the agent can talk over MCP",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("skill show missing %q:\n%s", want, out)
		}
	}
}

func TestEmbeddedAFSSkillMatchesRepoSkill(t *testing.T) {
	t.Helper()

	embedded, err := embeddedAFSSkillContent()
	if err != nil {
		t.Fatalf("embeddedAFSSkillContent() returned error: %v", err)
	}
	repoSkill, err := os.ReadFile(filepath.Join("..", "..", "skills", "agent-filesystem", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(repo skill) returned error: %v", err)
	}
	if embedded != string(repoSkill) {
		t.Fatalf("embedded AFS skill is out of sync with skills/agent-filesystem/SKILL.md")
	}
}

func TestSkillInstallLocalCopiesPackagedSkill(t *testing.T) {
	t.Helper()

	withWorkingDir(t, t.TempDir(), func() {
		out, err := captureStdout(t, func() error {
			return cmdSkill([]string{"skill", "install"})
		})
		if err != nil {
			t.Fatalf("cmdSkill(install) returned error: %v", err)
		}
		installPath := filepath.Join(".agents", "skills", "afs", "SKILL.md")
		content, err := os.ReadFile(installPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) returned error: %v", installPath, err)
		}
		if !strings.Contains(string(content), "# Agent Filesystem") {
			t.Fatalf("installed skill content = %q, want Agent Filesystem skill", string(content))
		}
		for _, want := range []string{
			"Installed AFS skill to",
			filepath.Join(".agents", "skills", "afs"),
			"Tip: create a Claude symlink manually",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("install output missing %q:\n%s", want, out)
			}
		}
	})
}

func TestSkillInstallForceAndClaudeSymlink(t *testing.T) {
	t.Helper()

	withWorkingDir(t, t.TempDir(), func() {
		if _, err := captureStdout(t, func() error {
			return cmdSkill([]string{"skill", "install", "--yes"})
		}); err != nil {
			t.Fatalf("cmdSkill(install --yes) returned error: %v", err)
		}
		if _, err := captureStdout(t, func() error {
			return cmdSkill([]string{"skill", "install", "--yes"})
		}); err == nil || !strings.Contains(err.Error(), "Skill already exists") {
			t.Fatalf("second install error = %v, want existing-skill error", err)
		}
		out, err := captureStdout(t, func() error {
			return cmdSkill([]string{"skill", "install", "--force", "--yes"})
		})
		if err != nil {
			t.Fatalf("cmdSkill(install --force --yes) returned error: %v", err)
		}
		linkPath := filepath.Join(".claude", "skills", "afs")
		info, err := os.Lstat(linkPath)
		if err != nil {
			t.Fatalf("Lstat(%s) returned error: %v", linkPath, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s is not a symlink", linkPath)
		}
		if !strings.Contains(out, "Linked Claude skill at") {
			t.Fatalf("install output missing Claude link message:\n%s", out)
		}
	})
}

func withWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() returned error: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s) returned error: %v", dir, err)
	}
	defer func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore Chdir(%s) returned error: %v", original, err)
		}
	}()
	fn()
}

func withArg0(t *testing.T, arg0 string, fn func()) {
	t.Helper()

	original := os.Args
	os.Args = append([]string{arg0}, os.Args[1:]...)
	defer func() {
		os.Args = original
	}()
	fn()
}
