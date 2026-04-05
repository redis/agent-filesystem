// redis-fs benchmark suite.
//
// Usage:
//
//	go run . --mount /path/to/redis-fs-mount [--rounds 10] [--keep]
//
// Creates a synthetic corpus in a local temp dir and in the mounted redis-fs
// path, then times identical OS-level operations against both, printing a
// comparison table.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ── corpus generation ────────────────────────────────────────────────────────

var goSourceTemplate = `package %s

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// ErrNotFound is returned when a resource cannot be located.
var ErrNotFound = errors.New("not found")

// Config holds service configuration.
type Config struct {
	Addr     string
	Timeout  time.Duration
	MaxRetry int
	Debug    bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Addr:     "localhost:8080",
		Timeout:  30 * time.Second,
		MaxRetry: 3,
	}
}

// Service manages lifecycle and request handling.
type Service struct {
	mu     sync.Mutex
	cfg    Config
	logger *log.Logger
	ready  bool
}

// New creates a new Service with the given config.
func New(cfg Config) *Service {
	return &Service{cfg: cfg, logger: log.Default()}
}

// Start initialises the service.
// TODO: add graceful shutdown support
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ready {
		return errors.New("already started")
	}
	s.logger.Printf("starting service at %%s", s.cfg.Addr)
	s.ready = true
	return nil
}

// Stop shuts down the service.
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready {
		return errors.New("not running")
	}
	s.ready = false
	return nil
}

// Handle processes a single request.
// TODO: implement rate limiting
func (s *Service) Handle(ctx context.Context, req string) (string, error) {
	if !s.ready {
		return "", errors.New("service not ready")
	}
	if req == "" {
		return "", fmt.Errorf("empty request: %%w", ErrNotFound)
	}
	// TODO: add request validation
	return fmt.Sprintf("handled: %%s", req), nil
}

// retryLoop runs fn up to maxRetry times.
func retryLoop(maxRetry int, fn func() error) error {
	var last error
	for i := 0; i < maxRetry; i++ {
		if err := fn(); err != nil {
			last = err
			log.Printf("retry %%d/%%d: %%v", i+1, maxRetry, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("all retries exhausted: %%w", last)
}
`

var markdownTemplate = `# %s

## Overview

This document describes the %s subsystem.

## Architecture

The system consists of three layers:

- **Transport**: handles incoming connections and TLS termination
- **Router**: dispatches requests to handlers based on path and method
- **Handler**: executes business logic, reads/writes data store

## Configuration

| Key | Default | Description |
|-----|---------|-------------|
| addr | :8080 | listen address |
| timeout | 30s | request timeout |
| max_retry | 3 | retry attempts |
| debug | false | enable debug logging |

## Error Handling

All errors are wrapped with context using %%w. Callers should use errors.Is
and errors.As for inspection.

TODO: document error codes

## Performance Notes

- Use connection pooling; each connection has ~2ms setup overhead
- disk full errors are retried automatically up to max_retry times
- cache miss on cold start adds ~50ms latency

## Examples

### Basic Usage

` + "```go" + `
cfg := DefaultConfig()
svc := New(cfg)
if err := svc.Start(ctx); err != nil {
    log.Fatal(err)
}
defer svc.Stop()
` + "```" + `

### Error Recovery

` + "```go" + `
resp, err := svc.Handle(ctx, req)
if errors.Is(err, ErrNotFound) {
    // handle not found
}
` + "```" + `
`

var jsonConfigTemplate = `{
  "service": {
    "addr": "0.0.0.0:%d",
    "timeout": "30s",
    "max_retry": 3,
    "debug": %v
  },
  "redis": {
    "addr": "localhost:6379",
    "db": 0,
    "pool_size": 16,
    "dial_timeout": "5s",
    "read_timeout": "3s",
    "write_timeout": "3s"
  },
  "logging": {
    "level": "info",
    "format": "json",
    "output": "stderr"
  },
  "auth": {
    "enabled": true,
    "token_ttl": "24h",
    "secret": "change-me-in-production"
  }
}
`

var shellScriptTemplate = `#!/usr/bin/env bash
# %s — generated deployment script
set -euo pipefail

APP_NAME="%s"
DEPLOY_DIR="/opt/${APP_NAME}"
LOG_DIR="/var/log/${APP_NAME}"
CONFIG_DIR="/etc/${APP_NAME}"

log() { echo "[$(date -u +%%T)] $*"; }
die() { log "ERROR: $*" >&2; exit 1; }

check_deps() {
    for cmd in redis-cli curl jq; do
        command -v "$cmd" >/dev/null 2>&1 || die "missing dependency: $cmd"
    done
}

setup_dirs() {
    mkdir -p "$DEPLOY_DIR" "$LOG_DIR" "$CONFIG_DIR"
    chmod 750 "$DEPLOY_DIR" "$LOG_DIR"
    # TODO: set proper ownership
}

deploy() {
    log "deploying $APP_NAME"
    # TODO: implement rolling deploy
    cp -r ./dist/* "$DEPLOY_DIR/"
    log "deploy complete"
}

health_check() {
    local url="http://localhost:8080/health"
    local max=10
    for i in $(seq 1 $max); do
        if curl -sf "$url" >/dev/null; then
            log "health check passed"
            return 0
        fi
        log "attempt $i/$max failed, retrying..."
        sleep 2
    done
    die "health check failed after $max attempts"
}

main() {
    check_deps
    setup_dirs
    deploy
    health_check
    log "done"
}

main "$@"
`

// logLine returns one log line for a given index.
func logLine(i int) string {
	levels := []string{"INFO", "INFO", "INFO", "WARN", "ERROR"}
	messages := []string{
		"request processed successfully",
		"cache miss, falling back to database",
		"connection established",
		"retry attempt 1/3 after timeout",
		"disk full on /var/lib, operation failed",
		"auth error: invalid token",
		"queue timeout waiting for worker",
		"TODO: implement circuit breaker here",
		"config reloaded from /etc/app/config.json",
		"health check passed",
		"rate limit exceeded for client 10.0.0.1",
		"database connection pool exhausted",
	}
	t := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * 30 * time.Second)
	level := levels[i%len(levels)]
	msg := messages[i%len(messages)]
	return fmt.Sprintf("%s [%s] pid=1234 req_id=%08x %s", t.Format("2006-01-02T15:04:05Z"), level, i*7+13, msg)
}

// accessLogLine returns one HTTP access log line.
func accessLogLine(i int) string {
	methods := []string{"GET", "POST", "PUT", "DELETE", "GET", "GET"}
	paths := []string{"/api/users", "/api/sessions", "/api/config", "/health", "/metrics", "/api/files"}
	statuses := []int{200, 200, 201, 204, 404, 500, 200, 200}
	method := methods[i%len(methods)]
	path := paths[i%len(paths)]
	status := statuses[i%len(statuses)]
	ms := 5 + (i*17)%200
	return fmt.Sprintf("10.0.%d.%d - - [01/Mar/2026:00:00:%02d +0000] \"%s %s HTTP/1.1\" %d %d \"-\" \"go-http-client/1.1\"",
		(i/256)%256, i%256, i%60, method, path, status, ms)
}

type fileSpec struct {
	relPath string
	content string
}

func generateCorpus() []fileSpec {
	var files []fileSpec

	// Go source files
	srcPkgs := []struct{ dir, pkg string }{
		{"src", "main"},
		{"src", "utils"},
		{"src", "config"},
		{"src", "errors"},
		{"src/handlers", "handlers"},
		{"src/handlers", "grpc"},
		{"src/models", "models"},
		{"src/models", "session"},
		{"src/models", "event"},
	}
	for _, s := range srcPkgs {
		files = append(files, fileSpec{
			relPath: filepath.Join(s.dir, s.pkg+".go"),
			content: fmt.Sprintf(goSourceTemplate, s.pkg),
		})
	}

	// Markdown docs
	docs := []string{"README", "API", "CHANGELOG", "DESIGN"}
	for _, d := range docs {
		files = append(files, fileSpec{
			relPath: filepath.Join("docs", d+".md"),
			content: fmt.Sprintf(markdownTemplate, d, strings.ToLower(d)),
		})
	}

	// JSON configs
	ports := map[string]int{"production": 8080, "staging": 8081, "development": 8082}
	debug := map[string]bool{"production": false, "staging": true, "development": true}
	for env, port := range ports {
		files = append(files, fileSpec{
			relPath: filepath.Join("config", env+".json"),
			content: fmt.Sprintf(jsonConfigTemplate, port, debug[env]),
		})
	}

	// Shell scripts
	scripts := []string{"deploy", "setup", "migrate"}
	for _, s := range scripts {
		files = append(files, fileSpec{
			relPath: filepath.Join("scripts", s+".sh"),
			content: fmt.Sprintf(shellScriptTemplate, s, "myapp"),
		})
	}

	// Log files
	var appLog strings.Builder
	for i := 0; i < 2000; i++ {
		appLog.WriteString(logLine(i))
		appLog.WriteByte('\n')
	}
	files = append(files, fileSpec{relPath: filepath.Join("logs", "app.log"), content: appLog.String()})

	var errLog strings.Builder
	for i := 0; i < 500; i++ {
		errLog.WriteString(logLine(i*5 + 4)) // every 5th line is an ERROR
		errLog.WriteByte('\n')
	}
	files = append(files, fileSpec{relPath: filepath.Join("logs", "error.log"), content: errLog.String()})

	var accessLog strings.Builder
	for i := 0; i < 3000; i++ {
		accessLog.WriteString(accessLogLine(i))
		accessLog.WriteByte('\n')
	}
	files = append(files, fileSpec{relPath: filepath.Join("logs", "access.log"), content: accessLog.String()})

	// Large binary-ish file (200KB of repeated pattern)
	const largeSize = 200 * 1024
	pattern := strings.Repeat("redis-fs-bench-data-padding-0123456789abcdef\n", 1)
	var large strings.Builder
	for large.Len() < largeSize {
		large.WriteString(pattern)
	}
	files = append(files, fileSpec{relPath: filepath.Join("data", "large.bin"), content: large.String()[:largeSize]})

	// Rename-dir corpus: a subdir with 10 files used for rename_dir benchmark.
	for i := 0; i < 10; i++ {
		files = append(files, fileSpec{
			relPath: fmt.Sprintf("rename_src/file%02d.txt", i),
			content: fmt.Sprintf("file %d content for rename benchmark\n", i),
		})
	}

	return files
}

func writeCorpus(root string, files []fileSpec) error {
	for _, f := range files {
		full := filepath.Join(root, f.relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ── timing helpers ────────────────────────────────────────────────────────────

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

func maxFloat(vals []float64) float64 {
	var m float64
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}

// timed runs fn `rounds` times and returns (median_ms, max_ms).
func timed(rounds int, fn func() error) (float64, float64, error) {
	samples := make([]float64, 0, rounds)
	for i := 0; i < rounds; i++ {
		t0 := time.Now()
		if err := fn(); err != nil {
			return 0, 0, err
		}
		samples = append(samples, float64(time.Since(t0).Microseconds())/1000.0)
	}
	return median(samples), maxFloat(samples), nil
}

// ── benchmark operations ──────────────────────────────────────────────────────

func benchStatFile(root string) func() error {
	target := filepath.Join(root, "src", "main.go")
	return func() error {
		_, err := os.Stat(target)
		return err
	}
}

func benchLsShallow(root string) func() error {
	target := filepath.Join(root, "config") // 3 entries
	return func() error {
		_, err := os.ReadDir(target)
		return err
	}
}

func benchLsDeep(root string) func() error {
	target := filepath.Join(root, "src") // 4 files + 2 subdirs
	return func() error {
		_, err := os.ReadDir(target)
		return err
	}
}

func benchReadSmall(root string) func() error {
	target := filepath.Join(root, "config", "production.json")
	return func() error {
		_, err := os.ReadFile(target)
		return err
	}
}

func benchReadMedium(root string) func() error {
	target := filepath.Join(root, "src", "main.go")
	return func() error {
		_, err := os.ReadFile(target)
		return err
	}
}

func benchReadLarge(root string) func() error {
	target := filepath.Join(root, "data", "large.bin")
	return func() error {
		_, err := os.ReadFile(target)
		return err
	}
}

func benchWriteNew(root string) func() error {
	i := 0
	return func() error {
		i++
		p := filepath.Join(root, fmt.Sprintf("_bench_write_%d.tmp", i))
		err := os.WriteFile(p, []byte(strings.Repeat("x", 2048)), 0o644)
		if err != nil {
			return err
		}
		return os.Remove(p)
	}
}

func benchWriteOverwrite(root string) func() error {
	p := filepath.Join(root, "_bench_overwrite.tmp")
	// pre-create
	_ = os.WriteFile(p, []byte("init"), 0o644)
	data := []byte(strings.Repeat("y", 2048))
	return func() error {
		return os.WriteFile(p, data, 0o644)
	}
}

func benchAppend(root string) func() error {
	p := filepath.Join(root, "_bench_append.tmp")
	_ = os.WriteFile(p, []byte("init\n"), 0o644)
	payload := []byte("appended line\n")
	return func() error {
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		_, err = f.Write(payload)
		f.Close()
		return err
	}
}

func grepAll(root, needle string, icase bool) error {
	if icase {
		needle = strings.ToLower(needle)
	}
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		content := string(data)
		if icase {
			content = strings.ToLower(content)
		}
		_ = strings.Contains(content, needle)
		return nil
	})
}

func grepRegex(root string, re *regexp.Regexp) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_ = re.Match(data)
		return nil
	})
}

func benchGrepLiteral(root string) func() error {
	return func() error { return grepAll(root, "error", false) }
}

func benchGrepIcase(root string) func() error {
	return func() error { return grepAll(root, "todo", true) }
}

func benchGrepRegex(root string) func() error {
	re := regexp.MustCompile(`func \w+\(`)
	return func() error { return grepRegex(root, re) }
}

func benchGrepPhrase(root string) func() error {
	return func() error { return grepAll(root, "disk full", false) }
}

// benchGrepLines counts matching lines (like grep -n), returning results.
func benchGrepLines(root string) func() error {
	needle := "error"
	return func() error {
		return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				_ = strings.Contains(scanner.Text(), needle)
			}
			return scanner.Err()
		})
	}
}

func benchFindExt(root string) func() error {
	return func() error {
		return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			_ = strings.HasSuffix(d.Name(), ".go")
			return nil
		})
	}
}

func benchWalkTree(root string) func() error {
	return func() error {
		count := 0
		return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			count++
			return nil
		})
	}
}

func benchRenameFile(root string) func() error {
	src := filepath.Join(root, "_bench_rename_src.tmp")
	dst := filepath.Join(root, "_bench_rename_dst.tmp")
	_ = os.WriteFile(src, []byte("rename me"), 0o644)
	_ = os.Remove(dst)
	i := 0
	return func() error {
		i++
		var from, to string
		if i%2 == 1 {
			from, to = src, dst
		} else {
			from, to = dst, src
		}
		return os.Rename(from, to)
	}
}

func benchRenameDir(root string) func() error {
	// Pre-create source dir with 10 files. Each round alternates a→b / b→a
	// so we measure pure rename cost without per-round setup or RemoveAll.
	dirA := filepath.Join(root, "_bench_mvdir_a")
	dirB := filepath.Join(root, "_bench_mvdir_b")
	_ = os.MkdirAll(dirA, 0o755)
	for i := 0; i < 10; i++ {
		_ = os.WriteFile(filepath.Join(dirA, fmt.Sprintf("f%d.txt", i)), []byte("data"), 0o644)
	}
	_ = os.RemoveAll(dirB)
	i := 0
	return func() error {
		i++
		var from, to string
		if i%2 == 1 {
			from, to = dirA, dirB
		} else {
			from, to = dirB, dirA
		}
		return os.Rename(from, to)
	}
}

func benchMkdirRmdir(root string) func() error {
	i := 0
	return func() error {
		i++
		p := filepath.Join(root, fmt.Sprintf("_bench_dir_%d", i))
		if err := os.Mkdir(p, 0o755); err != nil {
			return err
		}
		return os.Remove(p)
	}
}

// ── reporting ─────────────────────────────────────────────────────────────────

type result struct {
	name       string
	localMed   float64
	localMax   float64
	mountMed   float64
	mountMax   float64
}

func (r result) ratio() float64 {
	if r.localMed < 0.001 {
		return math.NaN()
	}
	return r.mountMed / r.localMed
}

func printResults(results []result) {
	const w1, w2 = 22, 12
	sep := strings.Repeat("─", w1+w2*4+6)

	fmt.Printf("\n%-*s  %*s %*s  %*s %*s  %s\n",
		w1, "Operation",
		w2, "Local med",
		w2, "Local max",
		w2, "Mount med",
		w2, "Mount max",
		"Ratio",
	)
	fmt.Println(sep)
	for _, r := range results {
		ratio := r.ratio()
		ratioStr := "n/a"
		if !math.IsNaN(ratio) {
			ratioStr = fmt.Sprintf("%.0fx", ratio)
		}
		fmt.Printf("%-*s  %*.2f %*.2f  %*.2f %*.2f  %s\n",
			w1, r.name,
			w2, r.localMed,
			w2, r.localMax,
			w2, r.mountMed,
			w2, r.mountMax,
			ratioStr,
		)
	}
	fmt.Println(sep)
	fmt.Println("(all times in ms)")
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	mountPath := flag.String("mount", "", "path to redis-fs mountpoint (required)")
	rounds := flag.Int("rounds", 10, "benchmark repetitions per operation")
	keep := flag.Bool("keep", false, "do not delete corpus after run")
	flag.Parse()

	if *mountPath == "" {
		fmt.Fprintln(os.Stderr, "error: --mount is required")
		flag.Usage()
		os.Exit(1)
	}
	if _, err := os.Stat(*mountPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: mount path %q: %v\n", *mountPath, err)
		os.Exit(1)
	}

	tag := fmt.Sprintf("bench-%s", time.Now().Format("20060102-150405"))

	// Create local temp corpus.
	localRoot, err := os.MkdirTemp("", "rfs-bench-local-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkdirtemp:", err)
		os.Exit(1)
	}
	localRoot = filepath.Join(localRoot, tag)
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir:", err)
		os.Exit(1)
	}

	// Create mounted corpus.
	mountRoot := filepath.Join(*mountPath, tag)
	if err := os.MkdirAll(mountRoot, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot create directory in mount %q: %v\n", mountRoot, err)
		os.Exit(1)
	}

	if !*keep {
		defer func() {
			os.RemoveAll(localRoot)
			os.RemoveAll(mountRoot)
			fmt.Println("\nCorpora cleaned up.")
		}()
	}

	fmt.Printf("Generating corpus (%d files)…\n", len(generateCorpus()))
	corpus := generateCorpus()

	fmt.Print("Writing local corpus…  ")
	t0 := time.Now()
	if err := writeCorpus(localRoot, corpus); err != nil {
		fmt.Fprintln(os.Stderr, "error writing local corpus:", err)
		os.Exit(1)
	}
	fmt.Printf("done (%s)\n", time.Since(t0).Round(time.Millisecond))

	fmt.Print("Writing mount corpus…  ")
	t0 = time.Now()
	if err := writeCorpus(mountRoot, corpus); err != nil {
		fmt.Fprintln(os.Stderr, "error writing mount corpus:", err)
		os.Exit(1)
	}
	fmt.Printf("done (%s)\n", time.Since(t0).Round(time.Millisecond))

	type bench struct {
		name  string
		local func() error
		mount func() error
	}

	benches := []bench{
		{"stat_file", benchStatFile(localRoot), benchStatFile(mountRoot)},
		{"ls_shallow", benchLsShallow(localRoot), benchLsShallow(mountRoot)},
		{"ls_deep", benchLsDeep(localRoot), benchLsDeep(mountRoot)},
		{"read_small", benchReadSmall(localRoot), benchReadSmall(mountRoot)},
		{"read_medium", benchReadMedium(localRoot), benchReadMedium(mountRoot)},
		{"read_large", benchReadLarge(localRoot), benchReadLarge(mountRoot)},
		{"write_new", benchWriteNew(localRoot), benchWriteNew(mountRoot)},
		{"write_overwrite", benchWriteOverwrite(localRoot), benchWriteOverwrite(mountRoot)},
		{"append_file", benchAppend(localRoot), benchAppend(mountRoot)},
		{"grep_literal", benchGrepLiteral(localRoot), benchGrepLiteral(mountRoot)},
		{"grep_icase", benchGrepIcase(localRoot), benchGrepIcase(mountRoot)},
		{"grep_regex", benchGrepRegex(localRoot), benchGrepRegex(mountRoot)},
		{"grep_phrase", benchGrepPhrase(localRoot), benchGrepPhrase(mountRoot)},
		{"grep_lines", benchGrepLines(localRoot), benchGrepLines(mountRoot)},
		{"find_ext", benchFindExt(localRoot), benchFindExt(mountRoot)},
		{"walk_tree", benchWalkTree(localRoot), benchWalkTree(mountRoot)},
		{"rename_file", benchRenameFile(localRoot), benchRenameFile(mountRoot)},
		{"rename_dir", benchRenameDir(localRoot), benchRenameDir(mountRoot)},
		{"mkdir_rmdir", benchMkdirRmdir(localRoot), benchMkdirRmdir(mountRoot)},
	}

	fmt.Printf("\nRunning %d benchmarks × %d rounds each…\n", len(benches), *rounds)

	var results []result
	for _, b := range benches {
		fmt.Printf("  %-22s", b.name)
		lMed, lMax, err := timed(*rounds, b.local)
		if err != nil {
			fmt.Printf("local error: %v\n", err)
			continue
		}
		mMed, mMax, err := timed(*rounds, b.mount)
		if err != nil {
			fmt.Printf("mount error: %v\n", err)
			continue
		}
		results = append(results, result{b.name, lMed, lMax, mMed, mMax})
		fmt.Printf("local=%.2fms  mount=%.2fms  ratio=%.0fx\n", lMed, mMed, mMed/lMed)
	}

	printResults(results)
}
