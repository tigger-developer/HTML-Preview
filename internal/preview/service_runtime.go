// ABOUTME: Owns the private runtime lock and authenticated Unix control listener.
// ABOUTME: Cleanup checks inode ownership and never removes unrelated runtime entries.
package preview

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

type serviceRuntime struct {
	root                 *os.Root
	lock                 *os.File
	listener             *net.UnixListener
	lockInfo, socketInfo os.FileInfo
}

func checkRuntime(path string, create bool) error {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) && create {
		if err = os.MkdirAll(path, 0700); err != nil {
			return err
		}
		st, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || !ownedByUser(st) || st.Mode().Perm()&0077 != 0 {
		return errors.New("runtime directory must be user-owned, private and not a symlink")
	}
	return nil
}

func openServiceRuntime(path string) (_ *serviceRuntime, err error) {
	if err = checkRuntime(path, true); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	r := &serviceRuntime{root: root}
	defer func() {
		if err != nil {
			err = errors.Join(err, r.close())
		}
	}()
	r.lock, err = root.OpenFile("instance.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	info, err := r.lock.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !ownedByUser(info) || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("unsafe service lock")
	}
	if err = syscall.Flock(int(r.lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, errors.New("service runtime already has an owner")
	}
	r.lockInfo = info
	if err = removeStaleSocket(root); err != nil {
		return nil, err
	}
	socketPath := filepath.Join(path, "control.sock")
	r.listener, err = net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("create control socket (choose a shorter HTMLPREVIEW_RUNTIME_DIR for long paths): %w", err)
	}
	r.listener.SetUnlinkOnClose(false)
	r.socketInfo, err = root.Lstat("control.sock")
	if err != nil {
		return nil, err
	}
	if err = root.Chmod("control.sock", 0600); err != nil {
		return nil, err
	}
	return r, nil
}

func removeStaleSocket(root *os.Root) error {
	st, err := root.Lstat("control.sock")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSocket == 0 || !ownedByUser(st) {
		return errors.New("refusing to replace unrelated control socket path")
	}
	return root.Remove("control.sock")
}

func (r *serviceRuntime) close() error {
	var err error
	if r.listener != nil {
		if closeErr := r.listener.Close(); !errors.Is(closeErr, net.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
	}
	for _, entry := range []struct {
		name string
		info os.FileInfo
	}{{"control.sock", r.socketInfo}, {"instance.lock", r.lockInfo}} {
		if entry.info == nil {
			continue
		}
		current, statErr := r.root.Lstat(entry.name)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			err = errors.Join(err, statErr)
			continue
		}
		if os.SameFile(current, entry.info) {
			err = errors.Join(err, r.root.Remove(entry.name))
		} else {
			err = errors.Join(err, errors.New("runtime ownership changed; entry retained"))
		}
	}
	if r.lock != nil {
		err = errors.Join(err, r.lock.Close())
	}
	if r.root != nil {
		err = errors.Join(err, r.root.Close())
	}
	return err
}

type privateListener struct{ *net.UnixListener }

func (l privateListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			return nil, err
		}
		if err = sameUserPeer(conn); err == nil {
			return conn, nil
		}
		if err = conn.Close(); err != nil {
			return nil, err
		}
	}
}
