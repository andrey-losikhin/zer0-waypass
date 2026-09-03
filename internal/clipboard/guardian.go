package clipboard

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/andrey-losikhin/zer0-waypass/internal/backend"
	"github.com/andrey-losikhin/zer0-waypass/internal/protocol"
)

const (
	// GuardianModeArgument is the only argv value used to enter the private
	// same-binary guardian. The operation request is carried by inherited FDs.
	GuardianModeArgument = "--zer0-waypass-internal-guardian"
	guardianProtocol     = 1
	guardianRequestFD    = uintptr(3)
	guardianStatusFD     = uintptr(4)
	guardianStopTimeout  = MaximumKillGrace + groupVerificationTimeout + processWaitDelay
)

type guardianStart struct {
	Protocol               int    `json:"protocol"`
	Type                   string `json:"type"`
	Action                 string `json:"action"`
	EntryPath              string `json:"entry_path"`
	Revision               string `json:"revision,omitempty"`
	FieldID                string `json:"field_id,omitempty"`
	AcquisitionNanoseconds int64  `json:"acquisition_nanoseconds"`
	OwnershipNanoseconds   int64  `json:"ownership_nanoseconds"`
	KillGraceNanoseconds   int64  `json:"kill_grace_nanoseconds"`
}

type guardianStatus struct {
	Protocol    int    `json:"protocol"`
	Type        string `json:"type"`
	Code        string `json:"code,omitempty"`
	OwnerPID    int    `json:"owner_pid,omitempty"`
	OwnerPGID   int    `json:"owner_pgid,omitempty"`
	BackendPID  int    `json:"backend_pid,omitempty"`
	BackendPGID int    `json:"backend_pgid,omitempty"`
}

// CopyGuarded starts one short-lived guardian subprocess of the current helper
// binary. The creating goroutine remains pinned to its OS thread until that
// guardian has exited, as required by Linux Pdeathsig thread semantics.
func CopyGuarded(ctx context.Context, action backend.SecretAction, entryPath string, policy Policy) error {
	executable, err := os.Executable()
	if err != nil {
		return ErrFailed
	}
	return copyGuardedWithExecutable(ctx, executable, action, entryPath, policy)
}

// CopyFieldGuarded resolves the schema-bound value inside the guardian, so a
// stale controller-side card cannot authorize an old value path.
func CopyFieldGuarded(ctx context.Context, entryPath, revision, fieldID string, policy Policy) error {
	executable, err := os.Executable()
	if err != nil {
		return ErrFailed
	}
	return copyGuardedRequest(ctx, executable, backend.SecretField, entryPath, revision, fieldID, policy)
}

func copyGuardedWithExecutable(ctx context.Context, executable string, action backend.SecretAction, entryPath string, policy Policy) error {
	return copyGuardedRequest(ctx, executable, action, entryPath, "", "", policy)
}

func copyGuardedRequest(ctx context.Context, executable string, action backend.SecretAction, entryPath, revision, fieldID string, policy Policy) error {
	if action != backend.SecretUsername && action != backend.SecretPassword && action != backend.SecretField {
		return backend.ErrInvalidSecretAction
	}
	if _, err := protocol.EncodeCanonicalPath(entryPath); err != nil {
		return backend.ErrInvalidEntry
	}
	if err := ValidatePolicy(policy); err != nil {
		return ErrInvalidPolicy
	}

	requestReader, requestWriter, err := os.Pipe()
	if err != nil {
		return ErrFailed
	}
	defer requestWriter.Close()
	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		requestReader.Close()
		return ErrFailed
	}
	defer statusReader.Close()

	command := exec.Command(executable, GuardianModeArgument)
	command.ExtraFiles = []*os.File{requestReader, statusWriter}
	command.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := command.Start(); err != nil {
		requestReader.Close()
		statusWriter.Close()
		return ErrFailed
	}
	requestReader.Close()
	statusWriter.Close()

	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	reader := bufio.NewReader(statusReader)

	ready, err := readGuardianStatus(ctx, reader)
	if err != nil || ready != (guardianStatus{Protocol: guardianProtocol, Type: "READY"}) {
		return stopGuardian(command, waitDone, ctx, ErrFailed)
	}
	if ctx.Err() != nil {
		return stopGuardian(command, waitDone, ctx, classifyClipboardError(ctx, ctx.Err()))
	}
	start := guardianStart{
		Protocol:               guardianProtocol,
		Type:                   "START",
		Action:                 guardianActionName(action),
		EntryPath:              entryPath,
		Revision:               revision,
		FieldID:                fieldID,
		AcquisitionNanoseconds: int64(policy.AcquisitionDeadline),
		OwnershipNanoseconds:   int64(policy.OwnershipBudget),
		KillGraceNanoseconds:   int64(policy.KillGrace),
	}
	if ctx.Err() != nil {
		return stopGuardian(command, waitDone, ctx, classifyClipboardError(ctx, ctx.Err()))
	}
	if err := json.NewEncoder(requestWriter).Encode(start); err != nil {
		return stopGuardian(command, waitDone, ctx, ErrFailed)
	}
	if err := requestWriter.Close(); err != nil {
		return stopGuardian(command, waitDone, ctx, ErrFailed)
	}

	registered := false
	for {
		status, err := readGuardianStatus(ctx, reader)
		if err != nil {
			if ctx.Err() != nil {
				return stopGuardian(command, waitDone, ctx, classifyClipboardError(ctx, ctx.Err()))
			}
			return stopGuardian(command, waitDone, ctx, ErrFailed)
		}
		switch status.Type {
		case "REGISTERED":
			if registered || status.Protocol != guardianProtocol || status.Code != "" ||
				status.OwnerPID <= 0 || status.OwnerPID != status.OwnerPGID ||
				status.BackendPID <= 0 || status.BackendPID != status.BackendPGID ||
				status.OwnerPGID == status.BackendPGID {
				return stopGuardian(command, waitDone, ctx, ErrFailed)
			}
			registered = true
		case "DONE":
			if status.Protocol != guardianProtocol || status.Code == "" || (status.Code == "ok" && !registered) {
				return stopGuardian(command, waitDone, ctx, ErrFailed)
			}
			if err := <-waitDone; err != nil {
				return ErrFailed
			}
			return decodeGuardianError(status.Code)
		default:
			return stopGuardian(command, waitDone, ctx, ErrFailed)
		}
	}
}

func readGuardianStatus(ctx context.Context, reader *bufio.Reader) (guardianStatus, error) {
	result := make(chan struct {
		status guardianStatus
		err    error
	}, 1)
	go func() {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			result <- struct {
				status guardianStatus
				err    error
			}{err: err}
			return
		}
		var status guardianStatus
		decoder := json.NewDecoder(bytesReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&status); err != nil {
			result <- struct {
				status guardianStatus
				err    error
			}{err: err}
			return
		}
		if err := requireJSONEOF(decoder); err != nil {
			result <- struct {
				status guardianStatus
				err    error
			}{err: err}
			return
		}
		result <- struct {
			status guardianStatus
			err    error
		}{status: status}
	}()
	select {
	case decoded := <-result:
		return decoded.status, decoded.err
	case <-ctx.Done():
		return guardianStatus{}, ctx.Err()
	}
}

// RunGuardianMode executes the hidden guardian using only inherited anonymous
// pipes at FDs 3 and 4. Missing or substituted descriptors fail closed before
// READY and therefore before any worker can start.
func RunGuardianMode() int {
	request := os.NewFile(guardianRequestFD, "guardian-request")
	status := os.NewFile(guardianStatusFD, "guardian-status")
	if !validInheritedPipe(request) || !validInheritedPipe(status) {
		if request != nil {
			request.Close()
		}
		if status != nil {
			status.Close()
		}
		return 1
	}
	defer request.Close()
	defer status.Close()
	return runGuardian(request, status, backend.NewGopass())
}

func validInheritedPipe(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeNamedPipe != 0
}

func runGuardian(request io.Reader, status io.Writer, source secretSource) int {
	guardianContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	encoder := json.NewEncoder(status)
	if err := encoder.Encode(guardianStatus{Protocol: guardianProtocol, Type: "READY"}); err != nil {
		return 1
	}

	startResult := make(chan struct {
		start guardianStart
		err   error
	}, 1)
	go func() {
		line, readErr := bufio.NewReader(request).ReadBytes('\n')
		decoder := json.NewDecoder(bytesReader(line))
		decoder.DisallowUnknownFields()
		var start guardianStart
		err := readErr
		if err == nil {
			err = decoder.Decode(&start)
		}
		if err == nil {
			err = requireJSONEOF(decoder)
		}
		startResult <- struct {
			start guardianStart
			err   error
		}{start: start, err: err}
	}()

	var start guardianStart
	select {
	case <-guardianContext.Done():
		return 1
	case decoded := <-startResult:
		if decoded.err != nil {
			return 1
		}
		start = decoded.start
	}
	action, policy, err := validateGuardianStart(start)
	if err != nil {
		_ = encoder.Encode(guardianStatus{Protocol: guardianProtocol, Type: "DONE", Code: "invalid_request"})
		return 0
	}

	entryPath := start.EntryPath
	if action == backend.SecretField {
		acquisitionEnd := time.Now().Add(policy.AcquisitionDeadline)
		resolveContext, cancelResolve := context.WithDeadline(guardianContext, acquisitionEnd)
		resolver, ok := source.(interface {
			ResolveField(context.Context, string, string, string) (string, error)
		})
		if !ok {
			err = ErrFailed
		} else {
			entryPath, err = resolver.ResolveField(resolveContext, start.EntryPath, start.Revision, start.FieldID)
		}
		cancelResolve()
		remaining := time.Until(acquisitionEnd)
		if err == nil && remaining <= 0 {
			err = backend.ErrTimeout
		}
		if err == nil {
			policy.AcquisitionDeadline = remaining
		}
		if err != nil {
			_ = encoder.Encode(guardianStatus{Protocol: guardianProtocol, Type: "DONE", Code: encodeGuardianError(err)})
			return 0
		}
	}
	err = copyWithPolicy(
		guardianContext,
		source,
		action,
		entryPath,
		policy,
		nil,
		func(ownerPGID, backendPGID int) error {
			return encoder.Encode(guardianStatus{
				Protocol:    guardianProtocol,
				Type:        "REGISTERED",
				OwnerPID:    ownerPGID,
				OwnerPGID:   ownerPGID,
				BackendPID:  backendPGID,
				BackendPGID: backendPGID,
			})
		},
	)
	if encodeErr := encoder.Encode(guardianStatus{Protocol: guardianProtocol, Type: "DONE", Code: encodeGuardianError(err)}); encodeErr != nil {
		return 1
	}
	return 0
}

func validateGuardianStart(start guardianStart) (backend.SecretAction, Policy, error) {
	if start.Protocol != guardianProtocol || start.Type != "START" {
		return 0, Policy{}, ErrFailed
	}
	var action backend.SecretAction
	switch start.Action {
	case "username":
		action = backend.SecretUsername
	case "password":
		action = backend.SecretPassword
	case "field":
		action = backend.SecretField
	default:
		return 0, Policy{}, ErrFailed
	}
	if _, err := protocol.EncodeCanonicalPath(start.EntryPath); err != nil {
		return 0, Policy{}, ErrFailed
	}
	if action == backend.SecretField {
		if len(start.Revision) != 43 || len(start.FieldID) != 22 {
			return 0, Policy{}, ErrFailed
		}
	} else if start.Revision != "" || start.FieldID != "" {
		return 0, Policy{}, ErrFailed
	}
	policy := Policy{
		AcquisitionDeadline: time.Duration(start.AcquisitionNanoseconds),
		OwnershipBudget:     time.Duration(start.OwnershipNanoseconds),
		KillGrace:           time.Duration(start.KillGraceNanoseconds),
	}
	if err := ValidatePolicy(policy); err != nil {
		return 0, Policy{}, err
	}
	return action, policy, nil
}

func guardianActionName(action backend.SecretAction) string {
	if action == backend.SecretUsername {
		return "username"
	}
	if action == backend.SecretField {
		return "field"
	}
	return "password"
}

func stopGuardian(command *exec.Cmd, waitDone <-chan error, ctx context.Context, result error) error {
	if command.Process != nil {
		_ = command.Process.Signal(syscall.SIGTERM)
	}
	timer := time.NewTimer(guardianStopTimeout)
	defer timer.Stop()
	select {
	case <-waitDone:
	case <-timer.C:
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		<-waitDone
	}
	if ctx.Err() != nil {
		return classifyClipboardError(ctx, ctx.Err())
	}
	return result
}

func encodeGuardianError(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, backend.ErrInvalidSecretAction):
		return "invalid_action"
	case errors.Is(err, backend.ErrInvalidEntry), errors.Is(err, backend.ErrEntryNotFound):
		return "invalid_entry"
	case errors.Is(err, backend.ErrUnavailable):
		return "backend_unavailable"
	case errors.Is(err, backend.ErrTimeout):
		return "backend_timeout"
	case errors.Is(err, backend.ErrCanceled):
		return "backend_canceled"
	case errors.Is(err, ErrUnavailable):
		return "clipboard_unavailable"
	case errors.Is(err, ErrTimeout):
		return "clipboard_timeout"
	case errors.Is(err, ErrCanceled):
		return "clipboard_canceled"
	case errors.Is(err, ErrEarlyExit):
		return "clipboard_early_exit"
	default:
		return "failed"
	}
}

func decodeGuardianError(code string) error {
	switch code {
	case "ok":
		return nil
	case "invalid_action":
		return backend.ErrInvalidSecretAction
	case "invalid_entry":
		return backend.ErrInvalidEntry
	case "backend_unavailable":
		return backend.ErrUnavailable
	case "backend_timeout":
		return backend.ErrTimeout
	case "backend_canceled":
		return backend.ErrCanceled
	case "clipboard_unavailable":
		return ErrUnavailable
	case "clipboard_timeout":
		return ErrTimeout
	case "clipboard_canceled":
		return ErrCanceled
	case "clipboard_early_exit":
		return ErrEarlyExit
	case "failed":
		return ErrFailed
	default:
		return ErrFailed
	}
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return ErrFailed
		}
		return err
	}
	return nil
}

type byteReader struct {
	bytes  []byte
	offset int
}

func bytesReader(data []byte) *byteReader { return &byteReader{bytes: data} }

func (r *byteReader) Read(target []byte) (int, error) {
	if r.offset >= len(r.bytes) {
		return 0, io.EOF
	}
	n := copy(target, r.bytes[r.offset:])
	r.offset += n
	return n, nil
}
