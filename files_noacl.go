//go:build unix && !linux

package main

// hasACL reports whether the file name has an access control list. Other
// systems' lists, such as macOS's, can't be read without cgo, so they
// aren't checked.
func hasACL(name string) bool { return false }
