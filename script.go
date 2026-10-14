package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"odyssey9/machine"
)

// Scripted input and screenshots are testing aids (-script, -screenshot).

var scriptWords = map[string]machine.Key{
	"RESET": machine.KeyReset, "ENTER": machine.KeyEnter, "CLEAR": machine.KeyClear,
	"YES": machine.KeyYes, "NO": machine.KeyNo, "SPACE": machine.KeySpace,
	"HELP": scriptHelp, "PROGRAMS": scriptPrograms, "WAIT": machine.KeyNone, "SAVE": scriptSave,
}

// shotDelay is how many frames to wait after the last scripted key before
// taking the screenshot, so the screen has settled.
const shotDelay = 90

// pseudo-keys that open the emulator's overlays
const (
	scriptHelp     machine.Key = -1
	scriptPrograms machine.Key = -2
	scriptSave     machine.Key = -3
)

func parseScript(s string) ([]machine.Key, error) {
	var keys []machine.Key
	for s != "" {
		if s[0] == '{' {
			end := strings.IndexByte(s, '}')
			if end < 0 {
				return nil, fmt.Errorf("script: missing } after %q", s)
			}
			k, ok := scriptWords[strings.ToUpper(s[1:end])]
			if !ok {
				return nil, fmt.Errorf("script: unknown key %q", s[:end+1])
			}
			keys = append(keys, k)
			s = s[end+1:]
			continue
		}
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		k, ok := keyForRune(r)
		if !ok {
			return nil, fmt.Errorf("script: no key for %q", r)
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// runScript types one scripted key every few frames.
func (g *game) runScript() {
	if len(g.script) > 0 && g.frame%4 == 0 {
		switch k := g.script[0]; k {
		case scriptHelp:
			g.toggleOverlay(helpPanel)
		case scriptPrograms:
			g.toggleOverlay(programsPanel)
		case scriptSave:
			g.save()
		case machine.KeyNone:
		default:
			g.typeKey(k)
		}
		g.script = g.script[1:]
		g.shotWait = shotDelay
	}
	if len(g.script) == 0 && g.shotPath != "" && g.shotWait > 0 {
		g.shotWait--
		if g.shotWait == 0 {
			g.shotNow = true
		}
	}
}

func (g *game) takeScreenshot(screen *ebiten.Image) {
	b := screen.Bounds()
	img := image.NewRGBA(b)
	screen.ReadPixels(img.Pix)
	f, err := os.Create(g.shotPath)
	if err == nil {
		err = png.Encode(f, img)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		g.quitErr = fmt.Errorf("screenshot: %w", err)
	}
	g.quit = true
}
