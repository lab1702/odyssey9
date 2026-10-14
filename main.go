// Command odyssey9 emulates the Magnavox Odyssey2 with the "Computer Intro!"
// cartridge (Philips Videopac 9) inserted: a TV screen showing the
// cartridge's virtual computer and the console's 49-key keyboard.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

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
		fmt.Fprintln(os.Stderr, err)
		// A missing or broken sound device is only reported once the game
		// is running, as an error from the audio library.
		if msg := err.Error(); sound && (strings.HasPrefix(msg, "oto:") || strings.HasPrefix(msg, "audio:")) {
			fmt.Fprintln(os.Stderr, "The sound output failed. Run with -mute to start without sound.")
		}
		// The error stops the emulator without asking, so keep the program,
		// unless this is a screenshot run: its program came from -load and
		// -script and can be made again.
		if g.unsaved() && *shot == "" {
			if name, err := g.saveRecovery(); err != nil {
				fmt.Fprintln(os.Stderr, "Your unsaved program could not be kept:", err)
			} else {
				fmt.Fprintln(os.Stderr, "Your unsaved program was saved in", name)
			}
		}
		os.Exit(1)
	}
}

// notifyStop returns a channel that receives the first signal asking the
// emulator to stop, such as Ctrl+C in the terminal it was started from, or
// the terminal closing. Update then stops it as an error does, so that an
// unsaved program is kept. A second signal stops it at once, as the game
// loop may be held up.
func notifyStop() <-chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, stopSignals...)
	stop := make(chan os.Signal, 1)
	go func() {
		s := <-c
		signal.Reset(stopSignals...)
		stop <- s
	}()
	return stop
}

// isFlagSet reports whether the named flag was given on the command line.
func isFlagSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}
