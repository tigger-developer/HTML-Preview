// ABOUTME: Exercises confirmed deletion against real files and atomic-write failures.
// ABOUTME: Protects changed notes, literals, other definitions and read-only sources.
package annotation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteFootnoteRefusals(t *testing.T) {
	for _, scenario := range []string{"changed note", "changed source", "duplicate", "read only", "write failure", "invalid body"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "note.org")
			original := "Body[fn:one].\n\n[fn:one] First.\n"
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			loc := Location{Root: root, Path: path, Format: "org"}
			snap, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			r := editRequest(snap, EditableFootnotes(snap.RawSource, "org", "embedded")[0], "")
			r.Action = "delete"
			w := NewWriter(FileOperations{})
			want := "footnote_conflict"
			switch scenario {
			case "changed note":
				original = strings.ReplaceAll(original, "First.", "External edit.")
			case "changed source":
				original = "New text.\n" + original
				want = "stale_source"
			case "duplicate":
				original += "\n[fn:one] Duplicate.\n"
			case "read only":
				if os.Geteuid() == 0 {
					t.Skip("requires permission enforcement")
				}
				if err := os.Chmod(path, 0400); err != nil {
					t.Fatal(err)
				}
				want = "read_only_footnote"
			case "write failure":
				w = NewWriter(FileOperations{Append: func(*os.File, []byte) (int, error) { return 0, errors.New("injected failure") }})
				want = "injected failure"
			case "invalid body":
				r.Text = "unexpected body"
				want = "body_limit"
			}
			if scenario == "changed note" || scenario == "changed source" || scenario == "duplicate" {
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err = w.Replace(t.Context(), loc, snap.SourceInfo, "Reviewer", "secret", r, nil)
			if err == nil || err.Error() != want {
				t.Fatalf("got %v, want %s", err, want)
			}
			after, err := Read(loc)
			if err != nil || string(after.RawSource) != original {
				t.Fatal("refused deletion changed file", err)
			}
		})
	}
}

func TestDeleteNewFootnoteAndSidecar(t *testing.T) {
	for _, sidecar := range []bool{false, true} {
		t.Run(map[bool]string{false: "embedded", true: "sidecar"}[sidecar], func(t *testing.T) {
			if sidecar && os.Geteuid() == 0 {
				t.Skip("requires permission enforcement")
			}
			root := t.TempDir()
			path := filepath.Join(root, "note.md")
			mode := os.FileMode(0600)
			if sidecar {
				mode = 0400
			}
			if err := os.WriteFile(path, []byte("Body.\n"), mode); err != nil {
				t.Fatal(err)
			}
			loc := Location{Root: root, Path: path, Format: "markdown"}
			snap, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			w := NewWriter(FileOperations{})
			r := Request{OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Revision: snap.Revision, SourceRevision: snap.SourceRevision, BodyRevision: Digest([]byte("Body.")), Action: "upsert", Label: "reviewer-001", Target: Target{Type: "point", Position: 4, Run: "Body.", RunOffset: 4}, Text: "Saved draft"}
			created, err := w.Replace(t.Context(), loc, snap.SourceInfo, "Reviewer", "secret", r, func(context.Context, Snapshot, *Target) (int, error) { return 4, nil })
			if err != nil || len(created.Receipt.FootnoteRevision) != 64 {
				t.Fatal("creation must acknowledge definition revision", err, created.Receipt)
			}
			current, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			r.Action = "delete"
			r.Sequence = 2
			r.Text = ""
			r.OperationID = "10000000-0000-4000-8000-000000000002"
			r.Revision = current.Revision
			r.SourceRevision = current.SourceRevision
			r.Target = Target{Type: "footnote", Exact: created.Receipt.FootnoteRevision, Run: created.Receipt.Storage}
			deleted, err := w.Replace(t.Context(), loc, current.SourceInfo, "Reviewer", "secret", r, nil)
			if err != nil {
				t.Fatal(err)
			}
			after, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			data, format := after.RawSource, "markdown"
			if sidecar {
				data, format = after.RawSidecar, "org"
			}
			if len(EditableFootnotes(data, format, r.Target.Run)) != 0 || footnoteLabelPresent(data, format, r.Label) {
				t.Fatal("deletion left footnote")
			}
			if sidecar && (string(after.RawSource) != "Body.\n" || !os.SameFile(snap.SourceInfo, after.SourceInfo)) {
				t.Fatal("read-only source changed")
			}
			retry, err := w.Replace(t.Context(), loc, deleted.SourceInfo, "Reviewer", "secret", r, nil)
			if err != nil || !retry.Retry {
				t.Fatal("lost response retry", err)
			}
		})
	}
}

func TestDeleteFootnotePreservesSiblingAndLiteralReferences(t *testing.T) {
	for _, tc := range []struct{ format, original, want string }{
		{"org", "Body[fn:one][fn:two].\n\n#+BEGIN_QUOTE\nQuote[fn:one].\n#+END_QUOTE\n\n~[fn:one]~ =literal [fn:one]= \\[fn:one]\n\nAnnotations: [fn:one][fn:two]\n\n[fn:one] Remove.\n\n[fn:two] Keep.\n", "Body[fn:two].\n\n#+BEGIN_QUOTE\nQuote.\n#+END_QUOTE\n\n~[fn:one]~ =literal [fn:one]= \\[fn:one]\n\nAnnotations: [fn:two]\n\n\n[fn:two] Keep.\n"},
		{"markdown", "Body[^one][^two].\r\n\r\n``literal [^one]`` <!-- [^one] --> \\[^one]\r\n\r\n[^one]: Remove.\r\n\r\n[^two]: Keep.\r\n\r\n    Cross-reference[^one].\r\n", "Body[^two].\r\n\r\n``literal [^one]`` <!-- [^one] --> \\[^one]\r\n\r\n\r\n[^two]: Keep.\r\n\r\n    Cross-reference.\r\n"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			data := []byte(tc.original)
			notes := EditableFootnotes(data, tc.format, "embedded")
			if len(notes) != 2 {
				t.Fatalf("bad fixture: %v", notes)
			}
			result, err := patchDeletedFootnote(data, tc.format, notes[0])
			if err != nil || string(result) != tc.want {
				t.Fatalf("preservation: %v\ngot %q\nwant %q", err, result, tc.want)
			}
		})
	}
}

func TestDeleteRetryAfterSyncFailureAndRecreatedLabel(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.org")
	original := "Body[fn:one].\n\n[fn:one] First.\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	loc := Location{Root: root, Path: path, Format: "org"}
	snap, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	r := editRequest(snap, EditableFootnotes(snap.RawSource, "org", "embedded")[0], "")
	r.Action = "delete"
	failed := false
	w := NewWriter(FileOperations{Sync: func(f *os.File) error {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if info.IsDir() && !failed {
			failed = true
			return errors.New("injected sync failure")
		}
		return f.Sync()
	}})
	result, err := w.Replace(t.Context(), loc, snap.SourceInfo, "Reviewer", "secret", r, nil)
	if err == nil || result.SourceInfo == nil {
		t.Fatal("uncertain delete must retain published identity", err)
	}
	result, err = w.Replace(t.Context(), loc, result.SourceInfo, "Reviewer", "secret", r, nil)
	if err != nil || !result.Retry {
		t.Fatal("retry must synchronize deletion", err)
	}
	recreated := strings.ReplaceAll(original, "First.", "Recreated by another editor.")
	if err := os.WriteFile(path, []byte(recreated), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = w.Replace(t.Context(), loc, result.SourceInfo, "Reviewer", "secret", r, nil)
	if err == nil || err.Error() != "footnote_conflict" {
		t.Fatal("stale deletion removed recreated note", err)
	}
	current, err := Read(loc)
	if err != nil || string(current.RawSource) != recreated {
		t.Fatal("recreated note changed", err)
	}
}
