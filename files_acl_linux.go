package main

import "syscall"

// hasACL reports whether the file name has an access control list giving
// more than its permission bits do. A new file wouldn't get the list, and
// its group permission bits, which are the list's mask, would be the new
// file's group's, so replacing the file would change who can use it.
func hasACL(name string) bool {
	_, err := syscall.Getxattr(name, "system.posix_acl_access", nil)
	return err == nil
}
