package main

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/mount/client"
)

func TestCmdGrepUsesCurrentWorkspaceAndPrintsLiteralMatches(t *testing.T) {
	t.Helper()

	_, store, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	writeLiveAFSFile(t, store, "repo", "/notes.txt", "hello\nbye\n")
	writeLiveAFSFile(t, store, "repo", "/nested/app.txt", "say hello again\n")

	output, err := captureStdout(t, func() error {
		return cmdGrep([]string{"grep", "hello"})
	})
	if err != nil {
		t.Fatalf("cmdGrep() returned error: %v", err)
	}

	if !strings.Contains(output, "/notes.txt:1:hello") {
		t.Fatalf("cmdGrep() output = %q, want /notes.txt literal match", output)
	}
	if !strings.Contains(output, "/nested/app.txt:1:say hello again") {
		t.Fatalf("cmdGrep() output = %q, want /nested/app.txt literal match", output)
	}
}

func TestCmdGrepSupportsPathScopeAndIgnoreCase(t *testing.T) {
	t.Helper()

	_, store, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	writeLiveAFSFile(t, store, "repo", "/logs/app.log", "Error: boom\nok\n")
	writeLiveAFSFile(t, store, "repo", "/src/main.go", "error should stay out of scope\n")

	output, err := captureStdout(t, func() error {
		return cmdGrep([]string{"grep", "--path", "/logs", "-i", "error"})
	})
	if err != nil {
		t.Fatalf("cmdGrep() returned error: %v", err)
	}

	if !strings.Contains(output, "/logs/app.log:1:Error: boom") {
		t.Fatalf("cmdGrep() output = %q, want scoped match", output)
	}
	if strings.Contains(output, "/src/main.go") {
		t.Fatalf("cmdGrep() output = %q, want path scope to exclude /src/main.go", output)
	}
}

func TestCmdGrepSupportsNativeGlobMode(t *testing.T) {
	t.Helper()

	_, store, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	writeLiveAFSFile(t, store, "repo", "/todo.txt", "TODO one\nprefix TODO two\n")

	output, err := captureStdout(t, func() error {
		return cmdGrep([]string{"grep", "--glob", "TODO*"})
	})
	if err != nil {
		t.Fatalf("cmdGrep() returned error: %v", err)
	}

	if !strings.Contains(output, "/todo.txt:1:TODO one") {
		t.Fatalf("cmdGrep() output = %q, want leading TODO glob match", output)
	}
	if strings.Contains(output, "prefix TODO two") {
		t.Fatalf("cmdGrep() output = %q, want glob mode to preserve anchored semantics", output)
	}
}

func TestCmdGrepSupportsRegexAndFilesWithMatches(t *testing.T) {
	t.Helper()

	_, store, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	writeLiveAFSFile(t, store, "repo", "/logs/app.log", "Error: boom\n")
	writeLiveAFSFile(t, store, "repo", "/logs/worker.log", "warning: queued\n")
	writeLiveAFSFile(t, store, "repo", "/logs/ok.log", "all clear\n")

	output, err := captureStdout(t, func() error {
		return cmdGrep([]string{"grep", "-E", "-l", "--path", "/logs", "Error|warning"})
	})
	if err != nil {
		t.Fatalf("cmdGrep() returned error: %v", err)
	}

	if !strings.Contains(output, "/logs/app.log") {
		t.Fatalf("cmdGrep() output = %q, want /logs/app.log", output)
	}
	if !strings.Contains(output, "/logs/worker.log") {
		t.Fatalf("cmdGrep() output = %q, want /logs/worker.log", output)
	}
	if strings.Contains(output, "/logs/ok.log") {
		t.Fatalf("cmdGrep() output = %q, want /logs/ok.log excluded", output)
	}
}

func TestCmdGrepSupportsWordLineAndCountModes(t *testing.T) {
	t.Helper()

	_, store, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	writeLiveAFSFile(t, store, "repo", "/words.txt", "token\nsubtoken\nTOKEN\n")

	wordOutput, err := captureStdout(t, func() error {
		return cmdGrep([]string{"grep", "-i", "-w", "token"})
	})
	if err != nil {
		t.Fatalf("cmdGrep(-w) returned error: %v", err)
	}
	if !strings.Contains(wordOutput, "/words.txt:1:token") || !strings.Contains(wordOutput, "/words.txt:3:TOKEN") {
		t.Fatalf("cmdGrep(-w) output = %q, want whole-word matches", wordOutput)
	}
	if strings.Contains(wordOutput, "subtoken") {
		t.Fatalf("cmdGrep(-w) output = %q, want subtoken excluded", wordOutput)
	}

	lineOutput, err := captureStdout(t, func() error {
		return cmdGrep([]string{"grep", "-x", "token"})
	})
	if err != nil {
		t.Fatalf("cmdGrep(-x) returned error: %v", err)
	}
	if !strings.Contains(lineOutput, "/words.txt:1:token") {
		t.Fatalf("cmdGrep(-x) output = %q, want exact line match", lineOutput)
	}
	if strings.Contains(lineOutput, "TOKEN") || strings.Contains(lineOutput, "subtoken") {
		t.Fatalf("cmdGrep(-x) output = %q, want only the exact lowercase token line", lineOutput)
	}

	countOutput, err := captureStdout(t, func() error {
		return cmdGrep([]string{"grep", "-c", "-i", "token"})
	})
	if err != nil {
		t.Fatalf("cmdGrep(-c) returned error: %v", err)
	}
	if !strings.Contains(countOutput, "/words.txt:3") {
		t.Fatalf("cmdGrep(-c) output = %q, want three matches counted", countOutput)
	}
}

func TestCmdGrepSupportsInvertMatchAndMaxCount(t *testing.T) {
	t.Helper()

	_, store, closeStore := setupAFSGrepTest(t)
	defer closeStore()

	writeLiveAFSFile(t, store, "repo", "/invert.txt", "hello\nskip\nhello again\n")

	invertOutput, err := captureStdout(t, func() error {
		return cmdGrep([]string{"grep", "-v", "hello"})
	})
	if err != nil {
		t.Fatalf("cmdGrep(-v) returned error: %v", err)
	}
	if !strings.Contains(invertOutput, "/invert.txt:2:skip") {
		t.Fatalf("cmdGrep(-v) output = %q, want the non-matching line", invertOutput)
	}
	if strings.Contains(invertOutput, "hello again") || strings.Contains(invertOutput, "/invert.txt:1:hello") {
		t.Fatalf("cmdGrep(-v) output = %q, want matching lines excluded", invertOutput)
	}

	limitedOutput, err := captureStdout(t, func() error {
		return cmdGrep([]string{"grep", "-m", "1", "hello"})
	})
	if err != nil {
		t.Fatalf("cmdGrep(-m) returned error: %v", err)
	}
	if strings.Count(limitedOutput, "/invert.txt:") != 1 {
		t.Fatalf("cmdGrep(-m) output = %q, want only one emitted match", limitedOutput)
	}
}

func setupAFSGrepTest(t *testing.T) (config, *afsStore, func()) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := defaultConfig()
	cfg.RedisAddr = mr.Addr()
	cfg.MountBackend = mountBackendNone
	cfg.WorkRoot = t.TempDir()
	cfg.CurrentWorkspace = "repo"
	saveTempConfig(t, cfg)

	loadedCfg, store, closeStore, err := openAFSStore(context.Background())
	if err != nil {
		t.Fatalf("openAFSStore() returned error: %v", err)
	}

	if err := createEmptyWorkspace(context.Background(), loadedCfg, store, "repo"); err != nil {
		closeStore()
		t.Fatalf("createEmptyWorkspace() returned error: %v", err)
	}

	return loadedCfg, store, closeStore
}

func writeLiveAFSFile(t *testing.T, store *afsStore, workspace, p, content string) {
	t.Helper()

	if err := client.New(store.rdb, workspace).Echo(context.Background(), p, []byte(content)); err != nil {
		t.Fatalf("Echo(%s) returned error: %v", p, err)
	}
}
