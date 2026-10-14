package main

// Saving and loading programs.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"odyssey9/machine"
)

// loadBytes loads a listing. If that would replace an unsaved program, the
// user must first repeat the action, which is named by action and prompt.
func (g *game) loadBytes(name string, data []byte, action, prompt string) {
	p, err := machine.ParseProgram(bytes.NewReader(data))
	if err != nil {
		g.showToast(name + ": " + err.Error())
		return
	}
	if !g.replace(p, action, prompt) {
		return
	}
	g.showToast("Loaded " + name + " — press E to execute")
}

// replace puts program p into memory, unless that would lose an unsaved
// program and the user hasn't confirmed it yet.
func (g *game) replace(p *[machine.Steps]byte, action, prompt string) bool {
	if g.unsaved() && *p != g.m.Mem && !g.confirm(action, prompt) {
		return false
	}
	g.power = true
	g.m.LoadProgram(p)
	g.clean = *p
	return true
}

func (g *game) save() {
	// A file that can't be written isn't replaced, so say so before asking.
	if err := checkWritable(g.file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		g.showToast("Save failed: " + err.Error())
		return
	}
	// Saving over a file that holds a different program, or one that can't
	// be read, needs confirming.
	old, err := readListing(openListing(g.file))
	switch {
	case err == nil:
		p, err := machine.ParseProgram(bytes.NewReader(old))
		if err == nil && *p == g.m.Mem {
			// Rewriting the file would only lose the remarks written in it.
			g.clean = g.m.Mem
			g.showToast("Already saved in " + g.file)
			return
		}
		if !g.confirm("save", "Press SAVE again to replace the program in "+g.file) {
			return
		}
	case !errors.Is(err, fs.ErrNotExist):
		if !g.confirm("save", "Cannot read "+g.file+" — press SAVE again to replace it") {
			return
		}
	}
	var buf bytes.Buffer
	machine.FormatProgram(&buf, "Saved "+time.Now().Format("2006-01-02 15:04"), &g.m.Mem)
	if err := writeFileAtomic(g.file, buf.Bytes()); err != nil {
		g.showToast("Save failed: " + err.Error())
		return
	}
	g.clean = g.m.Mem
	g.showToast("Saved to " + g.file)
}

// writeFileAtomic replaces the contents of name with data. It writes a
// temporary file next to name and then renames it, so a crash part-way
// through never leaves name truncated. If name is a symlink, the file it
// points to is written, and an existing file keeps its permissions. A new
// file is created directly, with permissions set by the umask. A file that
// can't be opened for writing, such as a read-only one, is left alone. A
// file with other hard links or another owner, or one that can't be
// replaced by renaming, such as in a folder where no temporary file can be
// made, is rewritten in place instead, so that it stays the same file.
func writeFileAtomic(name string, data []byte) error {
	fi, err := os.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return writeNewFile(name, data)
	}
	if err != nil {
		return err
	}
	if name, err = filepath.EvalSymlinks(name); err != nil {
		return err
	}
	// Renaming over a file only needs the folder to be writable, so check
	// the file is, rather than replace one the user made read-only.
	if err := checkWritable(name); err != nil {
		return err
	}
	if !replaceable(name, fi) {
		return writeInPlace(name, data)
	}
	f, err := os.CreateTemp(filepath.Dir(name), filepath.Base(name)+".*.tmp")
	if errors.Is(err, fs.ErrPermission) {
		return writeInPlace(name, data)
	}
	if err != nil {
		return err
	}
	// A new file may not get the old one's group, such as in a folder
	// whose files take its group. If it can't be given it, the rename would
	// change who can read the file.
	if !sameGroup(f, fi) {
		f.Close()
		os.Remove(f.Name())
		return writeInPlace(name, data)
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return err
	}
	// The new file is written, so if it can't be given the old one's
	// permissions or put in its place, the old one is rewritten instead.
	// That happens when the file is a mount point (EBUSY), the file system
	// doesn't do permissions, or on Windows another program has the file
	// open.
	if os.Chmod(f.Name(), fi.Mode().Perm()) != nil || rename(f.Name(), name) != nil {
		os.Remove(f.Name())
		return writeInPlace(name, data)
	}
	syncDir(filepath.Dir(name))
	removeStale(filepath.Dir(name), filepath.Base(name))
	return nil
}

// rename is os.Rename, replaced in tests to make it fail.
var rename = os.Rename

// staleAge is how old a temporary file left by an earlier save must be
// before a save removes it. A younger one could belong to a save still
// being made by another copy of the emulator.
const staleAge = time.Hour

// removeStale removes old temporary files left in dir by saves of the
// file base that were stopped, such as by a power cut, before they renamed
// them. CreateTemp names them base.<digits>.tmp.
func removeStale(dir, base string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		mid, pre := strings.CutPrefix(e.Name(), base+".")
		mid, suf := strings.CutSuffix(mid, ".tmp")
		if !pre || !suf || mid == "" || strings.Trim(mid, "0123456789") != "" {
			continue
		}
		n := filepath.Join(dir, e.Name())
		if fi, err := os.Lstat(n); err == nil && fi.Mode().IsRegular() &&
			time.Since(fi.ModTime()) > staleAge {
			os.Remove(n)
		}
	}
}

// writeNewFile creates name, which doesn't exist yet. If name is a symlink
// to a missing file, the system creates the file it points to, so it ends
// up where the link leads however the link's path is written.
func writeNewFile(name string, data []byte) error {
	flag := os.O_CREATE | os.O_EXCL
	link := false
	if fi, err := os.Lstat(name); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		// O_EXCL would refuse to follow the link.
		flag, link = os.O_CREATE|os.O_TRUNC, true
	}
	f, err := os.OpenFile(name, os.O_WRONLY|flag, 0o666)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		// Don't leave part of a program to be loaded later.
		if !link {
			os.Remove(name)
		}
		return err
	}
	if real, err := filepath.EvalSymlinks(name); err == nil {
		syncDir(filepath.Dir(real))
	}
	return nil
}

// errSpecialFile is the error for a file that isn't a plain file, such as
// a pipe or a device. Opening one can wait forever, freezing the window.
var errSpecialFile = errors.New("this is not a plain file")

// checkWritable returns the error from opening an existing file for
// writing, if any.
func checkWritable(name string) error {
	if fi, err := os.Stat(name); err == nil {
		if err := checkPlain(fi); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(name, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

// writeInPlace rewrites the existing file name with data, keeping it the
// same file. Rather than truncating the file first, it writes over the old
// contents, and if that fails, such as when the disk is full, it puts them
// back, so the old program isn't lost. The old contents' space is still
// the file's, so putting them back needs no more.
func writeInPlace(name string, data []byte) error {
	old, rerr := os.ReadFile(name)
	f, err := os.OpenFile(name, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	_, err = f.WriteAt(data, 0)
	if err == nil {
		err = f.Truncate(int64(len(data)))
	}
	if err == nil {
		err = f.Sync()
	}
	if err != nil && rerr == nil {
		if _, werr := f.WriteAt(old, 0); werr == nil {
			f.Truncate(int64(len(old)))
			f.Sync()
		}
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// maxListing is the largest file read as a listing. A full program with
// remarks is a few KB.
const maxListing = 1 << 20

var errFolder = errors.New("this is a folder, not a program listing")

// checkPlain refuses a folder or a special file, whose opening could
// freeze the window, as a program listing.
func checkPlain(fi fs.FileInfo) error {
	switch {
	case fi.IsDir():
		return errFolder
	case !fi.Mode().IsRegular():
		return errSpecialFile
	}
	return nil
}

// openListing opens the listing file name, refusing one checkPlain does.
func openListing(name string) (fs.File, error) {
	fi, err := os.Stat(name)
	if err == nil {
		err = checkPlain(fi)
	}
	if err != nil {
		return nil, err
	}
	return os.Open(name)
}

// readListing reads a listing file opened with openListing or checked with
// checkPlain, refusing a file too large to be a program.
func readListing(f fs.File, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxListing+1))
	if err == nil && len(data) > maxListing {
		err = errors.New("this file is too large to be a program listing")
	}
	return data, err
}

// saveRecovery writes the program to a new file next to the program file,
// or in the temporary folder if it can't go there, for when the emulator
// stops on an error, and returns the file's name.
func (g *game) saveRecovery() (string, error) {
	var buf bytes.Buffer
	machine.FormatProgram(&buf, "Recovered "+time.Now().Format("2006-01-02 15:04"), &g.m.Mem)
	name, err := writeRecovery(filepath.Dir(g.file), buf.Bytes())
	if err != nil {
		if tmp, terr := writeRecovery(os.TempDir(), buf.Bytes()); terr == nil {
			return tmp, nil
		}
	}
	return name, err
}

// writeRecovery writes data to a new recovery file in dir.
func writeRecovery(dir string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, "odyssey9-recovered-*.txt")
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func (g *game) load() {
	data, err := readListing(openListing(g.file))
	if err != nil {
		g.showToast("Load failed: " + err.Error())
		return
	}
	g.loadBytes(g.file, data, "load", "Press LOAD again to replace your unsaved program")
}

func (g *game) loadSample(i int) {
	s := machine.Samples[i]
	if !g.replace(s.Program(), fmt.Sprint("sample:", i), "Choose "+s.Name+" again to replace your unsaved program") {
		return
	}
	g.overlay = noPanel
	g.showToast(s.Name + " loaded. " + s.Usage)
}
