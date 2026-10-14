package main

// Keyboard, mouse and dropped-file input.

import (
	"io/fs"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"odyssey9/machine"
)

// fnKeys are the function keys that work like the buttons under the TV.
var fnKeys = map[ebiten.Key]buttonID{
	ebiten.KeyF1: helpButton,
	ebiten.KeyF2: saveButton,
	ebiten.KeyF3: loadButton,
	ebiten.KeyF4: programsButton,
	ebiten.KeyF5: speedButton,
	ebiten.KeyF6: monitorButton,
	ebiten.KeyF8: powerButton,
}

func (g *game) handleShortcuts() {
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		if id, ok := fnKeys[k]; ok {
			g.clickButton(id)
		} else if k == ebiten.KeyF7 {
			g.colors = !g.colors
		}
	}
}

// handleKeys passes typed keys to the console, or to the open panel.
func (g *game) handleKeys() {
	switch {
	case g.overlay == noPanel:
		keys, bs, held := pollKeyboard()
		for _, k := range keys {
			g.press(k)
		}
		if bs && g.power {
			if g.m.Mode() == machine.ModeInput {
				g.m.Backspace()
			} else if !held { // only erasing repeats, not CLEAR
				g.press(machine.KeyClear)
			}
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		g.overlay = noPanel
	case g.overlay == programsPanel:
		for _, r := range ebiten.AppendInputChars(nil) {
			if i := strings.IndexRune(sampleKeys, unicode.ToUpper(r)); i >= 0 && i < len(machine.Samples) {
				g.loadSample(i)
			}
		}
	}
}

func (g *game) cursor() (float32, float32) {
	x, y := ebiten.CursorPosition()
	return float32(x) / g.scale, float32(y) / g.scale
}

func (g *game) handleMouse() {
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		g.mouseKey = machine.KeyNone
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	x, y := g.cursor()
	if g.overlay == programsPanel {
		if i := g.sampleAt(x, y); i >= 0 {
			g.loadSample(i)
			return
		}
	}
	if g.overlay != noPanel {
		g.overlay = noPanel
		return
	}
	for _, b := range g.buttons {
		if x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h {
			g.clickButton(b.id)
			return
		}
	}
	if k := keyAt(x, y); k != machine.KeyNone {
		g.mouseKey = k
		g.press(k)
	}
}

func (g *game) clickButton(id buttonID) {
	switch id {
	case powerButton:
		g.togglePower()
	case speedButton:
		g.m.SetSpeed((g.m.Speed() + 1) % len(machine.Speeds))
	case programsButton:
		g.toggleOverlay(programsPanel)
	case saveButton:
		g.save()
	case loadButton:
		g.load()
	case monitorButton:
		g.toggleMonitor()
	case helpButton:
		g.toggleOverlay(helpPanel)
	}
}

func (g *game) handleDrops() {
	files := ebiten.DroppedFiles()
	if files == nil {
		return
	}
	entries, err := fs.ReadDir(files, ".")
	if err != nil || len(entries) == 0 {
		return
	}
	name := entries[0].Name()
	data, err := fs.ReadFile(files, name)
	if err != nil {
		g.showToast("Cannot read " + name)
		return
	}
	g.loadBytes(name, data, "drop:"+name, "Drop "+name+" again to replace your unsaved program")
}
