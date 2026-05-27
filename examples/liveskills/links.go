package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func EnsureRelativeSkillSymlink(agentSkillPath, canonicalSkillPath string) error {
	canonical, err := cleanAbs(canonicalSkillPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fail("Canonical skill path is not a directory: %s", canonicalSkillPath)
	}
	link, err := cleanAbs(agentSkillPath)
	if err != nil {
		return err
	}
	if err := validateOrRepairSymlink(link, canonical); err != nil {
		return err
	}
	return ensureRelativeSymlink(link, canonical)
}

func ensureCanonicalMountAlias(aliasPath, mountedPath string) error {
	mounted, err := cleanAbs(mountedPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(mounted)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fail("Mounted skill path is not a directory: %s", mountedPath)
	}
	alias, err := cleanAbs(aliasPath)
	if err != nil {
		return err
	}
	if filepath.Clean(alias) == filepath.Clean(mounted) {
		return nil
	}
	info, err = os.Lstat(alias)
	if errors.Is(err, os.ErrNotExist) {
		return ensureRelativeSymlink(alias, mounted)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if err := validateOrRepairSymlink(alias, mounted); err != nil {
			return err
		}
		return ensureRelativeSymlink(alias, mounted)
	}
	if !info.IsDir() {
		return fail("Refusing to overwrite unmanaged skill path: %s", alias)
	}
	entries, err := os.ReadDir(alias)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fail("Refusing to overwrite non-empty skill path: %s", alias)
	}
	if err := os.Remove(alias); err != nil {
		return err
	}
	return ensureRelativeSymlink(alias, mounted)
}

func ValidateSkillSymlink(agentSkillPath, canonicalSkillPath string) error {
	link, err := cleanAbs(agentSkillPath)
	if err != nil {
		return err
	}
	canonical, err := cleanAbs(canonicalSkillPath)
	if err != nil {
		return err
	}
	info, err := os.Lstat(link)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fail("Refusing to manage non-symlink skill path: %s", agentSkillPath)
	}
	if !symlinkTargetMatches(link, canonical) {
		return fail("Existing skill symlink points outside the managed LiveSkills target: %s", agentSkillPath)
	}
	return nil
}

func SkillSymlinkPointsTo(agentSkillPath, canonicalSkillPath string) bool {
	link, err := cleanAbs(agentSkillPath)
	if err != nil {
		return false
	}
	canonical, err := cleanAbs(canonicalSkillPath)
	if err != nil {
		return false
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	return symlinkTargetMatches(link, canonical)
}

func CopySkillFromCanonical(canonicalSkillPath, targetDir string) error {
	source, err := cleanAbs(canonicalSkillPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fail("Canonical skill path is not a directory: %s", canonicalSkillPath)
	}
	target, err := cleanAbs(targetDir)
	if err != nil {
		return err
	}
	if err := prepareCopyTarget(source, target); err != nil {
		return err
	}
	return copyTree(source, target, map[string]bool{".afs-version.json": true})
}

func validateOrRepairSymlink(link, canonical string) error {
	info, err := os.Lstat(link)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fail("Refusing to overwrite unmanaged skill path: %s", link)
	}
	if symlinkTargetMatches(link, canonical) {
		current, err := os.Readlink(link)
		if err != nil {
			return err
		}
		expected, err := filepath.Rel(filepath.Dir(link), canonical)
		if err != nil {
			return err
		}
		if current == expected {
			return nil
		}
		return os.Remove(link)
	}
	resolved, err := resolvedSymlinkTarget(link)
	if err != nil {
		return err
	}
	if _, err := os.Stat(resolved); errors.Is(err, os.ErrNotExist) && safeBrokenSkillTarget(resolved, canonical) {
		return os.Remove(link)
	}
	return fail("Refusing to retarget unmanaged skill symlink: %s", link)
}

func ensureRelativeSymlink(link, canonical string) error {
	if _, err := os.Lstat(link); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	relativeTarget, err := filepath.Rel(filepath.Dir(link), canonical)
	if err != nil {
		return err
	}
	return os.Symlink(relativeTarget, link)
}

func symlinkTargetMatches(link, canonical string) bool {
	resolved, err := resolvedSymlinkTarget(link)
	if err != nil {
		return false
	}
	if filepath.Clean(resolved) == filepath.Clean(canonical) {
		return true
	}
	resolvedReal, resolvedErr := filepath.EvalSymlinks(resolved)
	canonicalReal, canonicalErr := filepath.EvalSymlinks(canonical)
	return resolvedErr == nil && canonicalErr == nil && filepath.Clean(resolvedReal) == filepath.Clean(canonicalReal)
}

func resolvedSymlinkTarget(link string) (string, error) {
	target, err := os.Readlink(link)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}
	return filepath.Clean(filepath.Join(filepath.Dir(link), target)), nil
}

func safeBrokenSkillTarget(existing, canonical string) bool {
	existing = filepath.Clean(existing)
	canonical = filepath.Clean(canonical)
	if filepath.Base(existing) != filepath.Base(canonical) {
		return false
	}
	existingParent := filepath.Base(filepath.Dir(existing))
	canonicalParent := filepath.Base(filepath.Dir(canonical))
	return existingParent == "skills" && canonicalParent == "skills"
}

func prepareCopyTarget(source, target string) error {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(target, 0o755)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if !SkillSymlinkPointsTo(target, source) {
			return fail("Refusing to overwrite unmanaged skill symlink with a copy: %s", target)
		}
		if err := os.Remove(target); err != nil {
			return err
		}
		return os.MkdirAll(target, 0o755)
	}
	if !info.IsDir() {
		return fail("Refusing to overwrite unmanaged skill file: %s", target)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(target, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func cleanAbs(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fail("Path cannot be empty")
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}
