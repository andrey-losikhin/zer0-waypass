package backend

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const testColons = "sec:u:255:22:AAAA:1::::::scESC:::+::ed25519:::0:\n" +
	"grp:::::::::SIGNGRIP:\n" +
	"ssb:u:255:18:BBBB:1::::::e:::+::cv25519::\n" +
	"grp:::::::::ENCGRIP:\n"

type agentRunner struct {
	root    string
	keyinfo string
	gpgErr  error
	calls   []runnerCall
}

func (r *agentRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	switch name {
	case "gopass":
		return []byte(r.root + "\n"), nil
	case "gpg":
		return []byte(testColons), r.gpgErr
	case "gpg-connect-agent":
		return []byte(r.keyinfo), nil
	}
	return nil, ErrBackend
}

func agentStore(t *testing.T, gpgID string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gpg-id"), []byte(gpgID), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestKeyCachedUsesOnlyMetadataArgv(t *testing.T) {
	r := &agentRunner{root: agentStore(t, "# comment\n0xAAAA\n"), keyinfo: "S KEYINFO SIGNGRIP D - - - P - - -\nS KEYINFO ENCGRIP D - - 1 P - - -\nOK\n"}
	cached, err := (&Gopass{runner: r}).KeyCached(context.Background())
	if err != nil || !cached {
		t.Fatalf("cached=%v err=%v", cached, err)
	}
	want := []runnerCall{
		{name: "gopass", args: []string{"config", "mounts.path"}},
		{name: "gpg", args: []string{"--batch", "--with-colons", "--with-keygrip", "--list-secret-keys", "--", "0xAAAA"}},
		{name: "gpg-connect-agent", args: []string{"--no-autostart", "KEYINFO --list", "/bye"}},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls %#v", r.calls)
	}
}

func TestKeyCachedIgnoresCachedSigningKey(t *testing.T) {
	r := &agentRunner{root: agentStore(t, "0xAAAA\n"), keyinfo: "S KEYINFO SIGNGRIP D - - 1 P - - -\nS KEYINFO ENCGRIP D - - - P - - -\n"}
	cached, err := (&Gopass{runner: r}).KeyCached(context.Background())
	if err != nil || cached {
		t.Fatalf("cached=%v err=%v", cached, err)
	}
}

func TestKeyCachedTreatsUnprotectedKeyAsUsable(t *testing.T) {
	r := &agentRunner{root: agentStore(t, "0xAAAA\n"), keyinfo: "S KEYINFO ENCGRIP D - - - C - - -\n"}
	if cached, err := (&Gopass{runner: r}).KeyCached(context.Background()); err != nil || !cached {
		t.Fatalf("cached=%v err=%v", cached, err)
	}
}

func TestKeyCachedMissingSecretKeyIsLocked(t *testing.T) {
	r := &agentRunner{root: agentStore(t, "0xAAAA\n"), gpgErr: errors.New("exit status 2")}
	if cached, err := (&Gopass{runner: r}).KeyCached(context.Background()); err != nil || cached {
		t.Fatalf("cached=%v err=%v", cached, err)
	}
}

func TestKeyCachedRejectsOptionLikeRecipient(t *testing.T) {
	r := &agentRunner{root: agentStore(t, "--homedir=/tmp/evil\n")}
	if _, err := (&Gopass{runner: r}).KeyCached(context.Background()); err != ErrBackend {
		t.Fatalf("err=%v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("gpg was invoked: %#v", r.calls)
	}
}

func TestKeyCachedRejectsRelativeStorePath(t *testing.T) {
	r := &agentRunner{root: "relative/store"}
	if _, err := (&Gopass{runner: r}).KeyCached(context.Background()); err != ErrBackend {
		t.Fatalf("err=%v", err)
	}
}
