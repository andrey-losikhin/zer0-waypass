package backend

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fieldRunner struct {
	root  string
	calls []runnerCall
}

func (r *fieldRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	joined := strings.Join(args, "\x00")
	switch {
	// gopass >= 1.17 hides dot-prefixed sidecars from ls.
	case joined == "ls\x00--flat":
		return []byte("work/db\n"), nil
	case joined == "config\x00mounts.path":
		return []byte(r.root + "\n"), nil
	case strings.Contains(joined, "/manifests/"):
		return []byte(testManifest()), nil
	case strings.HasSuffix(joined, testOID(4)):
		return []byte("10.0.0.8\n"), nil
	default:
		return nil, ErrBackend
	}
}

type legacyFieldRunner struct {
	root  string
	calls []runnerCall
}

func (r *legacyFieldRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	joined := strings.Join(args, "\x00")
	if joined == "ls\x00--flat" {
		return []byte("legacy/account\n"), nil
	}
	if joined == "config\x00mounts.path" {
		return []byte(r.root + "\n"), nil
	}
	if joined == "show\x00--noparsing\x00--\x00legacy/account" {
		return []byte("hunter2\nusername: alice\nurl: https://example.test\napi_key: token-value\nProject: waypass\nEmpty: \n"), nil
	}
	return nil, ErrBackend
}

// sidecarStore creates empty encrypted-file placeholders for reserved paths.
func sidecarStore(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range paths {
		file := filepath.Join(root, filepath.FromSlash(p)+".gpg")
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func workDBStore(t *testing.T) string {
	t.Helper()
	manifestPath, _ := fieldManifestPath("work/db")
	bundle := ReservedFieldPrefix + "v1/" + testOID(1) + "/" + testOID(2) + "/"
	return sidecarStore(t, manifestPath, bundle+testOID(3), bundle+testOID(4))
}

func testOID(value byte) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat(string([]byte{value}), 16)))
}
func testManifest() string {
	return `{"format":"zer0-waypass/fields-v1","bundle_id":"` + testOID(1) + `","revision":"` + testOID(2) + `","fields":[` +
		`{"id":"` + testOID(3) + `","name":"Password","kind":"password","visibility":"secret","multiline":false},` +
		`{"id":"` + testOID(4) + `","name":"Host","kind":"host","visibility":"public","multiline":false}]}`
}
func testDigest() string {
	sum := sha256.Sum256([]byte(testManifest()))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestFieldsReturnsPublicValueAndNeverReadsSecretValue(t *testing.T) {
	r := &fieldRunner{root: workDBStore(t)}
	got, err := (&Gopass{runner: r}).Fields(context.Background(), "work/db")
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != testDigest() || len(got.Fields) != 2 || got.Fields[0].Value != "" || got.Fields[1].Value != "10.0.0.8" {
		t.Fatalf("unexpected fields: %#v", got)
	}
	secretPath := testOID(3)
	for _, call := range r.calls {
		if strings.Contains(strings.Join(call.args, "/"), secretPath) && len(call.args) > 0 && call.args[0] == "show" {
			t.Fatal("secret value was read")
		}
	}
}

func TestLegacyFieldsIncludeEveryNonEmptyNamedField(t *testing.T) {
	r := &legacyFieldRunner{root: sidecarStore(t)}
	got, err := (&Gopass{runner: r}).Fields(context.Background(), "legacy/account")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Fields) != 5 || got.Fields[1].Name != "username" || got.Fields[1].Value != "alice" || got.Fields[2].Value != "https://example.test" || got.Fields[3].Visibility != "secret" || got.Fields[4].Kind != "custom" {
		t.Fatalf("fields %#v", got)
	}
	if got.Fields[3].Value != "" || got.Fields[4].Value != "" {
		t.Fatalf("secret legacy values leaked: %#v", got.Fields)
	}
}

func TestResolveFieldBindsRevisionAndMembership(t *testing.T) {
	r := &fieldRunner{root: workDBStore(t)}
	got, err := (&Gopass{runner: r}).ResolveField(context.Background(), "work/db", testDigest(), testOID(3))
	if err != nil {
		t.Fatal(err)
	}
	want := ReservedFieldPrefix + "v1/" + testOID(1) + "/" + testOID(2) + "/" + testOID(3)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := (&Gopass{runner: r}).ResolveField(context.Background(), "work/db", testOID(9), testOID(3)); err != ErrInvalidManifest {
		t.Fatalf("stale error %v", err)
	}
}

func TestSidecarMembershipRequiresRegularFileInsideStore(t *testing.T) {
	outside := sidecarStore(t, ReservedFieldPrefix+"v1/manifests/escape")
	root := sidecarStore(t)
	if err := os.Symlink(filepath.Join(outside, ".zer0-waypass"), filepath.Join(root, ".zer0-waypass")); err != nil {
		t.Fatal(err)
	}
	g := &Gopass{runner: &fieldRunner{root: root}}
	if err := g.requireMember(context.Background(), ReservedFieldPrefix+"v1/manifests/escape"); err == nil {
		t.Fatal("symlink escape accepted")
	}
	clean := &Gopass{runner: &fieldRunner{root: sidecarStore(t)}}
	if err := clean.requireMember(context.Background(), ReservedFieldPrefix+"v1/manifests/missing"); err != ErrEntryNotFound {
		t.Fatalf("missing sidecar err=%v", err)
	}
}

func TestManifestRejectsUnsafeShapes(t *testing.T) {
	base := testManifest()
	tests := []string{
		strings.Replace(base, `"format":`, `"format":"x","format":`, 1),
		strings.Replace(base, `"name":"Host"`, `"name":"bad;name"`, 1),
		strings.Replace(base, `"visibility":"secret"`, `"visibility":"public"`, 1),
		strings.Replace(base, testOID(4), testOID(3), 1),
	}
	for _, raw := range tests {
		var m manifest
		if jsonUnmarshalForTest(raw, &m) == nil && validManifest(m) && rejectDuplicateKeys([]byte(raw)) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func jsonUnmarshalForTest(raw string, target any) error { return json.Unmarshal([]byte(raw), target) }

func TestStandardFieldPolicyStable(t *testing.T) {
	want := fieldPolicy{"public", true}
	if !reflect.DeepEqual(standardFields["notes"], want) {
		t.Fatalf("notes policy %#v", standardFields["notes"])
	}
	if standardFields["password"].visibility != "secret" || standardFields["private_key"].visibility != "secret" {
		t.Fatal("secret policy changed")
	}
}
