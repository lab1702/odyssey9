//go:build unix

package main

import (
	"os/signal"
	"syscall"
	"testing"
)

// A signal the emulator was started ignoring, as by nohup, stays ignored.
func TestIgnoredSignalStaysIgnored(t *testing.T) {
	signal.Ignore(syscall.SIGHUP)
	defer signal.Reset(stopSignals...)
	notifyStop()
	if !signal.Ignored(syscall.SIGHUP) {
		t.Error("SIGHUP no longer ignored")
	}
	if signal.Ignored(syscall.SIGTERM) {
		t.Error("SIGTERM ignored")
	}
}
