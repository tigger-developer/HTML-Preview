// ABOUTME: Reads rooted source and sidecar snapshots with bounded storage validation.
// ABOUTME: Derives independent authored and full-store revisions without modifying files.
package annotation

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type Location struct {
	Root, Path, Format string
	Limit              int64
}

type Snapshot struct {
	Embedded, Sidecar                Store
	RawSource, RawSidecar            []byte
	SideExists                       bool
	SourceInfo, SideInfo             os.FileInfo
	Revision, SourceRevision, Reason string
	Header                           *Header
	Events                           []Event
	Operations                       map[string]Event
}

type Failure struct{ Code string }

func (e *Failure) Error() string { return e.Code }
func fail(code string) error     { return &Failure{Code: code} }

func (loc Location) relative() (string, error) {
	rel, err := filepath.Rel(loc.Root, loc.Path)
	if err != nil || !filepath.IsLocal(rel) || rel == "." || (loc.Format != "org" && loc.Format != "markdown") {
		return "", fail("invalid_source")
	}
	return rel, nil
}

func Read(loc Location) (snap Snapshot, err error) {
	rel, err := loc.relative()
	if err != nil {
		return snap, err
	}
	root, err := os.OpenRoot(loc.Root)
	if err != nil {
		return snap, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	limit := loc.Limit
	if limit <= 0 || limit > MaxStore {
		limit = MaxStore
	}
	snap.RawSource, snap.SourceInfo, err = readRootFile(root, rel, limit)
	if err != nil {
		return snap, err
	}
	snap.Embedded = Parse(snap.RawSource, loc.Format)
	snap.RawSidecar, snap.SideInfo, err = readRootFile(root, rel+"-annotations.org", MaxStore)
	if os.IsNotExist(err) {
		err = nil
	} else if err != nil {
		return snap, err
	} else {
		snap.SideExists = true
	}
	snap.Sidecar = Parse(snap.RawSidecar, "org")
	snap.SourceRevision = Digest(snap.Embedded.Source)
	var tuple bytes.Buffer
	// bytes.Buffer writes cannot fail; binary.Append supplies explicit tuple lengths.
	tuple.Write(binary.BigEndian.AppendUint64(nil, uint64(len(snap.RawSource))))
	tuple.Write(snap.RawSource)
	if snap.SideExists {
		tuple.WriteByte(1)
		tuple.Write(binary.BigEndian.AppendUint64(nil, uint64(len(snap.RawSidecar))))
		tuple.Write(snap.RawSidecar)
	} else {
		tuple.WriteByte(0)
	}
	snap.Revision = Digest(tuple.Bytes())
	validateSnapshot(&snap, loc.Format)
	return snap, nil
}

func readRootFile(root *os.Root, path string, limit int64) (data []byte, info os.FileInfo, err error) {
	f, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err = f.Stat()
	if err != nil {
		return nil, nil, err
	}
	base, err := root.Stat(".")
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Sys().(*syscall.Stat_t).Dev != base.Sys().(*syscall.Stat_t).Dev {
		return nil, nil, fail("unsafe_source")
	}
	if info.Size() > limit {
		return nil, nil, fail("store_limit")
	}
	data, err = io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > limit {
		return nil, nil, fail("store_limit")
	}
	after, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	current, err := root.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !os.SameFile(info, current) || info.Size() != after.Size() || info.ModTime() != after.ModTime() {
		return nil, nil, fail("source_changed")
	}
	return data, info, nil
}

func validateSnapshot(s *Snapshot, format string) {
	s.Header = s.Embedded.Header
	if s.Header == nil {
		s.Header = s.Sidecar.Header
	}
	s.Reason = s.Embedded.Reason
	if s.Reason == "" {
		s.Reason = s.Sidecar.Reason
	}
	if s.SideExists && (s.Sidecar.Header == nil || len(s.Sidecar.Source) != 0) {
		s.Reason = "foreign_sidecar"
	}
	if !s.Embedded.Safe {
		s.Reason = "unsafe_source"
	}
	if s.Header != nil && s.Header.SourceFormat != format {
		s.Reason = "corrupt_store"
	}
	if s.Embedded.Header != nil && s.Sidecar.Header != nil && *s.Embedded.Header != *s.Sidecar.Header {
		s.Reason = "corrupt_store"
	}
	all := append(append([]Event{}, s.Embedded.Events...), s.Sidecar.Events...)
	var err error
	s.Events, s.Operations, err = Project(all)
	if err != nil {
		s.Reason = "corrupt_store"
	}
	if s.Embedded.TailBytes+s.Sidecar.TailBytes > MaxStore || len(all) > MaxEvents {
		s.Reason = "store_limit"
	}
}

func writableFile(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Sys().(*syscall.Stat_t).Nlink == 1
}

// Destination probes permission without creating or writing a file.
func Destination(loc Location, snap Snapshot) (choice string, err error) {
	if snap.Reason != "" {
		return "", fail(snap.Reason)
	}
	if !writableFile(snap.SourceInfo) {
		return "", fail("unsafe_source")
	}
	rel, err := loc.relative()
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(loc.Root)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	f, err := root.OpenFile(rel, os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err == nil {
		return "embedded", f.Close()
	}
	if !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EPERM) && !errors.Is(err, syscall.EROFS) {
		return "", err
	}
	if snap.SideExists {
		if !writableFile(snap.SideInfo) {
			return "", fail("unsafe_sidecar")
		}
		f, err = root.OpenFile(rel+"-annotations.org", os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return "", err
		}
		return "sidecar", f.Close()
	}
	// Actual exclusive creation is deferred until a validated first event.
	return "sidecar", nil
}
