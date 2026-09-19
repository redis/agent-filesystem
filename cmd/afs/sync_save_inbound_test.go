package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/agent-filesystem/mount/client"
)

type syncSaveInboundClient struct {
	client.Client
	entered chan context.Context
	release chan struct{}
	gated   atomic.Bool
}

func (c *syncSaveInboundClient) Cat(ctx context.Context, path string) ([]byte, error) {
	data, err := c.Client.Cat(ctx, path)
	if path == "/file" && c.gated.CompareAndSwap(false, true) {
		c.entered <- ctx
		// A client can return a successful read as cancellation arrives.
		<-c.release
	}
	return data, err
}

func TestSyncSaveCancelsInboundReadBeforeReplacingLocalEdit(t *testing.T) {
	env := newSyncTestEnv(t)
	gate := &syncSaveInboundClient{Client: env.fsClient, entered: make(chan context.Context, 1), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	d := env.startDaemon(t, func(cfg *syncDaemonConfig) { cfg.FS = gate })
	if err := d.watcher.Close(); err != nil {
		t.Fatal(err)
	}
	service := &syncSaveService{ctx: context.Background(), active: d}
	t.Cleanup(func() {
		if service.active != nil {
			service.active.Stop()
		}
	})
	abs := env.writeLocalFile(t, "file", "base")
	env.writeRemoteFile(t, "file", "peer")
	baseline := SyncEntry{Type: "file", Size: 4, Mode: 0o644,
		LocalHash: sha256Hex([]byte("base")), RemoteHash: sha256Hex([]byte("base"))}
	d.stateWriter.mu.Lock()
	d.stateWriter.state.Entries["file"] = baseline
	d.stateWriter.mu.Unlock()
	d.reconciler.queueDownload(downloadOp{Kind: opDownloadFile, Path: "file", AbsPath: abs,
		StoredEntry: baseline, HasStored: true})
	var readCtx context.Context
	select {
	case readCtx = <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not enter its read")
	}
	env.writeLocalFile(t, "file", "completed local edit")
	done := make(chan syncControlResult, 1)
	go func() { done <- service.save(saveRequestForDaemon(d, 5*time.Second)) }()
	select {
	case <-d.downloader.stopCh:
	case <-time.After(5 * time.Second):
		t.Fatal("save did not stop inbound work")
	}
	if !errors.Is(readCtx.Err(), context.Canceled) {
		t.Fatalf("inbound read survived generation stop: %v", readCtx.Err())
	}
	release.Do(func() { close(gate.release) })
	select {
	case result := <-done:
		if result.Success || result.Save != nil || !strings.Contains(result.Error, "conflict") {
			t.Fatalf("divergent peer bytes were certified: %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("save did not join the cancelled read")
	}
	if got := env.readLocalFile(t, "file"); got != "completed local edit" {
		t.Fatalf("inbound drain changed the completed local edit: %q", got)
	}
	if got := env.readRemoteFile(t, "file"); got != "peer" {
		t.Fatalf("save overwrote the conflicting remote edit: %q", got)
	}
}

func TestSyncSaveCancelledDownloadDoesNotMutateLocal(t *testing.T) {
	for _, kind := range []downloadOpKind{opDownloadFile, opDownloadSymlink, opDownloadMkdir, opDownloadDelete, opDownloadChmod} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "local")
			if err := os.WriteFile(path, []byte("local bytes"), 0o640); err != nil {
				t.Fatal(err)
			}
			results := make(chan downloadResult, 1)
			downloader := newDownloader(nil, results, root, newConflictNamer(), newEchoSuppressor(), false, newSyncLogger(false))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			downloader.process(ctx, downloadOp{Kind: kind, Path: "local", AbsPath: path, Mode: 0o777, Symlink: "other"})
			result := <-results
			if !errors.Is(result.Err, context.Canceled) {
				t.Fatalf("cancelled operation = %+v", result)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "local bytes" {
				t.Fatalf("cancelled operation changed bytes: %q, %v", data, err)
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o640 {
				t.Fatalf("cancelled operation changed type or mode: %v, %v", info, err)
			}
		})
	}
}

type syncSaveChunkReadClient struct {
	client.Client
	cancel context.CancelFunc
}

func (c *syncSaveChunkReadClient) InvalidateCache() {}
func (c *syncSaveChunkReadClient) ChunkMeta(context.Context, string) (int, []string, error) {
	return 4, []string{sha256Hex([]byte("abcd")), sha256Hex([]byte("EDIT"))}, nil
}

func (c *syncSaveChunkReadClient) Stat(context.Context, string) (*client.StatResult, error) {
	return &client.StatResult{Type: "file", Size: 8, Mode: 0o644}, nil
}

func (c *syncSaveChunkReadClient) ReadChunks(context.Context, string, []int, int) (map[int][]byte, error) {
	if c.cancel != nil {
		c.cancel()
	}
	return map[int][]byte{1: []byte("EDIT")}, nil
}

func TestSyncSaveChunkDownloadStagesDeltaAndHonorsCancellation(t *testing.T) {
	for _, cancelRead := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelRead), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "file")
			if err := os.WriteFile(path, []byte("abcdefgh"), 0o640); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			remote := &syncSaveChunkReadClient{}
			if cancelRead {
				remote.cancel = cancel
			}
			results := make(chan downloadResult, 1)
			downloader := newDownloader(remote, results, root, newConflictNamer(), newEchoSuppressor(), false, newSyncLogger(false))
			downloader.process(ctx, downloadOp{Kind: opDownloadFile, Path: "file", AbsPath: path,
				Chunked: true, ChunkSize: 4, FileSize: 8, DirtyChunks: []int{1},
				HasStored: true, StoredEntry: SyncEntry{Type: "file", Size: 8, Mode: 0o640, ChunkSize: 4, LocalHash: compositeHash([]string{sha256Hex([]byte("abcd")), sha256Hex([]byte("efgh"))})},
				ChunkHashes: []string{sha256Hex([]byte("abcd")), sha256Hex([]byte("EDIT"))}})
			result := <-results
			want := "abcdEDIT"
			if cancelRead {
				want = "abcdefgh"
				if !errors.Is(result.Err, context.Canceled) {
					t.Fatalf("cancelled chunk read = %+v", result)
				}
			} else if result.Err != nil {
				t.Fatalf("normal chunk delta = %+v", result)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != want {
				t.Fatalf("chunk destination = %q, %v; want %q", data, err, want)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 1 || entries[0].Name() != "file" {
				t.Fatalf("chunk download left temporary files: %v, %v", entries, err)
			}
		})
	}
}

func TestSyncSaveCancelledStagedDownloadPreservesDestination(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		root := t.TempDir()
		path := filepath.Join(root, "local")
		if err := os.WriteFile(path, []byte("local bytes"), 0o640); err != nil {
			t.Fatal(err)
		}
		downloader := newDownloader(nil, nil, root, newConflictNamer(), newEchoSuppressor(), false, newSyncLogger(false))
		ctx, cancel := context.WithCancel(context.Background())
		moved, err := downloader.writeLocalFile(ctx, downloadOp{Path: "local", AbsPath: path, Conflict: conflict}, 0o644, func(file *os.File) error {
			_, err := file.Write([]byte("partially staged remote bytes"))
			cancel()
			return err
		})
		if !errors.Is(err, context.Canceled) || moved != "" {
			t.Fatalf("cancelled staged write = %q, %v", moved, err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "local bytes" {
			t.Fatalf("cancelled staged write changed destination: %q, %v", data, err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 1 || entries[0].Name() != "local" {
			t.Fatalf("cancelled staged write left a temporary or conflict file: %v, %v", entries, err)
		}
	}
}

func TestSyncSaveInboundSymlinkReplacement(t *testing.T) {
	for _, kind := range []string{"file", "symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "local")
			var err error
			switch kind {
			case "file":
				err = os.WriteFile(path, []byte("old"), 0o644)
			case "symlink":
				err = os.Symlink("old", path)
			case "directory":
				err = os.Mkdir(path, 0o755)
			}
			if err != nil {
				t.Fatal(err)
			}
			stored := SyncEntry{Type: kind, Size: 3, Mode: 0o644, LocalHash: sha256Hex([]byte("old"))}
			if kind == "symlink" {
				stored.Target = "old"
			} else if kind == "directory" {
				stored.Type, stored.Mode = "dir", 0o755
			}
			results := make(chan downloadResult, 1)
			downloader := newDownloader(nil, results, root, newConflictNamer(), newEchoSuppressor(), false, newSyncLogger(false))
			downloader.process(context.Background(), downloadOp{Kind: opDownloadSymlink, Path: "local", AbsPath: path, Symlink: "new", HasStored: true, StoredEntry: stored})
			if result := <-results; result.Err != nil {
				t.Fatal(result.Err)
			}
			if target, err := os.Readlink(path); err != nil || target != "new" {
				t.Fatalf("replacement symlink = %q, %v", target, err)
			}
			if entries, err := os.ReadDir(root); err != nil || len(entries) != 1 {
				t.Fatalf("temporary symlink remains: %v, %v", entries, err)
			}
		})
	}
}

func TestSyncSaveStagedDownloadPreservesConcurrentLocalEdit(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "local")
			if err := os.WriteFile(path, []byte("baseline"), 0o640); err != nil {
				t.Fatal(err)
			}
			d := newDownloader(nil, nil, root, newConflictNamer(), newEchoSuppressor(), false, newSyncLogger(false))
			moved, err := d.writeLocalFile(context.Background(), downloadOp{Path: "local", AbsPath: path, Conflict: conflict}, 0o644, func(file *os.File) error {
				if _, err := file.Write([]byte("remote")); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("concurrent local edit"), 0o640)
			})
			if !errors.Is(err, errSyncDownloadLocalChanged) || moved != "" {
				t.Fatalf("staged write = %q, %v", moved, err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "concurrent local edit" {
				t.Fatalf("local edit lost: %q, %v", data, err)
			}
			if entries, err := os.ReadDir(root); err != nil || len(entries) != 1 {
				t.Fatalf("staging/conflict file left behind: %v, %v", entries, err)
			}
		})
	}
}
