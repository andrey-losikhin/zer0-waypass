package backend

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"zer0-waypass/internal/protocol"
)

// SecretAction is the closed allowlist of values that may be extracted from a
// password-store entry.
type SecretAction uint8

const (
	SecretUsername SecretAction = iota + 1
	SecretPassword
	SecretField
)

var (
	ErrInvalidSecretAction = errors.New("invalid secret action")
	ErrInvalidEntry        = errors.New("invalid backend entry")
	ErrEntryNotFound       = errors.New("backend entry not found")
)

// SecretProcess is an already-started gopass child. Wait never returns a raw
// process error.
type SecretProcess struct {
	ctx      context.Context
	cmd      *exec.Cmd
	pgidMu   sync.Mutex
	pgid     int
	waitOnce sync.Once
	waitErr  error
}

// SecretRequest is a validated, freshly membership-checked extraction request.
// Its fields are private so callers cannot bypass PrepareSecret.
type SecretRequest struct {
	action    SecretAction
	entryPath string
}

// PrepareSecret validates the closed action allowlist, validates every path in
// a fresh metadata listing, and requires an exact match. An EntryID alone is
// not authorization.
func (g *Gopass) PrepareSecret(ctx context.Context, action SecretAction, entryPath string) (SecretRequest, error) {
	if action != SecretUsername && action != SecretPassword && action != SecretField {
		return SecretRequest{}, ErrInvalidSecretAction
	}
	if err := g.requireMember(ctx, entryPath); err != nil {
		return SecretRequest{}, err
	}
	return SecretRequest{action: action, entryPath: entryPath}, nil
}

func (g *Gopass) requireMember(ctx context.Context, entryPath string) error {
	if _, err := protocol.EncodeCanonicalPath(entryPath); err != nil {
		return ErrInvalidEntry
	}

	entries, err := g.List(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, entry := range entries {
		if _, err := protocol.EncodeCanonicalPath(entry); err != nil {
			return ErrInvalidEntry
		}
		if entry == entryPath {
			found = true
		}
	}
	if !found {
		return ErrEntryNotFound
	}
	return nil
}

// StartSecret starts exactly one prepared single-entry extraction. stdout must
// be the write end of the operation pipe; this method never reads or buffers
// it. A request cannot be constructed outside this package without passing
// PrepareSecret's fresh membership check.
func (g *Gopass) StartSecret(ctx context.Context, request SecretRequest, stdout *os.File) (*SecretProcess, error) {
	if stdout == nil {
		return nil, ErrBackend
	}
	if _, err := protocol.EncodeCanonicalPath(request.entryPath); err != nil {
		return nil, ErrInvalidEntry
	}

	var args []string
	switch request.action {
	case SecretPassword:
		args = []string{"show", "--password", "--", request.entryPath}
	case SecretUsername:
		args = []string{"show", "--", request.entryPath, "username"}
	case SecretField:
		if !strings.HasPrefix(request.entryPath, ReservedFieldPrefix) {
			return nil, ErrInvalidSecretAction
		}
		args = []string{"show", "--noparsing", "--", request.entryPath}
	default:
		return nil, ErrInvalidSecretAction
	}

	cmd := exec.CommandContext(ctx, gopassExecutable, args...)
	cmd.Stdout = stdout
	cmd.WaitDelay = metadataProcessWaitDelay
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Stdin and stderr are deliberately disconnected. In particular, backend
	// diagnostics must never become protocol output or logs.
	if err := cmd.Start(); err != nil {
		return nil, classifyError(ctx, err)
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil || pgid != cmd.Process.Pid {
		// Do not signal an unverified negative PGID: the numeric PID may
		// identify an unrelated group after a failed/instantaneous Start.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, ErrBackend
	}
	return &SecretProcess{ctx: ctx, cmd: cmd, pgid: pgid}, nil
}

// ProcessGroupID returns the retained process group led by the direct gopass
// child. The group is created and verified before StartSecret returns.
func (p *SecretProcess) ProcessGroupID() int {
	p.pgidMu.Lock()
	defer p.pgidMu.Unlock()
	return p.pgid
}

// ForgetProcessGroup invalidates the numeric PGID after the single bounded
// cleanup pass. It must not be signalled again later because Linux may reuse it.
func (p *SecretProcess) ForgetProcessGroup() {
	p.pgidMu.Lock()
	p.pgid = 0
	p.pgidMu.Unlock()
}

// Wait reaps the direct gopass child and returns only a safe sentinel.
func (p *SecretProcess) Wait() error {
	p.waitOnce.Do(func() {
		if err := p.cmd.Wait(); err != nil {
			p.waitErr = classifyError(p.ctx, err)
		}
	})
	return p.waitErr
}
