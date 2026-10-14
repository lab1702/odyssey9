package main

import (
	"errors"
	"math"
	"slices"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"odyssey9/machine"
)

var errQuit = errors.New("quit")

// Logical window layout.
const (
	baseW    = 820
	baseH    = 890
	monitorW = 300
)

// Timings and positions used in several places.
const (
	keyLitFrames = 8               // frames a pressed key is drawn pressed
	toastTime    = 4 * time.Second // how long a message stays up
	confirmTime  = 4 * time.Second // how soon an action must be repeated to confirm it
	buttonY      = 420             // top of the row of buttons under the TV
	maxQueued    = 16              // typed keys that can wait for a running program
	resizeWait   = 15              // ticks to wait for the window to resize (see toggleMonitor)
)

// panel is the panel drawn over the window, if any.
type panel string

const (
	noPanel       panel = ""
	helpPanel     panel = "help"
	programsPanel panel = "programs"
)

// buttonID names a button under the TV. It is also the button's label.
type buttonID string

const (
	powerButton    buttonID = "POWER"
	speedButton    buttonID = "SPEED"
	programsButton buttonID = "PROGRAMS"
	saveButton     buttonID = "SAVE"
	loadButton     buttonID = "LOAD"
	monitorButton  buttonID = "MONITOR"
	helpButton     buttonID = "HELP"
)

type button struct {
	id         buttonID
	x, y, w, h float32
}

type game struct {
	m        *machine.Machine
	snd      *tone
	file     string
	scale    float32 // render pixels per logical unit
	monitor  bool
	width    float32 // logical width in use: lags logicalW() while the window resizes
	outsideW float64 // window width at the last Layout
	resizeOf float64 // window width when MONITOR asked for a new size
	resizeIn int     // ticks left to wait for that new size
	colors   bool    // colour characters like the manual's symbol chart
	overlay  panel   // drawn over the window
	toast    string
	toastFor time.Duration
	clean    [machine.Steps]byte // the program as last saved or loaded
	pending  string              // action waiting to be confirmed by repeating it
	pendFor  time.Duration
	keyLit   map[machine.Key]int // frames left to show a key as pressed
	mouseKey machine.Key
	keys     []machine.Key // typed keys on their way to the console, oldest first
	held     int           // how many of them were left waiting for a program
	buttons  []button
	bg       *ebiten.Image // drawBackground's picture, at scale bgScale
	bgScale  float32
	power    bool
	last     time.Time
	frame    int

	script   []machine.Key // testing aid: -script
	shotPath string        // testing aid: -screenshot
	shotWait int
	shotNow  bool
	quit     bool
	quitErr  error // reported when quitting
}

func newGame(m *machine.Machine, file string, sound bool) *game {
	g := &game{m: m, file: file, keyLit: map[machine.Key]int{}, scale: 1, width: baseW, power: true}
	g.clean = m.Mem
	g.snd = newAudio(sound)
	x := float32(20)
	for _, id := range []buttonID{powerButton, speedButton, programsButton, saveButton, loadButton, monitorButton, helpButton} {
		w := float32(92)
		if id == speedButton {
			w = 136
		}
		g.buttons = append(g.buttons, button{id, x, buttonY, w, 32})
		x += w + 12
	}
	return g
}

func (g *game) logicalW() float32 {
	if g.monitor {
		return baseW + monitorW
	}
	return baseW
}

func (g *game) windowSize() (int, int) {
	w, h := g.logicalW(), float32(baseH)
	if mon := ebiten.Monitor(); mon != nil {
		_, mh := mon.Size()
		if mh > 0 && float32(mh)*0.9 < h {
			f := float32(mh) * 0.9 / h
			w, h = w*f, h*f
		}
	}
	return int(w), int(h)
}

func (g *game) Layout(outsideW, outsideH int) (int, int) {
	w, h := g.LayoutF(float64(outsideW), float64(outsideH))
	return int(w), int(h)
}

func (g *game) LayoutF(outsideW, outsideH float64) (float64, float64) {
	dsf := 1.0
	if mon := ebiten.Monitor(); mon != nil {
		dsf = mon.DeviceScaleFactor()
	}
	// After MONITOR, keep the old width until the window has changed size
	// (or stopped waiting for it), so the picture doesn't shrink meanwhile.
	if g.width != g.logicalW() && (g.resizeIn <= 0 || outsideW != g.resizeOf) {
		g.width = g.logicalW()
	}
	g.outsideW = outsideW
	w, h, s := screenSize(float64(g.width), outsideW, outsideH, dsf)
	g.scale = float32(s)
	return w, h
}

// screenSize returns the size in device pixels of a picture lw logical
// pixels wide, scaled to fit a window, and the scale.
func screenSize(lw, outsideW, outsideH, dsf float64) (w, h, s float64) {
	s = math.Min(outsideW/lw, outsideH/baseH) * dsf
	if s <= 0 {
		s = 1
	}
	// A side that fills the window can come out a hair over its size, such
	// as 443.00000000000006, which mustn't round up: a picture 1 px larger
	// than the window is scaled down to fit and blurs.
	const slack = 1e-6
	return math.Ceil(lw*s - slack), math.Ceil(baseH*s - slack), s
}

func (g *game) showToast(s string) {
	g.toast, g.toastFor = s, toastTime
}

// unsaved reports whether program memory holds work that would be lost if
// it were erased or replaced.
func (g *game) unsaved() bool {
	return g.m.Mem != g.clean && g.m.Mem != [machine.Steps]byte{}
}

// confirm reports whether action was requested a second time within a few
// seconds. The first time, it shows prompt and returns false.
func (g *game) confirm(action, prompt string) bool {
	if g.pending == action && g.pendFor > 0 {
		g.pending = ""
		return true
	}
	g.pending, g.pendFor = action, confirmTime
	g.showToast(prompt)
	return false
}

// press queues a key for the console. deliverKeys passes the queued keys
// on in order. RESET is queued even when the queue is full, so that it can
// always stop a running program.
func (g *game) press(k machine.Key) {
	g.keyLit[k] = keyLitFrames
	if !g.power {
		return
	}
	if len(g.keys) < maxQueued {
		g.keys = append(g.keys, k)
	} else if k == machine.KeyReset {
		g.keys[len(g.keys)-1] = k
	}
}

// deliverKeys passes queued keys to the console. A running program only
// reads a key at INP or INA, so it gets them one at a time, as it reaches
// each one, and the rest wait. RESET doesn't wait: it stops the program,
// and the keys typed before it are dropped.
func (g *game) deliverKeys() {
	for len(g.keys) > 0 {
		k := g.keys[0]
		if g.m.Mode() == machine.ModeExecute && !g.m.WaitingForKey() && k != keyErase {
			i := slices.Index(g.keys, machine.KeyReset)
			if i < 0 {
				return
			}
			k, g.keys = machine.KeyReset, g.keys[i:]
		}
		g.keys = g.keys[1:]
		g.deliver(k)
	}
}

// deliver passes one key to the console.
func (g *game) deliver(k machine.Key) {
	switch k {
	case keyBackspace, keyErase:
		if g.m.Mode() == machine.ModeInput {
			g.m.Backspace()
			return
		}
		if k == keyErase {
			return // only erasing repeats, not CLEAR
		}
		k = machine.KeyClear
		g.keyLit[k] = keyLitFrames
	}
	g.m.Press(k)
}

// runConsole passes the queued keys to the console and runs it for dt.
func (g *game) runConsole(dt time.Duration) {
	if !g.power {
		g.snd.set(machine.SoundNone, 0)
		return
	}
	// Keys left waiting for a program are dropped once it has stopped, or
	// been replaced by a LOAD.
	if g.m.Mode() != machine.ModeExecute {
		g.keys = g.keys[min(g.held, len(g.keys)):]
	}
	g.deliverKeys()
	g.held = len(g.keys)
	g.m.Update(dt)
	g.snd.set(g.m.CurrentSound(), g.m.SoundLeft())
}

// closeWindow reports whether the window may close: when it would lose an
// unsaved program, only if the user closes it again.
func (g *game) closeWindow() bool {
	return !g.unsaved() || g.confirm("close", "Close again to quit and lose your unsaved program")
}

func (g *game) Update() error {
	now := time.Now()
	dt := now.Sub(g.last)
	if g.last.IsZero() || dt > 100*time.Millisecond {
		dt = time.Second / 60
	}
	g.last = now
	g.frame++
	if g.quit {
		if g.quitErr != nil {
			return g.quitErr
		}
		return errQuit
	}
	if ebiten.IsWindowBeingClosed() && g.closeWindow() {
		return errQuit
	}
	g.runScript()

	for k, n := range g.keyLit {
		if n <= 1 && k != g.mouseKey {
			delete(g.keyLit, k)
		} else if n > 1 {
			g.keyLit[k] = n - 1
		}
	}
	if g.toastFor > 0 {
		g.toastFor -= dt
	}
	if g.pendFor > 0 {
		g.pendFor -= dt
	}
	if g.resizeIn > 0 {
		g.resizeIn--
	}

	g.handleShortcuts()
	g.handleMouse()
	g.handleDrops()
	g.handleKeys()
	g.runConsole(dt)
	return nil
}

func (g *game) toggleOverlay(p panel) {
	if g.overlay == p {
		g.overlay = noPanel
	} else {
		g.overlay = p
	}
}

func (g *game) toggleMonitor() {
	ww, wh := ebiten.WindowSize()
	g.monitor = !g.monitor
	w := monitorWindowWidth(ww, wh, g.width, g.logicalW())
	if w != ww {
		ebiten.SetWindowSize(w, wh)
	}
	// Layout switches to the new width when the window changes size. A
	// window that keeps its size (it already fits, or it is maximized or
	// fullscreen) switches now.
	g.resizeOf, g.resizeIn = g.outsideW, resizeWait
	if w == ww || ebiten.IsWindowMaximized() || ebiten.IsFullscreen() {
		g.resizeIn = 0
	}
}

// monitorWindowWidth returns the window width to use when the logical width
// changes from oldW to newW in a window of ww by wh, keeping the picture's
// scale. The window grows only as far as the wider picture needs, and
// shrinks only if the picture filled its width, so a window with room to
// spare keeps its size.
func monitorWindowWidth(ww, wh int, oldW, newW float32) int {
	fits := float64(ww)/float64(oldW) <= float64(wh)/baseH // picture fills the width
	s := math.Min(float64(ww)/float64(oldW), float64(wh)/baseH)
	want := int(math.Round(float64(newW) * s))
	if newW > oldW {
		return max(ww, want)
	}
	if fits {
		return want
	}
	return ww
}

func (g *game) togglePower() {
	if g.power && g.unsaved() &&
		!g.confirm("power", "Press POWER again to switch off and erase your unsaved program") {
		return
	}
	g.power = !g.power
	g.m.PowerOn() // switching off erases the program, as on the real console
	g.keys, g.held = g.keys[:0], 0
}
