//go:build unix

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aymenkrifa/grove/internal/testutil"
)

// TestMain lets this test binary stand in for the grove executable: re-run
// with GROVE_TEST_SUBPROCESS set it calls Execute() and exits, instead of
// running any test. That is how the test below gets a real grove process to
// send a real signal to, without a `go build` on every run of this package.
//
// Execute() reads os.Args[1:], so the subprocess is driven the way a user
// drives grove. It returns before testing parses its own flags, so the -test.*
// arguments go test would otherwise insist on never come into it.
func TestMain(m *testing.M) {
	if os.Getenv("GROVE_TEST_SUBPROCESS") != "" {
		os.Exit(Execute())
	}
	os.Exit(m.Run())
}

// TestSignalStopsTheChildrenGroveStarted is the whole point of wiring
// signal.NotifyContext into ExecuteWith. Every command reaches its context
// through cmd.Context(); with no context supplied that is context.Background(),
// which is never cancelled, so exec.CommandContext has nothing to cancel on.
//
// The fixture is deliberately the case an interactive shell hides. grove is
// started in a process group of its own and the signal is sent to grove's pid
// alone — which is what a script's `kill`, or a Ctrl-C while grove is not the
// foreground group leader, actually does. Send it to the whole group instead
// and the children die of their own accord whatever grove does, which is why
// this went unnoticed: it looks fine every time you try it by hand.
//
// The assertion is about the child, not about grove. grove exits promptly
// either way — without the fix because SIGTERM's default action kills it on
// the spot — and it is the `sleep` it started that tells the two apart: reaped
// on the way out, or left running with nobody waiting on it.
func TestSignalStopsTheChildrenGroveStarted(t *testing.T) {
	root := t.TempDir()
	testutil.NewRepo(t, filepath.Join(root, "api", "gateway"), testutil.WithCommit())

	work := t.TempDir()
	pidFile := filepath.Join(work, "child.pid")
	// The child records its own pid and then waits far longer than this test.
	// `exec` so that sleep replaces the shell rather than running under it:
	// one process, and it is the one grove started, which is the one
	// exec.CommandContext can cancel.
	script := "echo $$ > " + pidFile + "; exec sleep 120"

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test binary: %v", err)
	}
	grove := exec.Command(self, "exec", "--root", root, "--", "sh", "-c", script)
	grove.Env = append(os.Environ(),
		"GROVE_TEST_SUBPROCESS=1",
		"XDG_CONFIG_HOME="+filepath.Join(work, "cfg"),
		"GROVE_ROOT=",
		"NO_COLOR=1",
	)
	// Its own process group, so the SIGTERM below reaches grove and nothing
	// else. This is the fixture's entire point; without it the kernel would
	// deliver the signal to the children too and the test would pass against
	// an unfixed grove.
	grove.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	grove.Stdin, grove.Stdout, grove.Stderr = devnull, devnull, devnull

	if err := grove.Start(); err != nil {
		t.Fatalf("starting grove: %v", err)
	}
	// Whatever happens below, take the whole group down rather than leaving a
	// two-minute sleep behind for the next test to trip over.
	t.Cleanup(func() { _ = syscall.Kill(-grove.Process.Pid, syscall.SIGKILL) })

	child := waitForChildPid(t, pidFile)
	if err := syscall.Kill(child, 0); err != nil {
		t.Fatalf("child %d is not running before the signal (%v); the fixture proves nothing", child, err)
	}

	if err := syscall.Kill(grove.Process.Pid, syscall.SIGTERM); err != nil {
		t.Fatalf("signalling grove: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- grove.Wait() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("grove did not exit within 30s of SIGTERM")
	}

	// Polled rather than checked once: grove killing the child and grove
	// exiting are not ordered with respect to each other.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := syscall.Kill(child, 0); err == syscall.ESRCH {
			return // gone, which is the whole assertion
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child grove started (pid %d) was still running 10s after grove exited — "+
				"a signal that reaches grove alone must not leave its git processes orphaned", child)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitForChildPid reads the pid the child wrote, waiting for it to appear.
// Anything unreadable is retried rather than failed on: the file is written by
// a shell redirection, so it exists for an instant before it has contents.
func waitForChildPid(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if body, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(body))); err == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child never recorded its pid in %s; grove may not have reached it", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
