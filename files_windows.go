package main

import (
	"io/fs"
	"os"
	"syscall"
)

// replaceable reports whether the file name can be replaced by a new one
// without anything but its contents changing: it has no other hard links
// and isn't hidden or a system file, which a new file wouldn't be. A new
// file also takes its folder's access rights rather than any the file was
// given of its own, which can't be checked here.
func replaceable(name string, fi fs.FileInfo) bool {
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok &&
		d.FileAttributes&(syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM) != 0 {
		return false
	}
	f, err := os.Open(name)
	if err != nil {
		return true
	}
	defer f.Close()
	var d syscall.ByHandleFileInformation
	if syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &d) != nil {
		return true
	}
	return d.NumberOfLinks <= 1
}

// sameGroup reports whether the new file f has the group of the file fi,
// which only Unix files have.
func sameGroup(f *os.File, fi fs.FileInfo) bool { return true }

// syncDir does nothing: Windows writes a folder's list of files to disk
// on its own.
func syncDir(dir string) {}
