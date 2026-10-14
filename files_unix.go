//go:build unix

package main

import (
	"io/fs"
	"os"
	"syscall"
)

// replaceable reports whether a file can be replaced by a new one without
// anything but its contents changing: it has no other hard links, and a new
// file made by this user would have the same owner and group.
func replaceable(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return st.Nlink == 1 && int(st.Uid) == os.Geteuid() && int(st.Gid) == os.Getegid()
}
