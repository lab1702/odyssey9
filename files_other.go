//go:build !unix && !windows

package main

import (
	"io/fs"
	"os"
)

// replaceable reports whether the file name can be replaced by a new one
// without anything but its contents changing. Only Unix and Windows report
// hard links, so elsewhere a file is taken to be replaceable.
func replaceable(name string, fi fs.FileInfo) bool { return true }

// sameGroup reports whether the new file f has the group of the file fi,
// which only Unix files have.
func sameGroup(f *os.File, fi fs.FileInfo) bool { return true }

// syncDir does nothing: only Unix lets a program write a folder's list of
// files to disk.
func syncDir(dir string) {}

// nonblock is 0: only Unix has pipes that a name in a folder can open.
const nonblock = 0
