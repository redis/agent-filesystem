package main

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	afsSkillInstallName = "afs"
	afsSkillEmbedRoot   = "embedded/skills/afs"
	afsSkillFilename    = "SKILL.md"
)

//go:embed embedded/skills/afs/**
var afsSkillFS embed.FS

type afsSkillInstallOptions struct {
	global bool
	force  bool
	yes    bool
}

func cmdSkill(args []string) error {
	bin := filepath.Base(os.Args[0])
	if len(args) == 1 || (len(args) > 1 && (args[1] == "help" || isHelpArg(args[1]))) {
		printSkillUsage(os.Stdout, bin)
		return nil
	}
	if containsHelpArg(args[2:]) {
		printSkillUsage(os.Stdout, bin)
		return nil
	}

	switch args[1] {
	case "show":
		return showAFSSkill()
	case "install":
		opts, err := parseAFSSkillInstallOptions(args[2:])
		if err != nil {
			return err
		}
		return installAFSSkill(opts)
	default:
		return fmt.Errorf("unknown skill subcommand %q\n\nRun '%s skill help' for usage", args[1], bin)
	}
}

func containsHelpArg(args []string) bool {
	for _, arg := range args {
		if isHelpArg(arg) {
			return true
		}
	}
	return false
}

func parseAFSSkillInstallOptions(args []string) (afsSkillInstallOptions, error) {
	var opts afsSkillInstallOptions
	for _, arg := range args {
		switch arg {
		case "--global":
			opts.global = true
		case "-f", "--force":
			opts.force = true
		case "--yes":
			opts.yes = true
		default:
			return opts, fmt.Errorf("unknown skill install option %q\n\n%s", arg, skillUsageText(filepath.Base(os.Args[0])))
		}
	}
	return opts, nil
}

func printSkillUsage(w *os.File, bin string) {
	fmt.Fprint(w, skillUsageText(bin))
}

func skillUsageText(bin string) string {
	return fmt.Sprintf(`Usage: %s skill <show|install> [options]

Commands:
  show                 Print the packaged AFS skill
  install              Install into ./.agents/skills/afs

Options:
  --global             Install into ~/.agents/skills/afs
  --yes                Also create the .claude/skills/afs symlink
  -f, --force          Replace existing install or symlink
`, bin)
}

func showAFSSkill() error {
	content, err := embeddedAFSSkillContent()
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "AFS Skill (embedded)")
	fmt.Fprintln(os.Stdout)
	if strings.HasSuffix(content, "\n") {
		fmt.Fprint(os.Stdout, content)
	} else {
		fmt.Fprintln(os.Stdout, content)
	}
	return nil
}

func installAFSSkill(opts afsSkillInstallOptions) error {
	installDir, err := afsSkillInstallDir(opts.global)
	if err != nil {
		return err
	}
	if err := writeEmbeddedAFSSkill(installDir, opts.force); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Installed AFS skill to %s\n", installDir)

	claudeLinkPath, err := afsClaudeSkillLinkPath(opts.global)
	if err != nil {
		return err
	}
	createLink, err := shouldCreateClaudeSymlink(claudeLinkPath, opts.yes)
	if err != nil {
		return err
	}
	if !createLink {
		return nil
	}
	linked, err := ensureClaudeSymlink(claudeLinkPath, installDir, opts.force)
	if err != nil {
		return err
	}
	if linked {
		fmt.Fprintf(os.Stdout, "Linked Claude skill at %s\n", claudeLinkPath)
	} else {
		fmt.Fprintf(os.Stdout, "Claude already sees the skill via %s\n", filepath.Dir(claudeLinkPath))
	}
	return nil
}

func afsSkillInstallDir(global bool) (string, error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".agents", "skills", afsSkillInstallName), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, ".agents", "skills", afsSkillInstallName), nil
}

func afsClaudeSkillLinkPath(global bool) (string, error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".claude", "skills", afsSkillInstallName), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, ".claude", "skills", afsSkillInstallName), nil
}

func embeddedAFSSkillContent() (string, error) {
	data, err := fs.ReadFile(afsSkillFS, filepath.ToSlash(filepath.Join(afsSkillEmbedRoot, afsSkillFilename)))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeEmbeddedAFSSkill(targetDir string, force bool) error {
	if pathExists(targetDir) {
		if !force {
			return fmt.Errorf("Skill already exists: %s (use --force to replace it)", targetDir)
		}
		if err := removePath(targetDir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(afsSkillFS, afsSkillEmbedRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(afsSkillEmbedRoot, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(targetDir, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		data, err := fs.ReadFile(afsSkillFS, path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o644)
	})
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func removePath(path string) error {
	return os.RemoveAll(path)
}

func shouldCreateClaudeSymlink(linkPath string, autoYes bool) (bool, error) {
	if autoYes {
		return true, nil
	}
	if !isTerminalFile(os.Stdin) || !isTerminalFile(os.Stdout) {
		fmt.Fprintf(os.Stdout, "Tip: create a Claude symlink manually at %s\n", linkPath)
		return false, nil
	}
	fmt.Fprintf(os.Stdout, "Create a symlink in %s? [y/N] ", linkPath)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, err
	}
	normalized := strings.ToLower(strings.TrimSpace(answer))
	return normalized == "y" || normalized == "yes", nil
}

func isTerminalFile(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func ensureClaudeSymlink(linkPath, targetDir string, force bool) (bool, error) {
	parentDir := filepath.Dir(linkPath)
	if pathExists(parentDir) {
		resolvedTargetParent, targetErr := filepath.EvalSymlinks(filepath.Dir(targetDir))
		resolvedLinkParent, linkErr := filepath.EvalSymlinks(parentDir)
		if targetErr == nil && linkErr == nil && resolvedTargetParent == resolvedLinkParent {
			return false, nil
		}
	}

	linkTarget, err := filepath.Rel(parentDir, targetDir)
	if err != nil || linkTarget == "" {
		linkTarget = targetDir
	}
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return false, err
	}
	if pathExists(linkPath) {
		target, readErr := os.Readlink(linkPath)
		if readErr == nil && target == linkTarget {
			return true, nil
		}
		if !force {
			return false, fmt.Errorf("Claude skill path already exists: %s (use --force to replace it)", linkPath)
		}
		if err := removePath(linkPath); err != nil {
			return false, err
		}
	}
	if err := os.Symlink(linkTarget, linkPath); err != nil {
		return false, err
	}
	return true, nil
}
