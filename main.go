// Command odyssey9 emulates the Magnavox Odyssey2 with the "Computer Intro!"
// cartridge (Philips Videopac 9) inserted: a TV screen showing the
// cartridge's virtual computer and the console's 49-key keyboard.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"odyssey9/machine"
)

func main() {
	home, _ := os.UserHomeDir()
	file := flag.String("file", filepath.Join(home, "odyssey9-program.txt"), "program file used by SAVE and LOAD")
	load := flag.String("load", "", "program listing to load at start-up")
	mute := flag.Bool("mute", false, "disable sound")
	monitor := flag.Bool("monitor", false, "start with the monitor panel open")
	script := flag.String("script", "", "testing aid: keys to type at start-up, e.g. \"{RESET}PMI6B{ENTER}\"")
	shot := flag.String("screenshot", "", "testing aid: save a PNG after the script runs, then quit")
	flag.Parse()

	m := machine.New()
	if *load != "" {
		f, err := os.Open(*load)
		if err != nil {
			log.Fatal(err)
		}
		p, err := machine.ParseProgram(f)
		f.Close()
		if err != nil {
			log.Fatalf("%s: %v", *load, err)
		}
		m.LoadProgram(p)
	}

	g := newGame(m, *file, !*mute)
	g.monitor = *monitor
	g.script, g.shotPath, g.shotWait = parseScript(*script), *shot, 90
	ebiten.SetWindowTitle("Odyssey² Computer Intro! (Videopac 9)")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	w, h := g.windowSize()
	ebiten.SetWindowSize(w, h)
	ebiten.SetTPS(60)
	if err := ebiten.RunGame(g); err != nil && err != errQuit {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
