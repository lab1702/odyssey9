package main

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"math"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"odyssey9/machine"
)

var errQuit = errors.New("quit")

// Logical window layout.
const (
	baseW    = 820
	baseH    = 890
	monitorW = 300
)

type button struct {
	id         string
	x, y, w, h float32
}

type game struct {
	m        *machine.Machine
	snd      *tone
	file     string
	scale    float32 // render pixels per logical unit
	monitor  bool
	colors   bool   // colour characters like the manual's symbol chart
	overlay  string // "", "help", "programs"
	toast    string
	toastFor time.Duration
	keyLit   map[machine.Key]int // frames left to show a key as pressed
	mouseKey machine.Key
	buttons  []button
	power    bool
	last     time.Time
	frame    int

	script   []machine.Key // testing aid: -script
	shotPath string        // testing aid: -screenshot
	shotWait int
	shotNow  bool
	quit     bool
}

func newGame(m *machine.Machine, file string, sound bool) *game {
	g := &game{m: m, file: file, keyLit: map[machine.Key]int{}, scale: 1, power: true}
	g.snd = newAudio(sound)
	x := float32(20)
	for _, id := range []string{"POWER", "SPEED", "PROGRAMS", "SAVE", "LOAD", "MONITOR", "HELP"} {
		w := float32(92)
		if id == "SPEED" {
			w = 136
		}
		g.buttons = append(g.buttons, button{id, x, 420, w, 32})
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
	lw := float64(g.logicalW())
	s := math.Min(outsideW/lw, outsideH/baseH) * dsf
	if s <= 0 {
		s = 1
	}
	g.scale = float32(s)
	return math.Ceil(lw * s), math.Ceil(baseH * s)
}

func (g *game) showToast(s string) {
	g.toast, g.toastFor = s, 4*time.Second
}

func (g *game) press(k machine.Key) {
	g.keyLit[k] = 8
	if !g.power {
		return
	}
	g.m.Press(k)
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

	g.handleShortcuts()
	g.handleMouse()
	g.handleDrops()

	if g.overlay == "" {
		keys, bs := pollKeyboard()
		for _, k := range keys {
			g.press(k)
		}
		if bs && g.power {
			if g.m.Mode() == machine.ModeInput {
				g.m.Backspace()
			} else {
				g.press(machine.KeyClear)
			}
		}
	} else if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.overlay = ""
	} else if g.overlay == "programs" {
		for _, r := range ebiten.AppendInputChars(nil) {
			if i := strings.IndexRune("123456789abcdef", unicode.ToLower(r)); i >= 0 && i < len(machine.Samples) {
				g.loadSample(i)
			}
		}
	}

	if g.power {
		g.m.Update(dt)
		g.snd.set(g.m.CurrentSound())
	} else {
		g.snd.set(machine.SoundNone)
	}
	return nil
}

func (g *game) handleShortcuts() {
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		switch k {
		case ebiten.KeyF1:
			g.toggleOverlay("help")
		case ebiten.KeyF2:
			g.save()
		case ebiten.KeyF3:
			g.load()
		case ebiten.KeyF4:
			g.toggleOverlay("programs")
		case ebiten.KeyF5:
			g.m.SetSpeed((g.m.Speed() + 1) % len(machine.Speeds))
		case ebiten.KeyF6:
			g.toggleMonitor()
		case ebiten.KeyF7:
			g.colors = !g.colors
		case ebiten.KeyF8:
			g.togglePower()
		}
	}
}

func (g *game) toggleOverlay(name string) {
	if g.overlay == name {
		g.overlay = ""
	} else {
		g.overlay = name
	}
}

func (g *game) toggleMonitor() {
	ww, wh := ebiten.WindowSize()
	old := g.logicalW()
	g.monitor = !g.monitor
	ebiten.SetWindowSize(int(float32(ww)*g.logicalW()/old), wh)
}

func (g *game) togglePower() {
	g.power = !g.power
	if g.power {
		g.m.PowerOn()
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
	if g.overlay == "programs" {
		if i := g.sampleAt(x, y); i >= 0 {
			g.loadSample(i)
			return
		}
	}
	if g.overlay != "" {
		g.overlay = ""
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

func (g *game) clickButton(id string) {
	switch id {
	case "POWER":
		g.togglePower()
	case "SPEED":
		g.m.SetSpeed((g.m.Speed() + 1) % len(machine.Speeds))
	case "PROGRAMS":
		g.toggleOverlay("programs")
	case "SAVE":
		g.save()
	case "LOAD":
		g.load()
	case "MONITOR":
		g.toggleMonitor()
	case "HELP":
		g.toggleOverlay("help")
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
	data, err := fs.ReadFile(files, entries[0].Name())
	if err != nil {
		g.showToast("Cannot read " + entries[0].Name())
		return
	}
	g.loadBytes(entries[0].Name(), data)
}

func (g *game) loadBytes(name string, data []byte) {
	p, err := machine.ParseProgram(bytes.NewReader(data))
	if err != nil {
		g.showToast(name + ": " + err.Error())
		return
	}
	g.power = true
	g.m.LoadProgram(p)
	g.showToast("Loaded " + name + " — press E to execute")
}

func (g *game) save() {
	var buf bytes.Buffer
	machine.FormatProgram(&buf, "Saved "+time.Now().Format("2006-01-02 15:04"), &g.m.Mem)
	if err := os.WriteFile(g.file, buf.Bytes(), 0o644); err != nil {
		g.showToast("Save failed: " + err.Error())
		return
	}
	g.showToast("Saved to " + g.file)
}

func (g *game) load() {
	data, err := os.ReadFile(g.file)
	if err != nil {
		g.showToast("Load failed: " + err.Error())
		return
	}
	g.loadBytes(g.file, data)
}

func (g *game) loadSample(i int) {
	s := machine.Samples[i]
	g.power = true
	g.m.LoadProgram(s.Program())
	g.overlay = ""
	g.showToast(s.Name + " loaded. " + s.Usage)
}

// ---------------------------------------------------------------------------
// Drawing

// charColor follows the colours of the "Computer Symbols" chart in the
// manual, which shade from purple through green and yellow to red.
func charColor(code byte) color.RGBA {
	palette := []uint32{0xC266DA, 0x9DB894, 0x52D163, 0xB2E23F, 0xF5C836, 0xF7613A, 0xEE3E45, 0xEE4580, 0xEE58B4}
	i := int(code) / 7
	if i >= len(palette) {
		i = len(palette) - 1
	}
	return rgb(palette[i])
}

const (
	tvX, tvY, tvW, tvH = 20, 16, 780, 388
	scrX, scrY         = 58, 40
	scrW, scrH         = 704, 300
)

func (g *game) Draw(screen *ebiten.Image) {
	c := canvas{screen, g.scale}
	screen.Fill(rgb(0x2A2622))
	g.drawTV(c)
	g.drawButtons(c)
	g.drawKeyboard(c)
	if g.monitor {
		g.drawMonitor(c)
	}
	switch g.overlay {
	case "help":
		g.drawHelp(c)
	case "programs":
		g.drawPrograms(c)
	}
	if g.shotNow {
		g.shotNow = false
		g.takeScreenshot(screen)
	}
}

func (g *game) drawTV(c canvas) {
	// cabinet and bezel
	c.rrect(tvX, tvY, tvW, tvH, 18, rgb(0x161412))
	c.rrect(tvX+8, tvY+8, tvW-16, tvH-16, 14, rgb(0x3B3631))
	c.rrect(scrX-10, scrY-10, scrW+20, scrH+20, 26, rgb(0x0C0B0A))
	bg := rgb(0x0A0F24)
	if !g.power {
		bg = rgb(0x060606)
	}
	c.rrect(scrX, scrY, scrW, scrH, 22, bg)

	if g.power {
		// The 11 output positions.
		const px, py = 7, 9
		const pitch = 62
		x0 := float32(scrX) + (scrW-(machine.Positions*pitch-6))/2
		y0 := float32(scrY + 70)
		for i, code := range g.m.Screen {
			x := x0 + float32(i)*pitch
			col := rgb(0xE6ECFF)
			if g.colors {
				col = charColor(code)
			}
			glow := color.RGBA{col.R / 5, col.G / 5, col.B / 5, 0x33}
			c.glyph(code, x-1.5, y0-1.5, px+3.0/8, py+3.0/7, glow)
			c.glyph(code, x, y0, px, py, col)
			c.text(fmt.Sprintf("%02X", i), fontMono, 10, x+28, y0+82, rgba(0x6C7AB0, 0x90), 0)
		}

		// The status line in the same character set.
		status := g.m.Status()
		const spx, spy, spitch = 3, 4, 28
		sx := float32(scrX) + (scrW-machine.StatusWidth*spitch)/2
		sy := float32(scrY + 205)
		codes := machine.TextCodes(status)
		for i, code := range codes {
			if i >= machine.StatusWidth {
				break
			}
			c.glyph(code, sx+float32(i)*spitch, sy, spx, spy, rgb(0xA9B8E8))
		}
		// blinking cursor while typing in Input mode
		if g.m.Mode() == machine.ModeInput && g.frame/20%2 == 0 && len(codes) < machine.StatusWidth {
			c.glyph(0x2F, sx+float32(len(codes))*spitch, sy, spx, spy, rgba(0xE8ECFF, 0xA0))
		}
	}

	// scan lines
	for y := float32(scrY + 2); y < scrY+scrH-2; y += 3 {
		c.rect(scrX+4, y, scrW-8, 1, color.RGBA{0, 0, 0, 0x30})
	}

	// nameplate and power light
	c.text("ODYSSEY²", fontBold, 15, tvX+40, tvY+tvH-22, rgb(0xD8D2C4), -1)
	c.text("COMPUTER INTRO!  ·  VIDEOPAC 9", fontRegular, 12, tvX+140, tvY+tvH-22, rgb(0x9C958A), -1)
	led := rgb(0x401010)
	if g.power {
		led = rgb(0xFF3030)
	}
	c.circle(tvX+tvW-44, tvY+tvH-22, 6, led)
	c.text(machine.Speeds[g.m.Speed()].Name, fontMono, 11, tvX+tvW-60, tvY+tvH-22, rgb(0x9C958A), 1)

	if g.toastFor > 0 && g.toast != "" {
		w := c.textWidth(g.toast, fontRegular, 13) + 24
		if w > scrW-20 {
			w = scrW - 20
		}
		x := float32(scrX) + (scrW-w)/2
		c.rrect(x, scrY+scrH-38, w, 26, 8, rgba(0x000000, 0xD0))
		c.text(g.toast, fontRegular, 13, x+w/2, scrY+scrH-25, rgb(0xFFFFFF), 0)
	}
}

func (g *game) drawButtons(c canvas) {
	for _, b := range g.buttons {
		label := b.id
		on := false
		switch b.id {
		case "POWER":
			on = g.power
		case "SPEED":
			label = "SPEED: " + machine.Speeds[g.m.Speed()].Name
		case "MONITOR":
			on = g.monitor
		case "PROGRAMS":
			on = g.overlay == "programs"
		case "HELP":
			on = g.overlay == "help"
		}
		face := rgb(0x4A443D)
		if on {
			face = rgb(0x6E655A)
		}
		c.rrect(b.x, b.y, b.w, b.h, 6, rgb(0x14120F))
		c.rrect(b.x+1, b.y+1, b.w-2, b.h-3, 5, face)
		ink := rgb(0xEDE6D8)
		if b.id == "POWER" && g.power {
			ink = rgb(0xFF7060)
		}
		c.text(label, fontBold, 12, b.x+b.w/2, b.y+b.h/2, ink, 0)
	}
}

// listing returns instruction start steps, decoding from step 00.
func listing(mem *[machine.Steps]byte) []int {
	var starts []int
	for s := 0; s < machine.Steps; {
		starts = append(starts, s)
		_, n := machine.Disassemble(mem, s)
		s += n
	}
	return starts
}

func (g *game) drawMonitor(c canvas) {
	x0 := float32(baseW)
	c.rect(x0, 0, monitorW, baseH, rgb(0x15171C))
	c.rect(x0, 0, 2, baseH, rgb(0x2E323B))
	m := g.m
	dim, ink, hi := rgb(0x7D8596), rgb(0xD6DBE6), rgb(0xFFD25A)
	y := float32(24)
	c.text("MONITOR", fontBold, 14, x0+20, y, ink, -1)
	c.text(m.Mode().String(), fontMono, 12, x0+monitorW-20, y, hi, 1)
	y += 30
	c.text(fmt.Sprintf("ACCUM %02X   PC %02d   SUBR %02d", m.A, m.PC, m.S), fontMono, 13, x0+20, y, ink, -1)
	y += 26
	for r := 0; r < 16; r++ {
		cx := x0 + 20 + float32(r%4)*68
		cy := y + float32(r/4)*20
		col := ink
		if r == machine.RegB {
			col = rgb(0x8FD3FF)
		}
		c.text(fmt.Sprintf("R%X %02X", r, m.R[r]), fontMono, 13, cx, cy, col, -1)
	}
	y += 4*20 + 14
	c.rect(x0+16, y, monitorW-32, 1, rgb(0x2E323B))
	y += 16

	focus := m.PC
	label := "PC"
	if m.WaitingForKey() {
		focus = (m.PC + machine.Steps - 1) % machine.Steps // the INP/INA being executed
	}
	switch m.Mode() {
	case machine.ModeInput, machine.ModeRoll, machine.ModeProgram, machine.ModeAssembler, machine.ModeHex:
		focus, label = m.EditStep(), "STEP"
	}
	c.text("PROGRAM  (> "+label+")", fontBold, 12, x0+20, y, dim, -1)
	y += 20
	starts := listing(&m.Mem)
	idx := 0
	for i, s := range starts {
		if s <= focus {
			idx = i
		}
	}
	const lines = 30
	first := idx - lines/2
	if first > len(starts)-lines {
		first = len(starts) - lines
	}
	if first < 0 {
		first = 0
	}
	for i := first; i < first+lines && i < len(starts); i++ {
		s := starts[i]
		text, n := machine.Disassemble(&m.Mem, s)
		hex := fmt.Sprintf("%02X", m.Mem[s])
		if n == 2 && s+1 < machine.Steps {
			hex += fmt.Sprintf(" %02X", m.Mem[s+1])
		}
		col := dim
		if m.Mem[s] != 0 || (n == 2 && s+1 < machine.Steps && m.Mem[s+1] != 0) {
			col = ink
		}
		mark := " "
		if i == idx {
			mark = ">"
			col = hi
			c.rect(x0+12, y-9, monitorW-24, 18, rgb(0x2A2E38))
		}
		c.text(fmt.Sprintf("%s %02d  %-5s  %s", mark, s, hex, text), fontMono, 13, x0+18, y, col, -1)
		y += 19
	}
}

func (g *game) drawPanel(c canvas, title string, w, h float32) (float32, float32) {
	c.rect(0, 0, g.logicalW(), baseH, rgba(0x000000, 0xB0))
	x := (baseW - w) / 2
	y := (baseH - h) / 2
	c.rrect(x, y, w, h, 12, rgb(0xF4F1EA))
	c.text(title, fontBold, 18, x+24, y+30, rgb(0x1A1A1A), -1)
	c.text("Esc or click to close", fontRegular, 12, x+w-24, y+30, rgb(0x777066), 1)
	return x, y
}

const progRowH = 38

func (g *game) programRows() (x, y, w float32) {
	w = 640
	h := float32(70 + progRowH*len(machine.Samples))
	return (baseW - w) / 2, (baseH-h)/2 + 56, w
}

func (g *game) sampleAt(mx, my float32) int {
	x, y, w := g.programRows()
	if mx < x || mx > x+w || my < y {
		return -1
	}
	i := int((my - y) / progRowH)
	if i < len(machine.Samples) {
		return i
	}
	return -1
}

func (g *game) drawPrograms(c canvas) {
	h := float32(70 + progRowH*len(machine.Samples))
	g.drawPanel(c, "Programs from the manual", 640, h)
	x, y, w := g.programRows()
	mx, my := g.cursor()
	hover := g.sampleAt(mx, my)
	for i, s := range machine.Samples {
		ry := y + float32(i)*progRowH
		if i == hover {
			c.rrect(x+12, ry, w-24, progRowH-4, 6, rgb(0xE2DCCD))
		}
		key := string("123456789ABCDEF"[i])
		c.text(key, fontBold, 14, x+30, ry+progRowH/2-2, rgb(0xC0392B), 0)
		c.text(s.Name, fontBold, 14, x+50, ry+12, rgb(0x1A1A1A), -1)
		c.text(s.Usage, fontRegular, 12, x+50, ry+27, rgb(0x5A554C), -1)
	}
}

var helpText = []string{
	"OPERATING THE COMPUTER  (see the manual's back gatefold)",
	"Power on shows SELECT GAME: press 1-4 for the built-in games",
	"   1 Creepy Crawler   2 Flash Card   3 Computer Telepathy   4 Between the Sheets",
	"RESET (▲) → COMMAND.   E execute · C continue after HLT · D display · P program",
	"PROGRAM: A assembler or M hex (machine) → I input · R roll",
	"INPUT: type an instruction (6B or LDV.B.00) and press ENTER.  CLEAR leaves.",
	"ROLL: U / D step up / down, CLEAR returns to PROGRAM mode.",
	"DISPLAY: 0-F registers · P program counter · S subroutine · X accumulator",
	"",
	"PC KEYBOARD",
	"Letters, digits, . ? + - = type directly   * → ×   / → ÷   space → SPACE",
	"[ → YES   ] → NO   Enter → ENTER   Delete → CLEAR   Esc → RESET",
	"Backspace erases the last character while typing an instruction",
	"",
	"EMULATOR",
	"F1 help · F2 save · F3 load · F4 programs · F5 speed · F6 monitor · F7 colours · F8 power",
	"Save/Load use the file given by -file. Drop a listing on the window to load it.",
	"Powering off erases the program, just like the real console.",
}

func (g *game) drawHelp(c canvas) {
	h := float32(70 + 22*len(helpText))
	x, y := g.drawPanel(c, "Help", 700, h)
	for i, line := range helpText {
		kind := fontRegular
		col := rgb(0x2A2722)
		if line == strings.ToUpper(line) && line != "" && !strings.HasPrefix(line, " ") {
			kind, col = fontBold, rgb(0xC0392B)
		}
		c.text(line, kind, 13, x+24, y+64+float32(i)*22, col, -1)
	}
}
