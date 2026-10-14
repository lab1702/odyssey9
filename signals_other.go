//go:build !unix

package main

import (
	"os"
	"syscall"
)

// stopSignals are the signals that ask the emulator to stop (see
// notifyStop). On Windows, closing the console window arrives as SIGTERM.
// Logging off and shutting down don't: Windows doesn't tell a console
// program that has windows about them.
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// raise does nothing: only Unix tells how a program stopped by its exit.
func raise(os.Signal) {}
