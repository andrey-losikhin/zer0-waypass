package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/andrey-losikhin/zer0-waypass/internal/backend"
	"github.com/andrey-losikhin/zer0-waypass/internal/clipboard"
	"github.com/andrey-losikhin/zer0-waypass/internal/protocol"
)

func TestParseCopyRequestAcceptsExactGrammar(t *testing.T) {
	entryID, err := protocol.EncodeCanonicalPath("synthetic/account")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		action     string
		seconds    string
		wantAction copyAction
		wantTTL    time.Duration
	}{
		{name: "minimum", action: "username", seconds: "5", wantAction: copyUsername, wantTTL: clipboard.MinimumOwnershipBudget},
		{name: "default with leading zero", action: "password", seconds: "00030", wantAction: copyPassword, wantTTL: clipboard.DefaultOwnershipBudget},
		{name: "maximum", action: "password", seconds: "120", wantAction: copyPassword, wantTTL: clipboard.MaximumOwnershipBudget},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, ok := parseCopyRequest([]string{"copy", test.action, string(entryID), "--ttl", test.seconds})
			if !ok {
				t.Fatal("valid copy request rejected")
			}
			if request.action != test.wantAction || request.entryPath != "synthetic/account" || request.ttl != test.wantTTL {
				t.Fatalf("request = %#v, want action/path/ttl %v/%q/%v", request, test.wantAction, "synthetic/account", test.wantTTL)
			}
		})
	}
}

func TestParseCopyRequestRejectsInvalidGrammar(t *testing.T) {
	entryID, err := protocol.EncodeCanonicalPath("synthetic/account")
	if err != nil {
		t.Fatal(err)
	}
	validID := string(entryID)
	tests := []struct {
		name string
		args []string
	}{
		{name: "empty", args: nil},
		{name: "missing ttl value", args: []string{"copy", "password", validID, "--ttl"}},
		{name: "extra argument", args: []string{"copy", "password", validID, "--ttl", "1", "extra"}},
		{name: "wrong command", args: []string{"COPY", "password", validID, "--ttl", "1"}},
		{name: "uppercase action", args: []string{"copy", "Password", validID, "--ttl", "1"}},
		{name: "unsupported action", args: []string{"copy", "url", validID, "--ttl", "1"}},
		{name: "action injection", args: []string{"copy", "password;id", validID, "--ttl", "1"}},
		{name: "empty action", args: []string{"copy", "", validID, "--ttl", "1"}},
		{name: "invalid id alphabet", args: []string{"copy", "password", "***", "--ttl", "1"}},
		{name: "id injection", args: []string{"copy", "password", "$(id)", "--ttl", "1"}},
		{name: "empty id", args: []string{"copy", "password", "", "--ttl", "1"}},
		{name: "padded id", args: []string{"copy", "password", validID + "=", "--ttl", "1"}},
		{name: "flag before id", args: []string{"copy", "password", "--ttl", validID, "1"}},
		{name: "wrong flag", args: []string{"copy", "password", validID, "--TTL", "1"}},
		{name: "empty ttl", args: []string{"copy", "password", validID, "--ttl", ""}},
		{name: "zero ttl", args: []string{"copy", "password", validID, "--ttl", "0"}},
		{name: "below minimum ttl", args: []string{"copy", "password", validID, "--ttl", "4"}},
		{name: "above maximum ttl", args: []string{"copy", "password", validID, "--ttl", "121"}},
		{name: "negative ttl", args: []string{"copy", "password", validID, "--ttl", "-1"}},
		{name: "positive sign", args: []string{"copy", "password", validID, "--ttl", "+1"}},
		{name: "leading space", args: []string{"copy", "password", validID, "--ttl", " 1"}},
		{name: "trailing space", args: []string{"copy", "password", validID, "--ttl", "1 "}},
		{name: "decimal point", args: []string{"copy", "password", validID, "--ttl", "1.0"}},
		{name: "unit suffix", args: []string{"copy", "password", validID, "--ttl", "1s"}},
		{name: "ttl injection", args: []string{"copy", "password", validID, "--ttl", "1;id"}},
		{name: "non ASCII digit", args: []string{"copy", "password", validID, "--ttl", "１"}},
		{name: "duration overflow", args: []string{"copy", "password", validID, "--ttl", "9223372037"}},
		{name: "uint64 overflow", args: []string{"copy", "password", validID, "--ttl", "18446744073709551616"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if request, ok := parseCopyRequest(test.args); ok || request != (copyRequest{}) {
				t.Fatalf("invalid args accepted as %#v", request)
			}

			store := &fakeBackend{}
			code, stdout, stderr := runForTest(t, test.args, store)
			if code != 2 || stdout != "" || stderr != errorLine(protocol.ErrorInvalidInvocation) || store.calls != 0 {
				t.Fatalf("code/stdout/stderr/calls = %d/%q/%q/%d, want 2/empty/invalid_invocation/0", code, stdout, stderr, store.calls)
			}
		})
	}
}

func TestCopyRequestRetainsOnlyTypedValidatedValues(t *testing.T) {
	wantFields := []string{"action", "entryPath", "ttl"}
	typeOfRequest := reflect.TypeOf(copyRequest{})
	fields := make([]string, typeOfRequest.NumField())
	for index := range typeOfRequest.NumField() {
		fields[index] = typeOfRequest.Field(index).Name
	}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("copy request fields = %v, want exact allowlist %v", fields, wantFields)
	}

	entryID, err := protocol.EncodeCanonicalPath("synthetic/raw-id-marker")
	if err != nil {
		t.Fatal(err)
	}
	request, ok := parseCopyRequest([]string{"copy", "username", string(entryID), "--ttl", "5"})
	if !ok {
		t.Fatal("valid copy request rejected")
	}
	if strings.Contains(request.entryPath, string(entryID)) || request.entryPath != "synthetic/raw-id-marker" {
		t.Fatal("request retained the raw transport ID instead of only its decoded path")
	}
}

func TestCopyCommandDispatchesAndRejectsInvalidRequests(t *testing.T) {
	original := copyDispatcher
	t.Cleanup(func() { copyDispatcher = original })
	var gotAction backend.SecretAction
	var gotPath string
	var gotPolicy clipboard.Policy
	copyDispatcher = func(_ context.Context, action backend.SecretAction, path string, policy clipboard.Policy) error {
		gotAction, gotPath, gotPolicy = action, path, policy
		return nil
	}
	entryID, err := protocol.EncodeCanonicalPath("synthetic/account")
	if err != nil {
		t.Fatal(err)
	}
	requests := []struct {
		args []string
		code int
		err  protocol.ErrorCode
	}{
		{[]string{"copy", "password", string(entryID), "--ttl", "30"}, 0, ""},
		{[]string{"copy", "username", string(entryID), "--ttl", "5"}, 0, ""},
		{[]string{"copy", "username", "invalid", "--ttl", "30"}, 2, protocol.ErrorInvalidInvocation},
		{[]string{"copy", "password", string(entryID), "--ttl", "0"}, 2, protocol.ErrorInvalidInvocation},
	}
	for _, request := range requests {
		store := &fakeBackend{}
		code, stdout, stderr := runForTest(t, request.args, store)
		wantStderr := ""
		if request.err != "" {
			wantStderr = errorLine(request.err)
		}
		if code != request.code || stdout != "" || stderr != wantStderr || store.calls != 0 {
			t.Fatalf("args/code/stdout/stderr/calls = %q/%d/%q/%q/%d", request.args, code, stdout, stderr, store.calls)
		}
	}
	if gotAction != backend.SecretUsername || gotPath != "synthetic/account" || gotPolicy.OwnershipBudget != 5*time.Second {
		t.Fatalf("dispatch = action %d path %q budget %v", gotAction, gotPath, gotPolicy.OwnershipBudget)
	}
}

func TestCopyDispatchErrorMapping(t *testing.T) {
	original := copyDispatcher
	t.Cleanup(func() { copyDispatcher = original })
	entryID, _ := protocol.EncodeCanonicalPath("synthetic/account")
	cases := []struct {
		err  error
		code protocol.ErrorCode
	}{
		{backend.ErrInvalidEntry, protocol.ErrorBackendInvalidData},
		{backend.ErrEntryNotFound, protocol.ErrorBackendInvalidData},
		{backend.ErrUnavailable, protocol.ErrorBackendUnavailable},
		{backend.ErrTimeout, protocol.ErrorBackendTimeout},
		{backend.ErrCanceled, protocol.ErrorOperationCanceled},
		{clipboard.ErrUnavailable, protocol.ErrorBackendUnavailable},
		{clipboard.ErrTimeout, protocol.ErrorBackendTimeout},
		{clipboard.ErrCanceled, protocol.ErrorOperationCanceled},
		{clipboard.ErrEarlyExit, protocol.ErrorBackend},
	}
	for _, tc := range cases {
		copyDispatcher = func(context.Context, backend.SecretAction, string, clipboard.Policy) error { return tc.err }
		code, stdout, stderr := runForTest(t, []string{"copy", "password", string(entryID), "--ttl", "30"}, &fakeBackend{})
		if code != 1 || stdout != "" || stderr != errorLine(tc.code) {
			t.Fatalf("err %v => %d/%q/%q, want 1/empty/%q", tc.err, code, stdout, stderr, errorLine(tc.code))
		}
	}
}
