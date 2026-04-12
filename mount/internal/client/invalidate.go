package client

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Invalidate operation kinds published to other clients sharing an FS key.
const (
	// InvalidateOpInode means an entry at the given path was created,
	// deleted, or had its metadata changed. Subscribers drop the path's
	// own cache entry AND its parent directory listing.
	InvalidateOpInode = "inode"

	// InvalidateOpDir means a directory's listing is stale (e.g. a child's
	// mtime bumped something in it) but the directory's own inode identity
	// is unchanged. Subscribers drop only the dir listing cache.
	InvalidateOpDir = "dir"

	// InvalidateOpPrefix means an entire subtree is stale (e.g. after a
	// directory rename). Subscribers drop every cached entry whose path
	// starts with the given prefix.
	InvalidateOpPrefix = "prefix"

	// InvalidateOpContent means a file's byte contents changed. Subscribers
	// drop the kernel page cache for the file (and refresh size/mtime).
	InvalidateOpContent = "content"
)

// InvalidateEvent is the payload broadcast on the per-FS-key pub/sub channel
// whenever a client mutates state that other clients may be caching.
type InvalidateEvent struct {
	// Origin is the publisher's opaque client ID. Subscribers skip messages
	// whose Origin matches their own ID (local state is already correct).
	Origin string `json:"origin"`
	// Op is one of InvalidateOp*.
	Op string `json:"op"`
	// Paths are the affected absolute paths. Most events have a single path;
	// rename-prefix events may carry two (src and dst).
	Paths []string `json:"paths"`
}

// encodeInvalidate marshals an event for transport. JSON keeps the wire format
// debuggable from redis-cli PSUBSCRIBE.
func encodeInvalidate(ev InvalidateEvent) ([]byte, error) {
	return json.Marshal(ev)
}

// decodeInvalidate parses a message received on the invalidation channel.
func decodeInvalidate(payload []byte) (*InvalidateEvent, error) {
	var ev InvalidateEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// ChangeStreamEntry is one entry from the per-workspace durable change
// stream. Sync clients read these on reconnect to replay missed events.
type ChangeStreamEntry struct {
	ID    string          // Redis stream message ID (e.g. "1681234567890-0")
	Event InvalidateEvent // Decoded event payload
}

// ErrStreamTrimmed is returned by ReadChangeStream when the client's saved
// cursor position has been trimmed from the stream (the client was offline
// longer than the stream's retention window). The caller should fall back
// to a full reconciliation.
var ErrStreamTrimmed = errors.New("change stream: saved position was trimmed, full reconcile required")

// newOriginID returns a fresh opaque client identifier. 16 random bytes (hex
// encoded) is plenty to make collisions astronomically unlikely for a Redis
// key's worth of concurrent mounts.
func newOriginID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// rand.Read on modern OSes does not fail in practice, but if it
		// ever did, fall back to a constant so we still boot. Dedup will
		// be broken for that process but correctness is unaffected.
		return "origin-fallback"
	}
	return hex.EncodeToString(buf[:])
}
