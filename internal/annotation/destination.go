// ABOUTME: Opens existing annotation destinations through rooted file handles.
// ABOUTME: Checks source and destination identity before replacement or synchronization.
package annotation

import (
	"errors"
	"os"
	"syscall"
)

func openDestination(loc Location, storage string, create bool) (*os.File, *os.Root, bool, error) {
	rel, err := loc.relative()
	if err != nil {
		return nil, nil, false, err
	}
	root, err := os.OpenRoot(loc.Root)
	if err != nil {
		return nil, nil, false, err
	}
	flags := os.O_WRONLY | os.O_APPEND | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if storage == "sidecar" {
		rel += "-annotations.org"
	} else if storage != "embedded" {
		return nil, nil, false, errors.Join(fail("invalid_storage"), root.Close())
	}
	f, err := root.OpenFile(rel, flags, 0)
	created := false
	if os.IsNotExist(err) && storage == "sidecar" && create {
		f, err = root.OpenFile(rel, flags|os.O_CREATE|os.O_EXCL, 0600)
		created = err == nil
	}
	if err != nil {
		return nil, nil, false, errors.Join(err, root.Close())
	}
	info, err := f.Stat()
	base, baseErr := root.Stat(".")
	if err != nil || baseErr != nil || !writableFile(info) || info.Sys().(*syscall.Stat_t).Dev != base.Sys().(*syscall.Stat_t).Dev {
		return nil, nil, false, errors.Join(fail("unsafe_destination"), err, baseErr, f.Close(), root.Close())
	}
	return f, root, created, nil
}

func destinationIdentity(loc Location, storage string, root *os.Root, f *os.File, snap Snapshot) error {
	rel, err := loc.relative()
	if err != nil {
		return err
	}
	if storage == "sidecar" {
		rel += "-annotations.org"
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := root.Lstat(rel)
	if err != nil {
		return err
	}
	logical, err := os.Stat(loc.Path)
	if err != nil {
		return err
	}
	if !writableFile(info) || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) || !os.SameFile(snap.SourceInfo, logical) {
		return fail("source_replaced")
	}
	if storage == "embedded" && !os.SameFile(info, snap.SourceInfo) {
		return fail("source_replaced")
	}
	if storage == "sidecar" && (snap.SideInfo == nil || !os.SameFile(info, snap.SideInfo)) {
		return fail("store_changed")
	}
	return nil
}
