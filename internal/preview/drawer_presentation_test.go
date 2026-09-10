// ABOUTME: Checks readable drawer content through the real preview command.
// ABOUTME: Delimiters are syntax; property values and free text remain content.
package preview

import (
	"strings"
	"testing"
)

func TestRT005_1_DrawerPresentation(t *testing.T) {
	root := t.TempDir()
	input := "* Heading\n:PROPERTIES:\n:CUSTOM_ID: sdlc-migration\n:MIGRATED: 2026-09-06\n:OWNER: <Reader> & writer\n:OWNER: Again\n:EMPTY:\n:O&<KEY>: safe\n:END:\n:LOGBOOK:\nCLOCK: [2026-09-10 Thu]\n:NOTE: Some text\nFree-form <text> & data\n:END:\n:EMPTY:\n:END:\n:UNTERMINATED:\n:KEY+: final value\nRemaining text\n"
	r := run(t, root, nil, source(t, root, "drawers.org", input))
	success(t, r, 1)
	drawers := nodes(documentNode(t, r.pages[0], "hp-document"), "details")
	if len(drawers) != 4 {
		t.Fatalf("drawers=%d, want 4", len(drawers))
	}
	for i, name := range []string{"PROPERTIES", "LOGBOOK", "EMPTY", "UNTERMINATED"} {
		summaries := nodes(drawers[i], "summary")
		if len(summaries) != 1 || textOf(summaries[0]) != "⚙ "+name {
			t.Errorf("drawer %d summary", i)
		}
		if strings.Contains(textOf(drawers[i]), ":"+name+":") || strings.Contains(textOf(drawers[i]), ":END:") {
			t.Errorf("drawer %s repeated source delimiters", name)
		}
	}
	keys, values := nodes(drawers[0], "dt"), nodes(drawers[0], "dd")
	wantKeys := []string{"CUSTOM_ID", "MIGRATED", "OWNER", "OWNER", "EMPTY", "O&<KEY>"}
	wantValues := []string{"sdlc-migration", "2026-09-06", "<Reader> & writer", "Again", "", "safe"}
	if len(keys) != len(wantKeys) || len(values) != len(wantValues) {
		t.Fatalf("property rows keys=%d values=%d, want %d", len(keys), len(values), len(wantKeys))
	}
	for i := range wantKeys {
		if textOf(keys[i]) != "⚙ "+wantKeys[i] || textOf(values[i]) != wantValues[i] {
			t.Errorf("property %d: %q = %q", i, textOf(keys[i]), textOf(values[i]))
		}
	}
	if len(nodes(drawers[0], "pre")) != 0 {
		t.Fatal("properties still rendered as a preformatted inner box")
	}
	log := textOf(drawers[1])
	if !strings.Contains(log, "CLOCK: [2026-09-10 Thu]") || !strings.Contains(log, "Free-form <text> & data") || len(nodes(drawers[1], "dt")) != 1 {
		t.Fatal("mixed drawer lost free-form or property content")
	}
	if len(nodes(drawers[2], "dl")) != 0 || len(nodes(drawers[2], "pre")) != 0 {
		t.Fatal("empty drawer gained content")
	}
	if textOf(nodes(drawers[3], "dt")[0]) != "⚙ KEY+" || !strings.Contains(textOf(drawers[3]), "Remaining text") {
		t.Fatal("unterminated drawer lost content")
	}
}
