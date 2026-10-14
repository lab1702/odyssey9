package main

// Saving and loading programs.

import (
	"bytes"
	"errors"
	"fmt"
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
	// Saving over a file that holds a different program, or one that can't
	// be read, needs confirming.
	old, err := os.ReadFile(g.file)
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

// writeFileAtomic writes data to a temporary file next to name and then
// renames it, so a crash part-way through never leaves name truncated. If
// name is a symlink, the file it points to is replaced, and an existing
// file keeps its permissions.
func writeFileAtomic(name string, data []byte) error {
	if target, err := filepath.EvalSymlinks(name); err == nil {
		name = target
	}
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(name); err == nil {
		mode = fi.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(name), filepath.Base(name)+".*.tmp")
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
		err = os.Chmod(f.Name(), mode)
	}
	if err == nil {
		err = os.Rename(f.Name(), name)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

func (g *game) load() {
	data, err := os.ReadFile(g.file)
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
