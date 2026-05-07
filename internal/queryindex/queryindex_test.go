package queryindex

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/agent-filesystem/internal/rediscontent"
	"github.com/redis/go-redis/v9"
)

func setupQueryIndexTest(t *testing.T) (context.Context, *redis.Client, string) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})
	return context.Background(), rdb, "queryindex-test"
}

func writeQueryIndexFile(t *testing.T, ctx context.Context, rdb *redis.Client, fsKey, inodeID, filePath string, data []byte) {
	t.Helper()
	writeQueryIndexFileWithDirty(t, ctx, rdb, fsKey, inodeID, filePath, data, true)
}

func writeQueryIndexFileWithDirty(t *testing.T, ctx context.Context, rdb *redis.Client, fsKey, inodeID, filePath string, data []byte, dirty bool) {
	t.Helper()
	if err := rdb.Set(ctx, ContentKey(fsKey, inodeID), data, 0).Err(); err != nil {
		t.Fatalf("SET content: %v", err)
	}
	if err := rdb.HSet(ctx, InodeKey(fsKey, inodeID), map[string]interface{}{
		"type":           "file",
		"path":           filePath,
		"path_ancestors": IndexedPathAncestors(filePath),
		"content_ref":    rediscontent.RefExternal,
		"size":           len(data),
	}).Err(); err != nil {
		t.Fatalf("HSET inode: %v", err)
	}
	if !dirty {
		return
	}
	if err := rdb.SAdd(ctx, DirtySetKey(fsKey), inodeID).Err(); err != nil {
		t.Fatalf("SADD dirty: %v", err)
	}
}

func TestProcessPendingIndexesTextFileChunks(t *testing.T) {
	ctx, rdb, fsKey := setupQueryIndexTest(t)
	writeQueryIndexFile(t, ctx, rdb, fsKey, "1", "/docs/guide.md", []byte("hello query index\ncheckpoint savepoint\n"))

	result, err := ProcessPending(ctx, rdb, fsKey, 10)
	if err != nil {
		t.Fatalf("ProcessPending() returned error: %v", err)
	}
	if result.Processed != 1 || result.Indexed != 1 || result.Pending != 0 {
		t.Fatalf("ProcessPending() = %+v, want one indexed item", result)
	}
	state, err := rdb.HGet(ctx, InodeKey(fsKey, "1"), "query_state").Result()
	if err != nil {
		t.Fatalf("HGET query_state: %v", err)
	}
	if state != StateReady {
		t.Fatalf("query_state = %q, want %q", state, StateReady)
	}
	chunkKeys, err := rdb.SMembers(ctx, ChunkSetKey(fsKey, "1")).Result()
	if err != nil {
		t.Fatalf("SMEMBERS chunk set: %v", err)
	}
	if len(chunkKeys) != 1 {
		t.Fatalf("chunk keys = %#v, want one chunk", chunkKeys)
	}
	text, err := rdb.HGet(ctx, chunkKeys[0], "text").Result()
	if err != nil {
		t.Fatalf("HGET chunk text: %v", err)
	}
	if !strings.Contains(text, "checkpoint savepoint") {
		t.Fatalf("chunk text = %q, want indexed file content", text)
	}
}

func TestBuildChunksSplitsAdjacentJSONLRecords(t *testing.T) {
	text := `{"display":"redis connection refused"} {"display":"module is not loaded"} {"display":"daemon status rows"}`

	chunks := BuildChunks("fs", "1", "/history.jsonl", "", "hash", text)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want one chunk per adjacent record", chunks)
	}
	for i, chunk := range chunks {
		if chunk.StartLine != 1 || chunk.EndLine != 1 {
			t.Fatalf("chunk %d line range = %d-%d, want physical line 1", i, chunk.StartLine, chunk.EndLine)
		}
	}
	if strings.Contains(chunks[1].Text, "connection refused") || !strings.Contains(chunks[1].Text, "module is not loaded") {
		t.Fatalf("middle chunk text = %q, want isolated module record", chunks[1].Text)
	}
}

func TestBuildChunksKeepsJSONLRecordsSeparateAcrossPhysicalLines(t *testing.T) {
	text := strings.Join([]string{
		`{"display":"redis connection refused"}`,
		`{"display":"module is not loaded"}`,
		`{"display":"daemon status rows"}`,
	}, "\n")

	chunks := BuildChunks("fs", "1", "/history.jsonl", "", "hash", text)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want one chunk per JSONL record", chunks)
	}
	if chunks[1].StartLine != 2 || chunks[1].EndLine != 2 || strings.Contains(chunks[1].Text, "connection refused") {
		t.Fatalf("middle chunk = %+v, want isolated second record", chunks[1])
	}
}

func TestInspectTreatsOldProjectionVersionAsUnindexed(t *testing.T) {
	ctx, rdb, fsKey := setupQueryIndexTest(t)
	writeQueryIndexFileWithDirty(t, ctx, rdb, fsKey, "1", "/docs/guide.md", []byte("existing file content\n"), false)
	if err := rdb.HSet(ctx, InodeKey(fsKey, "1"), "query_state", StateReady).Err(); err != nil {
		t.Fatalf("HSET old ready state: %v", err)
	}

	status, err := Inspect(ctx, rdb, fsKey, "/")
	if err != nil {
		t.Fatalf("Inspect() returned error: %v", err)
	}
	if status.Unindexed != 1 || status.Ready != 0 {
		t.Fatalf("Inspect() = %+v, want old projection marked unindexed", status)
	}
}

func TestInspectAndRebuildBackfillExistingFiles(t *testing.T) {
	ctx, rdb, fsKey := setupQueryIndexTest(t)
	writeQueryIndexFileWithDirty(t, ctx, rdb, fsKey, "1", "/docs/guide.md", []byte("existing file content\n"), false)

	status, err := Inspect(ctx, rdb, fsKey, "/")
	if err != nil {
		t.Fatalf("Inspect() returned error: %v", err)
	}
	if status.Files != 1 || status.Unindexed != 1 {
		t.Fatalf("Inspect() = %+v, want one unindexed file", status)
	}

	rebuild, err := Rebuild(ctx, rdb, fsKey, RebuildOptions{Path: "/", Wait: true})
	if err != nil {
		t.Fatalf("Rebuild() returned error: %v", err)
	}
	if rebuild.Enqueued != 1 || rebuild.Process.Indexed != 1 || rebuild.Status.Ready != 1 || rebuild.Status.Unindexed != 0 {
		t.Fatalf("Rebuild() = %+v, want indexed status", rebuild)
	}
}

func TestProcessPendingSkipsUnsupportedAndBinaryFiles(t *testing.T) {
	ctx, rdb, fsKey := setupQueryIndexTest(t)
	writeQueryIndexFile(t, ctx, rdb, fsKey, "pdf", "/docs/report.pdf", []byte("%PDF text that requires an extractor"))
	writeQueryIndexFile(t, ctx, rdb, fsKey, "bin", "/bin/blob.txt", []byte{'a', 0, 'b'})

	result, err := ProcessPending(ctx, rdb, fsKey, 10)
	if err != nil {
		t.Fatalf("ProcessPending() returned error: %v", err)
	}
	if result.Skipped != 2 || result.Pending != 0 {
		t.Fatalf("ProcessPending() = %+v, want two skipped items", result)
	}
	assertSkipReason(t, ctx, rdb, fsKey, "pdf", SkipUnsupported)
	assertSkipReason(t, ctx, rdb, fsKey, "bin", SkipBinary)
}

func TestReindexFileCleansDeletedProjection(t *testing.T) {
	ctx, rdb, fsKey := setupQueryIndexTest(t)
	chunkKey := ChunkKey(fsKey, "missing", "hash", 0)
	if err := rdb.HSet(ctx, chunkKey, "type", "chunk", "text", "stale").Err(); err != nil {
		t.Fatalf("HSET stale chunk: %v", err)
	}
	if err := rdb.SAdd(ctx, ChunkSetKey(fsKey, "missing"), chunkKey).Err(); err != nil {
		t.Fatalf("SADD stale chunk: %v", err)
	}
	if err := rdb.SAdd(ctx, DirtySetKey(fsKey), "missing").Err(); err != nil {
		t.Fatalf("SADD dirty: %v", err)
	}

	item, err := ReindexFile(ctx, rdb, fsKey, "missing")
	if err != nil {
		t.Fatalf("ReindexFile() returned error: %v", err)
	}
	if item.State != StateSkipped || item.Reason != SkipMissing {
		t.Fatalf("ReindexFile() = %+v, want skipped missing", item)
	}
	if exists, err := rdb.Exists(ctx, chunkKey, ChunkSetKey(fsKey, "missing")).Result(); err != nil {
		t.Fatalf("EXISTS stale keys: %v", err)
	} else if exists != 0 {
		t.Fatalf("stale projection keys still exist: %d", exists)
	}
	if pending, err := PendingCount(ctx, rdb, fsKey); err != nil {
		t.Fatalf("PendingCount() returned error: %v", err)
	} else if pending != 0 {
		t.Fatalf("pending = %d, want 0", pending)
	}
}

func assertSkipReason(t *testing.T, ctx context.Context, rdb *redis.Client, fsKey, inodeID, want string) {
	t.Helper()
	got, err := rdb.HGet(ctx, InodeKey(fsKey, inodeID), "query_skip_reason").Result()
	if err != nil {
		t.Fatalf("HGET query_skip_reason for %s: %v", inodeID, err)
	}
	if got != want {
		t.Fatalf("query_skip_reason for %s = %q, want %q", inodeID, got, want)
	}
}
