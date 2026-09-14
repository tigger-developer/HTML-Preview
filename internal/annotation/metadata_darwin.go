// ABOUTME: Detects macOS metadata that this bounded replacement writer cannot preserve.
// ABOUTME: Refuses ACLs, extended attributes and flags rather than silently dropping them.
package annotation

import (
	"encoding/binary"
	"os"
	"syscall"
	"unsafe"
)

func replacementMetadata(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Sys().(*syscall.Stat_t).Flags != 0 {
		return fail("storage_metadata_unsupported")
	}
	n, _, errno := syscall.Syscall6(syscall.SYS_FLISTXATTR, f.Fd(), 0, 0, 0, 0, 0)
	if errno != 0 {
		return errno
	}
	if n != 0 {
		return fail("storage_metadata_unsupported")
	}
	// Apple's getattrlist(2): five attribute groups; the returned variable
	// extended-security reference is length-prefixed. A nonzero ACL length
	// requires preservation support before replacement can be admitted.
	var attributes [24]byte
	binary.LittleEndian.PutUint16(attributes[:2], 5)
	binary.LittleEndian.PutUint32(attributes[4:8], 0x00400000)
	var result [12]byte
	_, _, errno = syscall.Syscall6(syscall.SYS_FGETATTRLIST, f.Fd(), uintptr(unsafe.Pointer(&attributes[0])), uintptr(unsafe.Pointer(&result[0])), uintptr(len(result)), 0, 0)
	if errno != 0 {
		return errno
	}
	if binary.LittleEndian.Uint32(result[8:12]) != 0 {
		return fail("storage_metadata_unsupported")
	}
	return nil
}
