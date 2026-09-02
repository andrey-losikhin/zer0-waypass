// Package backend provides metadata-only access to gopass.
package backend

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	gopassExecutable         = "gopass"
	maxMetadataOutputBytes   = 8 << 20
	metadataProcessWaitDelay = time.Second
)

var (
	// These sentinels are the complete safe backend failure surface. They do
	// not wrap or retain raw command errors.
	ErrUnavailable    = errors.New("backend unavailable")
	ErrTimeout        = errors.New("backend timeout")
	ErrCanceled       = errors.New("operation canceled")
	ErrOutputTooLarge = errors.New("backend output too large")
	ErrBackend        = errors.New("backend error")

	errMetadataOutputLimit = errors.New("metadata output limit exceeded")
)

type commandRunner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return execRunner{}.OutputLimit(ctx, maxMetadataOutputBytes, name, args...)
}

func (execRunner) OutputLimit(ctx context.Context, limit int, name string, args ...string) ([]byte, error) {
	commandContext, cancelCommand := context.WithCancel(ctx)
	defer cancelCommand()

	cmd := exec.CommandContext(commandContext, name, args...)
	cmd.WaitDelay = metadataProcessWaitDelay

	output := &limitedOutput{
		limit:   limit,
		onLimit: cancelCommand,
	}
	cmd.Stdout = output
	// Stderr is intentionally left disconnected and is never returned.

	if err := cmd.Run(); err != nil {
		if output.exceededLimit() {
			return nil, ErrOutputTooLarge
		}
		return nil, classifyError(ctx, err)
	}
	if output.exceededLimit() {
		return nil, ErrOutputTooLarge
	}
	return output.bytes(), nil
}

func classifyError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, ErrUnavailable):
		return ErrUnavailable
	case errors.Is(err, ErrTimeout):
		return ErrTimeout
	case errors.Is(err, ErrCanceled):
		return ErrCanceled
	case errors.Is(err, ErrOutputTooLarge):
		return ErrOutputTooLarge
	case errors.Is(err, ErrBackend):
		return ErrBackend
	}
	if errors.Is(err, errMetadataOutputLimit) {
		return ErrOutputTooLarge
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return ErrCanceled
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return ErrUnavailable
	}
	return ErrBackend
}

type limitedOutput struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	limit    int
	onLimit  func()
	once     sync.Once
	exceeded bool
}

func (w *limitedOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	remaining := w.limit - w.buffer.Len()
	if remaining >= len(data) {
		n, err := w.buffer.Write(data)
		w.mu.Unlock()
		return n, err
	}
	written := 0
	if remaining > 0 {
		written, _ = w.buffer.Write(data[:remaining])
	}
	w.exceeded = true
	w.mu.Unlock()

	w.once.Do(w.onLimit)
	return written, errMetadataOutputLimit
}

func (w *limitedOutput) exceededLimit() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.exceeded
}

func (w *limitedOutput) bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buffer.Bytes()...)
}

// Gopass reads entry metadata without decrypting entries.
type Gopass struct {
	runner commandRunner
}

// NewGopass creates the production gopass adapter.
func NewGopass() *Gopass {
	return &Gopass{runner: execRunner{}}
}

// List returns entry paths in backend order. It invokes only the metadata-only
// gopass listing command; filtering is deliberately left to the caller.
func (g *Gopass) List(ctx context.Context) ([]string, error) {
	output, err := g.runner.Output(ctx, gopassExecutable, "ls", "--flat")
	if err != nil {
		return nil, classifyError(ctx, err)
	}

	lines := strings.Split(string(output), "\n")
	entries := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			entries = append(entries, line)
		}
	}

	return entries, nil
}
