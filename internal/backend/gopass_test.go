package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type runnerCall struct {
	name string
	args []string
}

type fakeRunner struct {
	output []byte
	err    error
	calls  []runnerCall
}

func (r *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	return r.output, r.err
}

func TestGopassListUsesExactMetadataArgvAndPreservesOrder(t *testing.T) {
	runner := &fakeRunner{output: []byte("z-last\nexample.test/work\n\nplain\r\n")}
	gopass := &Gopass{runner: runner}

	got, err := gopass.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	wantCalls := []runnerCall{{name: "gopass", args: []string{"ls", "--flat"}}}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
	wantEntries := []string{"z-last", "example.test/work", "plain"}
	if !reflect.DeepEqual(got, wantEntries) {
		t.Fatalf("entries = %#v, want %#v", got, wantEntries)
	}
}

func TestGopassListReturnsBackendErrorWithoutOutput(t *testing.T) {
	wantErr := errors.New("synthetic backend marker")
	runner := &fakeRunner{output: []byte("must-not-be-used"), err: wantErr}
	gopass := &Gopass{runner: runner}

	got, err := gopass.List(context.Background())
	if err != ErrBackend || errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want exact safe backend sentinel", err)
	}
	if got != nil {
		t.Fatalf("entries = %#v, want nil", got)
	}
}

func TestExecRunnerCancellationKillsDirectChild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pids")
	t.Setenv("GO_WANT_ZER0_WAYPASS_HELPER_PROCESS", "1")
	t.Setenv("ZER0_WAYPASS_TEST_PID_FILE", pidFile)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := (execRunner{}).Output(ctx, os.Args[0], "-test.run=TestBackendHelperProcess", "--", "hang")
		done <- err
	}()

	pid := waitForPID(t, pidFile, 2*time.Second)
	cancelledAt := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != ErrCanceled {
			t.Fatalf("cancelled runner error = %v, want exact canceled sentinel", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled runner did not return within 2s")
	}
	if elapsed := time.Since(cancelledAt); elapsed > 2*time.Second {
		t.Fatalf("cancellation took %v, want <= 2s", elapsed)
	}

	waitProcessGone(t, pid, 2*time.Second)
}

func TestExecRunnerDeadlineIsTimeout(t *testing.T) {
	t.Setenv("GO_WANT_ZER0_WAYPASS_HELPER_PROCESS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := (execRunner{}).Output(ctx, os.Args[0], "-test.run=TestBackendHelperProcess", "--", "sleep")
	if err != ErrTimeout {
		t.Fatalf("deadline error = %v, want exact timeout sentinel", err)
	}
}

func TestExecRunnerCommandFailureIsRedactedBackendError(t *testing.T) {
	t.Setenv("GO_WANT_ZER0_WAYPASS_HELPER_PROCESS", "1")
	_, err := (execRunner{}).Output(context.Background(), os.Args[0], "-test.run=TestBackendHelperProcess", "--", "fail")
	if err != ErrBackend || strings.Contains(err.Error(), "SYNTHETIC_SECRET_STDERR_MARKER") {
		t.Fatalf("command failure = %v, want exact redacted backend sentinel", err)
	}
}

func TestExecRunnerRejectsOversizedOutputBounded(t *testing.T) {
	t.Setenv("GO_WANT_ZER0_WAYPASS_HELPER_PROCESS", "1")
	started := time.Now()
	output, err := (execRunner{}).Output(context.Background(), os.Args[0], "-test.run=TestBackendHelperProcess", "--", "oversize")
	if !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("error = %v, want metadata output limit error", err)
	}
	if output != nil {
		t.Fatalf("output length = %d, want nil", len(output))
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("oversized producer returned in %v, want <= 3s", elapsed)
	}
}

func TestGopassListMapsSafeSentinelsWithoutRetainingRawCause(t *testing.T) {
	marker := errors.New("SYNTHETIC_RAW_CAUSE_MARKER")
	cases := []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		err  error
		want error
	}{
		{
			name: "deadline",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				return ctx, cancel
			},
			err:  marker,
			want: ErrTimeout,
		},
		{
			name: "canceled",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			},
			err:  marker,
			want: ErrCanceled,
		},
		{name: "not found", ctx: backgroundContext, err: exec.ErrNotFound, want: ErrUnavailable},
		{name: "other", ctx: backgroundContext, err: marker, want: ErrBackend},
		{name: "safe unavailable remains exact", ctx: backgroundContext, err: ErrUnavailable, want: ErrUnavailable},
		{name: "safe too large remains exact", ctx: backgroundContext, err: ErrOutputTooLarge, want: ErrOutputTooLarge},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.ctx()
			defer cancel()
			_, err := (&Gopass{runner: &fakeRunner{err: test.err}}).List(ctx)
			if err != test.want || !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want exact %v", err, test.want)
			}
			if errors.Is(err, marker) || strings.Contains(err.Error(), marker.Error()) {
				t.Fatal("safe error retained raw cause")
			}
		})
	}
}

func TestExecRunnerMissingExecutableIsUnavailable(t *testing.T) {
	_, err := (execRunner{}).Output(context.Background(), "zer0-waypass-definitely-missing-executable")
	if err != ErrUnavailable {
		t.Fatalf("error = %v, want exact unavailable sentinel", err)
	}
}

func backgroundContext() (context.Context, context.CancelFunc) {
	return context.Background(), func() {}
}

func TestBackendHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_ZER0_WAYPASS_HELPER_PROCESS") != "1" {
		return
	}
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(90)
	}

	switch os.Args[separator+1] {
	case "sleep":
		time.Sleep(time.Second)
	case "fail":
		_, _ = os.Stderr.WriteString("SYNTHETIC_SECRET_STDERR_MARKER")
		os.Exit(17)
	case "hang":
		content := strconv.Itoa(os.Getpid())
		if err := os.WriteFile(os.Getenv("ZER0_WAYPASS_TEST_PID_FILE"), []byte(content), 0o600); err != nil {
			os.Exit(92)
		}
		for {
			time.Sleep(time.Second)
		}
	case "oversize":
		_, _ = os.Stderr.WriteString("SYNTHETIC_SECRET_STDERR_MARKER")
		chunk := bytesOf('x', 64<<10)
		for written := 0; written <= maxMetadataOutputBytes; written += len(chunk) {
			if _, err := os.Stdout.Write(chunk); err != nil {
				os.Exit(0)
			}
		}
		for {
			time.Sleep(time.Second)
		}
	default:
		os.Exit(93)
	}
}

func waitForPID(t *testing.T, path string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(string(content))
			if parseErr == nil {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("helper PID file was not ready within %v", timeout)
	return 0
}

func waitProcessGone(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processRunning(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d still running after %v", pid, timeout)
}

func processRunning(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return !errors.Is(err, syscall.ESRCH)
	}
	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	closing := strings.LastIndexByte(string(status), ')')
	return closing < 0 || closing+2 >= len(status) || status[closing+2] != 'Z'
}

func bytesOf(value byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return result
}
