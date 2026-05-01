// bench_md_workloads benchmarks markdown-heavy agent workflows on a plain local
// directory versus the current Redis-backed AFS client/search paths.
//
// The harness is self-contained:
//  1. generate a deterministic markdown corpus
//  2. start a temporary Redis server
//  3. build or reuse the afs binary
//  4. import the corpus into a temporary AFS workspace
//  5. benchmark grep plus nearby agent-style file activities
//
// Example:
//
//	go run ./tests/bench_md_workloads --markdown-files 4000 --rounds 5 --output-dir /tmp/afs-bench-md-run
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/agent-filesystem/mount/client"
	"github.com/redis/go-redis/v9"
)

const grepProfilePrefix = "AFS_GREP_PROFILE "

const (
	grepMaxDepth = 4096

	defaultMarkdownFiles = 4000
	defaultTargetBytes   = 8192
	defaultHotReadFiles  = 96
)

type configFile struct {
	Redis struct {
		Addr     string `json:"addr"`
		Username string `json:"username"`
		Password string `json:"password"`
		DB       int    `json:"db"`
		TLS      bool   `json:"tls"`
	} `json:"redis"`
	Mode             string `json:"mode"`
	CurrentWorkspace string `json:"currentWorkspace"`
	LocalPath        string `json:"localPath"`
	Mount            struct {
		Backend    string `json:"backend"`
		ReadOnly   bool   `json:"readOnly"`
		AllowOther bool   `json:"allowOther"`
		MountBin   string `json:"mountBin"`
		NFSBin     string `json:"nfsBin"`
		NFSHost    string `json:"nfsHost"`
		NFSPort    int    `json:"nfsPort"`
	} `json:"mount"`
	Logs struct {
		Mount string `json:"mount"`
		Sync  string `json:"sync"`
	} `json:"logs"`
	Sync struct {
		FileSizeCapMB int `json:"fileSizeCapMB"`
	} `json:"sync"`
}

type corpusSpec struct {
	Workspace      string   `json:"workspace"`
	MarkdownFiles  int      `json:"markdown_files"`
	ApproxBytes    int      `json:"approx_bytes_per_file"`
	TotalBytes     int64    `json:"total_bytes"`
	TotalDirs      int      `json:"total_dirs"`
	HotFiles       []string `json:"hot_files"`
	RareNeedle     string   `json:"rare_needle"`
	CommonNeedle   string   `json:"common_needle"`
	RegexPattern   string   `json:"regex_pattern"`
	FindPattern    string   `json:"find_pattern"`
	RareMatches    int      `json:"rare_matches"`
	CommonMatches  int      `json:"common_matches"`
	RegexMatches   int      `json:"regex_matches"`
	RunbookMatches int      `json:"runbook_matches"`
}

type countSample struct {
	ElapsedMS float64 `json:"elapsed_ms"`
	Count     int     `json:"count"`
}

type pairSummary struct {
	Name        string        `json:"name"`
	LocalLabel  string        `json:"local_label"`
	RedisLabel  string        `json:"redis_label"`
	Local       []countSample `json:"local"`
	Redis       []countSample `json:"redis"`
	LocalMedian float64       `json:"local_median_ms"`
	LocalMin    float64       `json:"local_min_ms"`
	LocalMax    float64       `json:"local_max_ms"`
	RedisMedian float64       `json:"redis_median_ms"`
	RedisMin    float64       `json:"redis_min_ms"`
	RedisMax    float64       `json:"redis_max_ms"`
	LocalCount  string        `json:"local_count_summary"`
	RedisCount  string        `json:"redis_count_summary"`
	Ratio       float64       `json:"redis_over_local_ratio"`
	Validation  string        `json:"validation"`
}

type report struct {
	TimestampUTC   string        `json:"timestamp_utc"`
	Platform       string        `json:"platform"`
	GoVersion      string        `json:"go_version"`
	RedisVersion   string        `json:"redis_version"`
	RedisSource    string        `json:"redis_source"`
	GrepVersion    string        `json:"grep_version"`
	RipgrepVersion string        `json:"ripgrep_version,omitempty"`
	AFSBin         string        `json:"afs_bin"`
	GrepBin        string        `json:"grep_bin"`
	RipgrepBin     string        `json:"ripgrep_bin,omitempty"`
	RedisAddr      string        `json:"redis_addr"`
	Rounds         int           `json:"rounds"`
	Warmup         int           `json:"warmup"`
	Corpus         corpusSpec    `json:"corpus"`
	GrepResults    []pairSummary `json:"grep_results"`
	RipgrepResults []pairSummary `json:"ripgrep_results,omitempty"`
	OpsResults     []pairSummary `json:"ops_results"`
	LocalRoot      string        `json:"local_root"`
	TempRoot       string        `json:"temp_root"`
}

type grepTarget struct {
	label   string
	kind    string
	cmd     []string
	cwd     string
	pattern string
}

type outputSnapshot struct {
	lines    []string
	exitCode int
	stderr   string
}

type countRunner struct {
	label string
	run   func(context.Context) (int, error)
}

type runtimeEnv struct {
	tempRoot   string
	corpusRoot string
	configPath string
	afsBin     string
	grepBin    string
	rgBin      string
	redisAddr  string
	redisStop  func() error
	redisLog   string
	rdb        *redis.Client
	fs         client.Client
	report     report
}

func main() {
	var (
		afsBin        = flag.String("afs-bin", "", "path to the afs binary; if empty the harness builds a temp binary")
		grepBin       = flag.String("grep-bin", "grep", "path to grep")
		rgBin         = flag.String("rg-bin", "rg", "path to ripgrep; empty disables ripgrep comparisons")
		redisBin      = flag.String("redis-bin", "redis-server", "path to redis-server")
		redisImage    = flag.String("redis-image", "redis:8", "docker image to use when the local redis-server does not support Search")
		workspace     = flag.String("workspace", "bench-md", "temporary workspace name")
		markdownFiles = flag.Int("markdown-files", defaultMarkdownFiles, "number of markdown files to generate")
		targetBytes   = flag.Int("target-bytes", defaultTargetBytes, "approximate bytes per markdown file")
		hotReads      = flag.Int("hot-read-files", defaultHotReadFiles, "number of hot files for repeated read/head benchmarks")
		rounds        = flag.Int("rounds", 5, "measured rounds per benchmark")
		warmup        = flag.Int("warmup", 1, "warmup rounds per benchmark")
		outputDir     = flag.String("output-dir", "", "directory for JSON/CSV/Markdown output")
		keep          = flag.Bool("keep", false, "keep the temporary benchmark workspace on disk")
	)
	flag.Parse()

	if *markdownFiles <= 0 {
		fatalf("--markdown-files must be > 0")
	}
	if *targetBytes < 1024 {
		fatalf("--target-bytes must be >= 1024")
	}
	if *hotReads <= 0 {
		fatalf("--hot-read-files must be > 0")
	}
	if *rounds <= 0 {
		fatalf("--rounds must be > 0")
	}
	if *warmup < 0 {
		fatalf("--warmup must be >= 0")
	}

	ctx := context.Background()
	env := &runtimeEnv{}
	var cleanup []func()
	defer func() {
		for i := len(cleanup) - 1; i >= 0; i-- {
			cleanup[i]()
		}
	}()

	tempRoot, err := os.MkdirTemp("", "afs-bench-md-")
	must(err)
	env.tempRoot = tempRoot
	if !*keep {
		cleanup = append(cleanup, func() { _ = os.RemoveAll(tempRoot) })
	}

	fmt.Printf("Benchmark temp root: %s\n", tempRoot)

	afsResolved, buildCleanup, err := resolveAFSBinary(tempRoot, *afsBin)
	must(err)
	env.afsBin = afsResolved
	if buildCleanup != nil {
		cleanup = append(cleanup, buildCleanup)
	}
	env.grepBin, err = resolveBinary(*grepBin)
	must(err)
	if strings.TrimSpace(*rgBin) != "" {
		env.rgBin, err = resolveOptionalBinary(*rgBin)
		must(err)
	}
	redisResolved, err := resolveBinary(*redisBin)
	must(err)

	corpusRoot := filepath.Join(tempRoot, "corpus")
	must(os.MkdirAll(corpusRoot, 0o755))
	env.corpusRoot = corpusRoot
	corpus := generateCorpus(corpusRoot, *workspace, *markdownFiles, *targetBytes, *hotReads)
	env.report.Corpus = corpus

	port, err := reservePort()
	must(err)
	redisAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	env.redisAddr = redisAddr
	redisLog := filepath.Join(tempRoot, "redis.log")
	env.redisLog = redisLog
	redisVersion, redisSource, redisStop, err := startRedisForSearch(ctx, tempRoot, redisResolved, *redisImage, port, redisLog)
	must(err)
	env.redisStop = redisStop
	cleanup = append(cleanup, func() {
		if env.rdb != nil {
			_ = env.rdb.Close()
		}
		if redisStop != nil {
			_ = redisStop()
		}
	})
	env.rdb = redis.NewClient(&redis.Options{Addr: redisAddr})
	must(waitForRedis(ctx, env.rdb))

	configPath := filepath.Join(tempRoot, "afs.config.json")
	must(writeConfig(configPath, redisAddr, *workspace, filepath.Join(tempRoot, "local-surface")))
	env.configPath = configPath

	fmt.Printf("Generated corpus: %d markdown files, %.1f MiB\n", corpus.MarkdownFiles, float64(corpus.TotalBytes)/(1024.0*1024.0))
	fmt.Printf("Importing workspace %q into Redis at %s\n", *workspace, redisAddr)
	importStarted := time.Now()
	must(importWorkspace(env.afsBin, configPath, *workspace, corpusRoot))
	fmt.Printf("Import completed in %s\n", time.Since(importStarted).Round(time.Millisecond))

	storageID, err := resolveWorkspaceStorageID(ctx, env.rdb, *workspace)
	must(err)
	if storageID != *workspace {
		fmt.Printf("Resolved workspace storage ID: %s\n", storageID)
	}
	env.fs = client.New(env.rdb, storageID)
	must(validateCorpus(ctx, corpus, corpusRoot, env.fs))

	env.report.TimestampUTC = time.Now().UTC().Format(time.RFC3339)
	env.report.Platform = runtime.GOOS + "/" + runtime.GOARCH
	env.report.GoVersion = runtime.Version()
	env.report.RedisVersion = redisVersion
	env.report.RedisSource = redisSource
	env.report.GrepVersion = firstNonEmptyLine(commandOutput(env.grepBin, "--help"))
	env.report.AFSBin = env.afsBin
	env.report.GrepBin = env.grepBin
	env.report.RipgrepBin = env.rgBin
	if env.rgBin != "" {
		env.report.RipgrepVersion = firstNonEmptyLine(commandOutput(env.rgBin, "--version"))
	}
	env.report.RedisAddr = env.redisAddr
	env.report.Rounds = *rounds
	env.report.Warmup = *warmup
	env.report.LocalRoot = env.corpusRoot
	env.report.TempRoot = env.tempRoot

	grepPairs := buildGrepPairs(env, corpus)
	grepResults := make([]pairSummary, 0, len(grepPairs))
	for _, pair := range grepPairs {
		fmt.Printf("\nRunning grep benchmark: %s\n", pair.name)
		summary, err := benchmarkCommandPair(pair.name, pair.local, pair.redis, *warmup, *rounds)
		must(err)
		grepResults = append(grepResults, summary)
		fmt.Printf("  validation: %s\n", summary.Validation)
		fmt.Printf("  median local=%.2f ms redis=%.2f ms ratio=%.2fx\n", summary.LocalMedian, summary.RedisMedian, summary.Ratio)
	}
	env.report.GrepResults = grepResults

	if env.rgBin != "" {
		rgPairs := buildRipgrepPairs(env, corpus)
		rgResults := make([]pairSummary, 0, len(rgPairs))
		for _, pair := range rgPairs {
			fmt.Printf("\nRunning ripgrep benchmark: %s\n", pair.name)
			summary, err := benchmarkCommandPair(pair.name, pair.local, pair.redis, *warmup, *rounds)
			must(err)
			rgResults = append(rgResults, summary)
			fmt.Printf("  validation: %s\n", summary.Validation)
			fmt.Printf("  median local=%.2f ms redis=%.2f ms ratio=%.2fx\n", summary.LocalMedian, summary.RedisMedian, summary.Ratio)
		}
		env.report.RipgrepResults = rgResults
	}

	opPairs := buildCountPairs(corpus, corpusRoot, env.fs)
	opResults := make([]pairSummary, 0, len(opPairs))
	for _, pair := range opPairs {
		fmt.Printf("\nRunning workload benchmark: %s\n", pair.name)
		summary, err := benchmarkCountPair(pair.name, pair.local, pair.redis, *warmup, *rounds)
		must(err)
		opResults = append(opResults, summary)
		fmt.Printf("  validation: %s\n", summary.Validation)
		fmt.Printf("  median local=%.2f ms redis=%.2f ms ratio=%.2fx\n", summary.LocalMedian, summary.RedisMedian, summary.Ratio)
	}
	env.report.OpsResults = opResults

	printReport(env.report)

	if strings.TrimSpace(*outputDir) != "" {
		must(writeReportFiles(*outputDir, env.report))
		fmt.Printf("\nWrote benchmark artifacts to %s\n", *outputDir)
	}

	if *keep {
		fmt.Printf("\nKept benchmark files at %s\n", tempRoot)
		fmt.Printf("Redis log: %s\n", redisLog)
	}
}

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func resolveAFSBinary(tempRoot, explicit string) (string, func(), error) {
	if strings.TrimSpace(explicit) != "" {
		path, err := resolveBinary(explicit)
		return path, nil, err
	}
	out := filepath.Join(tempRoot, "afs")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/afs")
	cmd.Dir = repoRoot()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", nil, fmt.Errorf("build afs: %w\n%s", err, stderr.String())
	}
	return out, nil, nil
}

func resolveBinary(raw string) (string, error) {
	if strings.Contains(raw, "/") || strings.HasPrefix(raw, ".") {
		abs, err := filepath.Abs(raw)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", err
		}
		return abs, nil
	}
	found, err := exec.LookPath(raw)
	if err != nil {
		return "", err
	}
	return found, nil
}

func resolveOptionalBinary(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	path, err := resolveBinary(raw)
	if err == nil {
		return path, nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "", nil
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) && execErr.Err == exec.ErrNotFound {
		return "", nil
	}
	return "", err
}

func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		fatalf("getwd: %v", err)
	}
	return wd
}

func reservePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func startRedis(tempRoot, redisBin string, port int, logPath string) (*exec.Cmd, error) {
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(redisBin,
		"--save", "",
		"--appendonly", "no",
		"--port", strconv.Itoa(port),
		"--bind", "127.0.0.1",
		"--dir", tempRoot,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
	}()
	return cmd, nil
}

func startRedisForSearch(ctx context.Context, tempRoot, redisBin, redisImage string, port int, logPath string) (string, string, func() error, error) {
	localProc, err := startRedis(tempRoot, redisBin, port, logPath)
	if err != nil {
		return "", "", nil, err
	}
	localStop := func() error { return stopProcess(localProc) }

	rdb := redis.NewClient(&redis.Options{Addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))})
	if err := waitForRedis(ctx, rdb); err != nil {
		_ = rdb.Close()
		_ = localStop()
		return "", "", nil, err
	}
	localVersion := redisVersionString(ctx, rdb)
	searchOK := redisSupportsSearch(ctx, rdb)
	_ = rdb.Close()
	if searchOK {
		return localVersion, "local redis-server", localStop, nil
	}
	_ = localStop()

	dockerBin, err := exec.LookPath("docker")
	if err != nil {
		return "", "", nil, fmt.Errorf("local redis-server (%s) does not support Search and docker is unavailable", localVersion)
	}
	containerID, err := startRedisDocker(dockerBin, redisImage, port)
	if err != nil {
		return "", "", nil, fmt.Errorf("local redis-server (%s) does not support Search and docker fallback failed: %w", localVersion, err)
	}
	dockerStop := func() error { return stopDockerContainer(dockerBin, containerID) }

	rdb = redis.NewClient(&redis.Options{Addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))})
	if err := waitForRedis(ctx, rdb); err != nil {
		_ = rdb.Close()
		_ = dockerStop()
		return "", "", nil, err
	}
	dockerVersion := redisVersionString(ctx, rdb)
	searchOK = redisSupportsSearch(ctx, rdb)
	_ = rdb.Close()
	if !searchOK {
		_ = dockerStop()
		return "", "", nil, fmt.Errorf("docker fallback %q started %s but Search commands are unavailable", redisImage, dockerVersion)
	}
	return dockerVersion, "docker " + redisImage, dockerStop, nil
}

func startRedisDocker(dockerBin, redisImage string, port int) (string, error) {
	cmd := exec.Command(dockerBin, "run", "-d", "-p", fmt.Sprintf("127.0.0.1:%d:6379", port), redisImage, "redis-server", "--save", "", "--appendonly", "no", "--bind", "0.0.0.0")
	out, err := cmd.Output()
	if err != nil {
		if ee := new(exec.ExitError); errors.As(err, &ee) {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func stopDockerContainer(dockerBin, containerID string) error {
	if strings.TrimSpace(containerID) == "" {
		return nil
	}
	cmd := exec.Command(dockerBin, "rm", "-f", containerID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker rm -f %s: %w (%s)", containerID, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func stopProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func waitForRedis(ctx context.Context, rdb *redis.Client) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pingCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		err := rdb.Ping(pingCtx).Err()
		cancel()
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("timed out waiting for redis-server")
}

func redisSupportsSearch(ctx context.Context, rdb *redis.Client) bool {
	return rdb.FT_List(ctx).Err() == nil
}

func redisVersionString(ctx context.Context, rdb *redis.Client) string {
	info, err := rdb.Info(ctx, "server").Result()
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "redis_version:") {
			return "Redis server v=" + strings.TrimPrefix(line, "redis_version:")
		}
	}
	return strings.TrimSpace(firstLine(info))
}

func writeConfig(path, redisAddr, workspace, localPath string) error {
	var cfg configFile
	cfg.Redis.Addr = redisAddr
	cfg.Mode = "sync"
	cfg.CurrentWorkspace = workspace
	cfg.LocalPath = localPath
	cfg.Mount.Backend = "none"
	cfg.Mount.NFSHost = "127.0.0.1"
	cfg.Mount.NFSPort = 20490
	cfg.Logs.Mount = filepath.Join(filepath.Dir(path), "mount.log")
	cfg.Logs.Sync = filepath.Join(filepath.Dir(path), "sync.log")
	cfg.Sync.FileSizeCapMB = 100

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func importWorkspace(afsBin, configPath, workspace, source string) error {
	cmd := exec.Command(afsBin, "--config", configPath, "ws", "import", workspace, source)
	cmd.Dir = repoRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("afs ws import failed: %w\n%s", err, string(out))
	}
	return nil
}

func resolveWorkspaceStorageID(ctx context.Context, rdb *redis.Client, workspace string) (string, error) {
	storageID, err := rdb.HGet(ctx, "afs:workspace:index:names", workspace).Result()
	if errors.Is(err, redis.Nil) {
		return workspace, nil
	}
	if err != nil {
		return "", err
	}
	storageID = strings.TrimSpace(storageID)
	if storageID == "" {
		return workspace, nil
	}
	return storageID, nil
}

func generateCorpus(root, workspace string, markdownFiles, targetBytes, hotReads int) corpusSpec {
	areas := []string{"runbooks", "designs", "meeting-notes", "playbooks"}
	teams := 24
	var totalBytes int64
	hot := make([]string, 0, hotReads)
	spec := corpusSpec{
		Workspace:     workspace,
		MarkdownFiles: markdownFiles,
		ApproxBytes:   targetBytes,
		RareNeedle:    "needle-redis-md-8675309",
		CommonNeedle:  "incident review action item",
		RegexPattern:  "^## Escalation$",
		FindPattern:   "*runbook*.md",
	}

	dirsSeen := map[string]struct{}{}
	for i := 0; i < markdownFiles; i++ {
		team := fmt.Sprintf("team-%02d", i%teams)
		area := areas[i%len(areas)]
		docType := area[:len(area)-1]
		filename := fmt.Sprintf("%s-%04d.md", docType, i)
		rel := filepath.ToSlash(filepath.Join(team, area, filename))
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		dirsSeen[filepath.Dir(rel)] = struct{}{}
		content, counts := renderMarkdownDoc(i, targetBytes, spec.RareNeedle, spec.CommonNeedle)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			fatalf("write %s: %v", full, err)
		}
		totalBytes += int64(len(content))
		spec.RareMatches += counts.rare
		spec.CommonMatches += counts.common
		spec.RegexMatches += counts.regex
		if strings.Contains(filename, "runbook") {
			spec.RunbookMatches++
		}
		if len(hot) < hotReads && i%int(math.Max(1, float64(markdownFiles/hotReads))) == 0 {
			hot = append(hot, "/"+filepath.ToSlash(rel))
		}
	}

	if len(hot) == 0 {
		hot = append(hot, "/team-00/runbooks/runbook-0000.md")
	}
	spec.TotalBytes = totalBytes
	spec.TotalDirs = len(dirsSeen)
	spec.HotFiles = hot
	return spec
}

type docCounts struct {
	rare   int
	common int
	regex  int
}

func renderMarkdownDoc(i, targetBytes int, rareNeedle, commonNeedle string) (string, docCounts) {
	var b strings.Builder
	counts := docCounts{}

	titleKind := []string{"Runbook", "Design", "Meeting Notes", "Playbook"}[i%4]
	fmt.Fprintf(&b, "---\nworkspace: bench-md\ndoc_id: %04d\nowner: team-%02d\n---\n\n", i, i%24)
	fmt.Fprintf(&b, "# %s %04d\n\n", titleKind, i)
	fmt.Fprintf(&b, "## Overview\n\nThis markdown document simulates agent-searchable product documentation for benchmark run %04d.\n\n", i)
	fmt.Fprintf(&b, "## Context\n\n- Service: service-%02d\n- Priority: p%d\n- Audience: agents and operators\n\n", i%17, i%4)

	for section := 0; b.Len() < targetBytes; section++ {
		switch section % 6 {
		case 0:
			b.WriteString("## Procedure\n\n")
			b.WriteString("1. Confirm the current state.\n2. Inspect related notes.\n3. Capture follow-up tasks.\n\n")
		case 1:
			b.WriteString("## Escalation\n\n")
			counts.regex++
			b.WriteString("Escalate when the issue persists beyond the retry budget or the rollback window is missed.\n\n")
		case 2:
			fmt.Fprintf(&b, "## Decision %d\n\n", section/2)
			b.WriteString("The preferred path is to keep the system readable for agents and humans, even when the implementation changes.\n\n")
		case 3:
			b.WriteString("## Notes\n\n")
			b.WriteString("Agents usually search for architecture terms, operational symptoms, and follow-up actions inside markdown docs.\n\n")
		case 4:
			b.WriteString("```text\n")
			b.WriteString("owner=docs-platform\nstatus=active\nsurface=markdown-search\n")
			b.WriteString("```\n\n")
		default:
			b.WriteString("## Appendix\n\n")
			b.WriteString("This section repeats structured prose so the corpus behaves more like real design docs than synthetic lorem ipsum.\n\n")
		}
		if i%9 == 0 && counts.common == 0 {
			fmt.Fprintf(&b, "%s\n\n", commonNeedle)
			counts.common++
		}
		if i%487 == 0 && counts.rare == 0 {
			fmt.Fprintf(&b, "%s\n\n", rareNeedle)
			counts.rare++
		}
	}

	return b.String(), counts
}

func validateCorpus(ctx context.Context, spec corpusSpec, localRoot string, fs client.Client) error {
	localFiles, err := countLocalMarkdown(localRoot)
	if err != nil {
		return err
	}
	if localFiles != spec.MarkdownFiles {
		return fmt.Errorf("local corpus count mismatch: got %d want %d", localFiles, spec.MarkdownFiles)
	}
	tree, err := fs.Tree(ctx, "/", grepMaxDepth)
	if err != nil {
		return err
	}
	redisFiles := 0
	for _, entry := range tree {
		if entry.Type == "file" {
			redisFiles++
		}
	}
	if redisFiles != spec.MarkdownFiles {
		return fmt.Errorf("redis corpus count mismatch: got %d want %d", redisFiles, spec.MarkdownFiles)
	}
	return nil
}

func buildGrepPairs(env *runtimeEnv, spec corpusSpec) []struct {
	name  string
	local grepTarget
	redis grepTarget
} {
	baseLocal := func(pattern string, regex bool) grepTarget {
		cmd := []string{env.grepBin, "-R", "-n", "--include=*.md"}
		if regex {
			cmd = append(cmd, "-E")
		} else {
			cmd = append(cmd, "-F")
		}
		cmd = append(cmd, pattern, ".")
		return grepTarget{
			label:   "grep",
			kind:    "local",
			cmd:     cmd,
			cwd:     env.corpusRoot,
			pattern: pattern,
		}
	}
	baseRedis := func(pattern string, regex bool) grepTarget {
		cmd := []string{env.afsBin, "--config", env.configPath, "grep", "--workspace", spec.Workspace}
		if regex {
			cmd = append(cmd, "-E")
		}
		cmd = append(cmd, pattern)
		return grepTarget{
			label:   "afs grep",
			kind:    "redis",
			cmd:     cmd,
			pattern: pattern,
		}
	}

	return []struct {
		name  string
		local grepTarget
		redis grepTarget
	}{
		{
			name:  "grep_literal_rare",
			local: baseLocal(spec.RareNeedle, false),
			redis: baseRedis(spec.RareNeedle, false),
		},
		{
			name:  "grep_literal_common",
			local: baseLocal(spec.CommonNeedle, false),
			redis: baseRedis(spec.CommonNeedle, false),
		},
		{
			name:  "grep_regex_escalation",
			local: baseLocal(spec.RegexPattern, true),
			redis: baseRedis(spec.RegexPattern, true),
		},
	}
}

func buildRipgrepPairs(env *runtimeEnv, spec corpusSpec) []struct {
	name  string
	local grepTarget
	redis grepTarget
} {
	baseLocal := func(pattern string, regex bool) grepTarget {
		cmd := []string{env.rgBin, "-n", "-g", "*.md"}
		if !regex {
			cmd = append(cmd, "-F")
		}
		cmd = append(cmd, pattern, ".")
		return grepTarget{
			label:   "ripgrep",
			kind:    "local",
			cmd:     cmd,
			cwd:     env.corpusRoot,
			pattern: pattern,
		}
	}
	baseRedis := func(pattern string, regex bool) grepTarget {
		cmd := []string{env.afsBin, "--config", env.configPath, "grep", "--workspace", spec.Workspace}
		if regex {
			cmd = append(cmd, "-E")
		}
		cmd = append(cmd, pattern)
		return grepTarget{
			label:   "afs grep",
			kind:    "redis",
			cmd:     cmd,
			pattern: pattern,
		}
	}

	return []struct {
		name  string
		local grepTarget
		redis grepTarget
	}{
		{
			name:  "rg_literal_rare",
			local: baseLocal(spec.RareNeedle, false),
			redis: baseRedis(spec.RareNeedle, false),
		},
		{
			name:  "rg_literal_common",
			local: baseLocal(spec.CommonNeedle, false),
			redis: baseRedis(spec.CommonNeedle, false),
		},
		{
			name:  "rg_regex_escalation",
			local: baseLocal(spec.RegexPattern, true),
			redis: baseRedis(spec.RegexPattern, true),
		},
	}
}

func buildCountPairs(spec corpusSpec, localRoot string, fs client.Client) []struct {
	name  string
	local countRunner
	redis countRunner
} {
	return []struct {
		name  string
		local countRunner
		redis countRunner
	}{
		{
			name: "tree_walk",
			local: countRunner{
				label: "filepath.WalkDir",
				run: func(ctx context.Context) (int, error) {
					return walkLocal(localRoot)
				},
			},
			redis: countRunner{
				label: "client.Tree",
				run: func(ctx context.Context) (int, error) {
					entries, err := fs.Tree(ctx, "/", grepMaxDepth)
					if err != nil {
						return 0, err
					}
					return len(entries), nil
				},
			},
		},
		{
			name: "find_runbook_names",
			local: countRunner{
				label: "local basename filter",
				run: func(ctx context.Context) (int, error) {
					return findLocalByBase(localRoot, "runbook")
				},
			},
			redis: countRunner{
				label: "client.Find",
				run: func(ctx context.Context) (int, error) {
					matches, err := fs.Find(ctx, "/", spec.FindPattern, "file")
					if err != nil {
						return 0, err
					}
					return len(matches), nil
				},
			},
		},
		{
			name: "read_hot_files",
			local: countRunner{
				label: "os.ReadFile",
				run: func(ctx context.Context) (int, error) {
					return readHotLocal(localRoot, spec.HotFiles)
				},
			},
			redis: countRunner{
				label: "client.Cat",
				run: func(ctx context.Context) (int, error) {
					return readHotRedis(ctx, fs, spec.HotFiles)
				},
			},
		},
		{
			name: "head_hot_files",
			local: countRunner{
				label: "local head(40)",
				run: func(ctx context.Context) (int, error) {
					return headHotLocal(localRoot, spec.HotFiles, 40)
				},
			},
			redis: countRunner{
				label: "client.Head(40)",
				run: func(ctx context.Context) (int, error) {
					return headHotRedis(ctx, fs, spec.HotFiles, 40)
				},
			},
		},
		{
			name: "line_window_hot_files",
			local: countRunner{
				label: "local lines(15..35)",
				run: func(ctx context.Context) (int, error) {
					return linesHotLocal(localRoot, spec.HotFiles, 15, 35)
				},
			},
			redis: countRunner{
				label: "client.Lines(15..35)",
				run: func(ctx context.Context) (int, error) {
					return linesHotRedis(ctx, fs, spec.HotFiles, 15, 35)
				},
			},
		},
	}
}

func benchmarkCommandPair(name string, local, redis grepTarget, warmup, rounds int) (pairSummary, error) {
	localSnapshot, err := collectOutput(local)
	if err != nil {
		return pairSummary{}, err
	}
	redisSnapshot, err := collectOutput(redis)
	if err != nil {
		return pairSummary{}, err
	}

	summary := pairSummary{
		Name:       name,
		LocalLabel: local.label,
		RedisLabel: redis.label,
	}
	if compareSnapshots(localSnapshot, redisSnapshot) {
		summary.Validation = "identical normalized output"
	} else {
		summary.Validation = fmt.Sprintf("different normalized output (local=%d redis=%d)", len(localSnapshot.lines), len(redisSnapshot.lines))
	}

	for i := 0; i < warmup; i++ {
		order := []grepTarget{local, redis}
		if i%2 == 1 {
			order = []grepTarget{redis, local}
		}
		for _, target := range order {
			if _, err := runCommandCount(target); err != nil {
				return pairSummary{}, err
			}
		}
	}

	localSamples := make([]countSample, 0, rounds)
	redisSamples := make([]countSample, 0, rounds)
	for i := 0; i < rounds; i++ {
		order := []grepTarget{local, redis}
		if i%2 == 1 {
			order = []grepTarget{redis, local}
		}
		for _, target := range order {
			sample, err := runCommandCount(target)
			if err != nil {
				return pairSummary{}, err
			}
			if target.kind == "local" {
				localSamples = append(localSamples, sample)
			} else {
				redisSamples = append(redisSamples, sample)
			}
		}
	}

	return finalizeSummary(summary, localSamples, redisSamples), nil
}

func benchmarkCountPair(name string, local, redis countRunner, warmup, rounds int) (pairSummary, error) {
	localCount, err := local.run(context.Background())
	if err != nil {
		return pairSummary{}, err
	}
	redisCount, err := redis.run(context.Background())
	if err != nil {
		return pairSummary{}, err
	}

	summary := pairSummary{
		Name:       name,
		LocalLabel: local.label,
		RedisLabel: redis.label,
	}
	if localCount == redisCount {
		summary.Validation = fmt.Sprintf("counts match (%d)", localCount)
	} else {
		summary.Validation = fmt.Sprintf("count mismatch local=%d redis=%d", localCount, redisCount)
	}

	for i := 0; i < warmup; i++ {
		order := []countRunner{local, redis}
		if i%2 == 1 {
			order = []countRunner{redis, local}
		}
		for _, runner := range order {
			if _, err := timedCountRun(context.Background(), runner.run); err != nil {
				return pairSummary{}, err
			}
		}
	}

	localSamples := make([]countSample, 0, rounds)
	redisSamples := make([]countSample, 0, rounds)
	for i := 0; i < rounds; i++ {
		order := []countRunner{local, redis}
		if i%2 == 1 {
			order = []countRunner{redis, local}
		}
		for _, runner := range order {
			sample, err := timedCountRun(context.Background(), runner.run)
			if err != nil {
				return pairSummary{}, err
			}
			if runner.label == local.label {
				localSamples = append(localSamples, sample)
			} else {
				redisSamples = append(redisSamples, sample)
			}
		}
	}

	return finalizeSummary(summary, localSamples, redisSamples), nil
}

func finalizeSummary(summary pairSummary, localSamples, redisSamples []countSample) pairSummary {
	summary.Local = localSamples
	summary.Redis = redisSamples
	summary.LocalMedian, summary.LocalMin, summary.LocalMax = summarize(localSamples)
	summary.RedisMedian, summary.RedisMin, summary.RedisMax = summarize(redisSamples)
	summary.LocalCount = formatCountSummary(localSamples)
	summary.RedisCount = formatCountSummary(redisSamples)
	if summary.LocalMedian > 0 {
		summary.Ratio = summary.RedisMedian / summary.LocalMedian
	}
	return summary
}

func timedCountRun(ctx context.Context, fn func(context.Context) (int, error)) (countSample, error) {
	started := time.Now()
	count, err := fn(ctx)
	if err != nil {
		return countSample{}, err
	}
	return countSample{
		ElapsedMS: float64(time.Since(started).Microseconds()) / 1000.0,
		Count:     count,
	}, nil
}

func collectOutput(target grepTarget) (outputSnapshot, error) {
	cmd := exec.Command(target.cmd[0], target.cmd[1:]...)
	if target.cwd != "" {
		cmd.Dir = target.cwd
	}
	out, err := cmd.CombinedOutput()
	cleanOut, _ := splitProfileOutput(out)
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
			if exitCode != 0 && exitCode != 1 {
				return outputSnapshot{}, fmt.Errorf("%s failed: %w\n%s", target.label, err, string(cleanOut))
			}
		} else {
			return outputSnapshot{}, err
		}
	}

	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(cleanOut))
	for scanner.Scan() {
		line := normalizeLine(target.kind, scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return outputSnapshot{}, err
	}
	sort.Strings(lines)
	return outputSnapshot{
		lines:    lines,
		exitCode: exitCode,
	}, nil
}

func runCommandCount(target grepTarget) (countSample, error) {
	started := time.Now()
	cmd := exec.Command(target.cmd[0], target.cmd[1:]...)
	if target.cwd != "" {
		cmd.Dir = target.cwd
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return countSample{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return countSample{}, err
	}
	scanner := bufio.NewScanner(stdout)
	count := 0
	for scanner.Scan() {
		if normalizeLine(target.kind, scanner.Text()) != "" {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return countSample{}, err
	}
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		cleanStderr, _ := splitProfileOutput(stderr.Bytes())
		if !errors.As(err, &exitErr) || (exitErr.ExitCode() != 0 && exitErr.ExitCode() != 1) {
			return countSample{}, fmt.Errorf("%s failed: %w\n%s", target.label, err, string(cleanStderr))
		}
	}
	if target.kind == "redis" && strings.TrimSpace(os.Getenv("AFS_BENCH_PRINT_GREP_PROFILE")) != "" {
		_, profiles := splitProfileOutput(stderr.Bytes())
		for _, profile := range profiles {
			fmt.Printf("  grep-profile: %s\n", profile)
		}
	}
	return countSample{
		ElapsedMS: float64(time.Since(started).Microseconds()) / 1000.0,
		Count:     count,
	}, nil
}

func splitProfileOutput(raw []byte) ([]byte, []string) {
	if len(raw) == 0 {
		return raw, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	var clean bytes.Buffer
	var profiles []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, grepProfilePrefix) {
			profiles = append(profiles, strings.TrimPrefix(line, grepProfilePrefix))
			continue
		}
		clean.WriteString(line)
		clean.WriteByte('\n')
	}
	if clean.Len() == 0 {
		return nil, profiles
	}
	return clean.Bytes(), profiles
}

func normalizeLine(kind, line string) string {
	line = strings.TrimRight(line, "\n")
	if line == "" {
		return ""
	}
	if kind == "redis" {
		return line
	}

	parts := strings.SplitN(line, ":", 3)
	if len(parts) < 3 {
		return ""
	}
	rel, lineNo, content := parts[0], parts[1], parts[2]
	if rel == "." {
		rel = "/"
	} else {
		rel = strings.TrimPrefix(rel, "./")
		rel = "/" + strings.TrimPrefix(rel, "/")
	}
	return rel + ":" + lineNo + ":" + content
}

func compareSnapshots(a, b outputSnapshot) bool {
	if len(a.lines) != len(b.lines) {
		return false
	}
	for i := range a.lines {
		if a.lines[i] != b.lines[i] {
			return false
		}
	}
	return true
}

func summarize(samples []countSample) (median, min, max float64) {
	if len(samples) == 0 {
		return 0, 0, 0
	}
	values := make([]float64, 0, len(samples))
	min = samples[0].ElapsedMS
	max = samples[0].ElapsedMS
	for _, sample := range samples {
		values = append(values, sample.ElapsedMS)
		if sample.ElapsedMS < min {
			min = sample.ElapsedMS
		}
		if sample.ElapsedMS > max {
			max = sample.ElapsedMS
		}
	}
	sort.Float64s(values)
	n := len(values)
	if n%2 == 0 {
		median = (values[n/2-1] + values[n/2]) / 2
	} else {
		median = values[n/2]
	}
	return median, min, max
}

func formatCountSummary(samples []countSample) string {
	if len(samples) == 0 {
		return ""
	}
	set := map[int]struct{}{}
	for _, sample := range samples {
		set[sample.Count] = struct{}{}
	}
	var values []int
	for count := range set {
		values = append(values, count)
	}
	sort.Ints(values)
	if len(values) == 1 {
		return strconv.Itoa(values[0])
	}
	return fmt.Sprintf("%d..%d", values[0], values[len(values)-1])
}

func countLocalMarkdown(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			count++
		}
		return nil
	})
	return count, err
}

func walkLocal(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

func findLocalByBase(root, fragment string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.Contains(strings.ToLower(d.Name()), fragment) {
			count++
		}
		return nil
	})
	return count, err
}

func readHotLocal(root string, hotFiles []string) (int, error) {
	total := 0
	for _, rel := range hotFiles {
		data, err := os.ReadFile(filepath.Join(root, strings.TrimPrefix(rel, "/")))
		if err != nil {
			return 0, err
		}
		total += len(data)
	}
	return total, nil
}

func readHotRedis(ctx context.Context, fs client.Client, hotFiles []string) (int, error) {
	total := 0
	for _, rel := range hotFiles {
		data, err := fs.Cat(ctx, rel)
		if err != nil {
			return 0, err
		}
		total += len(data)
	}
	return total, nil
}

func headHotLocal(root string, hotFiles []string, n int) (int, error) {
	total := 0
	for _, rel := range hotFiles {
		data, err := os.ReadFile(filepath.Join(root, strings.TrimPrefix(rel, "/")))
		if err != nil {
			return 0, err
		}
		total += len(firstNLines(string(data), n))
	}
	return total, nil
}

func headHotRedis(ctx context.Context, fs client.Client, hotFiles []string, n int) (int, error) {
	total := 0
	for _, rel := range hotFiles {
		data, err := fs.Head(ctx, rel, n)
		if err != nil {
			return 0, err
		}
		total += len(data)
	}
	return total, nil
}

func linesHotLocal(root string, hotFiles []string, start, end int) (int, error) {
	total := 0
	for _, rel := range hotFiles {
		data, err := os.ReadFile(filepath.Join(root, strings.TrimPrefix(rel, "/")))
		if err != nil {
			return 0, err
		}
		total += len(lineWindow(string(data), start, end))
	}
	return total, nil
}

func linesHotRedis(ctx context.Context, fs client.Client, hotFiles []string, start, end int) (int, error) {
	total := 0
	for _, rel := range hotFiles {
		data, err := fs.Lines(ctx, rel, start, end)
		if err != nil {
			return 0, err
		}
		total += len(data)
	}
	return total, nil
}

func firstNLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := splitLocalLines(s)
	if len(lines) > n {
		lines = lines[:n]
	}
	return joinLocalLines(lines)
}

func lineWindow(s string, start, end int) string {
	if start <= 0 {
		start = 1
	}
	if end < start {
		return ""
	}
	lines := splitLocalLines(s)
	startIdx := start - 1
	if startIdx >= len(lines) {
		return ""
	}
	endIdx := end
	if endIdx > len(lines) {
		endIdx = len(lines)
	}
	return joinLocalLines(lines[startIdx:endIdx])
}

func splitLocalLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func joinLocalLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func printReport(r report) {
	fmt.Printf("\nMarkdown Search Benchmark\n")
	fmt.Printf("  Platform:   %s\n", r.Platform)
	fmt.Printf("  Go:         %s\n", r.GoVersion)
	fmt.Printf("  Redis:      %s\n", r.RedisVersion)
	fmt.Printf("  Source:     %s\n", r.RedisSource)
	fmt.Printf("  Grep:       %s\n", r.GrepVersion)
	if strings.TrimSpace(r.RipgrepVersion) != "" {
		fmt.Printf("  Ripgrep:    %s\n", r.RipgrepVersion)
	}
	fmt.Printf("  Workspace:  %s\n", r.Corpus.Workspace)
	fmt.Printf("  Corpus:     %d markdown files, %d dirs, %.1f MiB\n",
		r.Corpus.MarkdownFiles, r.Corpus.TotalDirs, float64(r.Corpus.TotalBytes)/(1024.0*1024.0))
	fmt.Printf("  Needles:    rare=%q common=%q regex=%q\n", r.Corpus.RareNeedle, r.Corpus.CommonNeedle, r.Corpus.RegexPattern)

	fmt.Printf("\nCLI grep comparisons\n")
	printTable(r.GrepResults)
	if len(r.RipgrepResults) > 0 {
		fmt.Printf("\nRipgrep comparisons\n")
		printTable(r.RipgrepResults)
	}

	fmt.Printf("\nNearby agent workloads\n")
	printTable(r.OpsResults)
}

func printTable(rows []pairSummary) {
	fmt.Printf("  %-22s %-12s %-12s %-8s %-14s\n", "Operation", "Local ms", "Redis ms", "Ratio", "Validation")
	for _, row := range rows {
		fmt.Printf("  %-22s %-12.2f %-12.2f %-8s %-14s\n",
			row.Name, row.LocalMedian, row.RedisMedian, fmt.Sprintf("%.2fx", row.Ratio), row.Validation)
	}
}

func writeReportFiles(outDir string, r report) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	jsonData, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "report.json"), jsonData, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "summary.md"), []byte(markdownSummary(r)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "grep_results.csv"), []byte(csvSummary(r.GrepResults)), 0o644); err != nil {
		return err
	}
	if len(r.RipgrepResults) > 0 {
		if err := os.WriteFile(filepath.Join(outDir, "ripgrep_results.csv"), []byte(csvSummary(r.RipgrepResults)), 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(outDir, "ops_results.csv"), []byte(csvSummary(r.OpsResults)), 0o644); err != nil {
		return err
	}
	return nil
}

func markdownSummary(r report) string {
	var b strings.Builder
	b.WriteString("# Markdown Search Benchmark\n\n")
	fmt.Fprintf(&b, "- Platform: `%s`\n", r.Platform)
	fmt.Fprintf(&b, "- Go: `%s`\n", r.GoVersion)
	fmt.Fprintf(&b, "- Redis: `%s`\n", r.RedisVersion)
	fmt.Fprintf(&b, "- Redis source: `%s`\n", r.RedisSource)
	fmt.Fprintf(&b, "- Grep: `%s`\n", r.GrepVersion)
	if strings.TrimSpace(r.RipgrepVersion) != "" {
		fmt.Fprintf(&b, "- Ripgrep: `%s`\n", r.RipgrepVersion)
	}
	fmt.Fprintf(&b, "- Corpus: `%d` markdown files across `%d` directories (`%.1f MiB`)\n", r.Corpus.MarkdownFiles, r.Corpus.TotalDirs, float64(r.Corpus.TotalBytes)/(1024.0*1024.0))
	b.WriteString("\n## CLI Grep\n\n")
	b.WriteString(tableMarkdown(r.GrepResults))
	if len(r.RipgrepResults) > 0 {
		b.WriteString("\n## Ripgrep\n\n")
		b.WriteString(tableMarkdown(r.RipgrepResults))
	}
	b.WriteString("\n## Nearby Agent Workloads\n\n")
	b.WriteString(tableMarkdown(r.OpsResults))
	return b.String()
}

func tableMarkdown(rows []pairSummary) string {
	var b strings.Builder
	b.WriteString("| operation | local median ms | redis median ms | redis/local | validation |\n")
	b.WriteString("|---|---:|---:|---:|---|\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "| %s | %.2f | %.2f | %.2fx | %s |\n",
			row.Name, row.LocalMedian, row.RedisMedian, row.Ratio, row.Validation)
	}
	return b.String()
}

func csvSummary(rows []pairSummary) string {
	var b strings.Builder
	b.WriteString("name,local_label,redis_label,local_median_ms,redis_median_ms,ratio,local_count,redis_count,validation\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "%s,%s,%s,%.3f,%.3f,%.3f,%s,%s,%s\n",
			csvEscape(row.Name), csvEscape(row.LocalLabel), csvEscape(row.RedisLabel), row.LocalMedian, row.RedisMedian, row.Ratio,
			csvEscape(row.LocalCount), csvEscape(row.RedisCount), csvEscape(row.Validation))
	}
	return b.String()
}

func csvEscape(s string) string {
	if !strings.ContainsAny(s, ",\"\n") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func firstLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func commandOutput(bin string, args ...string) string {
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return err.Error()
	}
	return string(out)
}
