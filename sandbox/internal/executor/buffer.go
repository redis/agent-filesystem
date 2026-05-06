package executor

import "sync"

// boundedBuffer is a thread-safe byte buffer with a hard cap on retained
// bytes. Writes past the cap are silently dropped — the buffer keeps the
// first `cap` bytes and a "truncated" flag so callers can warn the user
// rather than letting a chatty process OOM the sandbox.
type boundedBuffer struct {
	mu        sync.Mutex
	buf       []byte
	cap       int
	truncated bool
}

func newBoundedBuffer(cap int) *boundedBuffer {
	return &boundedBuffer{buf: make([]byte, 0, 4096), cap: cap}
}

// Write appends p to the buffer up to the cap; bytes beyond the cap are
// dropped and Truncated() will report true. Always returns len(p) and nil
// error to keep io.Writer semantics intact for stdlib consumers.
func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.cap - len(b.buf)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.buf = append(b.buf, p[:remaining]...)
		b.truncated = true
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

// String returns the current buffer content. If the buffer hit the cap, a
// trailing marker is appended so consumers can see that more output was
// dropped.
func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		return string(b.buf) + "\n[output truncated: sandbox buffer cap reached]\n"
	}
	return string(b.buf)
}
