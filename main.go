// Command odyssey9 emulates the Magnavox Odyssey2 with the "Computer Intro!"
// cartridge (Philips Videopac 9) inserted: a TV screen showing the
// cartridge's virtual computer and the console's 49-key keyboard.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"odyssey9/machine"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "." // reported below, unless -file is given
	}
	file := flag.String("file", filepath.Join(home, "odyssey9-program.txt"), "program file used by SAVE and LOAD")
	load := flag.String("load", "", "program listing to load at start-up")
	mute := flag.Bool("mute", false, "disable sound")
	monitor := flag.Bool("monitor", false, "start with the monitor panel open")
	script := flag.String("script", "", "testing aid: keys to type at start-up, e.g. \"{RESET}PMI6B{ENTER}\"")
	shot := flag.String("screenshot", "", "testing aid: save a PNG after the script runs, then quit")
	flag.Parse()
	if flag.NArg() > 0 {
		// Flags after it would be ignored too, so don't start without them.
		fmt.Fprintf(os.Stderr, "Unexpected argument %q (use -load to open a program)\n", flag.Arg(0))
		flag.Usage()
		os.Exit(2)
	}
	if err != nil && !isFlagSet("file") {
		fmt.Fprintf(os.Stderr, "No home directory (%v): SAVE and LOAD use %s\n", err, *file)
	}
	keys, err := parseScript(*script)
	if err != nil {
		log.Fatal(err)
	}

	m := machine.New()
	if *load != "" {
		data, err := readListing(openListing(*load))
		if err != nil {
			log.Fatalf("%s: %v", *load, err)
		}
		p, err := machine.ParseProgram(bytes.NewReader(data))
		if err != nil {
			log.Fatalf("%s: %v", *load, err)
		}
		m.LoadProgram(p)
	}

	// Screenshot runs are for testing and often happen on machines without
	// a sound card, so they are silent.
	sound := !*mute && *shot == ""
	g := newGame(m, *file, sound)
	g.monitor = *monitor
	g.script, g.shotPath, g.shotWait = keys, *shot, shotDelay
	g.signals = notifyStop()
	ebiten.SetWindowTitle("Odyssey² Computer Intro! (Videopac 9)")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowClosingHandled(true) // so closing can ask before losing work
	w, h := g.windowSize()
	ebiten.SetWindowSize(w, h)
	ebiten.SetTPS(60)
	if err := ebiten.RunGame(g); err != nil && err != errQuit {
		// The error stops the emulator without asking, so keep the program,
		// unless this is a screenshot run: its program came from -load and
		// -script and can be made again. Keep it before saying anything, as
		// writing to a pipe whose reader has gone, such as a "| tee" that
		// Ctrl+C stopped too, stops the emulator.
		var kept string
		var keepErr error
		if g.unsaved() && *shot == "" {
			kept, keepErr = g.saveRecovery()
		}
		fmt.Fprintln(os.Stderr, err)
		// A missing or broken sound device is only reported once the game
		// is running, as an error from the audio library.
		if msg := err.Error(); sound && (strings.HasPrefix(msg, "oto:") || strings.HasPrefix(msg, "audio:")) {
			fmt.Fprintln(os.Stderr, "The sound output failed. Run with -mute to start without sound.")
		}
		if keepErr != nil {
			fmt.Fprintln(os.Stderr, "Your unsaved program could not be kept:", keepErr)
		} else if kept != "" {
			fmt.Fprintln(os.Stderr, "Your unsaved program was saved in", kept)
		}
		// Stopped by a signal, stop as the signal would have, so that a
		// script running the emulator stops too after Ctrl+C.
		var s stopSignal
		if errors.As(err, &s) {
			raise(s.Signal)
		}
		os.Exit(1)
	}
}

// stopSignal is the error Update returns when a signal stops the emulator.
type stopSignal struct{ os.Signal }

func (s stopSignal) Error() string { return "stopped by signal: " + s.String() }

// stopping is set when Update has taken the signal to stop, and the
// emulator is on its way to keeping any unsaved program.
var stopping atomic.Bool

// notifyStop returns a channel that receives the first signal asking the
// emulator to stop, such as Ctrl+C in the terminal it was started from, or
// the terminal closing. Update then stops it as an error does, so that an
// unsaved program is kept.
//
// A second Ctrl+C stops it at once, as the game loop may be held up. But
// some programs that run others, such as timeout, pass on a Ctrl+C the
// terminal sent them both, so first it gives the game loop up to three
// seconds to take the first (a minimized window can run its loop only once
// a second), and then up to five more to keep the program. Other signals
// that follow are ignored: closing a terminal can send two hangups, and
// logging out can send a hangup and SIGTERM.
func notifyStop() <-chan os.Signal {
	var sigs []os.Signal
	for _, s := range stopSignals {
		// SIGHUP or SIGINT the emulator was started ignoring, as by nohup,
		// stays ignored. (Go doesn't say if others were.)
		if !signal.Ignored(s) {
			sigs = append(sigs, s)
		}
	}
	stop := make(chan os.Signal, 1)
	if len(sigs) == 0 {
		return stop
	}
	c := make(chan os.Signal, 4)
	signal.Notify(c, sigs...)
	go func() {
		stop <- <-c
		for s := range c {
			if s != os.Interrupt {
				continue
			}
			for t := time.Now(); !stopping.Load() && time.Since(t) < 3*time.Second; {
				time.Sleep(10 * time.Millisecond)
			}
			msg := "Stopped at once, without keeping any unsaved program"
			if stopping.Load() {
				time.Sleep(5 * time.Second) // main exits once the program is kept
				msg = "Stopped before any unsaved program could be kept"
				if name := recovering.Load(); name != nil {
					msg = "Stopped while keeping your unsaved program in " + *name + ", which may be incomplete"
				}
			}
			fmt.Fprintln(os.Stderr, msg)
			raise(os.Interrupt)
			os.Exit(1)
		}
	}()
	return stop
}

// isFlagSet reports whether the named flag was given on the command line.
func isFlagSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}
