package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestAtomicCommitSameBaselineHasOneWinner(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	seed := New(rdb, "two-writers")
	if err := seed.Echo(ctx, "/f", []byte("base")); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadFileSnapshot(ctx, seed, "/f")
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		err    error
		result MutationResult
		data   []byte
	}
	outcomes := make(chan outcome, 2)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			fs := New(rdb, "two-writers")
			candidate := bytes.Repeat([]byte{byte('A' + i)}, 300000)
			var result MutationResult
			<-start
			err := fs.Echo(WithMutationResult(WithFileMode(snapshot.WriteContext(ctx), 0600), &result), "/f", candidate)
			outcomes <- outcome{err, result, candidate}
		}(i)
	}
	close(start)
	workers.Wait()
	close(outcomes)
	successes, conflicts := 0, 0
	for out := range outcomes {
		if out.err == nil {
			successes++
			got, err := ReadFileSnapshot(ctx, seed, "/f")
			if err != nil || !bytes.Equal(got.Content, out.data) || !SameFileRevision(got.Stat, out.result.Stat) {
				t.Fatalf("winner mismatch: %v", err)
			}
			if got.Stat.Mode != 0600 {
				t.Fatalf("mode not atomic: %+v", got.Stat)
			}
			ttl, err := rdb.TTL(ctx, "afs:{two-writers}:commit:"+out.result.OperationID).Result()
			if err != nil || ttl < commitReceiptTTL-time.Minute {
				t.Fatalf("missing receipt: %v %v", ttl, err)
			}
		} else if errors.Is(out.err, ErrWriteConflict) {
			conflicts++
		} else {
			t.Fatal(out.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflict=%d", successes, conflicts)
	}
	events, err := rdb.XRange(ctx, "afs:{two-writers}:changes", "-", "+").Result()
	if err != nil || len(events) != 2 {
		t.Fatalf("want one event per successful publication (seed and winner), got %d: %v", len(events), err)
	}
	for _, event := range events {
		payload, _ := event.Values["payload"].(string)
		decoded, err := decodeInvalidate([]byte(payload))
		if err != nil || decoded.OperationID == "" {
			t.Fatalf("missing commit identity: %s %v", payload, err)
		}
	}
	info, err := seed.Info(ctx)
	if err != nil || info.TotalDataBytes != 300000 || info.Files != 1 {
		t.Fatalf("accounting: %+v %v", info, err)
	}
	for _, pattern := range []string{"afs:{two-writers}:content:*:stage:*", "afs:{two-writers}:retired:*"} {
		keys, err := rdb.Keys(ctx, pattern).Result()
		if err != nil || len(keys) != 0 {
			t.Fatalf("staging leaked: %v %v", keys, err)
		}
	}
}

type atomicHistoryObserver struct{ before, after VersionedSnapshot }

func (o *atomicHistoryObserver) RecordMutation(_ context.Context, _ string, before, after VersionedSnapshot) error {
	o.before, o.after = before, after
	return nil
}

func TestAtomicCommitHistoryUsesPublishedBytesAfterPeerWrite(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	peer := New(rdb, "history-race")
	if err := peer.Echo(ctx, "/f", []byte("base")); err != nil {
		t.Fatal(err)
	}
	writerRedis := redis.NewClient(rdb.Options())
	t.Cleanup(func() { _ = writerRedis.Close() })
	writerRedis.AddHook(&publicationCommandHook{after: func() {
		if err := peer.Echo(ctx, "/f", []byte("peer")); err != nil {
			t.Fatal(err)
		}
	}})
	observer := &atomicHistoryObserver{}
	writer := NewWithObserver(writerRedis, "history-race", observer)
	if err := writer.Echo(ctx, "/f", []byte("mine")); err != nil {
		t.Fatal(err)
	}
	if string(observer.before.Content) != "base" || string(observer.after.Content) != "mine" {
		t.Fatalf("misattributed history: %q -> %q", observer.before.Content, observer.after.Content)
	}
	if got, _ := peer.Cat(ctx, "/f"); string(got) != "peer" {
		t.Fatalf("peer overwritten: %q", got)
	}
}

func TestAtomicCommitChunkCopyUsesSelectedDatabase(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	opts := *rdb.Options()
	opts.DB = 3
	selected := redis.NewClient(&opts)
	t.Cleanup(func() { _ = selected.Close() })
	c := New(selected, "selected-db")
	if err := c.Echo(ctx, "/f", []byte("aaaabbbb")); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteChunks(ctx, "/f", map[int][]byte{1: []byte("BBBB")}, 4, 8, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Cat(ctx, "/f"); err != nil || string(got) != "aaaaBBBB" {
		t.Fatalf("copied from wrong database: %q %v", got, err)
	}
	st, _ := c.Stat(ctx, "/f")
	if err := c.WriteInodeAt(ctx, st.Inode, []byte("A"), 0); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Cat(ctx, "/f"); err != nil || string(got) != "AaaaBBBB" {
		t.Fatalf("range copied from wrong database: %q %v", got, err)
	}
	if n, err := rdb.DBSize(ctx).Result(); err != nil || n != 0 {
		t.Fatalf("leaked staging into DB 0: %d %v", n, err)
	}
}

func BenchmarkAtomicFilePublication(b *testing.B) {
	for _, size := range []int{1024, 1 << 20} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			rdb, ctx := setupBenchRedis(b)
			c := New(rdb, "bench-publication")
			data := bytes.Repeat([]byte("x"), size)
			if err := c.Echo(ctx, "/f", data); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := c.Echo(ctx, "/f", data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestAtomicCommitMalformedBookkeepingLeavesFileUntouched(t *testing.T) {
	for _, field := range []string{"total_data_bytes", "files"} {
		t.Run(field, func(t *testing.T) {
			rdb, ctx := setupTestRedis(t)
			c := New(rdb, "bad-counter")
			if err := c.Echo(ctx, "/f", []byte("base")); err != nil {
				t.Fatal(err)
			}
			before, _ := ReadFileSnapshot(ctx, c, "/f")
			if err := rdb.HSet(ctx, "afs:{bad-counter}:info", field, "invalid").Err(); err != nil {
				t.Fatal(err)
			}
			err := c.Echo(before.WriteContext(ctx), "/f", []byte("candidate"))
			if err == nil {
				t.Fatal("malformed accounting accepted")
			}
			after, err := ReadFileSnapshot(ctx, c, "/f")
			if err != nil || !SameFileRevision(before.Stat, after.Stat) || !bytes.Equal(before.Content, after.Content) {
				t.Fatalf("partial commit: %+v %v", after.Stat, err)
			}
		})
	}
}

type readSnapshotRaceClient struct {
	Client
	afterCat func()
}

func (c *readSnapshotRaceClient) Cat(ctx context.Context, p string) ([]byte, error) {
	data, err := c.Client.Cat(ctx, p)
	if c.afterCat != nil {
		f := c.afterCat
		c.afterCat = nil
		f()
	}
	return data, err
}
func TestFileSnapshotRetriesRevisionChangeAndBypassesCache(t *testing.T) {
	rdb, ctx := setupTestRedis(t)
	peer := New(rdb, "snapshot-race")
	if err := peer.Echo(ctx, "/f", []byte("base")); err != nil {
		t.Fatal(err)
	}
	cached := NewWithCache(rdb, "snapshot-race", time.Hour)
	if _, err := cached.Stat(ctx, "/f"); err != nil {
		t.Fatal(err)
	}
	raced := &readSnapshotRaceClient{Client: cached, afterCat: func() {
		if err := peer.Echo(ctx, "/f", []byte("newer")); err != nil {
			t.Fatal(err)
		}
	}}
	got, err := ReadFileSnapshot(ctx, raced, "/f")
	if err != nil || string(got.Content) != "newer" {
		t.Fatalf("unstable snapshot: %q %v", got.Content, err)
	}
	current, _ := peer.Stat(ctx, "/f")
	if !SameFileRevision(got.Stat, current) {
		t.Fatalf("wrong token: %+v vs %+v", got.Stat, current)
	}
}

func TestAtomicCommitStagesEmptyAndChunkedContent(t *testing.T) {
	for _, size := range []int{0, 4097, 200000} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			rdb, ctx := setupTestRedis(t)
			c := New(rdb, "chunk-cas")
			data := []byte(strings.Repeat("x", size))
			snapshot, err := ReadFileSnapshot(ctx, c, "/new")
			if err != nil {
				t.Fatal(err)
			}
			chunks := map[int][]byte{}
			for off := 0; off < size; off += 4096 {
				chunks[off/4096] = data[off:min(size, off+4096)]
			}
			var result MutationResult
			err = c.WriteChunks(WithMutationResult(WithFileMode(snapshot.WriteContext(ctx), 0700), &result), "/new", chunks, 4096, int64(size), nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ReadFileSnapshot(ctx, c, "/new")
			if err != nil || !bytes.Equal(got.Content, data) || !SameFileRevision(got.Stat, result.Stat) || got.Stat.Mode != 0700 {
				t.Fatalf("incomplete staged commit: %+v %v", got.Stat, err)
			}
			if size > 0 {
				ttl, err := rdb.TTL(ctx, fmt.Sprintf("afs:{chunk-cas}:content:%d", got.Stat.Inode)).Result()
				if err != nil || ttl != -time.Nanosecond {
					t.Fatalf("live content inherited stage expiry: %v %v", ttl, err)
				}
			}
		})
	}
}
