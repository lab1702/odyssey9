package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

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
	// Windows only reports whether a file is read-only, so compare with
	// what is reported rather than with 0600.
	before, err := os.Stat(g.file)
	if err != nil {
		t.Fatal(err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	if fi, err := os.Stat(g.file); err != nil || fi.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("mode after SAVE = %v, %v; want %v", fi.Mode(), err, before.Mode())
	}
}

func TestSaveAsksBeforeReplacingUnreadableFile(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(g.file, 0o200); err != nil { // write-only
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

// A relative link's ".." leaves the folder the link is really in, even when
// that folder was reached through another link.
func TestSaveThroughDanglingSymlinkInLinkedFolder(t *testing.T) {
	root := t.TempDir()
	progs := filepath.Join(root, "data", "progs")
	for _, dir := range []string{progs, filepath.Join(root, "data", "other"), filepath.Join(root, "home", "other")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	linkdir := filepath.Join(root, "home", "linkdir")
	if err := os.Symlink(progs, linkdir); err != nil {
		t.Skip("no symlinks:", err)
	}
	if err := os.Symlink(filepath.Join("..", "other", "x.txt"), filepath.Join(progs, "prog.txt")); err != nil {
		t.Fatal(err)
	}
	g := newGame(machine.New(), filepath.Join(linkdir, "prog.txt"), false)
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	if _, err := os.Stat(filepath.Join(root, "home", "other", "x.txt")); err == nil {
		t.Error("SAVE wrote to the wrong folder")
	}
	if _, err := machine.ParseProgram(strings.NewReader(mustRead(t, filepath.Join(root, "data", "other", "x.txt")))); err != nil {
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
	if err != nil || replaceable(g.file, fi) {
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

func TestSaveRefusesReadOnlyFile(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if f, err := os.OpenFile(g.file, os.O_WRONLY, 0); err == nil {
		f.Close()
		t.Skip("read-only file still writable (running as root?)")
	}
	typeStep(g, machine.Key0, machine.Key8)
	g.save()
	if !strings.HasPrefix(g.toast, "Save failed") || !g.unsaved() {
		t.Errorf("SAVE over a read-only file: toast %q", g.toast)
	}
	if got := mustRead(t, g.file); got != "00 05\n" {
		t.Errorf("read-only file was replaced:\n%s", got)
	}
}

// SAVE of a program a read-only file already holds says so, rather than
// that it can't write the file.
func TestSaveUnchangedReadOnlyFile(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 08\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	g.save()
	if !strings.HasPrefix(g.toast, "Already saved") || g.unsaved() {
		t.Errorf("SAVE of the program in a read-only file: toast %q", g.toast)
	}
}

// Windows calls files in OneDrive and other cloud-synced folders
// irregular, but they are plain files that read like any other.
func TestIrregularFileIsPlain(t *testing.T) {
	if err := checkPlain(fileInfo{mode: 0o644 | fs.ModeIrregular}); err != nil {
		t.Errorf("irregular file refused: %v", err)
	}
	for _, m := range []fs.FileMode{fs.ModeNamedPipe, fs.ModeSocket, fs.ModeDevice, fs.ModeDevice | fs.ModeCharDevice} {
		if err := checkPlain(fileInfo{mode: 0o644 | m}); err != errSpecialFile {
			t.Errorf("mode %v: checkPlain = %v, want errSpecialFile", m, err)
		}
	}
}

// fileInfo is an fs.FileInfo with only a mode.
type fileInfo struct{ mode fs.FileMode }

func (fi fileInfo) Name() string       { return "prog.txt" }
func (fi fileInfo) Size() int64        { return 0 }
func (fi fileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi fileInfo) ModTime() time.Time { return time.Time{} }
func (fi fileInfo) IsDir() bool        { return fi.mode.IsDir() }
func (fi fileInfo) Sys() any           { return nil }

// The system follows a link's ".." from wherever the links before it lead,
// so SAVE must create the file there, where LOAD will look for it.
func TestSaveThroughDanglingSymlinkPastLinkedFolder(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "deep", "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("deep", "a", "b"), filepath.Join(root, "sub")); err != nil {
		t.Skip("no symlinks:", err)
	}
	file := filepath.Join(root, "prog.txt")
	// Not filepath.Join, which would drop "sub/..".
	sep := string(filepath.Separator)
	if err := os.Symlink("sub"+sep+".."+sep+"out.txt", file); err != nil {
		t.Fatal(err)
	}
	g := newGame(machine.New(), file, false)
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	if _, err := os.Stat(filepath.Join(root, "out.txt")); err == nil {
		t.Error("SAVE wrote to the wrong folder")
	}
	if p, err := machine.ParseProgram(strings.NewReader(mustRead(t, filepath.Join(root, "deep", "a", "out.txt")))); err != nil || p[0] != 0x08 {
		t.Fatalf("SAVE didn't create the link's target: %v", err)
	}
}

func TestRecoveryInReadOnlyFolder(t *testing.T) {
	g := testGame(t)
	typeStep(g, machine.Key0, machine.Key8)
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
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	name, err := g.saveRecovery()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(name) != tmp {
		t.Errorf("recovery file %s not in the temporary folder %s", name, tmp)
	}
	p, err := machine.ParseProgram(strings.NewReader(mustRead(t, name)))
	if err != nil || *p != g.m.Mem {
		t.Errorf("recovery file doesn't hold the program: %v", err)
	}
}

// A file that can't be replaced by renaming for any reason, such as being
// a mount point or open in another program, is rewritten in place.
func TestSaveWhenRenameFails(t *testing.T) {
	g := testGame(t)
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func(r func(string, string) error) { rename = r }(rename)
	rename = func(string, string) error { return &os.LinkError{Op: "rename", Err: syscall.EBUSY} }
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	if p, err := machine.ParseProgram(strings.NewReader(mustRead(t, g.file))); err != nil || p[0] != 0x08 {
		t.Errorf("SAVE didn't write the file: %v", err)
	}
	if tmp, _ := filepath.Glob(filepath.Join(filepath.Dir(g.file), tempPattern)); len(tmp) > 0 {
		t.Errorf("SAVE left %v", tmp)
	}
}

// SAVE removes temporary files that earlier saves left behind, once they
// are old, and nothing else, even files named the way some other programs
// name theirs.
func TestSaveRemovesStaleTempFiles(t *testing.T) {
	g := testGame(t)
	dir := filepath.Dir(g.file)
	old := time.Now().Add(-2 * staleAge)
	files := map[string]bool{ // name: whether SAVE should remove it
		".odyssey9-save-123456.tmp": true,
		".odyssey9-save-789.tmp":    false, // too new: perhaps a save being made
		".odyssey9-save-abc.tmp":    false,
		".odyssey9-save-.tmp":       false,
		"odyssey9-save-123.tmp":     false,
		"prog.txt.123456.tmp":       false,
		"123.tmp":                   false,
	}
	for name := range files {
		n := filepath.Join(dir, name)
		if err := os.WriteFile(n, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if name != ".odyssey9-save-789.tmp" {
			if err := os.Chtimes(n, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	for name, gone := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		if exists := err == nil; exists == gone {
			t.Errorf("%s: exists %v after SAVE", name, exists)
		}
	}
}

// SAVE works over a file whose name is as long as the system allows, too
// long to add anything to for a temporary file's name.
func TestSaveLongName(t *testing.T) {
	g := testGame(t)
	g.file = filepath.Join(filepath.Dir(g.file), strings.Repeat("p", 251)+".txt")
	if err := os.WriteFile(g.file, []byte("00 05\n"), 0o644); err != nil {
		t.Skip("no file names that long:", err)
	}
	typeStep(g, machine.Key0, machine.Key8)
	saveConfirmed(t, g)
	if p, err := machine.ParseProgram(strings.NewReader(mustRead(t, g.file))); err != nil || p[0] != 0x08 {
		t.Errorf("SAVE didn't write the file: %v", err)
	}
}
