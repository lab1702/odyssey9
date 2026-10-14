package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	var kb hostKeyboard
	for _, k := range kb.next(keyTick{chars: "0", pressed: []ebiten.Key{ebiten.KeyEnter, ebiten.KeyDigit0}}.input()) {
		g.press(k.k)
	}
	tick(g)
	if g.m.Mem[0] != 0x6B {
		t.Errorf("step 00 = %02X, want 6B", g.m.Mem[0])
	}
	if got := g.m.Status(); got != "STEP 01 0" {
		t.Errorf("status %q, want %q", got, "STEP 01 0")
	}
}

// keyTick is one tick of host keyboard input.
type keyTick struct {
	chars     string
	pressed   []ebiten.Key // keys just pressed
	held      []ebiten.Key // keys held down since an earlier tick
	sameFrame bool         // in the same frame as the tick before, so no new input
}

func (k keyTick) input() keyInput {
	return keyInput{
		chars:   []rune(k.chars),
		pressed: k.pressed,
		down:    append(slices.Clone(k.held), k.pressed...),
		newPoll: !k.sameFrame,
	}
}

// typed returns the keys a hostKeyboard makes of ticks, and which of them
// are repeats.
func typed(ticks ...keyTick) (keys []machine.Key, repeats []bool) {
	var kb hostKeyboard
	for _, k := range ticks {
		for _, hk := range kb.next(k.input()) {
			keys, repeats = append(keys, hk.k), append(repeats, hk.repeat)
		}
	}
	return keys, repeats
}

var (
	keyD     = []ebiten.Key{ebiten.KeyD}
	keyE     = []ebiten.Key{ebiten.KeyE}
	keyEnter = []ebiten.Key{ebiten.KeyEnter}
	key5     = []ebiten.Key{ebiten.KeyDigit5}
	sameTick = keyTick{sameFrame: true}
)

// With an input method, a key's character comes the next time Ebitengine
// reads the system's input, which can be several ticks later, and after
// Enter has been pressed. It must still come before Enter. A character
// typed in the same tick as Esc or Delete comes before them.
func TestLateCharacterKeepsItsPlace(t *testing.T) {
	esc := []ebiten.Key{ebiten.KeyEscape, ebiten.KeyE}
	for _, c := range []struct {
		ticks []keyTick
		want  []machine.Key
	}{
		{[]keyTick{{pressed: key5}, {chars: "5", pressed: keyEnter}}, []machine.Key{machine.Key5, machine.KeyEnter}},
		{[]keyTick{{pressed: key5}, {pressed: keyEnter}, {chars: "5"}}, []machine.Key{machine.Key5, machine.KeyEnter}},
		// three ticks a frame, as at 20 frames a second
		{[]keyTick{{pressed: key5}, sameTick, sameTick, {pressed: keyEnter}, sameTick, sameTick, {chars: "5"}},
			[]machine.Key{machine.Key5, machine.KeyEnter}},
		{[]keyTick{{chars: "E", pressed: esc}}, []machine.Key{machine.KeyE, machine.KeyReset}},
		{[]keyTick{{pressed: esc}, {chars: "E"}}, []machine.Key{machine.KeyE, machine.KeyReset}},
	} {
		if got, _ := typed(c.ticks...); !slices.Equal(got, c.want) {
			t.Errorf("%+v: typed %v, want %v", c.ticks, got, c.want)
		}
	}
	// A key that types nothing, such as a dead key, holds Enter up only
	// briefly.
	for _, frame := range []int{1, 3} {
		ticks := []keyTick{{pressed: key5}, {pressed: keyEnter}}
		for range (unknownLag + 2) * frame {
			ticks = append(ticks, keyTick{sameFrame: len(ticks)%frame != 0})
		}
		if got, _ := typed(ticks...); !slices.Equal(got, []machine.Key{machine.KeyEnter}) {
			t.Errorf("%d ticks a frame: Enter after a key that typed nothing: typed %v", frame, got)
		}
	}
}

// onTime returns ticks of n keys typed alone, their characters coming with
// them, as without an input method.
func onTime(n int) []keyTick {
	var ticks []keyTick
	for range n {
		ticks = append(ticks, keyTick{chars: "A", pressed: []ebiten.Key{ebiten.KeyA}}, keyTick{})
	}
	return ticks
}

// How late characters come is learned from keys pressed alone, even when
// a character comes later than expected.
func TestCharacterLagLearned(t *testing.T) {
	for _, c := range []struct {
		ticks []keyTick
		want  int
	}{
		{onTime(onTimeToKnow - 1), unknownLag},
		{onTime(onTimeToKnow), 0},
		{[]keyTick{{pressed: keyE}, {chars: "E", held: keyE}}, 1},
		{append(onTime(onTimeToKnow), keyTick{pressed: keyE}, keyTick{chars: "E"}), 1},
		{append(onTime(1), keyTick{pressed: keyE}, keyTick{}, keyTick{chars: "E"}), 2},
	} {
		var kb hostKeyboard
		for _, k := range c.ticks {
			kb.next(k.input())
		}
		if got := kb.lag(); got != c.want {
			t.Errorf("%+v: lag %d, want %d", c.ticks, got, c.want)
		}
	}
}

// Once characters are seen to come with their keys, as without an input
// method, a key that types nothing stops waiting at the next input, so it
// doesn't take the next key's character, and Enter typed after that keeps
// its place. Three ticks a frame, as at 20 frames a second.
func TestDeadKeyKeepsOrder(t *testing.T) {
	ticks := onTime(onTimeToKnow)
	frame := func(k keyTick) {
		ticks = append(ticks, k, sameTick, sameTick)
	}
	frame(keyTick{pressed: []ebiten.Key{ebiten.KeyQuote}}) // a dead key
	frame(keyTick{chars: "5", pressed: key5})
	frame(keyTick{pressed: keyEnter})
	frame(keyTick{chars: "0", pressed: []ebiten.Key{ebiten.KeyDigit0}})
	frame(keyTick{})
	want := []machine.Key{machine.Key5, machine.KeyEnter, machine.Key0}
	if got, _ := typed(ticks...); !slices.Equal(got[onTimeToKnow:], want) {
		t.Errorf("typed %v, want %v", got, want)
	}
}

// Two keys pressed in one tick don't leave a later press of one of them
// taken as a repeat. Four ticks a frame, as at 15 frames a second, with
// the space bar held throughout.
func TestKeysPressedTogetherThenAgain(t *testing.T) {
	var ticks []keyTick
	space := ebiten.KeySpace
	frame := func(k keyTick) {
		k.held = append(k.held, space)
		ticks = append(ticks, k, sameTick, sameTick, sameTick)
	}
	ticks = append(ticks, keyTick{chars: " ", pressed: []ebiten.Key{space}})
	frame(keyTick{})
	// 1 then M: Ebitengine lists M first
	frame(keyTick{chars: "1M", pressed: []ebiten.Key{ebiten.KeyM, ebiten.KeyDigit1}})
	frame(keyTick{})
	frame(keyTick{chars: "M", pressed: []ebiten.Key{ebiten.KeyM}})
	frame(keyTick{pressed: keyEnter, held: []ebiten.Key{ebiten.KeyM}})
	frame(keyTick{chars: "0", pressed: []ebiten.Key{ebiten.KeyDigit0}})
	for range 5 {
		frame(keyTick{})
	}
	keys, repeats := typed(ticks...)
	want := []machine.Key{machine.KeySpace, machine.Key1, machine.KeyM, machine.KeyM, machine.KeyEnter, machine.Key0}
	if !slices.Equal(keys, want) || slices.Contains(repeats, true) {
		t.Errorf("typed %v, repeats %v, want %v and none", keys, repeats, want)
	}
}

// A held key repeating is told apart from presses, even when a press's
// character comes late, or another key's press comes in the same tick.
func TestRepeatsToldApart(t *testing.T) {
	for _, c := range []struct {
		ticks []keyTick
		want  []bool
	}{
		{[]keyTick{{chars: "E", pressed: keyE}, {chars: "E", held: keyE}}, []bool{false, true}},
		{[]keyTick{{pressed: keyE}, {chars: "E", held: keyE}, {chars: "E", held: keyE}}, []bool{false, true}},
		// D pressed while E repeats
		{[]keyTick{{chars: "E", pressed: keyE}, {chars: "E", held: keyE}, {chars: "ED", pressed: keyD, held: keyE}},
			[]bool{false, true, true, false}},
		// and with an input method
		{[]keyTick{{pressed: keyE}, {chars: "E", held: keyE}, {chars: "E", held: keyE}, {chars: "E", pressed: keyD, held: keyE}, {chars: "D", held: keyD}},
			[]bool{false, true, true, false}},
		// E pressed again, its character late
		{[]keyTick{{pressed: keyE}, {chars: "E", held: keyE}, {}, {pressed: keyE}, {chars: "E", held: keyE}}, []bool{false, false}},
		// a key's last repeat, read with its release
		{[]keyTick{{chars: "E", pressed: keyE}, {chars: "E", held: keyE}, {chars: "E"}}, []bool{false, true, true}},
		// two keys pressed in one tick, the second held
		{[]keyTick{{chars: "ED", pressed: []ebiten.Key{ebiten.KeyD, ebiten.KeyE}}, {chars: "D", held: keyD}},
			[]bool{false, false, true}},
		// Shift let go while E repeats
		{[]keyTick{{chars: "E", pressed: keyE, held: []ebiten.Key{ebiten.KeyShiftLeft}}, {chars: "E", held: keyE}, {chars: "e", held: keyE}},
			[]bool{false, true, true}},
		// 2 pressed while 1 repeats, the last repeat coming late with 2's character
		{[]keyTick{{pressed: []ebiten.Key{ebiten.KeyDigit1}}, {chars: "1", held: []ebiten.Key{ebiten.KeyDigit1}}, {chars: "1", held: []ebiten.Key{ebiten.KeyDigit1}},
			{pressed: []ebiten.Key{ebiten.KeyDigit2}}, {chars: "12", held: []ebiten.Key{ebiten.KeyDigit2}}},
			[]bool{false, true, true, false}},
		// a key Ebitengine doesn't know types a character too
		{[]keyTick{{chars: "?"}}, []bool{false}},
	} {
		if _, got := typed(c.ticks...); !slices.Equal(got, c.want) {
			t.Errorf("%+v: repeats %v, want %v", c.ticks, got, c.want)
		}
	}
}

// typeHost types one tick of host keyboard input.
func (g *game) typeHost(k keyTick) {
	g.typeHostKeys(g.kb.next(k.input()))
}

// With an input method, the PROGRAMS panel still takes a program's key,
// whose character comes after its press.
func TestProgramsPanelTakesLateCharacter(t *testing.T) {
	g := testGame(t)
	g.overlay = programsPanel
	g.typeHost(keyTick{pressed: []ebiten.Key{ebiten.KeyDigit3}})
	g.typeHost(sameTick)
	g.typeHost(keyTick{chars: "3", held: []ebiten.Key{ebiten.KeyDigit3}})
	if want := machine.Samples[2]; g.m.Mem != *want.Program() || g.overlay != noPanel {
		t.Errorf("3 in the PROGRAMS panel didn't load %s and close the panel", want.Name)
	}
}

// A program's key held down in the PROGRAMS panel doesn't confirm the
// choice by repeating, even with another key's press, nor type at the
// console once it has loaded the program.
func TestHeldKeyStaysInPanel(t *testing.T) {
	g := testGame(t)
	typeStep(g, machine.Key0, machine.Key8)
	g.overlay = programsPanel
	hold := func(held []ebiten.Key) {
		for range 10 {
			g.typeHost(keyTick{chars: "E", held: held})
			tick(g)
		}
	}
	g.typeHost(keyTick{chars: "E", pressed: keyE})
	hold(keyE)
	g.typeHost(keyTick{chars: "EG", pressed: []ebiten.Key{ebiten.KeyG}, held: keyE})
	if !g.unsaved() || g.overlay != programsPanel {
		t.Fatal("E held down replaced an unsaved program without a second press")
	}
	g.typeHost(keyTick{chars: "E", pressed: keyE})
	hold(keyE)
	want := machine.Samples[strings.IndexByte(sampleKeys, 'E')]
	if g.m.Mem != *want.Program() || g.overlay != noPanel {
		t.Fatalf("second E didn't load %s", want.Name)
	}
	if g.m.Mode() != machine.ModeCommand || len(g.keys) > 0 {
		t.Errorf("E held down typed at the console: mode %v, %d keys queued", g.m.Mode(), len(g.keys))
	}
	g.typeHost(keyTick{chars: "E", pressed: keyE})
	g.typeHost(keyTick{chars: "E", held: keyE})
	if len(g.keys) != 2 {
		t.Errorf("E pressed and held after the panel closed: %d keys queued, want 2", len(g.keys))
	}
}

// With an input method, a held program key's last repeat, coming with the
// press of the next key as the held one is let go, doesn't confirm the
// choice, nor type at the console once the key has closed the panel.
func TestLastRepeatWithNextPress(t *testing.T) {
	hold := func(g *game, k ebiten.Key, chars string) {
		for range 29 {
			g.typeHost(keyTick{held: []ebiten.Key{k}})
		}
		for range 5 { // 30 repeats a second, coming a poll late
			g.typeHost(keyTick{chars: chars, held: []ebiten.Key{k}})
			g.typeHost(keyTick{held: []ebiten.Key{k}})
		}
	}
	g := testGame(t)
	typeStep(g, machine.Key0, machine.Key8)
	g.overlay = programsPanel
	g.typeHost(keyTick{pressed: keyE})
	g.typeHost(keyTick{chars: "E", held: keyE})
	hold(g, ebiten.KeyE, "E")
	g.typeHost(keyTick{chars: "E", pressed: []ebiten.Key{ebiten.KeyG}})
	g.typeHost(keyTick{chars: "G", held: []ebiten.Key{ebiten.KeyG}})
	if !g.unsaved() || g.overlay != programsPanel {
		t.Errorf("E's last repeat with G's press replaced the unsaved program")
	}

	g = testGame(t)
	g.overlay = programsPanel
	three := []ebiten.Key{ebiten.KeyDigit3}
	g.typeHost(keyTick{pressed: three})
	g.typeHost(keyTick{chars: "3", held: three})
	hold(g, ebiten.KeyDigit3, "3")
	g.typeHost(keyTick{chars: "3", pressed: []ebiten.Key{ebiten.KeyR}})
	g.typeHost(keyTick{chars: "R", held: []ebiten.Key{ebiten.KeyR}})
	if !slices.Equal(g.keys, []machine.Key{machine.KeyR}) {
		t.Errorf("3 held after closing the panel, then R: typed %v, want [R]", g.keys)
	}
}

// After Esc closes a panel, a key typed with an input method types.
func TestTypingAfterPanelCloses(t *testing.T) {
	g := testGame(t)
	g.overlay = programsPanel
	g.typeHost(keyTick{pressed: []ebiten.Key{ebiten.KeyEscape}})
	g.typeHost(keyTick{pressed: []ebiten.Key{ebiten.KeyR}})
	g.typeHost(keyTick{chars: "R", held: []ebiten.Key{ebiten.KeyR}})
	if g.overlay != noPanel || !slices.Equal(g.keys, []machine.Key{machine.KeyR}) {
		t.Errorf("R typed after Esc closed the panel: panel %q, keys %v", g.overlay, g.keys)
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
// RESET pressed when the queue is full is passed on even if the program
// has just stopped, which drops the keys left waiting for it.
func TestResetWithFullQueue(t *testing.T) {
	g := testGame(t)
	g.m.SetSpeed(0)
	g.m.LoadProgram(&[machine.Steps]byte{0x00, 0xFF}) // a step, then HLT
	g.press(machine.KeyE)
	tick(g)
	for range maxQueued {
		g.press(machine.KeyX)
	}
	for i := 0; g.m.Mode() == machine.ModeExecute; i++ {
		if i > 600 {
			t.Fatal("program didn't stop")
		}
		tick(g)
	}
	if len(g.keys) != maxQueued {
		t.Fatalf("%d keys waiting, want %d", len(g.keys), maxQueued)
	}
	g.press(machine.KeyReset)
	tick(g)
	if got := g.m.Status(); got != "COMMAND" {
		t.Errorf("status %q after RESET, want COMMAND", got)
	}
}

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
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(g.file), tempPattern)); len(left) > 0 {
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

// A confirmation is for the program it was asked about: POWER, or typing,
// before the second SAVE asks again.
func TestConfirmationIsForItsProgram(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g.m.LoadProgram(machine.Samples[0].Program())
	g.clean = g.m.Mem
	g.save() // asks to replace the program in the file
	g.togglePower()
	if g.power || g.toastFor > 0 {
		t.Fatalf("POWER: power %v, toast %q still up", g.power, g.toast)
	}
	g.save()
	if got := mustRead(t, g.file); got != "00 05\n" {
		t.Fatalf("SAVE after POWER erased the program replaced the file without asking:\n%s", got)
	}
	g.togglePower()
	typeStep(g, machine.Key0, machine.Key8)
	g.save()
	typeStep(g, machine.Key0, machine.Key9)
	g.save()
	if got := mustRead(t, g.file); got != "00 05\n" {
		t.Errorf("SAVE confirmed for a program since changed:\n%s", got)
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
		mw         int
		want       int
	}{
		{"open, picture fills the width", baseW, baseH, baseW, open, 0, open},
		{"close, picture fills the width", open, baseH, open, baseW, 0, baseW},
		{"open at double size", 2 * baseW, 2 * baseH, baseW, open, 0, 2 * open},
		{"open in a wide window", 2000, baseH, baseW, open, 0, 2000},
		{"close in a wide window", 2000, baseH, open, baseW, 0, 2000},
		{"open in a slightly wide window", 1000, baseH, baseW, open, 0, open},
		{"open on a wide monitor", baseW, baseH, baseW, open, 1920, open},
		{"open on a narrow monitor", baseW, baseH, baseW, open, 1000, 950},
		{"open in a window wider than the monitor", 1200, baseH, baseW, open, 1000, 1200},
	}
	for _, c := range cases {
		if got := monitorWindowWidth(c.ww, c.wh, c.oldW, c.newW, c.mw); got != c.want {
			t.Errorf("%s: width %d, want %d", c.name, got, c.want)
		}
	}
}

// Closing the monitor gives the window back its width from before it was
// opened, unless the user has resized it since, and toggling it twice before
// the window has resized leaves the window as it was.
func TestFlipMonitor(t *testing.T) {
	g := testGame(t)
	ww, wh := fitWindow(baseW, baseH, 1000, 1600)
	open := g.flipMonitor(ww, wh, 1000)
	if open != 950 {
		t.Errorf("opened at %d, want 950", open)
	}
	if w := g.flipMonitor(open, wh, 1000); w != ww {
		t.Errorf("closed at %d, want %d", w, ww)
	}

	// Resized by the user while open
	g.flipMonitor(baseW, baseH, 0)
	if w := g.flipMonitor(2*(baseW+monitorW), 2*baseH, 0); w != 2*baseW {
		t.Errorf("closed a resized window at %d, want %d", w, 2*baseW)
	}

	// Toggled twice before Layout has caught up with the first resize
	g = testGame(t)
	if w := g.flipMonitor(baseW, baseH, 0); w != baseW+monitorW {
		t.Fatalf("opened at %d, want %d", w, baseW+monitorW)
	}
	g.monSize = [2]int{} // as if the window manager changed the size it was given
	if w := g.flipMonitor(baseW+monitorW, baseH, 0); w != baseW {
		t.Errorf("closed before the resize at %d, want %d", w, baseW)
	}
}

// The first window fits the monitor both ways, keeping the picture's shape.
func TestFitWindow(t *testing.T) {
	const open = baseW + monitorW
	cases := []struct {
		name   string
		w      float32
		mw, mh int
		ww, wh int
	}{
		{"unknown monitor", open, 0, 0, open, baseH},
		{"large monitor", open, 3840, 2160, open, baseH},
		{"short monitor", baseW, 1920, baseH / 2, baseW * 9 / 20, baseH * 9 / 20},
		{"portrait monitor", open, open / 2, 1920, open * 95 / 200, baseH * 95 / 200},
	}
	for _, c := range cases {
		ww, wh := fitWindow(c.w, baseH, c.mw, c.mh)
		if ww > c.ww+1 || ww < c.ww-1 || wh > c.wh+1 || wh < c.wh-1 {
			t.Errorf("%s: window %d×%d, want %d×%d", c.name, ww, wh, c.ww, c.wh)
		}
		if c.mw > 0 && (ww > c.mw || wh > c.mh) {
			t.Errorf("%s: window %d×%d larger than monitor %d×%d", c.name, ww, wh, c.mw, c.mh)
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

// A message too long to fit loses as little of its middle as it must.
func TestShorten(t *testing.T) {
	fits := func(s string) bool { return utf8.RuneCountInString(s) <= 10 }
	for in, want := range map[string]string{
		"short":                   "short",
		"0123456789":              "0123456789",
		"0123456789A":             "0123…6789A",
		strings.Repeat("x", 5000): "xxxx…xxxxx",
	} {
		if got := shorten(in, fits); got != want {
			t.Errorf("shorten(%.20q) = %q, want %q", in, got, want)
		}
	}
	if got := shorten("abc", func(string) bool { return false }); got != "…c" {
		t.Errorf("shorten with no room = %q, want %q", got, "…c")
	}
}
