package client

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestEncodeDecodeInvalidateRoundTrip(t *testing.T) {
	in := InvalidateEvent{
		Origin: "abc123",
		Op:     InvalidateOpInode,
		Paths:  []string{"/foo", "/foo/bar"},
	}
	encoded, err := encodeInvalidate(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// JSON is documented as the wire format so sanity-check the shape.
	var raw map[string]interface{}
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("encoded payload is not valid JSON: %v", err)
	}
	if raw["origin"] != "abc123" || raw["op"] != InvalidateOpInode {
		t.Fatalf("unexpected JSON shape: %s", encoded)
	}

	out, err := decodeInvalidate(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Origin != in.Origin || out.Op != in.Op || len(out.Paths) != len(in.Paths) {
		t.Fatalf("round-trip mismatch: %+v -> %+v", in, out)
	}
	for i, p := range in.Paths {
		if out.Paths[i] != p {
			t.Fatalf("path[%d] mismatch: %q vs %q", i, p, out.Paths[i])
		}
	}
}

func TestNewOriginIDUnique(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		id := newOriginID()
		if id == "" {
			t.Fatal("empty origin id")
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate origin id: %s", id)
		}
		seen[id] = struct{}{}
	}
}

// TestCrossClientInvalidation verifies the main end-to-end flow:
// two clients sharing a Redis key, writes on A become visible to B's
// cached stat lookups via the pub/sub channel — not by waiting for the
// TTL to expire.
//
// We deliberately use a 1-hour cache TTL on B so if invalidation fails
// the test fails quickly (within the 5s subscription wait) instead of
// eventually passing because the TTL expired.
func TestCrossClientInvalidation(t *testing.T) {
	rdb, ctx := setupTestRedis(t)

	const fsKey = "crossclient-test"

	// Writer (short TTL is fine; it is the publisher).
	writer := NewWithCache(rdb, fsKey, time.Hour).(*nativeClient)
	// Reader keeps a long TTL so only pub/sub can make its cache stale.
	reader := NewWithCache(rdb, fsKey, time.Hour).(*nativeClient)

	// Start the reader subscriber and wait for it to actually be
	// subscribed before the writer publishes. We drive this off a
	// handler hook that signals via a channel.
	seen := make(chan InvalidateEvent, 16)
	subCtx, cancelSub := context.WithCancel(ctx)
	defer cancelSub()
	if err := reader.SubscribeInvalidations(subCtx, func(ev InvalidateEvent) {
		seen <- ev
	}); err != nil {
		t.Fatalf("SubscribeInvalidations: %v", err)
	}

	// The goroutine races with the first Publish: wait until a PubSub
	// round-trip confirms we are subscribed. Simplest reliable signal:
	// poll Redis PUBSUB NUMSUB until it sees us.
	waitForSubscriber(t, rdb, writer.keys.invalidateChannel())

	// Prime the reader cache: a Stat fills its path cache with a "does
	// not exist" (redis.Nil) which is NOT cached today; we instead cache
	// the positive entry after we create it. So let writer create first.
	if _, _, err := writer.CreateFile(ctx, "/foo.txt", 0o644, false); err != nil {
		t.Fatalf("writer CreateFile: %v", err)
	}

	// Reader warms its cache by stat'ing the file.
	st, err := reader.Stat(ctx, "/foo.txt")
	if err != nil {
		t.Fatalf("reader Stat #1: %v", err)
	}
	if st == nil {
		t.Fatal("reader Stat #1 returned nil")
	}
	if _, ok := reader.cache.Get("/foo.txt"); !ok {
		t.Fatal("reader cache was not primed after Stat")
	}

	// Drain any pending events from the CreateFile above so the next
	// Wait sees only the event produced by the upcoming Echo.
	drain(seen)

	// Writer changes the file content. The reader's cached entry
	// should be invalidated via pub/sub.
	if err := writer.Echo(ctx, "/foo.txt", []byte("after")); err != nil {
		t.Fatalf("writer Echo: %v", err)
	}

	// Wait for a content-op event for /foo.txt. Tolerate other events
	// the mutation may have also published.
	if !waitForEvent(seen, 2*time.Second, func(ev InvalidateEvent) bool {
		return ev.Op == InvalidateOpContent && len(ev.Paths) == 1 && ev.Paths[0] == "/foo.txt"
	}) {
		t.Fatal("did not receive content invalidation for /foo.txt within 2s")
	}

	// After the event landed, the reader's cache entry should be gone.
	if _, ok := reader.cache.Get("/foo.txt"); ok {
		t.Fatal("reader cache still has /foo.txt after peer invalidation")
	}
}

// TestCrossClientInvalidationOriginDedup proves the publisher never gets
// its own messages back as events in the handler — it already invalidated
// locally at the mutation site.
func TestCrossClientInvalidationOriginDedup(t *testing.T) {
	rdb, ctx := setupTestRedis(t)

	const fsKey = "origin-dedup-test"
	c := NewWithCache(rdb, fsKey, time.Hour).(*nativeClient)

	events := make(chan InvalidateEvent, 16)
	subCtx, cancelSub := context.WithCancel(ctx)
	defer cancelSub()
	if err := c.SubscribeInvalidations(subCtx, func(ev InvalidateEvent) {
		events <- ev
	}); err != nil {
		t.Fatalf("SubscribeInvalidations: %v", err)
	}
	waitForSubscriber(t, rdb, c.keys.invalidateChannel())

	// Do a mutation. Since we're both publisher and subscriber, we must
	// NOT see our own event in the handler channel (origin dedup).
	if _, _, err := c.CreateFile(ctx, "/self.txt", 0o644, false); err != nil {
		t.Fatalf("CreateFile: %v", err)
	}

	select {
	case ev := <-events:
		t.Fatalf("handler saw own event: %+v", ev)
	case <-time.After(250 * time.Millisecond):
		// No event is the expected outcome.
	}
}

// TestDisableInvalidationPublishingSilencesPublisher verifies the escape-
// hatch flag short-circuits every outbound PUBLISH so a peer never sees
// the write via pub/sub. (Local state is still correct — tested
// implicitly by reusing the cached Get.)
func TestDisableInvalidationPublishingSilencesPublisher(t *testing.T) {
	rdb, ctx := setupTestRedis(t)

	const fsKey = "disabled-publisher-test"
	writer := NewWithCache(rdb, fsKey, time.Hour).(*nativeClient)
	reader := NewWithCache(rdb, fsKey, time.Hour).(*nativeClient)
	writer.DisableInvalidationPublishing()

	events := make(chan InvalidateEvent, 4)
	subCtx, cancelSub := context.WithCancel(ctx)
	defer cancelSub()
	if err := reader.SubscribeInvalidations(subCtx, func(ev InvalidateEvent) {
		events <- ev
	}); err != nil {
		t.Fatalf("SubscribeInvalidations: %v", err)
	}
	waitForSubscriber(t, rdb, writer.keys.invalidateChannel())

	if _, _, err := writer.CreateFile(ctx, "/silent.txt", 0o644, false); err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	if err := writer.Echo(ctx, "/silent.txt", []byte("hello")); err != nil {
		t.Fatalf("Echo: %v", err)
	}

	select {
	case ev := <-events:
		t.Fatalf("reader received event from disabled publisher: %+v", ev)
	case <-time.After(250 * time.Millisecond):
		// Expected.
	}
}

// waitForSubscriber blocks until PUBSUB NUMSUB reports at least one
// subscriber on channel. Prevents races where the writer's PUBLISH beats
// the subscriber goroutine to the channel.
func waitForSubscriber(t *testing.T, rdb *redis.Client, channel string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for time.Now().Before(deadline) {
		result, err := rdb.Do(ctx, "PUBSUB", "NUMSUB", channel).Result()
		if err == nil {
			if count, ok := pubSubNumSubCount(result); ok && count > 0 {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no subscriber appeared on channel %s within 3s", channel)
}

// pubSubNumSubCount extracts the count from a `PUBSUB NUMSUB <chan>` reply,
// which in go-redis comes back as []interface{}{channel, int64 count}.
func pubSubNumSubCount(v interface{}) (int64, bool) {
	arr, ok := v.([]interface{})
	if !ok || len(arr) < 2 {
		return 0, false
	}
	switch n := arr[1].(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

// drain removes any buffered events without blocking.
func drain(ch chan InvalidateEvent) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// waitForEvent blocks up to timeout looking for an event that satisfies
// match. Events that don't match are consumed (kept out of the channel so
// later waits see fresh state). Returns true on match, false on timeout.
func waitForEvent(ch chan InvalidateEvent, timeout time.Duration, match func(InvalidateEvent) bool) bool {
	deadline := time.After(timeout)
	for {
		select {
		case ev := <-ch:
			if match(ev) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}
