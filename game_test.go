package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func mustRead(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
