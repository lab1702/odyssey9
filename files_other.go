//go:build !unix

package main

import "io/fs"

// linkCount returns the number of hard links to a file. Only Unix reports
// it, so elsewhere a file is taken to have one.
func linkCount(fi fs.FileInfo) uint64 { return 1 }
