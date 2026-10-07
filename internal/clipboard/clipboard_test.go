package clipboard

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/andrey-losikhin/zer0-waypass/internal/backend"
)

func TestCopyStreamsDirectlyWithExactArgvAndFreshMembership(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	logPath := filepath.Join(t.TempDir(), "argv.jsonl")
	okPath := filepath.Join(t.TempDir(), "consumed")
	t.Setenv("FAKE_ARGV_LOG", logPath)
	t.Setenv("FAKE_CONSUMED_FILE", okPath)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\nsynthetic/other\n")

	tests := []struct {
		name   string
		action backend.SecretAction
		show   []string
	}{
		{name: "password", action: backend.SecretPassword, show: []string{"gopass", "show", "--password", "--", "synthetic/account"}},
		{name: "username", action: backend.SecretUsername, show: []string{"gopass", "show", "--", "synthetic/account", "username"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(logPath, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			_ = os.Remove(okPath)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := Copy(ctx, backend.NewGopass(), test.action, "synthetic/account"); err != nil {
				t.Fatalf("Copy: %v", err)
			}
			if content, err := os.ReadFile(okPath); err != nil || string(content) != "ok" {
				t.Fatalf("fake clipboard did not confirm exact consumption: %q, %v", content, err)
			}

			calls := readArgvLog(t, logPath)
			if len(calls) != 3 || !reflect.DeepEqual(calls[0], []string{"gopass", "ls", "--flat"}) {
				t.Fatalf("argv calls = %#v, want fresh listing followed by exactly two workers", calls)
			}
			ownerCall := []string{"wl-copy", "--sensitive", "--foreground", "--trim-newline", "--type", "text/plain"}
			if !(reflect.DeepEqual(calls[1], ownerCall) && reflect.DeepEqual(calls[2], test.show)) &&
				!(reflect.DeepEqual(calls[2], ownerCall) && reflect.DeepEqual(calls[1], test.show)) {
				t.Fatalf("worker argv calls = %#v, want exact owner and show argv", calls[1:])
			}
			for _, call := range calls {
				if strings.Contains(strings.Join(call, "\x00"), "SYNTHETIC_") {
					t.Fatal("secret appeared in argv log")
				}
			}
		})
	}
}

func TestCopyRejectsMembershipFailuresBeforeDecryptOrClipboard(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	logPath := filepath.Join(t.TempDir(), "argv.jsonl")
	t.Setenv("FAKE_ARGV_LOG", logPath)

	tests := []struct {
		name    string
		entries string
		mode    string
		want    error
	}{
		{name: "missing", entries: "synthetic/other\n", want: backend.ErrEntryNotFound},
		{name: "invalid listing", entries: "synthetic/account\n../invalid\n", want: backend.ErrInvalidEntry},
		{name: "list error", mode: "list-fail", want: backend.ErrBackend},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(logPath, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("FAKE_ENTRIES", test.entries)
			t.Setenv("FAKE_GOPASS_MODE", test.mode)
			err := Copy(context.Background(), backend.NewGopass(), backend.SecretPassword, "synthetic/account")
			if err != test.want {
				t.Fatalf("error = %v, want exact %v", err, test.want)
			}
			if calls := readArgvLog(t, logPath); !reflect.DeepEqual(calls, [][]string{{"gopass", "ls", "--flat"}}) {
				t.Fatalf("calls = %#v, want listing only", calls)
			}
		})
	}
}

func TestCopyRejectsInvalidActionBeforeBackend(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	logPath := filepath.Join(t.TempDir(), "argv.jsonl")
	t.Setenv("FAKE_ARGV_LOG", logPath)

	err := Copy(context.Background(), backend.NewGopass(), backend.SecretAction(99), "synthetic/account")
	if err != backend.ErrInvalidSecretAction {
		t.Fatalf("error = %v, want exact invalid-action sentinel", err)
	}
	if calls := readArgvLog(t, logPath); len(calls) != 0 {
		t.Fatalf("invalid action started processes: %#v", calls)
	}
}

type acquisitionDeadlineProbe struct {
	t             *testing.T
	prepareCalled bool
	startCalled   bool
}

func (p *acquisitionDeadlineProbe) PrepareSecret(ctx context.Context, _ backend.SecretAction, _ string) (backend.SecretRequest, error) {
	p.prepareCalled = true
	deadline, ok := ctx.Deadline()
	if !ok {
		p.t.Fatal("PrepareSecret context has no acquisition deadline")
	}
	remaining := time.Until(deadline)
	if remaining < DefaultAcquisitionDeadline-time.Second || remaining > DefaultAcquisitionDeadline {
		p.t.Fatalf("acquisition deadline remaining = %v, want approximately %v", remaining, DefaultAcquisitionDeadline)
	}
	return backend.SecretRequest{}, backend.ErrEntryNotFound
}

func (p *acquisitionDeadlineProbe) StartSecret(context.Context, backend.SecretRequest, *os.File) (*backend.SecretProcess, error) {
	p.startCalled = true
	return nil, backend.ErrBackend
}

func TestCopyCreatesAcquisitionDeadlineBeforeMembershipAndWorkers(t *testing.T) {
	probe := &acquisitionDeadlineProbe{t: t}
	err := Copy(context.Background(), probe, backend.SecretPassword, "synthetic/account")
	if err != backend.ErrEntryNotFound || !probe.prepareCalled || probe.startCalled {
		t.Fatalf("error/prepare/start = %v/%v/%v, want entry-not-found/true/false", err, probe.prepareCalled, probe.startCalled)
	}
}

func TestCopyProcessFailuresAreRedactedAndReaped(t *testing.T) {
	tests := []struct {
		name       string
		gopassMode string
		ownerMode  string
		want       error
	}{
		{name: "decrypt failure", gopassMode: "show-fail", want: backend.ErrBackend},
		{name: "early clipboard exit", gopassMode: "hang-show", ownerMode: "early", want: ErrEarlyExit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binDir := buildFakeExecutables(t)
			t.Setenv("PATH", binDir)
			t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
			t.Setenv("FAKE_GOPASS_MODE", test.gopassMode)
			t.Setenv("FAKE_WLCOPY_MODE", test.ownerMode)
			gopassPID := filepath.Join(t.TempDir(), "gopass.pid")
			ownerPID := filepath.Join(t.TempDir(), "owner.pid")
			t.Setenv("FAKE_GOPASS_PID", gopassPID)
			t.Setenv("FAKE_WLCOPY_PID", ownerPID)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := Copy(ctx, backend.NewGopass(), backend.SecretPassword, "synthetic/account")
			if err != test.want || strings.Contains(err.Error(), "RAW_") || strings.Contains(err.Error(), "synthetic/account") {
				t.Fatalf("error = %v, want exact redacted %v", err, test.want)
			}
			for _, path := range []string{gopassPID, ownerPID} {
				pid := readPID(t, path)
				if processExists(pid) {
					t.Fatalf("direct child %d remains after failure", pid)
				}
			}
		})
	}
}

func TestCopyCancellationReapsDirectChildren(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	t.Setenv("FAKE_GOPASS_MODE", "hang-show-descendant")
	t.Setenv("FAKE_WLCOPY_MODE", "hang-descendant")
	gopassPID := filepath.Join(t.TempDir(), "gopass.pid")
	ownerPID := filepath.Join(t.TempDir(), "owner.pid")
	gopassDescendantPID := filepath.Join(t.TempDir(), "gopass-descendant.pid")
	ownerDescendantPID := filepath.Join(t.TempDir(), "owner-descendant.pid")
	t.Setenv("FAKE_GOPASS_PID", gopassPID)
	t.Setenv("FAKE_WLCOPY_PID", ownerPID)
	t.Setenv("FAKE_GOPASS_DESCENDANT_PID", gopassDescendantPID)
	t.Setenv("FAKE_WLCOPY_DESCENDANT_PID", ownerDescendantPID)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := Copy(ctx, backend.NewGopass(), backend.SecretPassword, "synthetic/account")
	elapsed := time.Since(started)
	if err != ErrTimeout {
		t.Fatalf("error = %v, want exact timeout", err)
	}
	if elapsed < 550*time.Millisecond || elapsed > 950*time.Millisecond {
		t.Fatalf("shared cleanup elapsed = %v, want one 500ms grace after deadline (not sequential grace)", elapsed)
	}
	for _, path := range []string{gopassPID, ownerPID, gopassDescendantPID, ownerDescendantPID} {
		pid := readPID(t, path)
		if processExists(pid) {
			t.Fatalf("operation process %d remains after Copy returned", pid)
		}
	}
}

func TestCopyHandledCancellationCleansOperationGroups(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	t.Setenv("FAKE_GOPASS_MODE", "hang-show-descendant")
	t.Setenv("FAKE_WLCOPY_MODE", "hang-descendant")
	paths := []string{
		filepath.Join(t.TempDir(), "gopass.pid"),
		filepath.Join(t.TempDir(), "owner.pid"),
		filepath.Join(t.TempDir(), "gopass-descendant.pid"),
		filepath.Join(t.TempDir(), "owner-descendant.pid"),
	}
	t.Setenv("FAKE_GOPASS_PID", paths[0])
	t.Setenv("FAKE_WLCOPY_PID", paths[1])
	t.Setenv("FAKE_GOPASS_DESCENDANT_PID", paths[2])
	t.Setenv("FAKE_WLCOPY_DESCENDANT_PID", paths[3])

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Copy(ctx, backend.NewGopass(), backend.SecretPassword, "synthetic/account") }()
	for _, path := range paths {
		if !waitForFile(path, 2*time.Second) {
			t.Fatalf("operation PID file not ready: %s", path)
		}
	}
	cancel()
	if err := <-done; err != ErrCanceled {
		t.Fatalf("error = %v, want handled cancellation", err)
	}
	for _, path := range paths {
		if pid := readPID(t, path); processExists(pid) {
			t.Fatalf("operation process %d remains after cancellation", pid)
		}
	}
}

func TestCopyOwnershipBudgetKillsOldOwnerAndPreservesNewerGroup(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	t.Setenv("FAKE_WLCOPY_MODE", "hang-stubborn-newer")
	backendDonePath := filepath.Join(t.TempDir(), "backend.done")
	termPath := filepath.Join(t.TempDir(), "owner.term")
	newerPIDPath := filepath.Join(t.TempDir(), "newer.pid")
	t.Setenv("FAKE_BACKEND_DONE", backendDonePath)
	t.Setenv("FAKE_WLCOPY_TERM", termPath)
	t.Setenv("FAKE_NEWER_OWNER_PID", newerPIDPath)

	policy := Policy{
		AcquisitionDeadline: MinimumAcquisitionDeadline,
		OwnershipBudget:     MinimumOwnershipBudget,
		KillGrace:           MinimumKillGrace,
	}
	if err := copyWithPolicy(context.Background(), backend.NewGopass(), backend.SecretPassword, "synthetic/account", policy, nil, nil); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	returnedAt := time.Now()
	backendDone, err := os.Stat(backendDonePath)
	if err != nil {
		t.Fatal(err)
	}
	term, err := os.Stat(termPath)
	if err != nil {
		t.Fatal(err)
	}
	termAfterBackend := term.ModTime().Sub(backendDone.ModTime())
	wantTerm := policy.OwnershipBudget - policy.KillGrace
	if termAfterBackend < wantTerm-150*time.Millisecond || termAfterBackend > wantTerm+150*time.Millisecond {
		t.Fatalf("TERM after backend = %v, want %v ±150ms", termAfterBackend, wantTerm)
	}
	returnAfterBackend := returnedAt.Sub(backendDone.ModTime())
	if returnAfterBackend < policy.OwnershipBudget || returnAfterBackend > policy.OwnershipBudget+250*time.Millisecond {
		t.Fatalf("bounded return after backend = %v, want %v..%v", returnAfterBackend, policy.OwnershipBudget, policy.OwnershipBudget+250*time.Millisecond)
	}
	newerPID := readPID(t, newerPIDPath)
	if !processExists(newerPID) {
		t.Fatalf("newer owner %d was removed by old ownership budget", newerPID)
	}
	_ = syscall.Kill(-newerPID, syscall.SIGKILL)
	waitProcessGone(t, newerPID, 2*time.Second)
}

func TestCopyWorkersUseDistinctIsolatedProcessGroups(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")

	type groups struct {
		owner   int
		backend int
	}
	run := func() (groups, error) {
		var observed groups
		var observationErr error
		err := copyWithProcessGroupObserver(
			context.Background(),
			backend.NewGopass(),
			backend.SecretPassword,
			"synthetic/account",
			nil,
			func(ownerPGID, backendPGID int) error {
				observed = groups{owner: ownerPGID, backend: backendPGID}
				for name, pgid := range map[string]int{"owner": ownerPGID, "backend": backendPGID} {
					actual, err := syscall.Getpgid(pgid)
					if err != nil || actual != pgid {
						observationErr = errors.New(name + " process group is not led by its retained PID")
					}
				}
				return nil
			},
		)
		if err != nil {
			return groups{}, err
		}
		if observationErr != nil {
			return groups{}, observationErr
		}
		if observed.owner <= 0 || observed.backend <= 0 || observed.owner == observed.backend {
			return groups{}, errors.New("worker process groups are not distinct")
		}
		return observed, nil
	}

	t.Run("repeated", func(t *testing.T) {
		seen := make(map[int]bool)
		for i := 0; i < 10; i++ {
			observed, err := run()
			if err != nil {
				t.Fatalf("copy %d: %v", i, err)
			}
			for _, pgid := range []int{observed.owner, observed.backend} {
				if seen[pgid] {
					t.Fatalf("process group %d was reused during repeated operations", pgid)
				}
				seen[pgid] = true
			}
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		const copies = 12
		results := make(chan groups, copies)
		errorsCh := make(chan error, copies)
		var start sync.WaitGroup
		start.Add(1)
		var workers sync.WaitGroup
		workers.Add(copies)
		for i := 0; i < copies; i++ {
			go func() {
				defer workers.Done()
				start.Wait()
				observed, err := run()
				if err != nil {
					errorsCh <- err
					return
				}
				results <- observed
			}()
		}
		start.Done()
		workers.Wait()
		close(results)
		close(errorsCh)
		for err := range errorsCh {
			t.Errorf("concurrent copy: %v", err)
		}
		seen := make(map[int]bool)
		count := 0
		for observed := range results {
			count++
			for _, pgid := range []int{observed.owner, observed.backend} {
				if seen[pgid] {
					t.Errorf("process group %d is shared by concurrent operations", pgid)
				}
				seen[pgid] = true
			}
		}
		if count != copies {
			t.Fatalf("successful concurrent copies = %d, want %d", count, copies)
		}
	})
}

func TestCopyStartFailuresAreRedactedAndDoNotDecrypt(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	logPath := filepath.Join(t.TempDir(), "argv.jsonl")
	t.Setenv("FAKE_ARGV_LOG", logPath)

	if err := os.Remove(filepath.Join(binDir, "wl-copy")); err != nil {
		t.Fatal(err)
	}
	err := Copy(context.Background(), backend.NewGopass(), backend.SecretPassword, "synthetic/account")
	if err != ErrUnavailable || strings.Contains(err.Error(), "synthetic/account") {
		t.Fatalf("error = %v, want redacted clipboard unavailable", err)
	}
	if calls := readArgvLog(t, logPath); !reflect.DeepEqual(calls, [][]string{{"gopass", "ls", "--flat"}}) {
		t.Fatalf("calls = %#v, want no decrypt after owner start failure", calls)
	}
}

func TestCopyBackendStartFailureCancelsAndReapsStartedOwner(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	t.Setenv("FAKE_WLCOPY_MODE", "hang-before-read")
	logPath := filepath.Join(t.TempDir(), "argv.jsonl")
	ownerPIDPath := filepath.Join(t.TempDir(), "owner.pid")
	t.Setenv("FAKE_ARGV_LOG", logPath)
	t.Setenv("FAKE_WLCOPY_PID", ownerPIDPath)

	gopassPath := filepath.Join(binDir, "gopass")
	started := time.Now()
	var setupErr error
	err := copyWithBeforeSecretStart(
		context.Background(),
		backend.NewGopass(),
		backend.SecretPassword,
		"synthetic/account",
		func() {
			if !waitForFile(ownerPIDPath, 2*time.Second) {
				setupErr = errors.New("clipboard owner PID was not ready")
			}
			if removeErr := os.Remove(gopassPath); removeErr != nil {
				setupErr = removeErr
			}
		},
	)
	if setupErr != nil {
		t.Fatalf("partial-start setup: %v", setupErr)
	}
	if err != backend.ErrUnavailable || strings.Contains(err.Error(), "RAW_") || strings.Contains(err.Error(), "synthetic/account") {
		t.Fatalf("error = %v, want exact redacted backend unavailable", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("partial-start cleanup took %v, want <= 3s", elapsed)
	}

	calls := readArgvLog(t, logPath)
	wantCalls := [][]string{
		{"gopass", "ls", "--flat"},
		{"wl-copy", "--sensitive", "--foreground", "--trim-newline", "--type", "text/plain"},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want listing and owner only (no decrypt)", calls)
	}
	ownerPID := readPID(t, ownerPIDPath)
	if processExists(ownerPID) {
		t.Fatalf("started clipboard owner %d remains after backend start failure", ownerPID)
	}
}

func TestProductionSecretPipeHasNoReadOrBufferedCommandAPI(t *testing.T) {
	for _, path := range []string{"clipboard.go", filepath.Join("..", "backend", "secret.go")} {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "Read", "ReadAll", "Output", "CombinedOutput":
				t.Errorf("forbidden secret-stream API %s in %s", selector.Sel.Name, path)
			}
			return true
		})
	}
}

func buildFakeExecutables(t *testing.T) string {
	t.Helper()
	random := make([]byte, 18)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_NONCE", hex.EncodeToString(random))
	dir := t.TempDir()
	buildFake(t, dir, "gopass", fakeGopassSource)
	buildFake(t, dir, "wl-copy", fakeWLCopySource)
	return dir
}

func buildFake(t *testing.T, dir, name, source string) {
	t.Helper()
	sourcePath := filepath.Join(dir, name+".go")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "build", "-o", filepath.Join(dir, name), sourcePath)
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build fake %s: %v: %s", name, err, output)
	}
}

func readArgvLog(t *testing.T, path string) [][]string {
	t.Helper()
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var calls [][]string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var call []string
		if err := json.Unmarshal(scanner.Bytes(), &call); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, call)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return calls
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(content))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func waitForFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func waitProcessGone(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processExists(pid) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("process %d still exists", pid)
}

func processExists(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	fields := strings.Fields(string(stat))
	return len(fields) < 3 || fields[2] != "Z"
}

const fakeGopassSource = `package main
import ("crypto/sha256"; "encoding/hex"; "encoding/json"; "io"; "os"; "os/exec"; "os/signal"; "strings"; "syscall"; "time")
func main() {
 if os.Getenv("FAKE_DESCENDANT_ROLE")=="gopass" { signal.Ignore(syscall.SIGTERM); for { time.Sleep(time.Second) } }
 if os.Getenv("FAKE_AGENT_ROLE")=="agent" { runAgent() }
 if os.Getenv("FAKE_AGENT_ROLE")=="pinentry" { signal.Ignore(syscall.SIGTERM); for { time.Sleep(time.Second) } }
 if len(os.Args)==3 && os.Args[1]=="config" && os.Args[2]=="mounts.path" { root:=os.Getenv("FAKE_STORE_ROOT"); if root=="" { root=os.TempDir() }; io.WriteString(os.Stdout, root+"\n"); return }
 log(os.Args)
 args := os.Args[1:]
 mode := os.Getenv("FAKE_GOPASS_MODE")
 if len(args)==2 && args[0]=="ls" && args[1]=="--flat" {
  if mode=="list-fail" { os.Stderr.WriteString("RAW_BACKEND_MARKER"); os.Exit(17) }
  io.WriteString(os.Stdout, os.Getenv("FAKE_ENTRIES")); if mode=="delete-after-list" { os.Remove(os.Getenv("FAKE_REMOVE_EXECUTABLE")) }; return
 }
 if p:=os.Getenv("FAKE_GOPASS_PID"); p!="" { os.WriteFile(p, []byte(itoa(os.Getpid())), 0600) }
 if mode=="hang-show" { for { time.Sleep(time.Second) } }
 if mode=="hang-show-descendant" { spawnDescendant("gopass",os.Getenv("FAKE_GOPASS_DESCENDANT_PID")); signal.Ignore(syscall.SIGTERM); for { time.Sleep(time.Second) } }
 if mode=="hang-with-agent" { spawnAgent(); ch:=make(chan os.Signal,1); signal.Notify(ch,syscall.SIGTERM); for { select { case <-ch: os.WriteFile(os.Getenv("FAKE_AGENT_CANCEL"),[]byte("cancel"),0600); default: time.Sleep(time.Millisecond) } } }
 if mode=="show-fail-descendant" { spawnDescendant("gopass",os.Getenv("FAKE_GOPASS_DESCENDANT_PID")); for i:=0;i<200;i++ { _,ownerErr:=os.Stat(os.Getenv("FAKE_WLCOPY_PID")); _,descendantErr:=os.Stat(os.Getenv("FAKE_WLCOPY_DESCENDANT_PID")); if ownerErr==nil && descendantErr==nil { break }; time.Sleep(5*time.Millisecond) }; os.Stderr.WriteString("RAW_SECRET_MARKER"); os.Exit(18) }
 if mode=="show-fail" { for i:=0;i<100;i++ { if _,err:=os.Stat(os.Getenv("FAKE_WLCOPY_PID")); err==nil { break }; time.Sleep(5*time.Millisecond) }; os.Stderr.WriteString("RAW_SECRET_MARKER"); os.Exit(18) }
 valid := (len(args)==4 && strings.Join(args,"\x00")=="show\x00--password\x00--\x00synthetic/account") || (len(args)==4 && strings.Join(args,"\x00")=="show\x00--\x00synthetic/account\x00username")
 if !valid { os.Exit(19) }
 io.WriteString(os.Stdout, secret()); if p:=os.Getenv("FAKE_BACKEND_DONE"); p!="" { os.WriteFile(p,[]byte("done"),0600) }
}
func secret() string { sum:=sha256.Sum256([]byte(os.Getenv("FAKE_NONCE"))); return "SYNTHETIC_"+hex.EncodeToString(sum[:]) }
func log(args []string) { if p:=os.Getenv("FAKE_ARGV_LOG"); p!="" { f,_:=os.OpenFile(p,os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600); if f!=nil { json.NewEncoder(f).Encode(append([]string{"gopass"},args[1:]...)); f.Close() } } }
func spawnDescendant(role,path string) { c:=exec.Command(os.Args[0]); c.Env=append(os.Environ(),"FAKE_DESCENDANT_ROLE="+role); if c.Start()!=nil { os.Exit(40) }; if path!="" { os.WriteFile(path,[]byte(itoa(c.Process.Pid)),0600) } }
func spawnAgent() { c:=exec.Command(os.Args[0]); c.Env=append(os.Environ(),"FAKE_AGENT_ROLE=agent"); c.SysProcAttr=&syscall.SysProcAttr{Setpgid:true}; if c.Start()!=nil { os.Exit(41) }; os.WriteFile(os.Getenv("FAKE_AGENT_PID"),[]byte(itoa(c.Process.Pid)),0600) }
func runAgent() { c:=exec.Command(os.Args[0]); c.Env=cleanAgentEnv("FAKE_AGENT_ROLE=pinentry"); if c.Start()!=nil { os.Exit(42) }; os.WriteFile(os.Getenv("FAKE_PINENTRY_PID"),[]byte(itoa(c.Process.Pid)),0600); for { _,cancelErr:=os.Stat(os.Getenv("FAKE_AGENT_CANCEL")); gopassAlive:=true; if b,err:=os.ReadFile(os.Getenv("FAKE_GOPASS_PID")); err==nil { gopassAlive=syscall.Kill(atoi(string(b)),0)==nil }; if cancelErr==nil || !gopassAlive { _=c.Process.Kill(); _=c.Wait(); if p:=os.Getenv("FAKE_AGENT_HEALTH"); p!="" { os.WriteFile(p,[]byte("OK"),0600) }; for { time.Sleep(time.Second) } }; time.Sleep(time.Millisecond) } }
func cleanAgentEnv(value string) []string { env:=make([]string,0); for _,item:=range os.Environ() { if strings.HasPrefix(item,"FAKE_AGENT_ROLE=") { continue }; env=append(env,item) }; return append(env,value) }
func itoa(v int) string { const d="0123456789"; if v==0{return "0"}; b:=make([]byte,0,20); for v>0 { b=append(b,d[v%10]); v/=10 }; for i,j:=0,len(b)-1;i<j;i,j=i+1,j-1 {b[i],b[j]=b[j],b[i]}; return string(b) }
func atoi(s string) int { n:=0; for i:=0;i<len(s);i++ { if s[i]>='0'&&s[i]<='9' { n=n*10+int(s[i]-'0') } }; return n }
`

const fakeWLCopySource = `package main
import ("bytes"; "crypto/sha256"; "encoding/hex"; "encoding/json"; "io"; "os"; "os/exec"; "os/signal"; "syscall"; "time")
func main() {
 if os.Getenv("FAKE_DESCENDANT_ROLE")=="wl-copy" { signal.Ignore(syscall.SIGTERM); for { time.Sleep(time.Second) } }
 if os.Getenv("FAKE_DESCENDANT_ROLE")=="newer-owner" { signal.Ignore(syscall.SIGTERM); for { time.Sleep(time.Second) } }
 log(os.Args)
 if p:=os.Getenv("FAKE_WLCOPY_PID"); p!="" { os.WriteFile(p, []byte(itoa(os.Getpid())), 0600) }
 if os.Getenv("FAKE_WLCOPY_MODE")=="hang-before-read" { os.Stderr.WriteString("RAW_CLIPBOARD_START_MARKER"); for { time.Sleep(time.Second) } }
 if os.Getenv("FAKE_WLCOPY_MODE")=="hang-descendant" { spawnDescendant("wl-copy",os.Getenv("FAKE_WLCOPY_DESCENDANT_PID")); signal.Ignore(syscall.SIGTERM); for { time.Sleep(time.Second) } }
 if os.Getenv("FAKE_WLCOPY_MODE")=="early" { for i:=0;i<100;i++ { if _,err:=os.Stat(os.Getenv("FAKE_GOPASS_PID")); err==nil { break }; time.Sleep(5*time.Millisecond) }; os.Exit(0) }
 data,err:=io.ReadAll(os.Stdin); if err!=nil || !bytes.Equal(data,[]byte(secret())) { time.Sleep(100*time.Millisecond); os.Exit(23) }
 if p:=os.Getenv("FAKE_CONSUMED_FILE"); p!="" { os.WriteFile(p,[]byte("ok"),0600) }
 if os.Getenv("FAKE_WLCOPY_MODE")=="hang-stubborn-newer" { spawnNewerOwner(); waitForTermAndHang() }
 if os.Getenv("FAKE_WLCOPY_MODE")=="hang" { for { time.Sleep(time.Second) } }
 time.Sleep(100*time.Millisecond)
}
func secret() string { sum:=sha256.Sum256([]byte(os.Getenv("FAKE_NONCE"))); return "SYNTHETIC_"+hex.EncodeToString(sum[:]) }
func log(args []string) { if p:=os.Getenv("FAKE_ARGV_LOG"); p!="" { f,_:=os.OpenFile(p,os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600); if f!=nil { json.NewEncoder(f).Encode(append([]string{"wl-copy"},args[1:]...)); f.Close() } } }
func spawnDescendant(role,path string) { c:=exec.Command(os.Args[0]); c.Env=append(os.Environ(),"FAKE_DESCENDANT_ROLE="+role); if c.Start()!=nil { os.Exit(40) }; if path!="" { os.WriteFile(path,[]byte(itoa(c.Process.Pid)),0600) } }
func spawnNewerOwner() { c:=exec.Command(os.Args[0]); c.Env=append(os.Environ(),"FAKE_DESCENDANT_ROLE=newer-owner"); c.SysProcAttr=&syscall.SysProcAttr{Setpgid:true}; if c.Start()!=nil { os.Exit(41) }; if p:=os.Getenv("FAKE_NEWER_OWNER_PID"); p!="" { os.WriteFile(p,[]byte(itoa(c.Process.Pid)),0600) } }
func waitForTermAndHang() { ch:=make(chan os.Signal,1); signal.Notify(ch,syscall.SIGTERM); <-ch; if p:=os.Getenv("FAKE_WLCOPY_TERM"); p!="" { os.WriteFile(p,[]byte("term"),0600) }; for { time.Sleep(time.Second) } }
func itoa(v int) string { const d="0123456789"; if v==0{return "0"}; b:=make([]byte,0,20); for v>0 { b=append(b,d[v%10]); v/=10 }; for i,j:=0,len(b)-1;i<j;i,j=i+1,j-1 {b[i],b[j]=b[j],b[i]}; return string(b) }
`
