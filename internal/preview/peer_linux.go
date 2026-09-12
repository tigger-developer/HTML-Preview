// ABOUTME: Authenticates Unix socket peers using Linux kernel SO_PEERCRED.
// ABOUTME: Compares effective user identity before control HTTP processing.
package preview

import (
	"errors"
	"net"
	"os"
	"syscall"
)

func sameUserPeer(conn *net.UnixConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var credentialErr error
	err = raw.Control(func(fd uintptr) {
		credential, getErr := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if getErr != nil {
			credentialErr = getErr
			return
		}
		if int64(credential.Uid) != int64(os.Geteuid()) {
			credentialErr = errors.New("control peer is not the service user")
		}
	})
	return errors.Join(err, credentialErr)
}
