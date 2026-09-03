package clipboard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/andrey-losikhin/zer0-waypass/internal/backend"
)

const (
	guardianControllerTestEnv = "ZER0_WAYPASS_TEST_GUARDIAN_CONTROLLER"
	guardianPIDFileEnv        = "ZER0_WAYPASS_TEST_GUARDIAN_PID_FILE"
	guardianPartialStartEnv   = "ZER0_WAYPASS_TEST_GUARDIAN_PARTIAL_START"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == GuardianModeArgument {
		if path := os.Getenv(guardianPIDFileEnv); path != "" {
			_ = os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600)
		}
		if os.Getenv(guardianPartialStartEnv) == "1" {
			request := os.NewFile(guardianRequestFD, "guardian-request")
			status := os.NewFile(guardianStatusFD, "guardian-status")
			if !validInheritedPipe(request) || !validInheritedPipe(status) {
				os.Exit(92)
			}
			defer request.Close()
			defer status.Close()
			os.Exit(runGuardian(request, status, &partialStartSource{delegate: backend.NewGopass()}))
		}
		os.Exit(RunGuardianMode())
	}
	if os.Getenv(guardianControllerTestEnv) == "1" {
		policy := Policy{
			AcquisitionDeadline: MinimumAcquisitionDeadline,
			OwnershipBudget:     MinimumOwnershipBudget,
			KillGrace:           MinimumKillGrace,
		}
		if err := CopyGuarded(context.Background(), backend.SecretPassword, "synthetic/account", policy); err != nil {
			os.Exit(91)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestGuardianHandshakeBlocksWorkersUntilStartAndRegistersBothGroups(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	t.Setenv("FAKE_GOPASS_MODE", "hang-show-descendant")
	t.Setenv("FAKE_WLCOPY_MODE", "hang-descendant")
	paths := guardianProcessPaths(t)
	setGuardianProcessPaths(t, paths)
	logPath := filepath.Join(t.TempDir(), "argv.jsonl")
	t.Setenv("FAKE_ARGV_LOG", logPath)

	command, request, status, waitDone := startGuardianForTest(t)
	reader := bufio.NewReader(status)
	ready, err := readGuardianStatus(context.Background(), reader)
	if err != nil || ready != (guardianStatus{Protocol: guardianProtocol, Type: "READY"}) {
		t.Fatalf("READY = %#v, %v", ready, err)
	}
	time.Sleep(100 * time.Millisecond)
	if calls := readArgvLog(t, logPath); len(calls) != 0 {
		t.Fatalf("workers started before START: %#v", calls)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("worker marker exists before START: %s", path)
		}
	}

	if err := json.NewEncoder(request).Encode(testGuardianStart()); err != nil {
		t.Fatal(err)
	}
	if err := request.Close(); err != nil {
		t.Fatal(err)
	}
	registered, err := readGuardianStatus(context.Background(), reader)
	if err != nil || registered.Protocol != guardianProtocol || registered.Type != "REGISTERED" ||
		registered.OwnerPID <= 0 || registered.OwnerPID != registered.OwnerPGID ||
		registered.BackendPID <= 0 || registered.BackendPID != registered.BackendPGID ||
		registered.OwnerPGID == registered.BackendPGID {
		t.Fatalf("REGISTERED = %#v, %v", registered, err)
	}
	for index, path := range paths {
		if !waitForFile(path, 2*time.Second) {
			t.Fatalf("REGISTERED preceded worker PID/PGID storage: %s", path)
		}
		pid := readPID(t, path)
		if index >= 2 {
			continue
		}
		registeredPID := registered.OwnerPID
		if index == 0 {
			registeredPID = registered.BackendPID
		}
		if pid != registeredPID {
			t.Fatalf("REGISTERED PID %d does not match worker marker %d", registeredPID, pid)
		}
		if pgid, err := syscall.Getpgid(pid); err != nil || pgid != pid {
			t.Fatalf("worker %d PGID = %d, %v", pid, pgid, err)
		}
	}

	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done, err := readGuardianStatus(context.Background(), reader)
	if err != nil || done.Type != "DONE" || done.Code != "clipboard_canceled" {
		t.Fatalf("DONE = %#v, %v", done, err)
	}
	if err := <-waitDone; err != nil {
		t.Fatalf("guardian wait: %v", err)
	}
	assertGuardianProcessesGone(t, paths)
}

func TestCopyGuardedNormalCompletion(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	policy := testGuardianPolicy()
	if err := copyGuardedWithExecutable(context.Background(), os.Args[0], backend.SecretPassword, "synthetic/account", policy); err != nil {
		t.Fatalf("CopyGuarded: %v", err)
	}
}

func TestCopyGuardedPartialStartCleanup(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	t.Setenv(guardianPartialStartEnv, "1")
	t.Setenv("FAKE_WLCOPY_MODE", "hang-before-read")
	ownerPath := filepath.Join(t.TempDir(), "owner.pid")
	t.Setenv("FAKE_WLCOPY_PID", ownerPath)

	err := copyGuardedWithExecutable(context.Background(), os.Args[0], backend.SecretPassword, "synthetic/account", testGuardianPolicy())
	if err != backend.ErrUnavailable {
		t.Fatalf("error = %v, want backend unavailable", err)
	}
	if !waitForFile(ownerPath, 2*time.Second) {
		t.Fatal("partial-start owner did not start")
	}
	ownerPID := readPID(t, ownerPath)
	if processExists(ownerPID) {
		t.Fatalf("partial-start owner %d remains", ownerPID)
	}
}

func TestCopyGuardedHandledCancellation(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	t.Setenv("FAKE_GOPASS_MODE", "hang-show-descendant")
	t.Setenv("FAKE_WLCOPY_MODE", "hang-descendant")
	paths := guardianProcessPaths(t)
	setGuardianProcessPaths(t, paths)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- copyGuardedWithExecutable(ctx, os.Args[0], backend.SecretPassword, "synthetic/account", testGuardianPolicy())
	}()
	for _, path := range paths {
		if !waitForFile(path, 2*time.Second) {
			t.Fatalf("worker marker not ready: %s", path)
		}
	}
	cancel()
	if err := <-done; err != ErrCanceled {
		t.Fatalf("error = %v, want canceled", err)
	}
	assertGuardianProcessesGone(t, paths)
}

func TestGuardianMissingInheritedFDsFailsClosed(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	logPath := filepath.Join(t.TempDir(), "argv.jsonl")
	t.Setenv("FAKE_ARGV_LOG", logPath)
	command := exec.Command(os.Args[0], GuardianModeArgument)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err == nil {
		t.Fatal("guardian without inherited FDs succeeded")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 || len(readArgvLog(t, logPath)) != 0 {
		t.Fatalf("invalid-FD guardian leaked output or started workers: %q/%q", stdout.String(), stderr.String())
	}
}

func TestGuardianSubstitutedRegularFDsFailClosed(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	logPath := filepath.Join(t.TempDir(), "argv.jsonl")
	t.Setenv("FAKE_ARGV_LOG", logPath)
	request, err := os.CreateTemp(t.TempDir(), "request")
	if err != nil {
		t.Fatal(err)
	}
	defer request.Close()
	status, err := os.CreateTemp(t.TempDir(), "status")
	if err != nil {
		t.Fatal(err)
	}
	defer status.Close()
	command := exec.Command(os.Args[0], GuardianModeArgument)
	command.ExtraFiles = []*os.File{request, status}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err == nil {
		t.Fatal("guardian with regular inherited FDs succeeded")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 || len(readArgvLog(t, logPath)) != 0 {
		t.Fatalf("substituted-FD guardian leaked output or started workers: %q/%q", stdout.String(), stderr.String())
	}
}

func TestCopyGuardedCanceledAtReadyDoesNotSendStart(t *testing.T) {
	directory := t.TempDir()
	buildFake(t, directory, "fake-guardian", fakeGuardianReadySource)
	executable := filepath.Join(directory, "fake-guardian")
	for repeat := 0; repeat < 10; repeat++ {
		readyPath := filepath.Join(t.TempDir(), "ready")
		releasePath := filepath.Join(t.TempDir(), "release")
		startPath := filepath.Join(t.TempDir(), "start")
		t.Setenv("FAKE_GUARDIAN_READY", readyPath)
		t.Setenv("FAKE_GUARDIAN_RELEASE", releasePath)
		t.Setenv("FAKE_GUARDIAN_START", startPath)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			if waitForFile(readyPath, 2*time.Second) {
				cancel()
				_ = os.WriteFile(releasePath, []byte("release"), 0o600)
			}
		}()
		err := copyGuardedWithExecutable(ctx, executable, backend.SecretPassword, "synthetic/account", testGuardianPolicy())
		cancel()
		if err != ErrCanceled {
			t.Fatalf("repeat %d: error = %v, want canceled", repeat, err)
		}
		if _, err := os.Stat(startPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("repeat %d: canceled controller sent START", repeat)
		}
	}
}

func TestControllerSIGKILLTriggersGuardianCleanupRepeated(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	t.Setenv("FAKE_GOPASS_MODE", "hang-show-descendant")
	t.Setenv("FAKE_WLCOPY_MODE", "hang-descendant")
	t.Setenv(guardianControllerTestEnv, "1")

	for repeat := 0; repeat < 3; repeat++ {
		t.Run(strconv.Itoa(repeat), func(t *testing.T) {
			paths := guardianProcessPaths(t)
			setGuardianProcessPaths(t, paths)
			guardianPIDPath := filepath.Join(t.TempDir(), "guardian.pid")
			argvLogPath := filepath.Join(t.TempDir(), "argv.jsonl")
			t.Setenv(guardianPIDFileEnv, guardianPIDPath)
			t.Setenv("FAKE_ARGV_LOG", argvLogPath)
			var stdout, stderr bytes.Buffer
			controller := exec.Command(os.Args[0], "--guardian-controller-test")
			controller.Stdout = &stdout
			controller.Stderr = &stderr
			if err := controller.Start(); err != nil {
				t.Fatal(err)
			}
			for _, path := range append([]string{guardianPIDPath}, paths...) {
				if !waitForFile(path, 2*time.Second) {
					t.Fatalf("process marker not ready: %s", path)
				}
			}
			guardianPID := readPID(t, guardianPIDPath)
			argv, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(guardianPID), "cmdline"))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(bytes.Split(bytes.TrimSuffix(argv, []byte{0}), []byte{0}), [][]byte{[]byte(os.Args[0]), []byte(GuardianModeArgument)}) ||
				bytes.Contains(argv, []byte("synthetic/account")) || bytes.Contains(argv, []byte("SYNTHETIC_")) {
				t.Fatalf("guardian argv is not closed: %q", argv)
			}

			unrelated := exec.Command("/bin/sleep", "30")
			unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := unrelated.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = syscall.Kill(-unrelated.Process.Pid, syscall.SIGKILL)
				_ = unrelated.Wait()
			}()

			started := time.Now()
			if err := controller.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = controller.Wait()
			waitProcessGone(t, guardianPID, 2*time.Second)
			assertGuardianProcessesGone(t, paths)
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Fatalf("parent-death cleanup took %v", elapsed)
			}
			if !processExists(unrelated.Process.Pid) {
				t.Fatal("unrelated process was signaled by guardian cleanup")
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("controller/guardian output leak: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			calls := readArgvLog(t, argvLogPath)
			if len(calls) != 3 || strings.Contains(string(mustJSON(t, calls)), "SYNTHETIC_") {
				t.Fatalf("unexpected crash-path argv log: %#v", calls)
			}
		})
	}
}

func TestGuardianRepeatedAndConcurrentLifecycleGate(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	t.Setenv("FAKE_GOPASS_MODE", "")
	t.Setenv("FAKE_WLCOPY_MODE", "")
	policy := testGuardianPolicy()

	for repeat := 0; repeat < 5; repeat++ {
		if err := copyGuardedWithExecutable(context.Background(), os.Args[0], backend.SecretPassword, "synthetic/account", policy); err != nil {
			t.Fatalf("normal repeat %d: %v", repeat, err)
		}
	}

	for repeat := 0; repeat < 5; repeat++ {
		paths := guardianProcessPaths(t)
		setGuardianProcessPaths(t, paths)
		t.Setenv("FAKE_GOPASS_MODE", "show-fail-descendant")
		t.Setenv("FAKE_WLCOPY_MODE", "hang-descendant")
		if err := copyGuardedWithExecutable(context.Background(), os.Args[0], backend.SecretPassword, "synthetic/account", policy); err != ErrFailed {
			t.Fatalf("post-start error repeat %d = %v, want redacted failure", repeat, err)
		}
		assertGuardianProcessesGone(t, paths)
	}

	t.Setenv("FAKE_GOPASS_MODE", "")
	const concurrent = 8
	results := make(chan error, concurrent)
	for index := 0; index < concurrent; index++ {
		go func() {
			results <- copyGuardedWithExecutable(context.Background(), os.Args[0], backend.SecretPassword, "synthetic/account", policy)
		}()
	}
	for index := 0; index < concurrent; index++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent operation %d: %v", index, err)
		}
	}

	for repeat := 0; repeat < 3; repeat++ {
		paths := guardianProcessPaths(t)
		setGuardianProcessPaths(t, paths)
		t.Setenv("FAKE_GOPASS_MODE", "hang-show-descendant")
		t.Setenv("FAKE_WLCOPY_MODE", "hang-descendant")
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		started := time.Now()
		err := copyGuardedWithExecutable(ctx, os.Args[0], backend.SecretPassword, "synthetic/account", policy)
		cancel()
		if err != ErrTimeout {
			t.Fatalf("deadline repeat %d = %v, want timeout", repeat, err)
		}
		if elapsed := time.Since(started); elapsed > 2*time.Second {
			t.Fatalf("deadline repeat %d returned after %v, want bounded cleanup", repeat, elapsed)
		}
		assertGuardianProcessesGone(t, paths)
	}
	assertNoSyntheticProcesses(t)
}

func TestPinentryCancellationLeavesIsolatedAgentAlive(t *testing.T) {
	binDir := buildFakeExecutables(t)
	t.Setenv("PATH", binDir)
	t.Setenv("FAKE_ENTRIES", "synthetic/account\n")
	t.Setenv("FAKE_GOPASS_MODE", "hang-with-agent")
	t.Setenv("FAKE_WLCOPY_MODE", "hang-before-read")
	t.Setenv("FAKE_GOPASS_PID", filepath.Join(t.TempDir(), "gopass.pid"))
	agentPIDPath := filepath.Join(t.TempDir(), "agent.pid")
	pinentryPIDPath := filepath.Join(t.TempDir(), "pinentry.pid")
	cancelPath := filepath.Join(t.TempDir(), "cancel")
	healthPath := filepath.Join(t.TempDir(), "agent-health")
	t.Setenv("FAKE_AGENT_PID", agentPIDPath)
	t.Setenv("FAKE_PINENTRY_PID", pinentryPIDPath)
	t.Setenv("FAKE_AGENT_CANCEL", cancelPath)
	t.Setenv("FAKE_AGENT_HEALTH", healthPath)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	err := copyGuardedWithExecutable(ctx, os.Args[0], backend.SecretPassword, "synthetic/account", testGuardianPolicy())
	cancel()
	if err != ErrTimeout {
		t.Fatalf("error = %v, want acquisition timeout", err)
	}
	if !waitForFile(agentPIDPath, 2*time.Second) || !waitForFile(pinentryPIDPath, 2*time.Second) {
		t.Fatal("isolated agent/pinentry markers were not created")
	}
	agentPID := readPID(t, agentPIDPath)
	pinentryPID := readPID(t, pinentryPIDPath)
	waitProcessGone(t, pinentryPID, 2*time.Second)
	if !waitForFile(healthPath, 2*time.Second) {
		t.Fatal("isolated agent did not complete post-cancel health roundtrip")
	}
	if !processExists(agentPID) {
		t.Fatalf("isolated gpg-agent substitute %d was terminated by operation cleanup", agentPID)
	}
	// The context may SIGKILL gopass before its signal handler runs; the
	// isolated agent therefore also observes gopass disappearance and cancels
	// pinentry. Either path proves pinentry is not left behind.
	_ = syscall.Kill(-agentPID, syscall.SIGKILL)
	waitProcessGone(t, agentPID, 2*time.Second)
}

type partialStartSource struct {
	delegate *backend.Gopass
}

func (s *partialStartSource) PrepareSecret(ctx context.Context, action backend.SecretAction, entryPath string) (backend.SecretRequest, error) {
	return s.delegate.PrepareSecret(ctx, action, entryPath)
}

func (s *partialStartSource) StartSecret(context.Context, backend.SecretRequest, *os.File) (*backend.SecretProcess, error) {
	if path := os.Getenv("FAKE_WLCOPY_PID"); path == "" || !waitForFile(path, 2*time.Second) {
		return nil, backend.ErrBackend
	}
	return nil, backend.ErrUnavailable
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func startGuardianForTest(t *testing.T) (*exec.Cmd, *os.File, *os.File, <-chan error) {
	t.Helper()
	requestReader, requestWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], GuardianModeArgument)
	command.ExtraFiles = []*os.File{requestReader, statusWriter}
	command.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	requestReader.Close()
	statusWriter.Close()
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	return command, requestWriter, statusReader, waitDone
}

func testGuardianStart() guardianStart {
	policy := testGuardianPolicy()
	return guardianStart{
		Protocol:               guardianProtocol,
		Type:                   "START",
		Action:                 "password",
		EntryPath:              "synthetic/account",
		AcquisitionNanoseconds: int64(policy.AcquisitionDeadline),
		OwnershipNanoseconds:   int64(policy.OwnershipBudget),
		KillGraceNanoseconds:   int64(policy.KillGrace),
	}
}

func testGuardianPolicy() Policy {
	return Policy{
		AcquisitionDeadline: MinimumAcquisitionDeadline,
		OwnershipBudget:     MinimumOwnershipBudget,
		KillGrace:           MinimumKillGrace,
	}
}

func guardianProcessPaths(t *testing.T) []string {
	t.Helper()
	directory := t.TempDir()
	return []string{
		filepath.Join(directory, "gopass.pid"),
		filepath.Join(directory, "owner.pid"),
		filepath.Join(directory, "gopass-descendant.pid"),
		filepath.Join(directory, "owner-descendant.pid"),
	}
}

func setGuardianProcessPaths(t *testing.T, paths []string) {
	t.Helper()
	t.Setenv("FAKE_GOPASS_PID", paths[0])
	t.Setenv("FAKE_WLCOPY_PID", paths[1])
	t.Setenv("FAKE_GOPASS_DESCENDANT_PID", paths[2])
	t.Setenv("FAKE_WLCOPY_DESCENDANT_PID", paths[3])
}

func assertGuardianProcessesGone(t *testing.T, paths []string) {
	t.Helper()
	for _, path := range paths {
		if !waitForFile(path, 2*time.Second) {
			t.Fatalf("process marker not created: %s", path)
		}
		pid := readPID(t, path)
		waitProcessGone(t, pid, 2*time.Second)
	}
}

func assertNoSyntheticProcesses(t *testing.T) {
	t.Helper()
	nonce := os.Getenv("FAKE_NONCE")
	if nonce == "" {
		t.Fatal("missing synthetic test nonce")
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !isDecimal(entry.Name()) {
			continue
		}
		environ, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "environ"))
		if err != nil {
			continue
		}
		if bytes.Contains(environ, []byte("FAKE_DESCENDANT_ROLE=")) &&
			bytes.Contains(environ, []byte("FAKE_NONCE="+nonce)) {
			t.Fatalf("synthetic descendant process %s remains", entry.Name())
		}
	}
}

func isDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

const fakeGuardianReadySource = `package main
import ("bufio"; "encoding/json"; "os"; "time")
func main() {
 request:=os.NewFile(3,"request"); status:=os.NewFile(4,"status")
 os.WriteFile(os.Getenv("FAKE_GUARDIAN_READY"),[]byte("ready"),0600)
 for { if _,err:=os.Stat(os.Getenv("FAKE_GUARDIAN_RELEASE")); err==nil { break }; time.Sleep(time.Millisecond) }
 json.NewEncoder(status).Encode(map[string]any{"protocol":1,"type":"READY"})
 line,_:=bufio.NewReader(request).ReadBytes('\n')
 if len(line)>0 { os.WriteFile(os.Getenv("FAKE_GUARDIAN_START"),[]byte("start"),0600) }
}
`
