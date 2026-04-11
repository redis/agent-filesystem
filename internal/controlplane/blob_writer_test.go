package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestBlobWriterFlushesAllBlobs(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	writer := NewBlobWriter(store.rdb, "demo", time.Unix(100, 0))
	blobs := map[string][]byte{
		"aaaa": bytes.Repeat([]byte("a"), 64),
		"bbbb": bytes.Repeat([]byte("b"), 128),
		"cccc": bytes.Repeat([]byte("c"), 256),
	}
	for id, data := range blobs {
		if err := writer.Submit(ctx, id, data, int64(len(data))); err != nil {
			t.Fatalf("submit %s: %v", id, err)
		}
	}
	if err := writer.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	for id, want := range blobs {
		got, err := store.rdb.Get(ctx, blobKey("demo", id)).Bytes()
		if err != nil {
			t.Fatalf("get blob %s: %v", id, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("blob %s mismatch", id)
		}
		refRaw, err := store.rdb.Get(ctx, blobRefKey("demo", id)).Bytes()
		if err != nil {
			t.Fatalf("get ref %s: %v", id, err)
		}
		var ref blobRef
		if err := json.Unmarshal(refRaw, &ref); err != nil {
			t.Fatalf("unmarshal ref %s: %v", id, err)
		}
		if ref.RefCount != 1 {
			t.Fatalf("ref %s count = %d, want 1", id, ref.RefCount)
		}
		if ref.Size != int64(len(want)) {
			t.Fatalf("ref %s size = %d, want %d", id, ref.Size, len(want))
		}
	}

	blobCount, byteCount := writer.Totals()
	if blobCount != 3 {
		t.Fatalf("blob count = %d, want 3", blobCount)
	}
	if byteCount != int64(64+128+256) {
		t.Fatalf("byte count = %d, want %d", byteCount, 64+128+256)
	}
}

func TestBlobWriterDedupesByID(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	writer := NewBlobWriter(store.rdb, "demo", time.Unix(100, 0))
	data := bytes.Repeat([]byte("x"), 32)
	for i := 0; i < 5; i++ {
		if err := writer.Submit(ctx, "same", data, 32); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	if err := writer.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	blobCount, byteCount := writer.Totals()
	if blobCount != 1 {
		t.Fatalf("blob count = %d, want 1", blobCount)
	}
	if byteCount != 32 {
		t.Fatalf("byte count = %d, want 32", byteCount)
	}
}

func TestBlobWriterFlushesOnCommandThreshold(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	writer := NewBlobWriter(store.rdb, "demo", time.Unix(100, 0))
	// Force flushes on every submit so batching/flush interleaving is exercised.
	writer.FlushMaxCommands = 2
	writer.FlushMaxBytes = 1 << 30

	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("blob-%d", i)
		if err := writer.Submit(ctx, id, []byte(id), int64(len(id))); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	if err := writer.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("blob-%d", i)
		got, err := store.rdb.Get(ctx, blobKey("demo", id)).Bytes()
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if string(got) != id {
			t.Fatalf("blob %s = %q, want %q", id, got, id)
		}
	}
}

func TestSaveBlobsBatchWritesMap(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	blobs := map[string][]byte{
		"one":   []byte("hello"),
		"two":   []byte("world"),
		"three": []byte("!!"),
	}
	sizes := map[string]int64{"one": 5, "two": 5, "three": 2}
	if err := store.SaveBlobsBatch(ctx, "demo", blobs, sizes, time.Now()); err != nil {
		t.Fatalf("SaveBlobsBatch: %v", err)
	}
	for id, want := range blobs {
		got, err := store.rdb.Get(ctx, blobKey("demo", id)).Bytes()
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("blob %s mismatch", id)
		}
	}
}
