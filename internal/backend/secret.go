package backend

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/andrey-losikhin/zer0-waypass/internal/protocol"
)

// SecretAction is the closed allowlist of values that may be extracted from a
// password-store entry.
type SecretAction uint8

const (
	SecretUsername SecretAction = iota + 1
	SecretPassword
	SecretField
	// SecretTOTP streams only the current code from `gopass otp`; the seed
	// never leaves the gopass process.
	SecretTOTP
	// SecretLegacyField streams one named key of a legacy key-value entry.
	SecretLegacyField
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
	key       string
}

// PrepareSecret validates the closed action allowlist, validates every path in
// a fresh metadata listing, and requires an exact match. An EntryID alone is
// not authorization.
func (g *Gopass) PrepareSecret(ctx context.Context, action SecretAction, entryPath string) (SecretRequest, error) {
	if action != SecretUsername && action != SecretPassword && action != SecretField && action != SecretTOTP {
		return SecretRequest{}, ErrInvalidSecretAction
	}
	if err := g.requireMember(ctx, entryPath); err != nil {
		return SecretRequest{}, err
	}
	if action == SecretField || strings.HasPrefix(entryPath, ReservedFieldPrefix) {
		return SecretRequest{action: action, entryPath: entryPath}, nil
	}
	// A field-bundle entry keeps its values in sidecars; its main entry may
	// hold only a compatibility marker, so resolve the value by field kind.
	valuePath, found, err := g.manifestValuePath(ctx, entryPath, secretFieldKind[action])
	if err != nil {
		return SecretRequest{}, err
	}
	if !found {
		return SecretRequest{action: action, entryPath: entryPath}, nil
	}
	if action == SecretTOTP {
		return SecretRequest{action: SecretTOTP, entryPath: valuePath}, nil
	}
	return SecretRequest{action: SecretField, entryPath: valuePath}, nil
}

var secretFieldKind = map[SecretAction]string{SecretUsername: "username", SecretPassword: "password", SecretTOTP: "totp_secret"}

// manifestValuePath reports found=false only for entries without a manifest.
func (g *Gopass) manifestValuePath(ctx context.Context, entryPath, kind string) (string, bool, error) {
	m, err := g.loadMemberManifest(ctx, entryPath)
	if errors.Is(err, ErrEntryNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	for _, field := range m.Fields {
		if field.Kind == kind {
			valuePath := fieldValuePath(m, field.ID)
			if err := g.requireMember(ctx, valuePath); err != nil {
				return "", false, ErrInvalidManifest
			}
			return valuePath, true, nil
		}
	}
	return "", false, ErrEntryNotFound
}

// PrepareLegacyField maps a legacy card field ID back to its key name. It reads
// the entry like Fields does, but returns only the validated key name.
func (g *Gopass) PrepareLegacyField(ctx context.Context, entryPath, fieldID string) (SecretRequest, error) {
	if strings.HasPrefix(entryPath, ReservedFieldPrefix) {
		return SecretRequest{}, ErrInvalidEntry
	}
	set, err := g.Fields(ctx, entryPath)
	if err != nil {
		return SecretRequest{}, err
	}
	if set.Revision != "" {
		return SecretRequest{}, ErrInvalidManifest
	}
	for _, field := range set.Fields {
		if field.ID != fieldID {
			continue
		}
		if field.ID == "legacy-password" {
			return SecretRequest{action: SecretPassword, entryPath: entryPath}, nil
		}
		if !validDisplayName(field.Name) || strings.HasPrefix(field.Name, "-") {
			return SecretRequest{}, ErrInvalidEntry
		}
		return SecretRequest{action: SecretLegacyField, entryPath: entryPath, key: field.Name}, nil
	}
	return SecretRequest{}, ErrEntryNotFound
}

func (g *Gopass) requireMember(ctx context.Context, entryPath string) error {
	if _, err := protocol.EncodeCanonicalPath(entryPath); err != nil {
		return ErrInvalidEntry
	}
	// gopass >= 1.17 omits dot-prefixed paths from `ls`, so reserved sidecars
	// are confirmed by their encrypted file in the root store instead.
	if strings.HasPrefix(entryPath, ReservedFieldPrefix) {
		return g.requireSidecar(ctx, entryPath)
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
	case SecretTOTP:
		args = []string{"otp", "--password", "--", request.entryPath}
	case SecretLegacyField:
		if request.key == "" || strings.HasPrefix(request.entryPath, ReservedFieldPrefix) {
			return nil, ErrInvalidSecretAction
		}
		args = []string{"show", "--nofuzzysearch", "--", request.entryPath, request.key}
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
