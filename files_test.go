package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"odyssey9/machine"
)

// saveConfirmed saves, repeating SAVE if it asks to be confirmed.
func saveConfirmed(t *testing.T, g *game) {
	t.Helper()
	g.save()
	if strings.Contains(g.toast, "again") {
		g.save()
	}
	if g.unsaved() {
		t.Fatalf("program still unsaved after SAVE: %q", g.toast)
	}
}

func TestSaveThroughSymlink(t *testing.T) {
	g := testGame(t)
	real := filepath.Join(t.TempDir(), "real.txt")
	if err := os.WriteFile(real, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, g.file); err != nil {
		t.Skip("no symlinks:", err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	if fi, err := os.Lstat(g.file); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("SAVE replaced the symlink: %v", err)
	}
	p, err := machine.ParseProgram(strings.NewReader(mustRead(t, real)))
	if err != nil || p[0] != 0x08 {
		t.Fatalf("SAVE didn't write the link's target: %v", err)
	}
}

func TestSaveKeepsFileMode(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(g.file, 0o600); err != nil {
		t.Fatal(err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	if fi, err := os.Stat(g.file); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode after SAVE = %v, %v; want -rw-------", fi.Mode(), err)
	}
}

func TestSaveAsksBeforeReplacingUnreadableFile(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(g.file, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(g.file); err == nil {
		t.Skip("file still readable without permission (running as root?)")
	}
	typeStep(g, machine.Key0, machine.Key8)
	g.save()
	if !strings.Contains(g.toast, "again") {
		t.Errorf("toast %q doesn't ask to confirm", g.toast)
	}
	if !g.unsaved() {
		t.Fatal("SAVE replaced an unreadable file without asking")
	}
	g.save()
	if g.unsaved() {
		t.Fatalf("second SAVE didn't save: %q", g.toast)
	}
}

// Saving a program that the file already holds must keep the file as it
// is, with its remarks, and count the program as saved.
func TestSaveUnchangedKeepsFile(t *testing.T) {
	g := testGame(t)
	const listing = "; my program\n00 08  pick a random number\n"
	if err := os.WriteFile(g.file, []byte(listing), 0o644); err != nil {
		t.Fatal(err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	if !g.unsaved() {
		t.Fatal("typed program not unsaved")
	}
	g.save()
	if got := mustRead(t, g.file); got != listing {
		t.Errorf("SAVE rewrote an unchanged program:\n%s", got)
	}
	if g.unsaved() {
		t.Errorf("program still unsaved after SAVE: %q", g.toast)
	}
}

func TestLoadMarksProgramSaved(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g.load()
	if g.m.Mem[0] != 0x05 {
		t.Fatalf("LOAD failed: %q", g.toast)
	}
	if g.unsaved() {
		t.Error("loaded program counts as unsaved")
	}
}

func TestSaveThroughDanglingSymlink(t *testing.T) {
	g := testGame(t)
	real := filepath.Join(t.TempDir(), "real.txt")
	if err := os.Symlink(real, g.file); err != nil {
		t.Skip("no symlinks:", err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	if fi, err := os.Lstat(g.file); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("SAVE replaced the symlink: %v", err)
	}
	if _, err := machine.ParseProgram(strings.NewReader(mustRead(t, real))); err != nil {
		t.Fatalf("SAVE didn't create the link's target: %v", err)
	}
}

func TestSaveKeepsHardLinks(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other.txt")
	if err := os.Link(g.file, other); err != nil {
		t.Skip("no hard links:", err)
	}
	fi, err := os.Stat(g.file)
	if err != nil || linkCount(fi) < 2 {
		t.Skip("link count not reported")
	}
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	if mustRead(t, other) != mustRead(t, g.file) {
		t.Error("SAVE broke the hard link")
	}
}

func TestSaveInReadOnlyFolder(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(g.file)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if f, err := os.CreateTemp(dir, "probe"); err == nil {
		f.Close()
		os.Remove(f.Name())
		t.Skip("folder still writable (running as root?)")
	}
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	if p, err := machine.ParseProgram(strings.NewReader(mustRead(t, g.file))); err != nil || p[0] != 0x08 {
		t.Fatalf("SAVE didn't write the file: %v", err)
	}
}

func TestLoadRefusesHugeFile(t *testing.T) {
	g := testGame(t)
	big := strings.Repeat("; padding\n", maxListing/10+1) + "00 05\n"
	if err := os.WriteFile(g.file, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	g.load()
	if g.m.Mem[0] == 0x05 || !strings.Contains(g.toast, "too large") {
		t.Errorf("LOAD of a %d-byte file: toast %q", len(big), g.toast)
	}
}

func TestRecoveryKeepsProgram(t *testing.T) {
	g := testGame(t)
	typeStep(g, machine.Key0, machine.Key8)
	name, err := g.saveRecovery()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(name) != filepath.Dir(g.file) {
		t.Errorf("recovery file %s not next to %s", name, g.file)
	}
	p, err := machine.ParseProgram(strings.NewReader(mustRead(t, name)))
	if err != nil || *p != g.m.Mem {
		t.Errorf("recovery file doesn't hold the program: %v", err)
	}
}
