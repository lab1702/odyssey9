//go:build unix

package main

import (
	"io/fs"
	"syscall"
)

// linkCount returns the number of hard links to a file.
func linkCount(fi fs.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 1
}
