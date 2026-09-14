// ABOUTME: Verifies native footnotes retain one readable current annotation value.
// ABOUTME: Covers W009 native storage, literal boundaries and authored-byte preservation.
package annotation

import (
	"bytes"
	"strings"
	"testing"
)

func TestRT009_6_UpdatesReplaceCurrentFootnote(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		t.Run(format, func(t *testing.T) {
			data := []byte(nativeFixture(format, "Old draft"))
			store := Parse(data, format)
			event := store.Events[0]
			for i := 0; i < 20; i++ {
				event.Text = strings.Repeat("Current ", i+1)
				event.Sequence++
				var err error
				data, err = UpdateFootnote(data, format, *store.Header, event, "tadg-002", -1)
				if err != nil {
					t.Fatal(err)
				}
				current := Parse(data, format)
				if current.Reason != "" || len(current.Events) != 1 || current.Events[0].Text != event.Text {
					t.Fatalf("upsert did not retain exactly the current value: %#v", current)
				}
				if bytes.Contains(data, []byte("Old draft")) || bytes.Contains(data, []byte("tadg-001")) || string(current.Source) != "A sentence.\n" {
					t.Fatalf("history, old label or authored-byte changes remained: %q", data)
				}
			}
		})
	}
}

func nativeFixture(format, text string) string {
	meta := "HTMLPREVIEW_ANNOTATION: 2\nDOCUMENT_ID: 10000000-0000-4000-8000-000000000001\nUUID: 20000000-0000-4000-8000-000000000001\nAUTHOR: Taḋg\nCREATED: 2026-09-14T10:00:00Z\nUPDATED: 2026-09-14T10:00:01Z\nSTATE: draft\nOPERATION: 30000000-0000-4000-8000-000000000001\nREVISION: 3\n"
	if format == "org" {
		return "A sentence[fn:tadg-001].\n\n\n[fn:tadg-001] " + text + "\n\n  Author: Taḋg; Created: 2026-09-14T10:00:00Z\n\n  #+BEGIN_COMMENT\n  " + strings.ReplaceAll(strings.TrimSuffix(meta, "\n"), "\n", "\n  ") + "\n  #+END_COMMENT\n"
	}
	return "A sentence[^tadg-001].\n\n\n[^tadg-001]: " + text + "\n\n    Author: Taḋg; Created: 2026-09-14T10:00:00Z\n\n    <!--\n    " + strings.ReplaceAll(strings.TrimSuffix(meta, "\n"), "\n", "\n    ") + "\n    -->\n"
}

func TestRT009_6_ReadNativeCurrentFootnote(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		t.Run(format, func(t *testing.T) {
			raw := []byte(nativeFixture(format, "The current comment."))
			store := Parse(raw, format)
			if store.Reason != "" || len(store.Events) != 1 {
				t.Fatalf("native current note was not recognized: reason=%q notes=%d", store.Reason, len(store.Events))
			}
			got := store.Events[0]
			if got.Text != "The current comment." || got.Author != "Taḋg" || got.Sequence != 3 {
				t.Fatalf("current value or attribution changed: %#v", got)
			}
			if !bytes.Equal(store.Source, []byte("A sentence.\n")) {
				t.Fatalf("authored bytes changed: %q", store.Source)
			}
		})
	}
}

func TestRT009_6_LiteralNativeExamplesAreNotOwned(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		text := nativeFixture(format, "Only an example.")
		if format == "org" {
			text = "#+BEGIN_SRC org\n" + text + "#+END_SRC\n"
		} else {
			text = "````markdown\n" + text + "````\n"
		}
		store := Parse([]byte(text), format)
		if len(store.Events) != 0 || string(store.Source) != text {
			t.Fatalf("%s literal example was adopted or changed", format)
		}
	}
}
