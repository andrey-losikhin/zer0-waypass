package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andrey-losikhin/zer0-waypass/internal/backend"
	"github.com/andrey-losikhin/zer0-waypass/internal/clipboard"
	"github.com/andrey-losikhin/zer0-waypass/internal/protocol"
)

const (
	fakeGopassEnabledEnv = "ZER0_WAYPASS_TEST_FAKE_GOPASS"
	fakeGopassLogEnv     = "ZER0_WAYPASS_TEST_FAKE_GOPASS_LOG"
	fakeGopassOutputEnv  = "ZER0_WAYPASS_TEST_FAKE_GOPASS_OUTPUT"
	fakeGopassModeEnv    = "ZER0_WAYPASS_TEST_FAKE_GOPASS_MODE"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeGopassEnabledEnv) == "1" {
		os.Exit(runFakeGopassExecutable())
	}
	os.Exit(m.Run())
}

func runFakeGopassExecutable() int {
	// Fail closed if production ever adds a decrypt/show argument.
	if filepath.Base(os.Args[0]) != "gopass" || !reflect.DeepEqual(os.Args[1:], []string{"ls", "--flat"}) {
		return 90
	}
	logFile, err := os.OpenFile(os.Getenv(fakeGopassLogEnv), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 91
	}
	if _, err := logFile.WriteString("ls\x00--flat\n"); err != nil {
		_ = logFile.Close()
		return 92
	}
	if err := logFile.Close(); err != nil {
		return 93
	}
	if os.Getenv(fakeGopassModeEnv) == "fail" {
		_, _ = os.Stderr.WriteString("SYNTHETIC_SECRET_FAKE_GOPASS_STDERR")
		return 17
	}
	if _, err := os.Stdout.WriteString(os.Getenv(fakeGopassOutputEnv)); err != nil {
		return 94
	}
	return 0
}

type fakeBackend struct {
	entries     []string
	err         error
	calls       int
	fieldSet    backend.FieldSet
	fieldErr    error
	fieldCalls  int
	locked      bool
	lockChecked int
}

func (b *fakeBackend) KeyCached(context.Context) (bool, error) {
	b.lockChecked++
	return !b.locked, nil
}

func (b *fakeBackend) List(context.Context) ([]string, error) {
	b.calls++
	return b.entries, b.err
}

func (b *fakeBackend) Fields(context.Context, string) (backend.FieldSet, error) {
	b.fieldCalls++
	return b.fieldSet, b.fieldErr
}
func (b *fakeBackend) ResolveField(context.Context, string, string, string) (string, error) {
	return "", b.fieldErr
}

func runForTest(t *testing.T, args []string, store *fakeBackend) (int, string, string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(context.Background(), args, store, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func errorLine(code protocol.ErrorCode) string {
	return `{"protocol":1,"error":{"code":"` + string(code) + `"}}` + "\n"
}

func TestListOutputsOneVersionedJSONLineInBackendOrder(t *testing.T) {
	store := &fakeBackend{entries: []string{"synthetic/alice", "example.test"}}
	code, stdout, stderr := runForTest(t, []string{"list"}, store)

	want := "{\"protocol\":1,\"items\":[{\"id\":\"AXN5bnRoZXRpYy9hbGljZQ\",\"label\":\"synthetic/alice\"},{\"id\":\"AWV4YW1wbGUudGVzdA\",\"label\":\"example.test\"}]}\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("code/stdout/stderr = %d/%q/%q, want 0/%q/empty", code, stdout, stderr, want)
	}
}

func TestListOutputsEmptyArray(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"list"}, &fakeBackend{})
	want := "{\"protocol\":1,\"items\":[]}\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("code/stdout/stderr = %d/%q/%q, want 0/%q/empty", code, stdout, stderr, want)
	}
}

func TestListFiltersCaseInsensitiveLiteralQuery(t *testing.T) {
	store := &fakeBackend{entries: []string{"Shell/A.B+", "shell/plain", "other"}}
	code, stdout, stderr := runForTest(t, []string{"list", "a.b+"}, store)

	want := "{\"protocol\":1,\"items\":[{\"id\":\"AVNoZWxsL0EuQis\",\"label\":\"Shell/A.B+\"}]}\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("code/stdout/stderr = %d/%q/%q, want 0/%q/empty", code, stdout, stderr, want)
	}
	if store.calls != 1 {
		t.Fatalf("backend calls = %d, want 1", store.calls)
	}
}

func TestListQueryInputBoundsAndUTF8(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantCode   int
		wantCalls  int
		wantStderr string
	}{
		{name: "4096 bytes accepted", query: strings.Repeat("a", maxQueryBytes), wantCode: 0, wantCalls: 1},
		{name: "4097 bytes rejected", query: strings.Repeat("a", maxQueryBytes+1), wantCode: 2, wantStderr: errorLine(protocol.ErrorInvalidInvocation)},
		{name: "invalid UTF-8 rejected", query: string([]byte{0xff}), wantCode: 2, wantStderr: errorLine(protocol.ErrorInvalidInvocation)},
		{name: "bounded Unicode and spaces accepted", query: "  пароль 東京  ", wantCode: 0, wantCalls: 1},
		{name: "bounded control text stays literal", query: "line\n\tend", wantCode: 0, wantCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeBackend{entries: []string{"safe/entry"}}
			code, stdout, stderr := runForTest(t, []string{"list", test.query}, store)
			if code != test.wantCode || store.calls != test.wantCalls || stderr != test.wantStderr {
				t.Fatalf("code/calls/stderr = %d/%d/%q, want %d/%d/%q", code, store.calls, stderr, test.wantCode, test.wantCalls, test.wantStderr)
			}
			if code == 0 && stdout != "{\"protocol\":1,\"items\":[]}\n" {
				t.Fatalf("stdout = %q, want empty list", stdout)
			}
			if code != 0 && stdout != "" {
				t.Fatalf("failed invocation stdout = %q, want empty", stdout)
			}
		})
	}
}

func TestListTreatsBoundedShellMetacharactersAsLiteralQuery(t *testing.T) {
	query := `; $() ' " <> & | * ? ! [] {}`
	store := &fakeBackend{entries: []string{"safe/entry"}}
	code, stdout, stderr := runForTest(t, []string{"list", query}, store)
	if code != 0 || stdout != "{\"protocol\":1,\"items\":[]}\n" || stderr != "" || store.calls != 1 {
		t.Fatalf("code/stdout/stderr/calls = %d/%q/%q/%d", code, stdout, stderr, store.calls)
	}
}

func TestProductionCLIUsesDurableFakeGopassExecutable(t *testing.T) {
	directory := t.TempDir()
	logPath := filepath.Join(directory, "argv.log")
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(os.Args[0], filepath.Join(directory, "gopass")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	t.Setenv(fakeGopassEnabledEnv, "1")
	t.Setenv(fakeGopassLogEnv, logPath)
	t.Setenv(fakeGopassModeEnv, "")

	runProduction := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := run(context.Background(), args, backend.NewGopass(), &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	t.Setenv(fakeGopassOutputEnv, "safe/first\nsafe/second\n")
	code, stdout, stderr := runProduction("list")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"label":"safe/first"`) || !strings.Contains(stdout, `"label":"safe/second"`) {
		t.Fatalf("initial list code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
	}

	query := `;$(show --password) "'`
	code, stdout, stderr = runProduction("list", query)
	if code != 0 || stdout != "{\"protocol\":1,\"items\":[]}\n" || stderr != "" {
		t.Fatalf("query list code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
	}

	code, stdout, stderr = runProduction("status")
	if code != 0 || stdout != "{\"protocol\":1,\"backend\":\"ready\"}\n" || stderr != "" {
		t.Fatalf("status code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
	}

	// A changed executable result must be fetched by a new process; no result
	// from a prior invocation may be cached by the helper or adapter.
	t.Setenv(fakeGopassOutputEnv, "changed/fresh\n")
	code, stdout, stderr = runProduction("list")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"label":"changed/fresh"`) || strings.Contains(stdout, "safe/first") {
		t.Fatalf("fresh list code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
	}

	t.Setenv(fakeGopassModeEnv, "fail")
	code, stdout, stderr = runProduction("list")
	if code != 1 || stdout != "" || stderr != errorLine(protocol.ErrorBackend) || strings.Contains(stderr, "SYNTHETIC_SECRET") {
		t.Fatalf("failing backend code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	wantLog := strings.Repeat("ls\x00--flat\n", 5)
	if string(logBytes) != wantLog || strings.Contains(string(logBytes), query) || strings.Contains(string(logBytes), "show") {
		t.Fatalf("fake argv log = %q, want five exact metadata-only invocations", logBytes)
	}
	if strings.Contains(stdout+stderr, "safe/first") || strings.Contains(stdout+stderr, query) {
		t.Fatal("failure output retained prior metadata or query")
	}
}

func TestProductionCLIMissingGopassIsRedactedUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"status"}, backend.NewGopass(), &stdout, &stderr)
	if code != 1 || stdout.String() != "" || stderr.String() != errorLine(protocol.ErrorBackendUnavailable) {
		t.Fatalf("code/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestListFailsClosedOnInvalidBackendPath(t *testing.T) {
	const marker = "SYNTHETIC_PRIVATE_PATH_MARKER"
	for _, args := range [][]string{{"list"}, {"list", "valid"}} {
		store := &fakeBackend{entries: []string{"valid/entry", "../" + marker, "later/entry"}}
		code, stdout, stderr := runForTest(t, args, store)
		if code == 0 || stdout != "" {
			t.Fatalf("code/stdout = %d/%q, want nonzero/empty", code, stdout)
		}
		if stderr != errorLine(protocol.ErrorBackendInvalidData) || strings.Contains(stderr, marker) {
			t.Fatalf("stderr = %q, want generic error without backend path", stderr)
		}
	}
}

func TestListUsesExactLivePathLabelsWithoutFieldLookupOrCache(t *testing.T) {
	store := &fakeBackend{entries: []string{"example.test/work"}}
	code, stdout, stderr := runForTest(t, []string{"list"}, store)
	want := "{\"protocol\":1,\"items\":[{\"id\":\"AWV4YW1wbGUudGVzdC93b3Jr\",\"label\":\"example.test/work\"}]}\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("first list code/stdout/stderr = %d/%q/%q, want 0/%q/empty", code, stdout, stderr, want)
	}

	// A changed backend result must be visible on the next invocation: list
	// results and labels are not retained in process memory between calls.
	store.entries = []string{"changed/current-path"}
	code, stdout, stderr = runForTest(t, []string{"list"}, store)
	if code != 0 || !strings.Contains(stdout, `"label":"changed/current-path"`) || strings.Contains(stdout, "example.test/work") || stderr != "" {
		t.Fatalf("second list code/stdout/stderr = %d/%q/%q, want only current backend label", code, stdout, stderr)
	}
	if store.calls != 2 {
		t.Fatalf("backend calls = %d, want one fresh call per list invocation", store.calls)
	}
}

func TestListQueryMatchesOnlyPathLabel(t *testing.T) {
	const path = "example.test/work"
	for _, query := range []string{"synthetic-user@example.test", "https://example.test/login", "custom-field-marker"} {
		t.Run(query, func(t *testing.T) {
			store := &fakeBackend{entries: []string{path}}
			code, stdout, stderr := runForTest(t, []string{"list", query}, store)
			want := "{\"protocol\":1,\"items\":[]}\n"
			if code != 0 || stdout != want || stderr != "" {
				t.Fatalf("code/stdout/stderr = %d/%q/%q, want 0/%q/empty", code, stdout, stderr, want)
			}
			if store.calls != 1 {
				t.Fatalf("backend calls = %d, want 1", store.calls)
			}
		})
	}
}

func TestStatusOutputsClosedResponseAfterProbe(t *testing.T) {
	store := &fakeBackend{entries: []string{"entry-name-must-not-leak"}}
	code, stdout, stderr := runForTest(t, []string{"status"}, store)

	want := "{\"protocol\":1,\"backend\":\"ready\"}\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("code/stdout/stderr = %d/%q/%q, want 0/%q/empty", code, stdout, stderr, want)
	}
	if store.calls != 1 {
		t.Fatalf("backend calls = %d, want 1", store.calls)
	}
}

func TestStatusAlwaysPerformsFreshProbeWithoutExposingPaths(t *testing.T) {
	store := &fakeBackend{entries: []string{"first/private-path"}}
	for _, changedEntries := range [][]string{{"first/private-path"}, {"second/private-path"}} {
		store.entries = changedEntries
		code, stdout, stderr := runForTest(t, []string{"status"}, store)
		if code != 0 || stdout != "{\"protocol\":1,\"backend\":\"ready\"}\n" || stderr != "" {
			t.Fatalf("status code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
		}
		for _, path := range []string{"first/private-path", "second/private-path"} {
			if strings.Contains(stdout, path) || strings.Contains(stderr, path) {
				t.Fatalf("status exposed path %q in stdout/stderr", path)
			}
		}
	}
	if store.calls != 2 {
		t.Fatalf("backend calls = %d, want one fresh probe per status invocation", store.calls)
	}
}

// TestProductionPersistenceAndExecTripwire is a deliberately non-exhaustive,
// milestone-local guard. It complements behavioral tests and source review; it
// is not proof that persistence is impossible.
func TestProductionPersistenceAndExecTripwire(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	forbiddenCalls := map[string]bool{
		"os.Create": true, "os.CreateTemp": true, "os.NewFile": true, "os.OpenFile": true, "os.WriteFile": true,
		"io/ioutil.WriteFile": true,
	}
	// This allowlist describes the reviewed boundary through M3 step 5d. The
	// expected call count makes additional exec sites in an allowed file fail.
	allowedExecConstructorFiles := map[string]int{
		filepath.Join("internal", "backend", "gopass.go"):      1,
		filepath.Join("internal", "backend", "secret.go"):      1,
		filepath.Join("internal", "clipboard", "clipboard.go"): 1,
		filepath.Join("internal", "clipboard", "guardian.go"):  1,
	}
	allowedNewFileFiles := map[string]int{
		filepath.Join("internal", "clipboard", "guardian.go"): 2,
	}
	execConstructorCounts := make(map[string]int)
	newFileCounts := make(map[string]int)

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileSet := token.NewFileSet()
		file, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		importPaths := make(map[string]string)
		for _, imported := range file.Imports {
			name, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			if name == "database" || strings.HasPrefix(name, "database/") || name == "encoding/gob" {
				t.Errorf("production import %q permits a plaintext metadata index: %s", name, path)
			}
			localName := filepath.Base(name)
			if imported.Name != nil {
				localName = imported.Name.Name
			}
			importPaths[localName] = name
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			packageName, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			qualifiedName := importPaths[packageName.Name] + "." + selector.Sel.Name
			position := fileSet.Position(call.Pos())
			relativePath, relErr := filepath.Rel(root, path)
			if relErr != nil {
				t.Errorf("relative production path at %s: %v", position, relErr)
				return true
			}
			if qualifiedName == "os.NewFile" && allowedNewFileFiles[relativePath] > 0 {
				newFileCounts[relativePath]++
			} else if forbiddenCalls[qualifiedName] {
				t.Errorf("production file-write primitive %s at %s", qualifiedName, position)
			}
			if importPaths[packageName.Name] == "os/exec" && (selector.Sel.Name == "Command" || selector.Sel.Name == "CommandContext") {
				if relErr != nil || allowedExecConstructorFiles[relativePath] == 0 {
					t.Errorf("production command construction outside reviewed allowlist at %s", position)
				} else if selector.Sel.Name != "CommandContext" && relativePath != filepath.Join("internal", "clipboard", "guardian.go") {
					t.Errorf("production command construction without context at %s", position)
				} else {
					execConstructorCounts[relativePath]++
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("inspect production Go: %v", err)
	}
	for path, want := range allowedExecConstructorFiles {
		if got := execConstructorCounts[path]; got != want {
			t.Errorf("production exec constructor count in %s = %d, want %d", path, got, want)
		}
	}
	for path, want := range allowedNewFileFiles {
		if got := newFileCounts[path]; got != want {
			t.Errorf("production os.NewFile count in %s = %d, want %d", path, got, want)
		}
	}
}

func TestBackendErrorHasNoStdoutOrRawErrorLeak(t *testing.T) {
	const marker = "SYNTHETIC_SECRET_BACKEND_MARKER"
	for _, command := range [][]string{{"list"}, {"status"}} {
		t.Run(command[0], func(t *testing.T) {
			code, stdout, stderr := runForTest(t, command, &fakeBackend{err: errors.New(marker)})
			if code == 0 || stdout != "" {
				t.Fatalf("code/stdout = %d/%q, want nonzero/empty", code, stdout)
			}
			if strings.Contains(stderr, marker) || stderr != errorLine(protocol.ErrorBackend) {
				t.Fatalf("stderr = %q, want generic error without marker", stderr)
			}
		})
	}
}

func TestInvalidCommandAndArityDoNotCallBackend(t *testing.T) {
	cases := [][]string{
		nil,
		{"show"},
		{"list", "one", "two"},
		{"status", "extra"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			store := &fakeBackend{}
			code, stdout, stderr := runForTest(t, args, store)
			if code != 2 || stdout != "" || stderr != errorLine(protocol.ErrorInvalidInvocation) {
				t.Fatalf("code/stdout/stderr = %d/%q/%q, want nonzero/empty/generic", code, stdout, stderr)
			}
			if store.calls != 0 {
				t.Fatalf("backend calls = %d, want 0", store.calls)
			}
		})
	}
}

func TestBackendErrorsMapToStableCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code protocol.ErrorCode
	}{
		{"unavailable", backend.ErrUnavailable, protocol.ErrorBackendUnavailable},
		{"timeout", backend.ErrTimeout, protocol.ErrorBackendTimeout},
		{"canceled", backend.ErrCanceled, protocol.ErrorOperationCanceled},
		{"too large", backend.ErrOutputTooLarge, protocol.ErrorBackendOutputTooLarge},
		{"backend", backend.ErrBackend, protocol.ErrorBackend},
		{"unknown raw error", errors.New("SYNTHETIC_RAW_ERROR_MARKER"), protocol.ErrorBackend},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			for _, command := range [][]string{{"list"}, {"status"}} {
				code, stdout, stderr := runForTest(t, command, &fakeBackend{err: test.err})
				if code != 1 || stdout != "" || stderr != errorLine(test.code) {
					t.Fatalf("code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
				}
				if strings.Contains(stdout+stderr, "SYNTHETIC_RAW_ERROR_MARKER") {
					t.Fatal("raw backend error leaked")
				}
			}
		})
	}
}

type failingWriter struct {
	limit int
	err   error
	data  bytes.Buffer
}

func (w *failingWriter) Write(data []byte) (int, error) {
	count := len(data)
	if count > w.limit {
		count = w.limit
	}
	_, _ = w.data.Write(data[:count])
	return count, w.err
}

func TestMetadataOutputWriteFailuresUseOutputError(t *testing.T) {
	for _, test := range []struct {
		name   string
		writer *failingWriter
	}{
		{"write error", &failingWriter{limit: 0, err: errors.New("SYNTHETIC_WRITE_MARKER")}},
		{"short write", &failingWriter{limit: 5}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := run(context.Background(), []string{"list"}, &fakeBackend{entries: []string{"synthetic/entry"}}, test.writer, &stderr)
			if code != 1 || stderr.String() != errorLine(protocol.ErrorOutput) {
				t.Fatalf("code/stderr = %d/%q", code, stderr.String())
			}
			if strings.Contains(test.writer.data.String()+stderr.String(), "SYNTHETIC_WRITE_MARKER") {
				t.Fatal("writer error leaked")
			}
		})
	}
}

func TestErrorWriterFailureDoesNotRecurse(t *testing.T) {
	stderr := &failingWriter{limit: 3, err: errors.New("write failed")}
	code := run(context.Background(), nil, &fakeBackend{}, &bytes.Buffer{}, stderr)
	if code != 2 || stderr.data.Len() != 3 {
		t.Fatalf("code/written = %d/%d, want 2/3", code, stderr.data.Len())
	}
}

func TestUntrustedMarkersNeverEnterErrorJSON(t *testing.T) {
	const marker = "SYNTHETIC_UNTRUSTED_MARKER"
	cases := []struct {
		args  []string
		store *fakeBackend
	}{
		{[]string{"list", marker}, &fakeBackend{err: errors.New(marker)}},
		{[]string{"list"}, &fakeBackend{entries: []string{"../" + marker}}},
		{[]string{marker}, &fakeBackend{}},
	}
	for _, test := range cases {
		code, stdout, stderr := runForTest(t, test.args, test.store)
		if code == 0 || stdout != "" || strings.Contains(stderr, marker) {
			t.Fatalf("code/stdout/stderr = %d/%q/%q", code, stdout, stderr)
		}
	}
}

type blockingBackend struct {
	contextDone chan struct{}
}

func (b *blockingBackend) List(ctx context.Context) ([]string, error) {
	<-ctx.Done()
	close(b.contextDone)
	return nil, ctx.Err()
}

func TestCancellationReachesMetadataBackend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &blockingBackend{contextDone: make(chan struct{})}
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"list"}, store, &bytes.Buffer{}, &bytes.Buffer{})
	}()

	cancel()
	select {
	case <-store.contextDone:
	case <-time.After(time.Second):
		t.Fatal("cancelled context did not reach metadata backend")
	}
	select {
	case code := <-done:
		if code == 0 {
			t.Fatal("cancelled operation returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled CLI operation did not return")
	}
}

func TestDeadlineReachesMetadataBackend(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	store := &blockingBackend{contextDone: make(chan struct{})}
	started := time.Now()
	code := run(ctx, []string{"status"}, store, &bytes.Buffer{}, &bytes.Buffer{})
	if code == 0 {
		t.Fatal("expired operation returned success")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("deadline propagated in %v, want <= 1s", elapsed)
	}
	select {
	case <-store.contextDone:
	default:
		t.Fatal("deadline did not reach metadata backend")
	}
}

func TestMetadataOperationTimeout(t *testing.T) {
	if metadataOperationTimeout != 10*time.Second {
		t.Fatalf("metadataOperationTimeout = %v, want 10s", metadataOperationTimeout)
	}
}

func TestFieldsCLISeparatesPublicAndSecretValues(t *testing.T) {
	entryID, _ := protocol.EncodeCanonicalPath("work/db")
	store := &fakeBackend{fieldSet: backend.FieldSet{Revision: tokenForTest(32, 7), Fields: []backend.Field{
		{ID: tokenForTest(16, 1), Name: "Password", Kind: "password", Visibility: "secret"},
		{ID: tokenForTest(16, 2), Name: "Notes", Kind: "notes", Visibility: "public", Multiline: true, Value: "visible notes"},
	}}}
	code, stdout, stderr := runForTest(t, []string{"fields", string(entryID)}, store)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"protocol":2`) || !strings.Contains(stdout, `"value":"visible notes"`) {
		t.Fatalf("result %d %q %q", code, stdout, stderr)
	}
	if strings.Contains(stdout, "secret-marker") || strings.Contains(stderr, "secret-marker") {
		t.Fatal("secret leaked")
	}
}

func TestCopyFieldValidatesTokensAndDispatches(t *testing.T) {
	entryID, _ := protocol.EncodeCanonicalPath("work/db")
	revision, fieldID := tokenForTest(32, 3), tokenForTest(16, 4)
	old := fieldCopyDispatcher
	defer func() { fieldCopyDispatcher = old }()
	called := false
	fieldCopyDispatcher = func(_ context.Context, entry, gotRevision, gotField string, _ clipboard.Policy) error {
		called = true
		if entry != "work/db" || gotRevision != revision || gotField != fieldID {
			t.Fatal("wrong dispatch")
		}
		return nil
	}
	code, stdout, stderr := runForTest(t, []string{"copy", "field", string(entryID), revision, fieldID, "--ttl", "30"}, &fakeBackend{})
	if code != 0 || stdout != "" || stderr != "" || !called {
		t.Fatalf("result %d %q %q called=%v", code, stdout, stderr, called)
	}
	called = false
	code, _, _ = runForTest(t, []string{"copy", "field", string(entryID), "-bad", fieldID, "--ttl", "30"}, &fakeBackend{})
	if code != 2 || called {
		t.Fatal("invalid revision reached dispatcher")
	}
}

func TestFieldsReportsLockedKeyWithoutDecrypting(t *testing.T) {
	entryID, _ := protocol.EncodeCanonicalPath("work/db")
	store := &fakeBackend{locked: true}
	code, stdout, stderr := runForTest(t, []string{"fields", string(entryID)}, store)
	if code != 1 || stdout != "" || stderr != errorLine(protocol.ErrorBackendLocked) || store.fieldCalls != 0 {
		t.Fatalf("result %d %q %q decrypts=%d", code, stdout, stderr, store.fieldCalls)
	}
}

func TestFieldsAllowPromptSkipsLockCheck(t *testing.T) {
	entryID, _ := protocol.EncodeCanonicalPath("work/db")
	store := &fakeBackend{locked: true}
	code, _, stderr := runForTest(t, []string{"fields", string(entryID), "--allow-prompt"}, store)
	if code != 0 || stderr != "" || store.lockChecked != 0 || store.fieldCalls != 1 {
		t.Fatalf("result %d %q checks=%d decrypts=%d", code, stderr, store.lockChecked, store.fieldCalls)
	}
	if code, _, _ := runForTest(t, []string{"fields", string(entryID), "--other"}, store); code != 2 {
		t.Fatalf("unknown flag accepted: %d", code)
	}
}

func TestUnlockDecryptsWithoutOutput(t *testing.T) {
	entryID, _ := protocol.EncodeCanonicalPath("work/db")
	store := &fakeBackend{locked: true, fieldSet: backend.FieldSet{Fields: []backend.Field{{ID: "x", Name: "Notes", Kind: "notes", Visibility: "public", Value: "visible"}}}}
	code, stdout, stderr := runForTest(t, []string{"unlock", string(entryID)}, store)
	if code != 0 || stdout != "" || stderr != "" || store.fieldCalls != 1 || store.lockChecked != 0 {
		t.Fatalf("result %d %q %q", code, stdout, stderr)
	}
	store.fieldErr = backend.ErrCanceled
	if code, _, stderr := runForTest(t, []string{"unlock", string(entryID)}, store); code != 1 || stderr != errorLine(protocol.ErrorOperationCanceled) {
		t.Fatalf("canceled unlock %d %q", code, stderr)
	}
	if code, _, _ := runForTest(t, []string{"unlock", "../etc"}, store); code != 2 {
		t.Fatal("invalid entry ID accepted")
	}
}

func TestUnlockTimeoutFitsNoctaliaCallbackCap(t *testing.T) {
	if unlockOperationTimeout >= 60*time.Second {
		t.Fatalf("unlockOperationTimeout = %v, must stay below 60s", unlockOperationTimeout)
	}
}

func tokenForTest(size int, value byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{value}, size))
}
