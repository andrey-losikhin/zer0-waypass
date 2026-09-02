package backend

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestPrepareSecretUsesFreshExactListing(t *testing.T) {
	runner := &fakeRunner{output: []byte("synthetic/account\nsynthetic/other\n")}
	store := &Gopass{runner: runner}

	if _, err := store.PrepareSecret(context.Background(), SecretPassword, "synthetic/account"); err != nil {
		t.Fatalf("PrepareSecret: %v", err)
	}
	wantCalls := []runnerCall{{name: "gopass", args: []string{"ls", "--flat"}}}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
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
