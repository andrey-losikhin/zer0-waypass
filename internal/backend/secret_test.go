package backend

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// routedRunner answers listing and store-root queries separately.
type routedRunner struct {
	listing string
	root    string
	calls   []runnerCall
}

func (r *routedRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	switch strings.Join(args, " ") {
	case "ls --flat":
		return []byte(r.listing), nil
	case "config mounts.path":
		return []byte(r.root + "\n"), nil
	}
	return nil, ErrBackend
}

func TestPrepareSecretUsesFreshExactListing(t *testing.T) {
	runner := &routedRunner{listing: "synthetic/account\nsynthetic/other\n", root: sidecarStore(t)}
	store := &Gopass{runner: runner}

	request, err := store.PrepareSecret(context.Background(), SecretPassword, "synthetic/account")
	if err != nil {
		t.Fatalf("PrepareSecret: %v", err)
	}
	if request.action != SecretPassword || request.entryPath != "synthetic/account" {
		t.Fatalf("request = %#v", request)
	}
	if !reflect.DeepEqual(runner.calls[0], runnerCall{name: "gopass", args: []string{"ls", "--flat"}}) {
		t.Fatalf("first call = %#v", runner.calls[0])
	}
	for _, call := range runner.calls {
		if call.args[0] == "show" || call.args[0] == "otp" {
			t.Fatalf("legacy prepare decrypted: %#v", call)
		}
	}
}

func TestPrepareSecretResolvesBundleFieldsByKind(t *testing.T) {
	r := &fieldRunner{root: workDBStore(t)}
	g := &Gopass{runner: r}
	bundle := ReservedFieldPrefix + "v1/" + testOID(1) + "/" + testOID(2) + "/"
	password, err := g.PrepareSecret(context.Background(), SecretPassword, "work/db")
	if err != nil || password.action != SecretField || password.entryPath != bundle+testOID(3) {
		t.Fatalf("password %#v %v", password, err)
	}
	if _, err := g.PrepareSecret(context.Background(), SecretUsername, "work/db"); err != ErrEntryNotFound {
		t.Fatalf("missing username kind err=%v", err)
	}
}

func TestStartSecretArgvForTOTPAndLegacyField(t *testing.T) {
	writer, err := createDiscardPipeWriter(t)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	store := NewGopass()
	if _, err := store.StartSecret(context.Background(), SecretRequest{action: SecretLegacyField, entryPath: "synthetic/account"}, writer); err != ErrInvalidSecretAction {
		t.Fatalf("legacy field without key err=%v", err)
	}
	if _, err := store.StartSecret(context.Background(), SecretRequest{action: SecretLegacyField, entryPath: ReservedFieldPrefix + "x", key: "host"}, writer); err != ErrInvalidSecretAction {
		t.Fatalf("legacy field on sidecar err=%v", err)
	}
}

func TestPrepareLegacyFieldMapsIDToKeyName(t *testing.T) {
	r := &legacyFieldRunner{root: sidecarStore(t)}
	g := &Gopass{runner: r}
	set, err := g.Fields(context.Background(), "legacy/account")
	if err != nil {
		t.Fatal(err)
	}
	request, err := g.PrepareLegacyField(context.Background(), "legacy/account", set.Fields[4].ID)
	if err != nil || request.action != SecretLegacyField || request.key != "Project" {
		t.Fatalf("request %#v %v", request, err)
	}
	request, err = g.PrepareLegacyField(context.Background(), "legacy/account", "legacy-password")
	if err != nil || request.action != SecretPassword || request.key != "" {
		t.Fatalf("password request %#v %v", request, err)
	}
	if _, err := g.PrepareLegacyField(context.Background(), "legacy/account", "legacy-unknown"); err != ErrEntryNotFound {
		t.Fatalf("unknown id err=%v", err)
	}
}

func TestPrepareSecretFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		listing string
		listErr error
		want    error
	}{
		{name: "missing exact match", path: "synthetic/account", listing: "synthetic/account-extra\n", want: ErrEntryNotFound},
		{name: "invalid requested path", path: "../account", listing: "../account\n", want: ErrInvalidEntry},
		{name: "invalid unrelated listing path", path: "synthetic/account", listing: "synthetic/account\n../invalid\n", want: ErrInvalidEntry},
		{name: "listing failure", path: "synthetic/account", listErr: ErrBackend, want: ErrBackend},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{output: []byte(test.listing), err: test.listErr}
			_, err := (&Gopass{runner: runner}).PrepareSecret(context.Background(), SecretPassword, test.path)
			if err != test.want || !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want exact %v", err, test.want)
			}
			if test.name == "invalid requested path" && len(runner.calls) != 0 {
				t.Fatal("invalid requested path reached backend listing")
			}
		})
	}
}

func TestPrepareSecretRejectsActionBeforeListing(t *testing.T) {
	runner := &fakeRunner{output: []byte("synthetic/account\n")}
	request, err := (&Gopass{runner: runner}).PrepareSecret(context.Background(), SecretAction(99), "synthetic/account")
	if request != (SecretRequest{}) || err != ErrInvalidSecretAction || len(runner.calls) != 0 {
		t.Fatalf("invalid action request/error/calls = %#v/%v/%#v", request, err, runner.calls)
	}
}

func TestStartSecretRejectsUnpreparedRequestBeforeProcessStart(t *testing.T) {
	writer, err := createDiscardPipeWriter(t)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	store := NewGopass()
	if process, err := store.StartSecret(context.Background(), SecretRequest{action: SecretAction(99), entryPath: "synthetic/account"}, writer); process != nil || err != ErrInvalidSecretAction {
		t.Fatalf("invalid action process/error = %#v/%v", process, err)
	}
	if process, err := store.StartSecret(context.Background(), SecretRequest{action: SecretPassword, entryPath: "../account"}, writer); process != nil || err != ErrInvalidEntry {
		t.Fatalf("invalid path process/error = %#v/%v", process, err)
	}
}

func createDiscardPipeWriter(t *testing.T) (*os.File, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err == nil {
		_ = reader.Close()
	}
	return writer, err
}
