//go:build !unix

package main

import "io/fs"

// replaceable reports whether a file can be replaced by a new one without
// anything but its contents changing. Only Unix reports hard links and
// owners, so elsewhere a file is taken to be replaceable.
func replaceable(fi fs.FileInfo) bool { return true }
