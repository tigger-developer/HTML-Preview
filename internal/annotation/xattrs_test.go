// ABOUTME: Verifies annotation saves preserve real filesystem extended attributes.
// ABOUTME: Exercises native footnote writes and concurrent metadata changes.
package annotation

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"testing"

	"golang.org/x/sys/unix"
)

func TestAnnotationSavePreservesExtendedAttributes(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		t.Run(format, func(t *testing.T) {
			loc, _ := sourceFixture(t, "notes")
			loc.Format = format
			name := "user.htmlpreview-test"
			value := []byte{0, 1, 2, 255, 0, 17}
			if runtime.GOOS == "darwin" {
				name = "com.apple.lastuseddate#PS"
				value = make([]byte, 16)
				value[0] = 42
			}
			if err := unix.Setxattr(loc.Path, name, value, 0); err != nil {
				t.Fatal(err)
			}
			if err := unix.Setxattr(loc.Path, "user.htmlpreview-empty", []byte{}, 0); err != nil {
				t.Fatal(err)
			}
			snap, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			w := NewWriter(FileOperations{})
			r := requestFixture(t, snap)
			result, err := w.Replace(t.Context(), loc, snap.SourceInfo, "Reviewer", "secret", r, func(context.Context, Snapshot, *Target) (int, error) { return 10, nil })
			if err != nil {
				t.Fatal("annotation save with extended attributes:", err)
			}
			current, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			notes := EditableFootnotes(current.RawSource, format, "embedded")
			if len(notes) != 1 || notes[0].Text != r.Text {
				t.Fatal("annotation not saved")
			}
			if current.SourceInfo.Mode() != snap.SourceInfo.Mode() {
				t.Fatal("permissions changed")
			}
			if _, err := w.Replace(t.Context(), loc, result.SourceInfo, "Reviewer", "edit", editRequest(current, notes[0], "Edited note"), nil); err != nil {
				t.Fatal("edit:", err)
			}
			got := make([]byte, 128)
			n, err := unix.Getxattr(loc.Path, name, got)
			if err != nil || !bytes.Equal(got[:n], value) {
				t.Fatalf("attribute changed: %x %v", got[:n], err)
			}
			n, err = unix.Getxattr(loc.Path, "user.htmlpreview-empty", got)
			if err != nil || n != 0 {
				t.Fatalf("empty attribute lost: %d %v", n, err)
			}
		})
	}
}

func TestAnnotationSaveRejectsConcurrentMetadataChange(t *testing.T) {
	loc, snap := sourceFixture(t, "notes.org")
	name := "user.htmlpreview-test"
	if err := unix.Setxattr(loc.Path, name, []byte("before"), 0); err != nil {
		t.Fatal(err)
	}
	w := NewWriter(FileOperations{Append: func(f *os.File, data []byte) (int, error) {
		if err := unix.Setxattr(loc.Path, name, []byte("external edit"), 0); err != nil {
			return 0, err
		}
		return f.Write(data)
	}})
	_, err := w.Replace(t.Context(), loc, snap.SourceInfo, "Reviewer", "secret", requestFixture(t, snap), func(context.Context, Snapshot, *Target) (int, error) { return 10, nil })
	if err == nil {
		t.Fatal("concurrent metadata edit was overwritten")
	}
	current, readErr := Read(loc)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(current.RawSource, snap.RawSource) {
		t.Fatal("failed save replaced source")
	}
	got := make([]byte, 64)
	n, readErr := unix.Getxattr(loc.Path, name, got)
	if readErr != nil || string(got[:n]) != "external edit" {
		t.Fatalf("external metadata not retained: %q %v (save %v)", got[:n], readErr, err)
	}
}

func TestAnnotationSidecarPreservesExtendedAttributes(t *testing.T) {
	loc, snap := sourceFixture(t, "notes.org")
	if err := os.Chmod(loc.Path, 0400); err != nil {
		t.Fatal(err)
	}
	w := NewWriter(FileOperations{})
	r := requestFixture(t, snap)
	if _, err := w.Replace(t.Context(), loc, snap.SourceInfo, "Reviewer", "secret", r, func(context.Context, Snapshot, *Target) (int, error) { return 10, nil }); err != nil {
		t.Fatal(err)
	}
	side := loc.Path + "-annotations.org"
	if err := unix.Setxattr(side, "user.htmlpreview-test", []byte("sidecar metadata"), 0); err != nil {
		t.Fatal(err)
	}
	current, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	notes := EditableFootnotes(current.RawSidecar, "org", "sidecar")
	if len(notes) != 1 {
		t.Fatal("sidecar footnote missing")
	}
	if _, err := w.Replace(t.Context(), loc, current.SourceInfo, "Reviewer", "edit", editRequest(current, notes[0], "Edited sidecar"), nil); err != nil {
		t.Fatal(err)
	}
	after, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after.RawSource, snap.RawSource) || after.SourceInfo.Mode().Perm() != 0400 {
		t.Fatal("read-only source changed")
	}
	got := make([]byte, 64)
	n, err := unix.Getxattr(side, "user.htmlpreview-test", got)
	if err != nil || string(got[:n]) != "sidecar metadata" {
		t.Fatalf("sidecar attribute changed: %q %v", got[:n], err)
	}
}
