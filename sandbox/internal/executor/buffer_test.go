package executor

import (
	"strings"
	"testing"
)

func TestBoundedBufferUnderCap(t *testing.T) {
	b := newBoundedBuffer(100)
	if _, err := b.Write([]byte("hello")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if got := b.String(); got != "hello" {
		t.Fatalf("String() = %q, want %q", got, "hello")
	}
}

func TestBoundedBufferAtCap(t *testing.T) {
	b := newBoundedBuffer(5)
	if _, err := b.Write([]byte("hello")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if got := b.String(); got != "hello" {
		t.Fatalf("String() = %q, want %q", got, "hello")
	}
}

func TestBoundedBufferTruncates(t *testing.T) {
	b := newBoundedBuffer(5)
	n, err := b.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	// boundedBuffer reports the full length so callers don't see short writes.
	if n != len("hello world") {
		t.Fatalf("Write returned n=%d, want %d", n, len("hello world"))
	}
	got := b.String()
	if !strings.HasPrefix(got, "hello") {
		t.Fatalf("String() = %q, expected hello prefix", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("String() = %q, expected truncation marker", got)
	}
}

func TestBoundedBufferContinuesDroppingAfterCap(t *testing.T) {
	b := newBoundedBuffer(3)
	b.Write([]byte("abc"))
	b.Write([]byte("defghi"))
	got := b.String()
	if !strings.HasPrefix(got, "abc") {
		t.Fatalf("String() = %q, expected abc prefix", got)
	}
	if strings.Contains(got, "def") {
		t.Fatalf("String() = %q, second write should have been dropped", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("String() = %q, expected truncation marker", got)
	}
}
