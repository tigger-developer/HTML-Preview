// ABOUTME: Preserves extended attributes across atomic annotation replacement.
// ABOUTME: Uses open descriptors and detects concurrent metadata changes before publication.
package annotation

import (
	"errors"
	"maps"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// Bound metadata allocations independently of filesystem-specific limits.
const maxReplacementAttributes = 16 << 20

type fileAttributes map[string]string

func readFileAttributes(f *os.File) (fileAttributes, error) {
	fd := int(f.Fd())
	n, err := unix.Flistxattr(fd, nil)
	if errors.Is(err, unix.ENOTSUP) {
		return fileAttributes{}, nil
	}
	if err != nil {
		return nil, err
	}
	if n > maxReplacementAttributes {
		return nil, fail("storage_metadata_unsupported")
	}
	names := make([]byte, n)
	if n > 0 {
		n, err = unix.Flistxattr(fd, names)
		if err != nil {
			return nil, err
		}
		if n > len(names) {
			return nil, fail("store_changed")
		}
	}
	attrs := fileAttributes{}
	total := n
	for _, name := range strings.Split(string(names[:n]), "\x00") {
		if name == "" {
			continue
		}
		size, err := unix.Fgetxattr(fd, name, nil)
		if err != nil {
			return nil, err
		}
		if size > maxReplacementAttributes-total {
			return nil, fail("storage_metadata_unsupported")
		}
		// A nonempty buffer also distinguishes a zero-byte value growing during the read.
		value := make([]byte, max(size, 1))
		size, err = unix.Fgetxattr(fd, name, value)
		if err != nil {
			return nil, err
		}
		if size > len(value) {
			return nil, fail("store_changed")
		}
		total += size
		attrs[name] = string(value[:size])
	}
	return attrs, nil
}

func (attrs fileAttributes) apply(f *os.File) error {
	current, err := readFileAttributes(f)
	if err != nil {
		return err
	}
	for name := range current {
		if _, exists := attrs[name]; !exists {
			if err := unix.Fremovexattr(int(f.Fd()), name); err != nil {
				return err
			}
		}
	}
	for name, value := range attrs {
		if err := unix.Fsetxattr(int(f.Fd()), name, []byte(value), 0); err != nil {
			return err
		}
	}
	return attrs.verify(f)
}

func (attrs fileAttributes) verify(f *os.File) error {
	current, err := readFileAttributes(f)
	if err != nil {
		return err
	}
	if !maps.Equal(attrs, current) {
		return fail("store_changed")
	}
	return nil
}
