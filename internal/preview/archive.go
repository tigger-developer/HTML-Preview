// ABOUTME: Validates ZIP document containers before passing bytes to Pandoc.
// ABOUTME: Bounds entry counts and actual inflation without extracting source paths.
package preview

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

const maxArchiveEntries = 4096
const maxArchiveBytes int64 = 100 << 20
const maxAssetBytes int64 = 10 << 20

func validateArchive(ctx context.Context, data []byte, budget int64) (int64, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid document archive: %w", err)
	}
	if len(archive.File) > maxArchiveEntries {
		return 0, errors.New("document archive exceeds 4096 entries")
	}
	seen := make(map[string]bool)
	var total int64
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		name := path.Clean(entry.Name)
		if strings.ContainsAny(entry.Name, "\\:\x00") || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return total, errors.New("document archive has an unsafe member path")
		}
		if seen[name] {
			return total, errors.New("document archive has duplicate normalized member names")
		}
		seen[name] = true
		// Directory entries carry no content; links and special files are never admitted.
		if entry.Mode().IsDir() && entry.UncompressedSize64 == 0 {
			continue
		}
		if !entry.Mode().IsRegular() {
			return total, errors.New("document archive contains a non-regular member")
		}
		limit := min(maxArchiveBytes-total, budget-total)
		if strings.Contains("/"+strings.ToLower(name), "/media/") || strings.Contains("/"+strings.ToLower(name), "/pictures/") {
			limit = min(limit, maxAssetBytes)
		}
		if limit < 0 || entry.UncompressedSize64 > uint64(limit) {
			return total, errors.New("document archive exceeds remaining inflated-byte budget")
		}
		n, err := countArchiveEntry(ctx, entry, limit)
		total += n
		if err != nil {
			return total, fmt.Errorf("document archive member: %w", err)
		}
	}
	return total, nil
}

func countArchiveEntry(ctx context.Context, entry *zip.File, limit int64) (n int64, err error) {
	reader, err := entry.Open()
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, reader.Close()) }()
	var buffer [32768]byte
	for {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		count, readErr := reader.Read(buffer[:])
		n += int64(count)
		if n > limit {
			return n, errors.New("inflated data exceeds byte budget")
		}
		if errors.Is(readErr, io.EOF) {
			return n, nil
		}
		if readErr != nil {
			return n, readErr
		}
	}
}
