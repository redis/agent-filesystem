package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// BlobWriterMaxCommands is the max number of queued Redis commands per
	// pipeline flush. Mirrors workspaceFSWriteBatchEntries so both writers
	// exert similar pressure on Redis.
	BlobWriterMaxCommands = 512
	// BlobWriterMaxBytes is the max queued payload bytes per pipeline flush.
	BlobWriterMaxBytes = 8 << 20 // 8 MiB
)

// BlobWriter pipelines blob and blob-ref writes to Redis, flushing on byte or
// command-count thresholds. It is safe for a single goroutine to call Submit
// sequentially; multiple producers must serialize externally or use a mutex.
//
// On a fresh import every blob reference is brand new (ref count 1 with no
// prior record), so BlobWriter does not read existing refs before writing,
// which cuts the Redis round trips in half.
type BlobWriter struct {
	rdb       redis.Cmdable
	workspace string
	createdAt time.Time

	mu            sync.Mutex
	pipe          redis.Pipeliner
	queuedCmds    int
	queuedBytes   int64
	seen          map[string]struct{}
	totalBlobs    int64
	totalBytes    int64

	FlushMaxCommands int
	FlushMaxBytes    int64
}

// NewBlobWriter returns a BlobWriter that writes into the given workspace.
// The `rdb` argument can be either a *redis.Client or a redis.Pipeliner from
// an enclosing transaction.
func NewBlobWriter(rdb redis.Cmdable, workspace string, createdAt time.Time) *BlobWriter {
	return &BlobWriter{
		rdb:              rdb,
		workspace:        workspace,
		createdAt:        createdAt.UTC(),
		seen:             make(map[string]struct{}),
		FlushMaxCommands: BlobWriterMaxCommands,
		FlushMaxBytes:    BlobWriterMaxBytes,
	}
}

// Submit queues a blob body and its ref-count record for writing. Duplicate
// blob IDs submitted during the lifetime of the writer are silently deduped:
// the second submission is a no-op (the first copy is assumed to be identical
// since blob IDs are content hashes).
func (w *BlobWriter) Submit(ctx context.Context, blobID string, data []byte, size int64) error {
	if blobID == "" {
		return fmt.Errorf("BlobWriter.Submit: empty blobID")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.seen[blobID]; ok {
		return nil
	}
	w.seen[blobID] = struct{}{}

	ref := blobRef{
		BlobID:    blobID,
		Size:      size,
		RefCount:  1,
		CreatedAt: w.createdAt,
	}
	refBytes, err := json.Marshal(ref)
	if err != nil {
		return err
	}

	weight := int64(len(data) + len(refBytes) + len(blobID)*2 + 128)
	if w.pipe == nil {
		w.pipe = w.newPipeline()
	}
	if w.queuedCmds >= w.FlushMaxCommands || (w.queuedBytes > 0 && w.queuedBytes+weight > w.FlushMaxBytes) {
		if err := w.flushLocked(ctx); err != nil {
			return err
		}
	}
	if w.pipe == nil {
		w.pipe = w.newPipeline()
	}
	w.pipe.Set(ctx, blobKey(w.workspace, blobID), data, 0)
	w.pipe.Set(ctx, blobRefKey(w.workspace, blobID), refBytes, 0)
	w.queuedCmds += 2
	w.queuedBytes += weight
	w.totalBlobs++
	w.totalBytes += int64(len(data))
	return nil
}

// Flush drains any queued commands to Redis. Must be called before the import
// is considered complete.
func (w *BlobWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked(ctx)
}

// Totals returns the cumulative number of unique blobs written and their total
// byte size.
func (w *BlobWriter) Totals() (int64, int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.totalBlobs, w.totalBytes
}

func (w *BlobWriter) flushLocked(ctx context.Context) error {
	if w.pipe == nil || w.queuedCmds == 0 {
		w.queuedCmds = 0
		w.queuedBytes = 0
		return nil
	}
	if _, err := w.pipe.Exec(ctx); err != nil {
		return err
	}
	w.pipe = nil
	w.queuedCmds = 0
	w.queuedBytes = 0
	return nil
}

func (w *BlobWriter) newPipeline() redis.Pipeliner {
	switch rdb := w.rdb.(type) {
	case *redis.Client:
		return rdb.Pipeline()
	case redis.Pipeliner:
		return rdb
	default:
		// Fall back to a best-effort pipeline. Callers must pass a client or
		// pipeliner; other redis.Cmdable implementations are not supported.
		if pipelined, ok := w.rdb.(interface{ Pipeline() redis.Pipeliner }); ok {
			return pipelined.Pipeline()
		}
		panic(fmt.Sprintf("BlobWriter: unsupported redis.Cmdable %T", w.rdb))
	}
}

// SaveBlobsBatch writes an entire map of blobs (plus their ref records) to
// Redis using the pipelined writer. Intended for callers that already hold a
// complete in-memory map (e.g., HTTP fork). Import path uses Submit directly
// to stream blobs as they're hashed.
func (s *Store) SaveBlobsBatch(ctx context.Context, workspace string, blobs map[string][]byte, sizes map[string]int64, createdAt time.Time) error {
	writer := NewBlobWriter(s.rdb, workspace, createdAt)
	for id, data := range blobs {
		size, ok := sizes[id]
		if !ok {
			size = int64(len(data))
		}
		if err := writer.Submit(ctx, id, data, size); err != nil {
			return err
		}
	}
	return writer.Flush(ctx)
}
