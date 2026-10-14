//go:build unix

package main

import (
	"os"
	"syscall"
)

// stopSignals are the signals that ask the emulator to stop (see
// notifyStop): Ctrl+C, the system's request, and the terminal closing.
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
