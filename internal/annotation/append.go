// ABOUTME: Appends complete annotation frames through rooted, locked file handles.
// ABOUTME: Preserves interrupted writes and verifies bytes and identity before acknowledgement.
package annotation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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

func (w *Writer) commit(ctx context.Context, loc Location, snap Snapshot, storage string, header *Header, event Event) (updated Snapshot, err error) {
	store, format, prior := snap.Embedded, loc.Format, snap.RawSource
	if storage == "sidecar" {
		store, format, prior = snap.Sidecar, "org", snap.RawSidecar
	}
	addition, err := AppendBytes(store, header, event, format)
	if err != nil {
		return updated, err
	}
	if int64(len(prior)+len(addition)) > MaxStore || snap.Embedded.TailBytes+snap.Sidecar.TailBytes+len(addition) > MaxStore || len(snap.Embedded.Events)+len(snap.Sidecar.Events) >= MaxEvents {
		return updated, fail("store_limit")
	}
	if storage == "embedded" && loc.Limit > 0 && int64(len(prior)+len(addition)) > loc.Limit {
		return updated, fail("store_limit")
	}
	f, root, created, err := openDestination(loc, storage, true)
	if err != nil {
		return updated, err
	}
	defer func() { err = errors.Join(err, f.Close(), root.Close()) }()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return updated, fail("busy")
		}
		return updated, fail("locking_unavailable")
	}
	defer func() { err = errors.Join(err, syscall.Flock(int(f.Fd()), syscall.LOCK_UN)) }()
	current, err := Read(loc)
	if err != nil {
		return updated, err
	}
	// A newly exclusive-created empty sidecar is the only permitted difference.
	if current.SourceRevision != snap.SourceRevision || !os.SameFile(snap.SourceInfo, current.SourceInfo) {
		return updated, fail("stale_source")
	}
	if !bytes.Equal(current.RawSource, snap.RawSource) || !bytes.Equal(current.RawSidecar, snap.RawSidecar) || (!created && current.Revision != snap.Revision) {
		return updated, fail("store_changed")
	}
	if err = destinationIdentity(loc, storage, root, f, current); err != nil {
		return updated, err
	}
	if err = ctx.Err(); err != nil {
		return updated, err
	}
	n, err := w.operations.Append(f, addition)
	if err != nil {
		return updated, err
	}
	if n != len(addition) {
		return updated, io.ErrShortWrite
	}
	if err = w.operations.Sync(f); err != nil {
		return updated, err
	}
	if created {
		rel, relErr := loc.relative()
		if relErr != nil {
			return updated, relErr
		}
		parent, openErr := root.Open(filepath.Dir(rel))
		if openErr != nil {
			return updated, openErr
		}
		if err = errors.Join(w.operations.Sync(parent), parent.Close()); err != nil {
			return updated, err
		}
	}
	updated, err = Read(loc)
	if err != nil {
		return updated, err
	}
	written := updated.RawSource
	if storage == "sidecar" {
		written = updated.RawSidecar
	}
	if !bytes.Equal(written, append(append([]byte{}, prior...), addition...)) || updated.Reason != "" {
		return updated, fail("store_changed")
	}
	if err = destinationIdentity(loc, storage, root, f, updated); err != nil {
		return updated, err
	}
	return updated, nil
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
