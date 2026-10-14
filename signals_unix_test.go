//go:build unix

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// A signal the emulator was started ignoring, as by nohup, stays ignored.
// Ignoring a signal can't be undone within a process, so this runs in a
// child process started with SIGHUP ignored.
func TestIgnoredSignalStaysIgnored(t *testing.T) {
	if os.Getenv("ODYSSEY9_IGNORED_SIGHUP") != "" {
		notifyStop()
		if !signal.Ignored(syscall.SIGHUP) {
			os.Exit(3)
		}
		syscall.Kill(syscall.Getpid(), syscall.SIGHUP)
		time.Sleep(100 * time.Millisecond)
		return // still running: ignored
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip(err)
	}
	cmd := exec.Command(sh, "-c", `trap "" HUP; exec "$0" -test.run='^TestIgnoredSignalStaysIgnored$'`, os.Args[0])
	cmd.Env = append(os.Environ(), "ODYSSEY9_IGNORED_SIGHUP=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("SIGHUP not ignored: %v\n%s", err, out)
	}
}
