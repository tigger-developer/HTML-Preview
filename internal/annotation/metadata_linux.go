// ABOUTME: Detects Linux xattrs, including POSIX ACLs, before current-value replacement.
// ABOUTME: Refuses unsupported metadata rather than losing it during atomic rename.
package annotation

import (
	"errors"
	"os"
	"syscall"
)

func replacementMetadata(f *os.File) error {
	n, _, errno := syscall.Syscall(syscall.SYS_FLISTXATTR, f.Fd(), 0, 0)
	if errors.Is(errno, syscall.ENOTSUP) {
		return nil
	}
	if errno != 0 {
		return errno
	}
	if n != 0 {
		return fail("storage_metadata_unsupported")
	}
	return nil
}
