// Package clipboard connects a validated gopass extraction directly to the
// Wayland clipboard owner without exposing the secret to the Go process.
package clipboard

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/andrey-losikhin/zer0-waypass/internal/backend"
)

const (
	wlCopyExecutable         = "wl-copy"
	processWaitDelay         = time.Second
	groupVerificationTimeout = 250 * time.Millisecond
)

var (
	ErrUnavailable = errors.New("clipboard unavailable")
	ErrFailed      = errors.New("clipboard error")
	ErrTimeout     = errors.New("clipboard operation timeout")
	ErrCanceled    = errors.New("clipboard operation canceled")
	ErrEarlyExit   = errors.New("clipboard owner exited early")
)

type secretSource interface {
	PrepareSecret(context.Context, backend.SecretAction, string) (backend.SecretRequest, error)
	StartSecret(context.Context, backend.SecretRequest, *os.File) (*backend.SecretProcess, error)
}

type retainedProcessGroup interface {
	ProcessGroupID() int
	ForgetProcessGroup()
	Wait() error
}

type backendResult struct {
	err       error
	completed time.Time
}

type retainedGroup struct {
	process retainedProcessGroup
	pgid    int
}

type ownerProcess struct {
	ctx      context.Context
	cmd      *exec.Cmd
	pgidMu   sync.Mutex
	pgid     int
	waitOnce sync.Once
	waitErr  error
}

func (p *ownerProcess) ProcessGroupID() int {
	p.pgidMu.Lock()
	defer p.pgidMu.Unlock()
	return p.pgid
}

func (p *ownerProcess) ForgetProcessGroup() {
	p.pgidMu.Lock()
	p.pgid = 0
	p.pgidMu.Unlock()
}

func (p *ownerProcess) Wait() error {
	p.waitOnce.Do(func() {
		p.waitErr = classifyClipboardError(p.ctx, p.cmd.Wait())
	})
	return p.waitErr
}

// Copy performs the worker lifecycle used by the hidden guardian. Public CLI
// dispatch remains deliberately deferred to a later Milestone 3 step.
func Copy(ctx context.Context, source secretSource, action backend.SecretAction, entryPath string) error {
	return copyWithPolicy(ctx, source, action, entryPath, DefaultPolicy(), nil, nil)
}

func copyWithBeforeSecretStart(ctx context.Context, source secretSource, action backend.SecretAction, entryPath string, beforeSecretStart func()) error {
	return copyWithPolicy(ctx, source, action, entryPath, DefaultPolicy(), beforeSecretStart, nil)
}

func copyWithProcessGroupObserver(ctx context.Context, source secretSource, action backend.SecretAction, entryPath string, beforeSecretStart func(), observe func(ownerPGID, backendPGID int) error) error {
	return copyWithPolicy(ctx, source, action, entryPath, DefaultPolicy(), beforeSecretStart, observe)
}

func copyWithPolicy(ctx context.Context, source secretSource, action backend.SecretAction, entryPath string, policy Policy, beforeSecretStart func(), observe func(ownerPGID, backendPGID int) error) error {
	if action != backend.SecretUsername && action != backend.SecretPassword && action != backend.SecretField && action != backend.SecretTOTP {
		return backend.ErrInvalidSecretAction
	}
	prepare := func(ctx context.Context) (backend.SecretRequest, error) {
		return source.PrepareSecret(ctx, action, entryPath)
	}
	return copyPreparedWithPolicy(ctx, source, prepare, policy, beforeSecretStart, observe)
}

// copyPreparedWithPolicy runs prepare inside the acquisition deadline, so
// resolving the request counts against the same bound as gopass/pinentry.
func copyPreparedWithPolicy(ctx context.Context, source secretSource, prepare func(context.Context) (backend.SecretRequest, error), policy Policy, beforeSecretStart func(), observe func(ownerPGID, backendPGID int) error) error {
	if err := ValidatePolicy(policy); err != nil {
		return ErrInvalidPolicy
	}

	operationContext, cancelOperation := context.WithCancel(ctx)
	defer cancelOperation()
	acquisitionContext, cancelAcquisition := context.WithTimeout(operationContext, policy.AcquisitionDeadline)
	defer cancelAcquisition()

	request, err := prepare(acquisitionContext)
	if err != nil {
		return err
	}

	pipeReader, pipeWriter, err := os.Pipe()
	if err != nil {
		return ErrFailed
	}

	// --trim-newline drops the terminator gopass stores after every value, so a
	// paste into a terminal or form never submits it; the secret stays FD-to-FD.
	owner := exec.CommandContext(operationContext, wlCopyExecutable,
		"--sensitive", "--foreground", "--trim-newline", "--type", "text/plain;charset=utf-8")
	owner.Stdin = pipeReader
	owner.WaitDelay = processWaitDelay
	owner.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := owner.Start(); err != nil {
		_ = pipeReader.Close()
		_ = pipeWriter.Close()
		return classifyClipboardError(ctx, err)
	}
	ownerPGID, err := syscall.Getpgid(owner.Process.Pid)
	if err != nil || ownerPGID != owner.Process.Pid {
		_ = pipeReader.Close()
		_ = pipeWriter.Close()
		// Do not signal an unverified negative PGID: the numeric PID may
		// identify an unrelated group after a failed/instantaneous Start.
		_ = owner.Process.Kill()
		_ = owner.Wait()
		return ErrFailed
	}
	ownerGroup := &ownerProcess{ctx: operationContext, cmd: owner, pgid: ownerPGID}
	_ = pipeReader.Close()
	if beforeSecretStart != nil {
		beforeSecretStart()
	}

	secret, err := source.StartSecret(acquisitionContext, request, pipeWriter)
	_ = pipeWriter.Close()
	if err != nil {
		cancelOperation()
		if cleanupRetainedGroups([]retainedProcessGroup{ownerGroup}, policy.KillGrace) != nil {
			return ErrFailed
		}
		return err
	}
	backendPGID := secret.ProcessGroupID()
	if backendPGID == ownerPGID {
		cancelOperation()
		if cleanupRetainedGroups([]retainedProcessGroup{secret, ownerGroup}, policy.KillGrace) != nil {
			return ErrFailed
		}
		return ErrFailed
	}
	if observe != nil {
		if err := observe(ownerPGID, backendPGID); err != nil {
			cancelOperation()
			if cleanupRetainedGroups([]retainedProcessGroup{secret, ownerGroup}, policy.KillGrace) != nil {
				return ErrFailed
			}
			return ErrFailed
		}
	}

	backendDone := make(chan backendResult, 1)
	ownerDone := make(chan error, 1)
	go func() {
		err := secret.Wait()
		backendDone <- backendResult{err: err, completed: time.Now()}
	}()
	go func() { ownerDone <- ownerGroup.Wait() }()
	groups := []retainedProcessGroup{secret, ownerGroup}

	select {
	case backend := <-backendDone:
		acquisitionErr := acquisitionContext.Err()
		if backend.err != nil || acquisitionErr != nil {
			cancelOperation()
			if cleanupRetainedGroups(groups, policy.KillGrace) != nil {
				return ErrFailed
			}
			if acquisitionErr != nil {
				return classifyClipboardError(acquisitionContext, acquisitionErr)
			}
			return backend.err
		}
		// Fail closed if the owner exit was already observable at the backend
		// success boundary, even when select chose the backend channel first.
		select {
		case <-ownerDone:
			cancelOperation()
			if cleanupRetainedGroups(groups, policy.KillGrace) != nil {
				return ErrFailed
			}
			if ctx.Err() != nil {
				return classifyClipboardError(ctx, ctx.Err())
			}
			return ErrEarlyExit
		default:
		}
		cancelAcquisition()
		return superviseOwnership(ctx, cancelOperation, secret, ownerGroup, ownerDone, policy, backend.completed)
	case <-ownerDone:
		cancelOperation()
		if cleanupRetainedGroups(groups, policy.KillGrace) != nil {
			return ErrFailed
		}
		if ctx.Err() != nil {
			return classifyClipboardError(ctx, ctx.Err())
		}
		return ErrEarlyExit
	case <-acquisitionContext.Done():
		cancelOperation()
		if cleanupRetainedGroups(groups, policy.KillGrace) != nil {
			return ErrFailed
		}
		return classifyClipboardError(acquisitionContext, acquisitionContext.Err())
	}
}

func superviseOwnership(ctx context.Context, cancelOperation context.CancelFunc, backendGroup *backend.SecretProcess, ownerGroup *ownerProcess, ownerDone <-chan error, policy Policy, ownershipStarted time.Time) error {
	backendPGID := backendGroup.ProcessGroupID()
	if groupExists(backendPGID) {
		cancelOperation()
		if cleanupRetainedGroups([]retainedProcessGroup{backendGroup, ownerGroup}, policy.KillGrace) != nil {
			return ErrFailed
		}
		return ErrFailed
	}
	backendGroup.ForgetProcessGroup()

	termDelay := time.Until(ownershipStarted.Add(policy.OwnershipBudget - policy.KillGrace))
	if termDelay < 0 {
		termDelay = 0
	}
	termTimer := time.NewTimer(termDelay)
	defer termTimer.Stop()
	for {
		select {
		case ownerErr := <-ownerDone:
			if ctx.Err() != nil {
				cancelOperation()
				if cleanupOwnerWithinBudget(ownerGroup, policy, ownershipStarted) != nil {
					return ErrFailed
				}
				return classifyClipboardError(ctx, ctx.Err())
			}
			if groupExists(ownerGroup.ProcessGroupID()) {
				cancelOperation()
				if cleanupOwnerWithinBudget(ownerGroup, policy, ownershipStarted) != nil {
					return ErrFailed
				}
				if ownerErr != nil {
					return ownerErr
				}
				return ErrFailed
			}
			ownerGroup.ForgetProcessGroup()
			if ownerErr != nil {
				return ownerErr
			}
			return nil
		case <-ctx.Done():
			cancelOperation()
			if cleanupOwnerWithinBudget(ownerGroup, policy, ownershipStarted) != nil {
				return ErrFailed
			}
			return classifyClipboardError(ctx, ctx.Err())
		case <-termTimer.C:
			if cleanupRetainedGroups([]retainedProcessGroup{ownerGroup}, policy.KillGrace) != nil {
				return ErrFailed
			}
			if time.Since(ownershipStarted) > policy.OwnershipBudget+groupVerificationTimeout {
				return ErrFailed
			}
			if ctx.Err() != nil {
				return classifyClipboardError(ctx, ctx.Err())
			}
			return nil
		}
	}
}

func cleanupOwnerWithinBudget(owner retainedProcessGroup, policy Policy, ownershipStarted time.Time) error {
	remaining := time.Until(ownershipStarted.Add(policy.OwnershipBudget))
	if remaining <= 0 {
		remaining = time.Nanosecond
	}
	if remaining > policy.KillGrace {
		remaining = policy.KillGrace
	}
	return cleanupRetainedGroups([]retainedProcessGroup{owner}, remaining)
}

// cleanupRetainedGroups performs one immediate, bounded cleanup pass. Callers
// must discard the retained numeric PGIDs after it returns and must never
// signal them again later.
func cleanupRetainedGroups(groups []retainedProcessGroup, grace time.Duration) error {
	active := make([]retainedGroup, 0, len(groups))
	seen := make(map[int]struct{}, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		pgid := group.ProcessGroupID()
		if pgid <= 0 {
			continue
		}
		if _, duplicate := seen[pgid]; duplicate {
			continue
		}
		seen[pgid] = struct{}{}
		active = append(active, retainedGroup{process: group, pgid: pgid})
	}
	defer func() {
		for _, group := range active {
			group.process.ForgetProcessGroup()
		}
	}()

	cleanupFailed := false
	for _, group := range active {
		if err := signalGroup(group.pgid, syscall.SIGTERM); err != nil {
			cleanupFailed = true
		}
	}
	var waits sync.WaitGroup
	waits.Add(len(active))
	waitDone := make(chan struct{})
	for _, group := range active {
		go func() {
			defer waits.Done()
			_ = group.process.Wait()
		}()
	}
	go func() {
		waits.Wait()
		close(waitDone)
	}()

	deadline := time.Now().Add(grace)
	for anyGroupExists(active) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	for _, group := range active {
		if groupExists(group.pgid) {
			if err := signalGroup(group.pgid, syscall.SIGKILL); err != nil {
				cleanupFailed = true
			}
		}
	}

	verificationDeadline := time.Now().Add(groupVerificationTimeout)
	select {
	case <-waitDone:
	case <-time.After(time.Until(verificationDeadline)):
		return ErrFailed
	}

	for anyGroupExists(active) && time.Now().Before(verificationDeadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if cleanupFailed || anyGroupExists(active) {
		return ErrFailed
	}
	return nil
}

func signalGroup(pgid int, signal syscall.Signal) error {
	err := syscall.Kill(-pgid, signal)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func groupExists(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func anyGroupExists(groups []retainedGroup) bool {
	for _, group := range groups {
		if groupExists(group.pgid) {
			return true
		}
	}
	return false
}

func classifyClipboardError(ctx context.Context, err error) error {
	if err == nil {
		return nil
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
	return ErrFailed
}
