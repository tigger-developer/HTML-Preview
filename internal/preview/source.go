// ABOUTME: Resolves logical source contexts and reads bounded file snapshots.
// ABOUTME: Confines opened handles to canonical roots before examining contents.
package preview

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type sourceContext struct {
	input                         inputFormat
	selected                      bool
	logical, canonical, key, root string
	device                        string
	explicit                      bool
	depth                         int64
}

func deviceOf(st os.FileInfo) string { return fmt.Sprint(st.Sys().(*syscall.Stat_t).Dev) }

func contained(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func identify(path, root string) (sourceContext, error) {
	s := sourceContext{}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return s, err
	}
	s.logical = filepath.Clean(absolute)
	s.canonical, err = filepath.EvalSymlinks(s.logical)
	if err != nil {
		return s, err
	}
	s.canonical, err = filepath.Abs(s.canonical)
	if err != nil {
		return s, err
	}
	st, err := os.Stat(s.canonical)
	if err != nil {
		return s, err
	}
	if !st.Mode().IsRegular() {
		return s, errors.New("source must be a regular file")
	}
	s.device = fmt.Sprint(st.Sys().(*syscall.Stat_t).Dev)
	s.key = s.canonical + "\x00" + filepath.Dir(s.logical)
	s.root = root
	if s.root == "" {
		s.root = filepath.Dir(s.canonical)
	}
	if !contained(s.root, s.canonical) {
		return s, errors.New("source is outside HTMLPREVIEW_ROOT")
	}
	return s, nil
}

func snapshot(s sourceContext, limit int64) (data []byte, err error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	rel, err := filepath.Rel(s.root, s.canonical)
	if err != nil {
		return nil, err
	}
	f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || fmt.Sprint(st.Sys().(*syscall.Stat_t).Dev) != s.device {
		return nil, errors.New("source handle changed type or filesystem")
	}
	if st.Size() > limit {
		return nil, fmt.Errorf("source exceeds remaining byte limit %d", limit)
	}
	data = make([]byte, st.Size())
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if after.Size() != st.Size() || after.ModTime() != st.ModTime() {
		return nil, errors.New("source changed during snapshot")
	}
	return data, nil
}
