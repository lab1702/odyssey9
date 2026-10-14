package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

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

// SAVE and LOAD refuse a pipe rather than wait, freezing the window, for
// another program to open its other end.
func TestPipeRefused(t *testing.T) {
	g := testGame(t)
	if err := syscall.Mkfifo(g.file, 0o644); err != nil {
		t.Skip("no pipes:", err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	for name, f := range map[string]func(){"SAVE": g.save, "LOAD": g.load} {
		done := make(chan struct{})
		go func() { f(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s of a pipe hung", name)
		}
		if !strings.Contains(g.toast, "not a plain file") {
			t.Errorf("%s of a pipe: toast %q", name, g.toast)
		}
	}
	if g.m.Mem[0] != 0x08 {
		t.Errorf("program changed to %02X", g.m.Mem[0])
	}
}

// A save through a symlink to a file not yet made, which fails part-way,
// such as when the disk is full, doesn't leave part of the program there.
func TestFailedSaveThroughLinkLeavesNothing(t *testing.T) {
	g := testGame(t)
	target := filepath.Join(t.TempDir(), "target.txt")
	if err := os.Symlink(target, g.file); err != nil {
		t.Skip("no symlinks:", err)
	}
	g.m.LoadProgram(&[machine.Steps]byte{0x05, 0x06, 0x07, 0x08, 0x09, 0xFF})
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		t.Skip(err)
	}
	small := lim
	small.Cur = 10
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
	if _, err := os.Stat(target); err == nil {
		t.Errorf("failed SAVE left part of the program in %s", target)
	}
	if fi, err := os.Lstat(g.file); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("failed SAVE removed the symlink: %v", err)
	}
}

// A new file that can't be written in full isn't left half written.
func TestFailedNewSaveLeavesNothing(t *testing.T) {
	g := testGame(t)
	g.m.LoadProgram(&[machine.Steps]byte{0x05, 0x06, 0x07, 0x08, 0x09, 0xFF})
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		t.Skip(err)
	}
	small := lim
	small.Cur = 10
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
	if _, err := os.Stat(g.file); err == nil {
		t.Errorf("failed SAVE left part of the program in %s", g.file)
	}
}

// SAVE over a file with an access control list rewrites it in place, as
// replacing it would lose the list and give its group the list's mask.
func TestSaveKeepsACL(t *testing.T) {
	setfacl, err := exec.LookPath("setfacl")
	if err != nil {
		t.Skip("no setfacl")
	}
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(setfacl, "-m", "u:nobody:rw", g.file).CombinedOutput(); err != nil {
		t.Skipf("no access control lists here: %v %s", err, out)
	}
	before, err := os.Stat(g.file)
	if err != nil {
		t.Fatal(err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	after, err := os.Stat(g.file)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Errorf("SAVE replaced a file with an access control list")
	}
	if !hasACL(g.file) || after.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("SAVE changed the file's access: mode %v, was %v; list kept %v", after.Mode(), before.Mode(), hasACL(g.file))
	}
	if p, err := machine.ParseProgram(strings.NewReader(mustRead(t, g.file))); err != nil || p[0] != 0x08 {
		t.Errorf("SAVE didn't write the file: %v", err)
	}
}

// A file swapped for a pipe after it was checked is still refused, not
// waited on.
func TestOpenPlainRefusesPipe(t *testing.T) {
	name := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(name, 0o644); err != nil {
		t.Skip("no pipes:", err)
	}
	// Open a reader, so that opening the pipe for writing would succeed,
	// but without nonblock opening it for reading would wait for a writer.
	r, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, flag := range []int{os.O_RDONLY, os.O_WRONLY} {
		done := make(chan error)
		go func() {
			// openChecked, as if the file were swapped after openPlain
			// checked it
			f, err := openChecked(name, flag, 0)
			if err == nil {
				f.Close()
			}
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("flag %#x: pipe accepted", flag)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("flag %#x: opening a pipe hung", flag)
		}
	}
}
