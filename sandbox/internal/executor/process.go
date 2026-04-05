// Package executor manages process lifecycle in the sandbox.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// ProcessState represents the current state of a process.
type ProcessState string

const (
	StateRunning  ProcessState = "running"
	StateExited   ProcessState = "exited"
	StateKilled   ProcessState = "killed"
	StateTimedOut ProcessState = "timed_out"
)

// Process represents a managed process in the sandbox.
type Process struct {
	ID        string       `json:"id"`
	Command   string       `json:"command"`
	Cwd       string       `json:"cwd"`
	State     ProcessState `json:"state"`
	ExitCode  int          `json:"exit_code"`
	StartedAt time.Time    `json:"started_at"`
	EndedAt   *time.Time   `json:"ended_at,omitempty"`
	PID       int          `json:"pid,omitempty"`

	cmd    *exec.Cmd
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	stdin  io.WriteCloser
	mu     sync.RWMutex
	done   chan struct{}
}

// Manager handles process creation and lifecycle.
type Manager struct {
	processes map[string]*Process
	workspace string
	mu        sync.RWMutex
}

// NewManager creates a new process manager.
func NewManager(workspace string) *Manager {
	return &Manager{
		processes: make(map[string]*Process),
		workspace: workspace,
	}
}

// LaunchOptions configures process launch behavior.
type LaunchOptions struct {
	Command       string        `json:"command"`
	Cwd           string        `json:"cwd,omitempty"`
	Timeout       time.Duration `json:"timeout,omitempty"`
	Wait          bool          `json:"wait"`
	KeepStdinOpen bool          `json:"keep_stdin_open,omitempty"`
}

// LaunchResult contains the result of launching a process.
type LaunchResult struct {
	ID       string       `json:"id"`
	PID      int          `json:"pid"`
	State    ProcessState `json:"state"`
	ExitCode int          `json:"exit_code,omitempty"`
	Stdout   string       `json:"stdout,omitempty"`
	Stderr   string       `json:"stderr,omitempty"`
}

// Launch starts a new process.
func (m *Manager) Launch(ctx context.Context, opts LaunchOptions) (*LaunchResult, error) {
	id := uuid.New().String()[:8]

	cwd := opts.Cwd
	if cwd == "" {
		cwd = m.workspace
	} else if cwd[0] != '/' {
		cwd = m.workspace + "/" + cwd
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", opts.Command)
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	var stdin io.WriteCloser
	if opts.KeepStdinOpen {
		var err error
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("stdin pipe: %w", err)
		}
	}

	proc := &Process{
		ID:        id,
		Command:   opts.Command,
		Cwd:       cwd,
		State:     StateRunning,
		StartedAt: time.Now(),
		cmd:       cmd,
		stdout:    stdout,
		stderr:    stderr,
		stdin:     stdin,
		done:      make(chan struct{}),
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	proc.PID = cmd.Process.Pid

	m.mu.Lock()
	m.processes[id] = proc
	m.mu.Unlock()

	go m.monitor(proc, opts.Timeout)

	result := &LaunchResult{ID: id, PID: proc.PID, State: StateRunning}

	if opts.Wait {
		select {
		case <-proc.done:
		case <-ctx.Done():
		}
		proc.mu.RLock()
		result.State = proc.State
		result.ExitCode = proc.ExitCode
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		proc.mu.RUnlock()
	}

	return result, nil
}

