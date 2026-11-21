package job

import (
	"io"
	"os"
	"sync"
	"syscall"
)

// Bus connects a running job to an interactive client (the WebSocket
// endpoint). All methods used by the job are safe to call on a nil *Bus, which
// is how non-interactive jobs run.
type Bus struct {
	OnStdout func(data string)
	OnStderr func(data string)
	OnStage  func(stage string)
	OnExit   func(stage string, code *int, signal *string)

	mu    sync.Mutex
	proc  *os.Process
	stdin io.WriteCloser
}

func (b *Bus) attach(p *os.Process, stdin io.WriteCloser) {
	b.mu.Lock()
	b.proc, b.stdin = p, stdin
	b.mu.Unlock()
}

func (b *Bus) detach() {
	b.mu.Lock()
	b.proc, b.stdin = nil, nil
	b.mu.Unlock()
}

func (b *Bus) stdout(s string) {
	if b != nil && b.OnStdout != nil {
		b.OnStdout(s)
	}
}

func (b *Bus) stderr(s string) {
	if b != nil && b.OnStderr != nil {
		b.OnStderr(s)
	}
}

func (b *Bus) stage(s string) {
	if b != nil && b.OnStage != nil {
		b.OnStage(s)
	}
}

func (b *Bus) exit(stage string, r *StageResult) {
	if b != nil && b.OnExit != nil {
		b.OnExit(stage, r.Code, r.Signal)
	}
}

// WriteStdin sends data to the stdin of the current stage's process.
func (b *Bus) WriteStdin(data string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stdin == nil {
		return nil
	}
	_, err := io.WriteString(b.stdin, data)
	return err
}

// Signal sends a signal to the current stage's process.
func (b *Bus) Signal(sig syscall.Signal) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.proc == nil {
		return nil
	}
	return b.proc.Signal(sig)
}

// Kill terminates the current stage's process.
func (b *Bus) Kill() { _ = b.Signal(syscall.SIGKILL) }
