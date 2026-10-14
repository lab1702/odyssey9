//go:build !unix

package main

import (
	"os"
	"syscall"
)

// stopSignals are the signals that ask the emulator to stop (see
// notifyStop). On Windows, closing the console window, logging off and
// shutting down arrive as SIGTERM.
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
