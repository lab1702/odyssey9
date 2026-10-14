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
	old, err := readListing(os.Open(g.file))
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
	name = followLinks(name)
	fi, err := os.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return writeFile(name, data, os.O_CREATE|os.O_EXCL)
	}
	if err != nil {
		return err
	}
	// Renaming over a file only needs the folder to be writable, so check
	// the file is, rather than replace one the user made read-only.
	if err := checkWritable(name); err != nil {
		return err
	}
	if !replaceable(fi) {
		return writeFile(name, data, os.O_TRUNC)
	}
	f, err := os.CreateTemp(filepath.Dir(name), filepath.Base(name)+".*.tmp")
	if errors.Is(err, fs.ErrPermission) {
		return writeFile(name, data, os.O_TRUNC)
	}
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
	if err == nil {
		err = os.Chmod(f.Name(), fi.Mode().Perm())
	}
	if err == nil {
		err = os.Rename(f.Name(), name)
		if errors.Is(err, fs.ErrPermission) {
			os.Remove(f.Name())
			return writeFile(name, data, os.O_TRUNC)
		}
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// checkWritable returns the error from opening an existing file for
// writing, if any.
func checkWritable(name string) error {
	f, err := os.OpenFile(name, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

// writeFile writes data to name, opened with the extra flags.
func writeFile(name string, data []byte, flag int) error {
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
	return err
}

// followLinks returns the file that name refers to, following symlinks even
// when the file they point to doesn't exist yet.
func followLinks(name string) string {
	if target, err := filepath.EvalSymlinks(name); err == nil {
		return target
	}
	for range 40 { // a limit, in case the links form a loop
		target, err := os.Readlink(name)
		if err != nil {
			break
		}
		if !filepath.IsAbs(target) {
			// Resolve the link's folder first, as the system does: a ".."
			// in target leaves the real folder, not the path to a link.
			dir := filepath.Dir(name)
			if d, err := filepath.EvalSymlinks(dir); err == nil {
				dir = d
			}
			target = filepath.Join(dir, target)
		}
		name = target
	}
	return name
}

// maxListing is the largest file read as a listing. A full program with
// remarks is a few KB.
const maxListing = 1 << 20

// readListing reads a listing file opened by the caller, refusing a folder
// or a file too large to be a program.
func readListing(f fs.File, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.IsDir() {
		return nil, errors.New("this is a folder, not a program listing")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxListing+1))
	if err == nil && len(data) > maxListing {
		err = errors.New("this file is too large to be a program listing")
	}
	return data, err
}

// saveRecovery writes the program to a new file next to the program file,
// for when the emulator stops on an error, and returns the file's name.
func (g *game) saveRecovery() (string, error) {
	var buf bytes.Buffer
	machine.FormatProgram(&buf, "Recovered "+time.Now().Format("2006-01-02 15:04"), &g.m.Mem)
	f, err := os.CreateTemp(filepath.Dir(g.file), "odyssey9-recovered-*.txt")
	if err != nil {
		return "", err
	}
	_, err = f.Write(buf.Bytes())
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
	data, err := readListing(os.Open(g.file))
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
