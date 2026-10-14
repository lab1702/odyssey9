package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"odyssey9/machine"
)

func testGame(t *testing.T) *game {
	t.Helper()
	return newGame(machine.New(), filepath.Join(t.TempDir(), "prog.txt"), false)
}

// typeStep enters a program step in Hex Input mode, so memory holds unsaved work.
func typeStep(g *game, keys ...machine.Key) {
	for _, k := range append([]machine.Key{machine.KeyReset, machine.KeyP, machine.KeyM, machine.KeyI}, keys...) {
		g.press(k)
	}
	g.press(machine.KeyEnter)
	tick(g)
}

// tick runs the console for one tick, as Update does.
func tick(g *game) {
	g.runConsole(time.Second / 60)
}

func TestKeysTypedTogetherKeepASafeOrder(t *testing.T) {
	g := testGame(t)
	typeStep(g, machine.Key0, machine.Key8)
	for _, k := range []machine.Key{machine.KeyReset, machine.KeyP, machine.KeyM, machine.KeyI, machine.Key6, machine.KeyB} {
		g.press(k)
	}
	tick(g)
	// ENTER and the next digit typed within one tick
	for _, k := range hostKeys([]rune{'0'}, []ebiten.Key{ebiten.KeyEnter}, 0) {
		g.press(k)
	}
	tick(g)
	if g.m.Mem[0] != 0x6B {
		t.Errorf("step 00 = %02X, want 6B", g.m.Mem[0])
	}
	if got := g.m.Status(); got != "STEP 01 0" {
		t.Errorf("status %q, want %q", got, "STEP 01 0")
	}
}

func TestRunningProgramGetsEveryKey(t *testing.T) {
	g := testGame(t)
	g.m.LoadProgram(&[machine.Steps]byte{0x70, 0x71, 0xFF}) // INP.0 INP.1 HLT
	for _, k := range []machine.Key{machine.KeyE, machine.Key5, machine.Key6} {
		g.press(k) // all in one tick
	}
	for range 10 {
		tick(g)
	}
	if g.m.R[0] != machine.Key5.Code() || g.m.R[1] != machine.Key6.Code() {
		t.Errorf("registers 0, 1 = %02X, %02X; want the codes of 5 and 6", g.m.R[0], g.m.R[1])
	}
	if g.m.Mode() != machine.ModeCommand {
		t.Errorf("mode %v, want the program to have stopped", g.m.Mode())
	}

	// Keys wait for an INP, but RESET doesn't.
	g.m.LoadProgram(&[machine.Steps]byte{0x12, 0x00}) // GTO.00
	g.press(machine.KeyE)
	tick(g)
	for _, k := range []machine.Key{machine.Key5, machine.KeyReset, machine.KeyD} {
		g.press(k)
	}
	tick(g)
	if g.m.Mode() != machine.ModeDisplay || len(g.keys) != 0 {
		t.Errorf("after 5 RESET D: mode %v, %d keys queued; want DISPLAY and none", g.m.Mode(), len(g.keys))
	}

	// RESET still stops the program when the queue is full.
	g.press(machine.KeyReset)
	g.press(machine.KeyE)
	tick(g)
	if g.m.Mode() != machine.ModeExecute {
		t.Fatalf("mode %v, want the program running", g.m.Mode())
	}
	for range maxQueued + 4 {
		g.press(machine.KeyX)
	}
	tick(g)
	g.press(machine.KeyReset)
	tick(g)
	if g.m.Mode() != machine.ModeCommand || len(g.keys) != 0 {
		t.Errorf("RESET after a full queue: mode %v, %d keys queued; want COMMAND and none", g.m.Mode(), len(g.keys))
	}
}

// Keys left waiting for a program when the power goes off don't count
// against the first key typed after it comes back on.
func TestPowerOffDropsWaitingKeys(t *testing.T) {
	g := testGame(t)
	g.m.LoadProgram(&[machine.Steps]byte{0x12, 0x00}) // GTO.00
	g.clean = g.m.Mem
	g.press(machine.KeyE)
	tick(g)
	g.press(machine.Key5) // waits, as the program never reads a key
	tick(g)
	g.togglePower()
	g.togglePower()
	g.press(machine.KeyReset)
	tick(g)
	if g.m.Mode() != machine.ModeCommand {
		t.Errorf("mode %v after power on and RESET; want COMMAND", g.m.Mode())
	}
}

func TestScriptChoosesProgram(t *testing.T) {
	g := testGame(t)
	keys, err := parseScript("{PROGRAMS}C")
	if err != nil {
		t.Fatal(err)
	}
	g.script = keys
	for range 20 {
		g.frame++
		g.runScript()
		tick(g)
	}
	want := machine.Samples[strings.IndexByte(sampleKeys, 'C')]
	if g.m.Mem != *want.Program() || g.overlay != noPanel {
		t.Errorf("{PROGRAMS}C didn't load %s and close the panel", want.Name)
	}
}

func TestCloseAsksBeforeLosingWork(t *testing.T) {
	g := testGame(t)
	if !g.closeWindow() {
		t.Fatal("closing with nothing unsaved needed confirming")
	}
	typeStep(g, machine.Key0, machine.Key8)
	if g.closeWindow() {
		t.Fatal("closing lost unsaved work without asking")
	}
	if !g.closeWindow() {
		t.Fatal("second close didn't quit")
	}
}

func TestSaveAsksBeforeReplacingAnotherProgram(t *testing.T) {
	g := testGame(t)
	typeStep(g, machine.Key0, machine.Key8) // RND
	g.save()                                // no file yet: saves at once
	if g.unsaved() {
		t.Fatal("program still unsaved after SAVE")
	}
	first, _ := os.ReadFile(g.file)

	typeStep(g, machine.Key0, machine.Key9) // MOV over step 00
	g.save()
	if now, _ := os.ReadFile(g.file); string(now) != string(first) {
		t.Fatal("SAVE replaced a different program without asking")
	}
	if !strings.Contains(g.toast, "again") {
		t.Errorf("toast %q doesn't ask to confirm", g.toast)
	}
	g.save()
	p, err := machine.ParseProgram(strings.NewReader(mustRead(t, g.file)))
	if err != nil || p[0] != 0x09 {
		t.Fatalf("second SAVE didn't write the program: %v", err)
	}
	if left, _ := filepath.Glob(g.file + ".*.tmp"); len(left) > 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
}

func TestLoadAndPowerAskBeforeLosingWork(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	typeStep(g, machine.Key0, machine.Key8)

	g.load()
	if g.m.Mem[0] != 0x08 {
		t.Fatal("LOAD replaced unsaved work without asking")
	}
	g.togglePower() // a different action cancels the pending LOAD
	g.load()
	if g.m.Mem[0] != 0x08 {
		t.Fatal("LOAD confirmed by a different action")
	}
	g.load()
	if g.m.Mem[0] != 0x05 {
		t.Fatal("second LOAD didn't load")
	}

	typeStep(g, machine.Key0, machine.Key8)
	g.togglePower()
	if !g.power {
		t.Fatal("POWER switched off unsaved work without asking")
	}
	g.togglePower()
	if g.power || g.m.Mem != [machine.Steps]byte{} {
		t.Fatal("power off didn't erase the program")
	}
	g.togglePower()
	if !g.power {
		t.Fatal("power on with an empty program needed confirming")
	}
}

func TestParseScriptErrors(t *testing.T) {
	keys, err := parseScript("{reset}PMI6B{ENTER}{WAIT}")
	if err != nil || len(keys) != 8 {
		t.Errorf("valid script: %v, %v", keys, err)
	}
	for _, bad := range []string{"{ENTR}", "{RESET", "PMI→"} {
		if _, err := parseScript(bad); err == nil {
			t.Errorf("%q parsed without error", bad)
		}
	}
}

// The help panel and the README are written by hand, so check that they
// cover the keys the code actually handles.
func TestKeyDocs(t *testing.T) {
	readme := mustRead(t, "README.md")
	readmePC := between(t, readme, "### PC keyboard", "### Emulator controls")
	help := strings.Join(helpText, "\n")
	helpPC := between(t, help, "PC KEYBOARD", "EMULATOR")

	for k, id := range fnKeys {
		if want := fmt.Sprintf("**%s (%s)**", id, k); !strings.Contains(readme, want) {
			t.Errorf("README doesn't describe %s", want)
		}
		if want := fmt.Sprintf("%s %s", k, strings.ToLower(string(id))); !strings.Contains(help, want) {
			t.Errorf("help doesn't mention %q", want)
		}
	}
	if !strings.Contains(readme, "**F7** colours") || !strings.Contains(help, "F7 colours") {
		t.Errorf("README or help doesn't describe F7")
	}

	for r := range typedKeys {
		inReadme, inHelp := fmt.Sprintf("`%c`", r), string(r)
		if r == ' ' {
			inReadme, inHelp = "space", "space"
		}
		if !strings.Contains(readmePC, inReadme) {
			t.Errorf("README's PC keyboard table doesn't mention %s", inReadme)
		}
		if !strings.Contains(helpPC, inHelp) {
			t.Errorf("help's PC keyboard lines don't mention %q", inHelp)
		}
	}
}

// between returns the part of s from the line starting with from up to the
// line starting with to.
func between(t *testing.T, s, from, to string) string {
	t.Helper()
	i := strings.Index(s, from)
	j := strings.Index(s, to)
	if i < 0 || j < i {
		t.Fatalf("can't find the section from %q to %q", from, to)
	}
	return s[i:j]
}

func mustRead(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestShortcutsDontType(t *testing.T) {
	cases := []struct {
		goos string
		held []ebiten.Key
		want bool
	}{
		{"linux", nil, false},
		{"linux", []ebiten.Key{ebiten.KeyShift}, false},
		{"linux", []ebiten.Key{ebiten.KeyControl}, true},
		{"windows", []ebiten.Key{ebiten.KeyAlt}, true},
		{"windows", []ebiten.Key{ebiten.KeyControl, ebiten.KeyAlt, ebiten.KeyAltRight}, false}, // AltGr
		{"linux", []ebiten.Key{ebiten.KeyAlt, ebiten.KeyAltRight}, false},                      // AltGr
		{"linux", []ebiten.Key{ebiten.KeyMeta}, true},
		{"darwin", []ebiten.Key{ebiten.KeyMeta}, true}, // Cmd
		{"darwin", []ebiten.Key{ebiten.KeyControl}, true},
		{"darwin", []ebiten.Key{ebiten.KeyAlt}, false}, // Option types characters
	}
	for _, c := range cases {
		held := func(k ebiten.Key) bool { return slices.Contains(c.held, k) }
		if got := shortcutHeld(c.goos, held); got != c.want {
			t.Errorf("%s with %v held: shortcut = %v, want %v", c.goos, c.held, got, c.want)
		}
	}
}

func TestMonitorWindowWidth(t *testing.T) {
	const open = baseW + monitorW
	cases := []struct {
		name       string
		ww, wh     int
		oldW, newW float32
		want       int
	}{
		{"open, picture fills the width", baseW, baseH, baseW, open, open},
		{"close, picture fills the width", open, baseH, open, baseW, baseW},
		{"open at double size", 2 * baseW, 2 * baseH, baseW, open, 2 * open},
		{"open in a wide window", 2000, baseH, baseW, open, 2000},
		{"close in a wide window", 2000, baseH, open, baseW, 2000},
		{"open in a slightly wide window", 1000, baseH, baseW, open, open},
	}
	for _, c := range cases {
		if got := monitorWindowWidth(c.ww, c.wh, c.oldW, c.newW); got != c.want {
			t.Errorf("%s: width %d, want %d", c.name, got, c.want)
		}
	}
}

// The picture must never come out larger than the window, which would make
// the whole frame be scaled down and blur.
func TestScreenFitsWindow(t *testing.T) {
	for _, dsf := range []float64{1, 2} {
		for _, lw := range []float64{baseW, baseW + monitorW} {
			for ow := 400; ow <= 2600; ow += 7 {
				for oh := 400; oh <= 2600; oh += 3 {
					w, h, _ := screenSize(lw, float64(ow), float64(oh), dsf)
					if w > float64(ow)*dsf || h > float64(oh)*dsf {
						t.Fatalf("window %d×%d at scale %g: picture %g×%g", ow, oh, dsf, w, h)
					}
				}
			}
		}
	}
}
