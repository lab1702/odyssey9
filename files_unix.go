//go:build unix

package main

import (
	"io/fs"
	"os"
	"syscall"
)

// replaceable reports whether the file name, whose details are fi, can be
// replaced by a new one without its owner changing: it has no other hard
// links, and this user owns it. A new file's group is checked once it is
// made (see sameGroup). A new file doesn't get the old one's extended
// attributes, such as macOS Finder tags, which aren't checked.
func replaceable(name string, fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return st.Nlink == 1 && int(st.Uid) == os.Geteuid()
}

// sameGroup gives the new file f the group of the file fi, if it hasn't
// got it, and reports whether f now has it. Which group a new file gets
// depends on the system and its folder, so it is checked rather than
// assumed.
func sameGroup(f *os.File, fi fs.FileInfo) bool {
	want, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	nfi, err := f.Stat()
	if err != nil {
		return false
	}
	got, ok := nfi.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return got.Gid == want.Gid || f.Chown(-1, int(want.Gid)) == nil
}

// syncDir writes a folder's list of files to disk, so that a file just
// made or renamed in it is still there after a power cut. Not every system
// can do this, so errors are ignored.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}
