package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"odyssey9/machine"
)

// A file rewritten in place, here because of a hard link, keeps the old
// program when the new one can't be written, such as when the disk is full.
func TestFailedSaveKeepsOldProgram(t *testing.T) {
	g := testGame(t)
	const old = "; my old program\n00 05 06 07\n"
	if err := os.WriteFile(g.file, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(g.file, filepath.Join(t.TempDir(), "other.txt")); err != nil {
		t.Skip("no hard links:", err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	g.save() // asks to replace the program in the file

	// Files can't grow past the limit, as if the disk were full.
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		t.Skip(err)
	}
	small := lim
	small.Cur = uint64(len(old) + 10)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &small); err != nil {
		t.Skip(err)
	}
	g.save()
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(g.toast, "Save failed") {
		t.Fatalf("SAVE past the size limit: toast %q", g.toast)
	}
	if got := mustRead(t, g.file); got != old {
		t.Errorf("failed SAVE lost the old program; file holds:\n%s", got)
	}
}

// SAVE keeps the file's group, though the folder gives new files its own.
func TestSaveKeepsGroup(t *testing.T) {
	groups, err := os.Getgroups()
	if err != nil {
		t.Skip(err)
	}
	other := -1
	for _, gid := range groups {
		if gid != os.Getegid() {
			other = gid
			break
		}
	}
	if other < 0 {
		t.Skip("needs a second group")
	}
	g := testGame(t)
	dir := filepath.Dir(g.file)
	if err := os.Chown(dir, -1, other); err != nil {
		t.Skip(err)
	}
	if err := os.Chmod(dir, 0o755|os.ModeSetgid); err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(g.file, -1, os.Getegid()); err != nil {
		t.Fatal(err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	fi, err := os.Stat(g.file)
	if err != nil {
		t.Fatal(err)
	}
	if gid := int(fi.Sys().(*syscall.Stat_t).Gid); gid != os.Getegid() {
		t.Errorf("SAVE changed the file's group from %d to %d", os.Getegid(), gid)
	}
}
