package controlplane

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type cliTarget struct {
	GOOS     string
	GOARCH   string
	Filename string
}

var cliBuildMu sync.Mutex

func normalizeCLITarget(rawOS, rawArch string) (cliTarget, error) {
	goos := strings.ToLower(strings.TrimSpace(rawOS))
	goarch := strings.ToLower(strings.TrimSpace(rawArch))

	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	switch goos {
	case "macos":
		goos = "darwin"
	case "darwin", "linux", "windows":
	default:
		return cliTarget{}, fmt.Errorf("unsupported os %q", rawOS)
	}

	switch goarch {
	case "x86_64", "x64":
		goarch = "amd64"
	case "aarch64":
		goarch = "arm64"
	case "amd64", "arm64":
	default:
		return cliTarget{}, fmt.Errorf("unsupported arch %q", rawArch)
	}

	filename := "afs"
	if goos == "windows" {
		filename = "afs.exe"
	}

	return cliTarget{
		GOOS:     goos,
		GOARCH:   goarch,
		Filename: filename,
	}, nil
}

func resolveCLIBinaryForTarget(target cliTarget) (string, func(), error) {
	if path := findPrebuiltCLIBinary(target); path != "" {
		return path, func() {}, nil
	}

	if target.GOOS == runtime.GOOS && target.GOARCH == runtime.GOARCH {
		if path, err := findCLIBinary(); err == nil {
			return path, func() {}, nil
		}
	}

	sourceRoot, ok := findAFSSourceRoot()
	if !ok {
		return "", func() {}, fmt.Errorf("CLI binary for %s/%s is not available on this control plane", target.GOOS, target.GOARCH)
	}

	return buildCLIBinaryForTarget(sourceRoot, target)
}

func findPrebuiltCLIBinary(target cliTarget) string {
	rel := filepath.Join(target.GOOS+"-"+target.GOARCH, target.Filename)
	for _, base := range cliArtifactDirCandidates() {
		if strings.TrimSpace(base) == "" {
			continue
		}
		path := filepath.Join(base, rel)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func cliArtifactDirCandidates() []string {
	candidates := make([]string, 0, 6)
	if env := strings.TrimSpace(os.Getenv("AFS_CLI_ARTIFACT_DIR")); env != "" {
		candidates = append(candidates, env)
	}
	if exe, err := executablePath(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "cli"),
			filepath.Join(exeDir, "..", "cli"),
		)
	}
	// Working-directory-relative lookup. On Vercel's Go runtime the function
	// starts with the deployment root as its working directory, and the
	// executable lives in a scratch path like /tmp, so neither env nor exeDir
	// point at our baked-in binaries.
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "cli"))
	}
	candidates = append(candidates, "/opt/afs-cli")
	return candidates
}

func findAFSSourceRoot() (string, bool) {
	candidates := make([]string, 0, 2)
	if exe, err := executablePath(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}

	for _, start := range candidates {
		dir := start
		for {
			if isAFSSourceRoot(dir) {
				return dir, true
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return "", false
}

func isAFSSourceRoot(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil || info.IsDir() {
		return false
	}
	if info, err := os.Stat(filepath.Join(dir, "cmd", "afs")); err != nil || !info.IsDir() {
		return false
	}
	return true
}

func buildCLIBinaryForTarget(sourceRoot string, target cliTarget) (string, func(), error) {
	cliBuildMu.Lock()
	defer cliBuildMu.Unlock()

	tmpDir, err := os.MkdirTemp("", "afs-cli-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	outPath := filepath.Join(tmpDir, target.Filename)
	cacheDir := filepath.Join(os.TempDir(), "afs-go-build-cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		cleanup()
		return "", func() {}, err
	}

	cmd := exec.Command("go", "build", "-trimpath", "-o", outPath, "./cmd/afs")
	cmd.Dir = sourceRoot
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+target.GOOS,
		"GOARCH="+target.GOARCH,
		"GOCACHE="+cacheDir,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("build CLI for %s/%s: %w\n%s", target.GOOS, target.GOARCH, err, strings.TrimSpace(string(out)))
	}

	return outPath, cleanup, nil
}
