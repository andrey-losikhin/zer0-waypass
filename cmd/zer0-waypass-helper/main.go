package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"zer0-waypass/internal/backend"
	"zer0-waypass/internal/clipboard"
	"zer0-waypass/internal/protocol"
)

const (
	metadataOperationTimeout = 10 * time.Second
	maxQueryBytes            = 4096
)

type metadataBackend interface {
	List(context.Context) ([]string, error)
}

type fieldBackend interface {
	Fields(context.Context, string) (backend.FieldSet, error)
	ResolveField(context.Context, string, string, string) (string, error)
}

type copyAction uint8

const (
	copyUsername copyAction = iota + 1
	copyPassword
)

type copyRequest struct {
	action    copyAction
	entryPath string
	ttl       time.Duration
}

var copyDispatcher = func(ctx context.Context, action backend.SecretAction, entryPath string, policy clipboard.Policy) error {
	return clipboard.CopyGuarded(ctx, action, entryPath, policy)
}
var fieldCopyDispatcher = clipboard.CopyFieldGuarded

// parseCopyRequest validates the complete copy CLI grammar without retaining
// the untrusted transport ID.
func parseCopyRequest(args []string) (copyRequest, bool) {
	if len(args) != 5 || args[0] != "copy" || args[3] != "--ttl" {
		return copyRequest{}, false
	}

	var action copyAction
	switch args[1] {
	case "username":
		action = copyUsername
	case "password":
		action = copyPassword
	default:
		return copyRequest{}, false
	}

	entryPath, err := protocol.DecodeEntryID(protocol.EntryID(args[2]))
	if err != nil {
		return copyRequest{}, false
	}

	ttl, err := clipboard.ParseOwnershipBudgetSeconds(args[4])
	if err != nil {
		return copyRequest{}, false
	}

	return copyRequest{
		action:    action,
		entryPath: entryPath,
		ttl:       ttl,
	}, true
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == clipboard.GuardianModeArgument {
		os.Exit(clipboard.RunGuardianMode())
	}
	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	os.Exit(run(signalContext, os.Args[1:], backend.NewGopass(), os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, store metadataBackend, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return invocationError(stderr)
	}

	switch args[0] {
	case "list":
		if len(args) > 2 {
			return invocationError(stderr)
		}
		query := ""
		if len(args) == 2 {
			query = args[1]
			if len(query) > maxQueryBytes || !utf8.ValidString(query) {
				return invocationError(stderr)
			}
		}
		metadataContext, cancel := context.WithTimeout(ctx, metadataOperationTimeout)
		defer cancel()
		return runList(metadataContext, store, query, stdout, stderr)
	case "status":
		if len(args) != 1 {
			return invocationError(stderr)
		}
		metadataContext, cancel := context.WithTimeout(ctx, metadataOperationTimeout)
		defer cancel()
		return runStatus(metadataContext, store, stdout, stderr)
	case "fields":
		if len(args) != 2 {
			return invocationError(stderr)
		}
		entryPath, err := protocol.DecodeEntryID(protocol.EntryID(args[1]))
		if err != nil {
			return invocationError(stderr)
		}
		fieldStore, ok := store.(fieldBackend)
		if !ok {
			return operationError(stderr, protocol.ErrorBackend)
		}
		metadataContext, cancel := context.WithTimeout(ctx, metadataOperationTimeout)
		defer cancel()
		set, err := fieldStore.Fields(metadataContext, entryPath)
		if err != nil {
			return operationError(stderr, backendErrorCode(err))
		}
		fields := make([]protocol.FieldItem, len(set.Fields))
		for i, f := range set.Fields {
			fields[i] = protocol.FieldItem{ID: f.ID, Name: f.Name, Kind: f.Kind, Visibility: f.Visibility, Multiline: f.Multiline, Value: f.Value}
		}
		return writeJSON(stdout, stderr, protocol.FieldsEnvelope{Protocol: protocol.FieldProtocolVersion, Revision: set.Revision, Fields: fields})
	case "copy":
		if len(args) == 7 && args[1] == "field" && args[5] == "--ttl" {
			entryPath, err := protocol.DecodeEntryID(protocol.EntryID(args[2]))
			if err != nil {
				return invocationError(stderr)
			}
			ttl, err := clipboard.ParseOwnershipBudgetSeconds(args[6])
			if err != nil {
				return invocationError(stderr)
			}
			if !validRawURLToken(args[3], 32) || !validRawURLToken(args[4], 16) {
				return invocationError(stderr)
			}
			policy := clipboard.DefaultPolicy()
			policy.OwnershipBudget = ttl
			if err := fieldCopyDispatcher(ctx, entryPath, args[3], args[4], policy); err != nil {
				return operationError(stderr, copyErrorCode(err))
			}
			return 0
		}
		request, ok := parseCopyRequest(args)
		if !ok {
			return invocationError(stderr)
		}
		policy := clipboard.DefaultPolicy()
		policy.OwnershipBudget = request.ttl
		if err := copyDispatcher(ctx, backendAction(request.action), request.entryPath, policy); err != nil {
			return operationError(stderr, copyErrorCode(err))
		}
		return 0
	default:
		return invocationError(stderr)
	}
}

func validRawURLToken(value string, size int) bool {
	b, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(b) == size && base64.RawURLEncoding.EncodeToString(b) == value
}

func backendAction(action copyAction) backend.SecretAction {
	if action == copyUsername {
		return backend.SecretUsername
	}
	return backend.SecretPassword
}

func copyErrorCode(err error) protocol.ErrorCode {
	switch {
	case errors.Is(err, backend.ErrInvalidEntry), errors.Is(err, backend.ErrEntryNotFound):
		return protocol.ErrorBackendInvalidData
	case errors.Is(err, backend.ErrUnavailable), errors.Is(err, clipboard.ErrUnavailable):
		return protocol.ErrorBackendUnavailable
	case errors.Is(err, backend.ErrTimeout), errors.Is(err, clipboard.ErrTimeout):
		return protocol.ErrorBackendTimeout
	case errors.Is(err, backend.ErrCanceled), errors.Is(err, clipboard.ErrCanceled):
		return protocol.ErrorOperationCanceled
	default:
		return protocol.ErrorBackend
	}
}

func runList(ctx context.Context, store metadataBackend, query string, stdout, stderr io.Writer) int {
	entries, err := store.List(ctx)
	if err != nil {
		return operationError(stderr, backendErrorCode(err))
	}

	query = strings.ToLower(query)
	items := make([]protocol.Item, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry, backend.ReservedFieldPrefix) {
			continue
		}
		entryID, err := protocol.EncodeCanonicalPath(entry)
		if err != nil {
			return operationError(stderr, protocol.ErrorBackendInvalidData)
		}
		if query != "" && !strings.Contains(strings.ToLower(entry), query) {
			continue
		}
		items = append(items, protocol.Item{
			ID:    entryID,
			Label: entry,
		})
	}

	return writeJSON(stdout, stderr, protocol.ListEnvelope{
		Protocol: protocol.ProtocolVersion,
		Items:    items,
	})
}

func runStatus(ctx context.Context, store metadataBackend, stdout, stderr io.Writer) int {
	if _, err := store.List(ctx); err != nil {
		return operationError(stderr, backendErrorCode(err))
	}

	return writeJSON(stdout, stderr, protocol.StatusEnvelope{
		Protocol: protocol.ProtocolVersion,
		Backend:  "ready",
	})
}

func writeJSON(stdout, stderr io.Writer, response any) int {
	encoded, err := json.Marshal(response)
	if err != nil {
		return operationError(stderr, protocol.ErrorOutput)
	}
	encoded = append(encoded, '\n')
	if written, err := stdout.Write(encoded); err != nil || written != len(encoded) {
		return operationError(stderr, protocol.ErrorOutput)
	}
	return 0
}

func invocationError(stderr io.Writer) int {
	writeError(stderr, protocol.ErrorInvalidInvocation)
	return 2
}

func operationError(stderr io.Writer, code protocol.ErrorCode) int {
	writeError(stderr, code)
	return 1
}

func writeError(stderr io.Writer, code protocol.ErrorCode) {
	encoded, ok := protocol.MarshalError(code)
	if !ok {
		return
	}
	_, _ = stderr.Write(encoded)
}

func backendErrorCode(err error) protocol.ErrorCode {
	switch {
	case errors.Is(err, backend.ErrUnavailable):
		return protocol.ErrorBackendUnavailable
	case errors.Is(err, backend.ErrTimeout):
		return protocol.ErrorBackendTimeout
	case errors.Is(err, backend.ErrCanceled):
		return protocol.ErrorOperationCanceled
	case errors.Is(err, backend.ErrOutputTooLarge):
		return protocol.ErrorBackendOutputTooLarge
	default:
		return protocol.ErrorBackend
	}
}
