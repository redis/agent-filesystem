package executor

import (
	"context"
	"testing"
	"time"
)

// quickProcess launches `true` and waits for it to exit, returning the
// process ID after the entry is in StateExited. Useful for setting up GC
// scenarios without coupling to internal state directly.
func quickProcess(t *testing.T, m *Manager) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := m.Launch(ctx, LaunchOptions{
		Command: "true",
		Wait:    true,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if res.State != StateExited {
		t.Fatalf("expected StateExited, got %s", res.State)
	}
	return res.ID
}

func TestGCSkipsRunningProcesses(t *testing.T) {
	m := NewManager(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Launch a long-running sleep without waiting.
	res, err := m.Launch(ctx, LaunchOptions{Command: "sleep 30"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer m.Kill(res.ID)

	// Even with maxAge=0, running processes must not be pruned.
	if removed := m.GC(0); removed != 0 {
		t.Fatalf("GC pruned %d running processes; want 0", removed)
	}
	if list := m.List(); len(list) != 1 {
		t.Fatalf("expected 1 process after GC, got %d", len(list))
	}
}

func TestGCPrunesOldExitedProcesses(t *testing.T) {
	m := NewManager(t.TempDir())
	id := quickProcess(t, m)

	// Process is in StateExited but EndedAt is recent. Calling GC with a
	// large maxAge must not prune it.
	if removed := m.GC(time.Hour); removed != 0 {
		t.Fatalf("GC with large maxAge pruned %d entries; want 0", removed)
	}
	if list := m.List(); len(list) != 1 {
		t.Fatalf("expected 1 process before GC, got %d", len(list))
	}

	// Backdate the EndedAt so the GC sees the entry as old.
	m.mu.Lock()
	proc := m.processes[id]
	m.mu.Unlock()
	proc.mu.Lock()
	old := time.Now().Add(-2 * time.Hour)
	proc.EndedAt = &old
	proc.mu.Unlock()

	if removed := m.GC(time.Hour); removed != 1 {
		t.Fatalf("GC removed %d entries; want 1", removed)
	}
	if list := m.List(); len(list) != 0 {
		t.Fatalf("expected 0 processes after GC, got %d", len(list))
	}
}
