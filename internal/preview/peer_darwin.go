// ABOUTME: Authenticates Unix socket peers using the Darwin kernel credential interface.
// ABOUTME: Reads only the stable version/UID prefix of the documented xucred result.
package preview

import (
	"encoding/binary"
	"errors"
	"net"
	"os"
	"syscall"
	"unsafe"
)

func sameUserPeer(conn *net.UnixConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var credentialErr error
	err = raw.Control(func(fd uintptr) {
		// Darwin sys/un.h defines SOL_LOCAL=0 and LOCAL_PEERCRED=1. The
		// sys/ucred.h external result begins with uint32 version and uid.
		var data [256]byte
		size := uint32(len(data))
		// #nosec G103 -- The kernel receives a live fixed-size buffer and its exact capacity during RawConn.Control; neither pointer escapes the call.
		_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, 0, 1, uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&size)), 0)
		if errno != 0 {
			credentialErr = errno
			return
		}
		if size < 8 || size > uint32(len(data)) || binary.NativeEndian.Uint32(data[:4]) != 0 || int64(binary.NativeEndian.Uint32(data[4:8])) != int64(os.Geteuid()) {
			credentialErr = errors.New("control peer is not the service user")
		}
	})
	return errors.Join(err, credentialErr)
}
