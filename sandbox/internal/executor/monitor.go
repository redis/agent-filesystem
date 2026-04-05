package executor

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// monitor watches a process and updates its state when it exits.
func (m *Manager) monitor(proc *Process, timeout time.Duration) {
	defer close(proc.done)

	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timeoutCh = time.After(timeout)
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- proc.cmd.Wait()
	}()

	select {
	case err := <-waitDone:
		proc.mu.Lock()
		now := time.Now()
		proc.EndedAt = &now
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				proc.ExitCode = exitErr.ExitCode()
			} else {
				proc.ExitCode = -1
			}
		}
		proc.State = StateExited
		proc.mu.Unlock()

	case <-timeoutCh:
		proc.mu.Lock()
		proc.State = StateTimedOut
		proc.mu.Unlock()
		syscall.Kill(-proc.PID, syscall.SIGKILL)
		<-waitDone
		proc.mu.Lock()
		now := time.Now()
		proc.EndedAt = &now
		proc.mu.Unlock()
	}
}

// ReadResult contains process output.
type ReadResult struct {
	ID       string       `json:"id"`
	State    ProcessState `json:"state"`
	ExitCode int          `json:"exit_code"`
	Stdout   string       `json:"stdout"`
	Stderr   string       `json:"stderr"`
}

// Read returns the current output of a process.
func (m *Manager) Read(id string) (*ReadResult, error) {
	m.mu.RLock()
	proc, ok := m.processes[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("process %s not found", id)
	}

	proc.mu.RLock()
	defer proc.mu.RUnlock()

	return &ReadResult{
		ID:       proc.ID,
		State:    proc.State,
		ExitCode: proc.ExitCode,
		Stdout:   proc.stdout.String(),
		Stderr:   proc.stderr.String(),
	}, nil
}

// Write sends input to a process's stdin.
func (m *Manager) Write(id string, input string) error {
	m.mu.RLock()
	proc, ok := m.processes[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("process %s not found", id)
	}

	proc.mu.RLock()
	stdin := proc.stdin
	state := proc.State
	proc.mu.RUnlock()

	if state != StateRunning {
		return fmt.Errorf("process %s is not running", id)
	}
	if stdin == nil {
		return fmt.Errorf("process %s stdin not open", id)
	}

	_, err := stdin.Write([]byte(input))
	return err
}

// Kill terminates a process.
func (m *Manager) Kill(id string) error {
	m.mu.RLock()
	proc, ok := m.processes[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("process %s not found", id)
	}

	proc.mu.Lock()
	if proc.State != StateRunning {
		proc.mu.Unlock()
		return nil
	}
	proc.State = StateKilled
	proc.mu.Unlock()

	return syscall.Kill(-proc.PID, syscall.SIGKILL)
}

// ProcessInfo is a summary of a process for listing.
type ProcessInfo struct {
	ID        string       `json:"id"`
	Command   string       `json:"command"`
	Cwd       string       `json:"cwd"`
	State     ProcessState `json:"state"`
	ExitCode  int          `json:"exit_code"`
	PID       int          `json:"pid"`
	StartedAt time.Time    `json:"started_at"`
	EndedAt   *time.Time   `json:"ended_at,omitempty"`
}

// List returns all processes.
func (m *Manager) List() []*ProcessInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ProcessInfo, 0, len(m.processes))
	for _, proc := range m.processes {
		proc.mu.RLock()
		result = append(result, &ProcessInfo{
			ID:        proc.ID,
			Command:   proc.Command,
			Cwd:       proc.Cwd,
			State:     proc.State,
			ExitCode:  proc.ExitCode,
			PID:       proc.PID,
			StartedAt: proc.StartedAt,
			EndedAt:   proc.EndedAt,
		})
		proc.mu.RUnlock()
	}
	return result
}

// Wait blocks until a process completes.
func (m *Manager) Wait(ctx context.Context, id string) (*ReadResult, error) {
	m.mu.RLock()
	proc, ok := m.processes[id]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("process %s not found", id)
	}

	select {
	case <-proc.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return m.Read(id)
}

