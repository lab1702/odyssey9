package main

import (
	"image/color"
	"runtime"
	"slices"
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

// hostKey is an Odyssey2 key typed on the host keyboard.
type hostKey struct {
	k      machine.Key
	repeat bool // typed by a key held down repeating, not by a press
}

// hostKeyboard turns the host keyboard's input into Odyssey2 keys, one tick
// at a time. With an input method, as on most Linux desktops, a key's
// character can come the next time the system's input is read (a poll)
// after the key is reported pressed, which can be several ticks later when
// the game loop runs more than one tick a frame. So it keeps the keys whose
// characters are still to come.
type hostKeyboard struct {
	waiting   []ebiten.Key // keys pressed whose characters haven't come, oldest first
	waitPolls int          // polls left to wait for them
	waitTicks int          // ticks left to wait for them, in case polls aren't counted
	held      []hostKey    // keys pressed after those, waiting for them
	downPolls int          // polls left in which a key held down can repeat
	lastKey   ebiten.Key   // the key whose character came last
	lastChar  rune         // and that character
}

// How long a pressed key's character is waited for: until the poll after
// next, or about half a second. A key's last repeat can come as late after
// it is let go.
const (
	charPolls = 3
	charTicks = 30
)

// keyInput is one tick of the host keyboard's input.
type keyInput struct {
	chars     []rune       // characters typed
	pressed   []ebiten.Key // keys just pressed
	down      []ebiten.Key // keys held down, including those just pressed
	backspace int          // ticks Backspace has been held
	newPoll   bool         // the system's input was read since the last tick
}

// poll returns the Odyssey2 keys typed on the host keyboard this tick.
// newPoll says whether the system's input was read since the last tick.
// Keys typed with a shortcut modifier held are left to the system,
// including Backspace, Enter, Delete and Esc, so that a habit such as
// Cmd+Backspace on a Mac doesn't CLEAR.
func (h *hostKeyboard) poll(newPoll bool) []hostKey {
	in := keyInput{down: inpututil.AppendPressedKeys(nil), newPoll: newPoll}
	if !shortcutHeld(runtime.GOOS, ebiten.IsKeyPressed) {
		in.chars = ebiten.AppendInputChars(nil)
		in.pressed = inpututil.AppendJustPressedKeys(nil)
		in.backspace = inpututil.KeyPressDuration(ebiten.KeyBackspace)
	}
	return h.next(in)
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

// next turns one tick of host keyboard input into Odyssey2 keys.
//
// A character is a press's when a key waiting for its character takes it.
// A held key repeats only until another key is pressed, so its repeats come
// before the characters of keys pressed in the same tick: those keys take
// the tick's last characters. Any others are a held key repeating, or,
// with no key held, a key Ebitengine doesn't know.
//
// Characters of keys pressed before this tick come first, as they were
// typed first, and the keys pressed in this tick wait for any that haven't
// come yet. Otherwise Ebitengine doesn't say in what order characters and
// other keys came within a tick, so they are put in the order that does
// least harm when two land in the same tick. Backspace and Enter go first:
// if that is wrong, the line on the screen shows it or the console beeps,
// where the other order could silently store a wrong step. Delete and Esc
// go last, after the characters of keys pressed with them, so a character
// typed with them is lost rather than taken as a command.
func (h *hostKeyboard) next(in keyInput) []hostKey {
	if in.newPoll {
		h.waitPolls--
		h.downPolls--
	}
	h.waitTicks--
	if h.waitPolls <= 0 || h.waitTicks <= 0 {
		h.waiting = nil // such as a dead key, which types nothing by itself
	}
	if slices.ContainsFunc(in.down, typesChar) {
		h.downPolls = charPolls
	}

	var first, last []hostKey
	// Backspace repeats while held, like typed characters: after half a
	// second, 15 times a second.
	if in.backspace == 1 {
		first = append(first, hostKey{keyBackspace, false})
	} else if in.backspace >= 30 && (in.backspace-30)%4 == 0 {
		first = append(first, hostKey{keyErase, true})
	}
	var typing []ebiten.Key
	for _, p := range in.pressed {
		switch {
		case p == ebiten.KeyEnter || p == ebiten.KeyNumpadEnter:
			first = append(first, hostKey{machine.KeyEnter, false})
		case p == ebiten.KeyDelete:
			last = append(last, hostKey{machine.KeyClear, false})
		case p == ebiten.KeyEscape:
			last = append(last, hostKey{machine.KeyReset, false})
		case typesChar(p):
			typing = append(typing, p)
		}
	}
	old := len(h.waiting) // keys pressed before this tick still waiting
	if len(typing) > 0 {
		h.waiting = append(h.waiting, typing...)
		h.waitPolls, h.waitTicks = charPolls, charTicks
	}

	// Which characters the waiting keys take. With an input method, the
	// last repeats of a key held down can come with the press of the next
	// one, before its character: the last key's character, with a key held
	// that isn't waiting, is such a repeat.
	repeating := slices.ContainsFunc(in.down, func(k ebiten.Key) bool {
		return typesChar(k) && !slices.Contains(h.waiting, k)
	})
	var takers []int
	for i, r := range in.chars {
		if !(repeating && r == h.lastChar && !slices.Contains(h.waiting, h.lastKey)) {
			takers = append(takers, i)
		}
	}
	takers = takers[max(0, len(takers)-len(h.waiting)):]
	late := min(len(takers), old) // taken by keys pressed before this tick

	var keys, now []hostKey
	for i, r := range in.chars {
		k, ok := keyForRune(r)
		t := slices.Index(takers, i)
		switch {
		case t < 0:
			// a key held down repeating
		case t < late:
			h.lastKey, h.lastChar = h.waiting[t], r
		default:
			h.lastKey, h.lastChar = h.waiting[t], r
			if ok {
				now = append(now, hostKey{k, false})
			}
			continue
		}
		if ok {
			keys = append(keys, hostKey{k, t < 0 && h.downPolls > 0})
		}
	}
	h.waiting = h.waiting[len(takers):]

	if late < old {
		// A key pressed before this tick still hasn't typed its character
		// (and none of this tick's characters were its).
		h.held = append(append(h.held, first...), last...)
		return keys
	}
	keys = append(keys, h.held...)
	h.held = nil
	keys = append(append(keys, first...), now...)
	if len(h.waiting) > 0 {
		h.held = append(h.held, last...)
	} else {
		keys = append(keys, last...)
	}
	return keys
}

// typesChar reports whether the host key k types a character.
func typesChar(k ebiten.Key) bool {
	switch {
	case k >= ebiten.KeyA && k <= ebiten.KeyZ,
		k >= ebiten.KeyDigit0 && k <= ebiten.KeyDigit9,
		k >= ebiten.KeyNumpad0 && k <= ebiten.KeyNumpad9:
		return true
	}
	switch k {
	case ebiten.KeySpace, ebiten.KeyMinus, ebiten.KeyEqual, ebiten.KeyBracketLeft,
		ebiten.KeyBracketRight, ebiten.KeyBackslash, ebiten.KeySemicolon,
		ebiten.KeyQuote, ebiten.KeyBackquote, ebiten.KeyComma, ebiten.KeyPeriod,
		ebiten.KeySlash, ebiten.KeyIntlBackslash, ebiten.KeyNumpadAdd,
		ebiten.KeyNumpadSubtract, ebiten.KeyNumpadMultiply, ebiten.KeyNumpadDivide,
		ebiten.KeyNumpadDecimal, ebiten.KeyNumpadEqual:
		return true
	}
	return false
}
