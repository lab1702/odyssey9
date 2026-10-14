package main

import (
	"image/color"
	"runtime"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"odyssey9/machine"
)

// keyCap is one key drawn on the on-screen keyboard.
type keyCap struct {
	key        machine.Key
	label      string
	x, y, w, h float32
}

func (k *keyCap) contains(x, y float32) bool {
	return x >= k.x && x < k.x+k.w && y >= k.y && y < k.y+k.h
}

// Keyboard geometry, modelled on the "Keyboard" page of the manual: a
// NUMERIC group, a FUNCTION / INPUT / RESET group and an ALPHA group.
const (
	kbX, kbY, kbW, kbH = 20, 470, 780, 400
	keyW, keyH         = 54, 30
	keyPitch           = 69
	keyRow0X           = kbX + 47
)

var keyCaps = func() []*keyCap {
	var caps []*keyCap
	row := func(y, x0 float32, keys []machine.Key, labels []string) {
		for i, k := range keys {
			caps = append(caps, &keyCap{k, labels[i], x0 + float32(i)*keyPitch, y, keyW, keyH})
		}
	}
	digits := []machine.Key{machine.Key0, machine.Key1, machine.Key2, machine.Key3, machine.Key4,
		machine.Key5, machine.Key6, machine.Key7, machine.Key8, machine.Key9}
	row(kbY+45, keyRow0X, digits, []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"})
	row(kbY+125, keyRow0X,
		[]machine.Key{machine.KeyPlus, machine.KeyMinus, machine.KeyTimes, machine.KeyDivide, machine.KeyEquals,
			machine.KeyYes, machine.KeyNo, machine.KeyClear, machine.KeyEnter, machine.KeyReset},
		[]string{"+", "–", "×", "÷", "=", "YES", "NO", "CLEAR", "ENTER", "▲"})
	letters := func(s string) ([]machine.Key, []string) {
		var ks []machine.Key
		var ls []string
		for _, r := range s {
			switch r {
			case '.':
				ks, ls = append(ks, machine.KeyPeriod), append(ls, "•")
			case '?':
				ks, ls = append(ks, machine.KeyQuestion), append(ls, "?")
			default:
				ks, ls = append(ks, machine.KeyForLetter(r)), append(ls, string(r))
			}
		}
		return ks, ls
	}
	k, l := letters("QWERTYUIOP")
	row(kbY+215, keyRow0X, k, l)
	k, l = letters("ASDFGHJKL")
	row(kbY+262, keyRow0X+keyPitch/2+8, k, l)
	k, l = letters("ZXCVBNM.?")
	row(kbY+309, keyRow0X+keyPitch, k, l)
	caps = append(caps, &keyCap{machine.KeySpace, "SPACE", kbX + kbW/2 - 85, kbY + 352, 170, 24})
	return caps
}()

var (
	colKeyFace   = rgb(0xF4F2EC)
	colKeyDown   = rgb(0x9C9A94)
	colKeyLabel  = rgb(0x111111)
	colPanel     = rgb(0x050505)
	colGroupLine = rgb(0xF4F2EC)
	colRed       = rgb(0xE8222A)
)

// drawKeyboard draws the keyboard with no key pressed.
func drawKeyboard(c canvas) {
	c.rrect(kbX-6, kbY-6, kbW+12, kbH+12, 6, rgb(0xE9E6DD))
	c.rect(kbX, kbY, kbW, kbH, colPanel)

	group := func(x, y, w, h float32) {
		c.rframe(x, y, w, h, 16, 3, colGroupLine, colPanel)
	}
	label := func(s string, x, y float32, col color.Color) {
		w := c.textWidth(s, fontBold, 12)
		c.rect(x-w/2-6, y-7, w+12, 14, colPanel)
		c.text(s, fontBold, 12, x, y, col, 0)
	}
	gx, gw := float32(kbX+22), float32(kbW-44)
	group(gx, kbY+30, gw, 60)
	label("NUMERIC", kbX+kbW/2, kbY+30, colGroupLine)
	group(gx, kbY+110, gw, 60)
	label("FUNCTION", keyRow0X+2*keyPitch+keyW/2, kbY+110, colRed)
	label("INPUT", keyRow0X+6.5*keyPitch+keyW/2, kbY+110, colRed)
	label("RESET", keyRow0X+9*keyPitch+keyW/2, kbY+110, colRed)
	group(gx, kbY+192, gw, 198)
	label("ALPHA", kbX+kbW/2, kbY+192, colGroupLine)

	// the hatched strips either side of the space bar
	for x := float32(gx + 14); x < kbX+kbW/2-95; x += 4 {
		c.line(x, kbY+357, x, kbY+371, 1.4, colGroupLine)
	}
	for x := float32(kbX + kbW/2 + 95); x < gx+gw-14; x += 4 {
		c.line(x, kbY+357, x, kbY+371, 1.4, colGroupLine)
	}

	for _, k := range keyCaps {
		k.draw(c, false)
	}
}

// drawLitKeys draws the keys shown pressed over the background.
func (g *game) drawLitKeys(c canvas) {
	for _, k := range keyCaps {
		if g.keyLit[k.key] > 0 {
			// Cover the idle key first: its antialiased corners would show
			// round the lit one's.
			c.rect(k.x-1, k.y-1, k.w+2, k.h+2, colPanel)
			k.draw(c, true)
		}
	}
}

func (k *keyCap) draw(c canvas, lit bool) {
	face, ink := colKeyFace, colKeyLabel
	if k.key == machine.KeyReset {
		face = colRed
	}
	if lit {
		face = colKeyDown
	}
	c.rrect(k.x, k.y, k.w, k.h, 5, face)
	if k.key == machine.KeyReset {
		c.text("▲", fontBold, 15, k.x+k.w/2, k.y+k.h/2, ink, 0)
		return
	}
	size := float32(17)
	if utf8.RuneCountInString(k.label) > 1 && k.key != machine.KeyQuestion {
		size = 10
	}
	c.text(k.label, fontBold, size, k.x+k.w/2, k.y+k.h/2+1, ink, 0)
}

// keyAt returns the key under a logical point.
func keyAt(x, y float32) machine.Key {
	for _, k := range keyCaps {
		if k.contains(x, y) {
			return k.key
		}
	}
	return machine.KeyNone
}

// typedKeys translates characters typed on the computer keyboard.
var typedKeys = map[rune]machine.Key{
	'.': machine.KeyPeriod, '?': machine.KeyQuestion, '+': machine.KeyPlus,
	'-': machine.KeyMinus, '*': machine.KeyTimes, '/': machine.KeyDivide,
	'=': machine.KeyEquals, ' ': machine.KeySpace, '[': machine.KeyYes,
	']': machine.KeyNo,
}

// keyForRune returns the Odyssey2 key for a character typed on the computer
// keyboard.
func keyForRune(r rune) (machine.Key, bool) {
	if k := machine.KeyForLetter(r); k != machine.KeyNone {
		return k, true
	}
	k, ok := typedKeys[r]
	return k, ok
}

// Pseudo-keys for Backspace, which the console doesn't have. What they do
// depends on the mode when they reach the console (see game.deliver).
const (
	keyBackspace machine.Key = -10 // erases in Input mode, otherwise CLEAR
	keyErase     machine.Key = -11 // Backspace held down: only erases
)

// pollKeyboard returns the Odyssey2 keys pressed on the host keyboard
// this tick.
func pollKeyboard() []machine.Key {
	var chars []rune
	if !shortcutHeld(runtime.GOOS, ebiten.IsKeyPressed) {
		chars = ebiten.AppendInputChars(nil)
	}
	return hostKeys(chars, inpututil.AppendJustPressedKeys(nil),
		inpututil.KeyPressDuration(ebiten.KeyBackspace))
}

// shortcutHeld reports whether a modifier is held that makes keys a
// shortcut rather than typing, such as Cmd+S on a Mac or Alt+E on Windows.
// Ebitengine still reports the characters of some shortcuts. AltGr, which
// types [ and ] on many European keyboards, is Right Alt (with Ctrl on
// Windows), so it is let through, and so is Option on a Mac.
func shortcutHeld(goos string, held func(ebiten.Key) bool) bool {
	if held(ebiten.KeyMeta) {
		return true
	}
	if goos == "darwin" {
		return held(ebiten.KeyControl)
	}
	return (held(ebiten.KeyControl) || held(ebiten.KeyAlt)) && !held(ebiten.KeyAltRight)
}

// hostKeys turns one tick of host keyboard input into Odyssey2 keys: the
// characters typed, the keys just pressed and how many ticks Backspace has
// been held. Ebitengine doesn't say in what order characters and other keys
// came within a tick, so they are put in the order that does least harm
// when two land in the same tick. Backspace and Enter go first: if that is
// wrong, the line on the screen shows it or the console beeps, where the
// other order could silently store a wrong step. Delete and Esc go last, so
// a character typed with them is lost rather than taken as a command.
func hostKeys(chars []rune, pressed []ebiten.Key, backspace int) []machine.Key {
	var keys, last []machine.Key
	// Backspace repeats while held, like typed characters: after half a
	// second, 15 times a second.
	if backspace == 1 {
		keys = append(keys, keyBackspace)
	} else if backspace >= 30 && (backspace-30)%4 == 0 {
		keys = append(keys, keyErase)
	}
	for _, p := range pressed {
		switch p {
		case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
			keys = append(keys, machine.KeyEnter)
		case ebiten.KeyDelete:
			last = append(last, machine.KeyClear)
		case ebiten.KeyEscape:
			last = append(last, machine.KeyReset)
		}
	}
	for _, r := range chars {
		if k, ok := keyForRune(r); ok {
			keys = append(keys, k)
		}
	}
	return append(keys, last...)
}
