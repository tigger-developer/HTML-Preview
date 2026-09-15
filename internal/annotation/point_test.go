// ABOUTME: Verifies bounded source candidates for clicked native footnote points.
// ABOUTME: Rejects ambiguous and literal contexts before the Pandoc render proof.
package annotation

import "testing"

func TestRT009_2_SourcePointCandidates(t *testing.T) {
	for _, c := range []struct {
		format, source, run string
		offset, want        int
	}{
		{"org", "* Heading\n\nA sentence.\n", "A sentence.", 10, len("* Heading\n\nA sentence")},
		{"markdown", "A café 😀 here.\n", "A café 😀 here.", 8, len("A café 😀")},
		{"org", "A sentence.\n\n", "A sentence.", 11, len("A sentence.")},
		{"org", "A sentence.\n\nA sentence.\n", "A sentence.", 5, -1},
		{"org", "#+BEGIN_SRC text\nA sentence.\n#+END_SRC\n", "A sentence.", 5, -1},
		{"markdown", "```\nA sentence.\n```\n", "A sentence.", 5, -1},
	} {
		got, err := SourcePointCandidate([]byte(c.source), c.format, c.run, c.offset)
		if c.want < 0 {
			if err == nil {
				t.Fatalf("unsafe candidate accepted: %#v", c)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("candidate=%d error=%v want=%d", got, err, c.want)
		}
	}
}

func TestHeadingPointCandidates(t *testing.T) {
	for _, tc := range []struct {
		format, source, run string
		offset, want        int
	}{
		{"org", "* TODO Heading text :tag:\n", "Heading text", 7, len("* TODO Heading")},
		{"markdown", "## Heading text ##\n", "Heading text", 7, len("## Heading")},
		{"org", "* TODO Heading :tag:\n", "TODO", 2, -1},
		{"org", "* TODO Heading :tag:\n", "tag", 1, -1},
	} {
		at, err := SourcePointCandidate([]byte(tc.source), tc.format, tc.run, tc.offset)
		if tc.want < 0 {
			if err == nil {
				t.Fatal("forbidden heading metadata admitted", tc)
			}
		} else if err != nil || at != tc.want {
			t.Fatalf("%+v: got %d %v", tc, at, err)
		}
	}
}

func TestLinkPointCandidates(t *testing.T) {
	for _, tc := range []struct {
		format, source, run string
		want                int
	}{
		{"org", "See [[file:notes.org][the guide]] next.", "the guide", len("See [[file:notes.org][the guide]]")},
		{"markdown", "See [the guide](notes.md) next.", "the guide", len("See [the guide](notes.md)")},
		{"markdown", "See [the guide](notes(a).md \"Title\") next.", "the guide", len("See [the guide](notes(a).md \"Title\")")},
		{"markdown", "See [the guide][ref] next.\n\n[ref]: notes.md", "the guide", len("See [the guide][ref]")},
		{"org", "#+BEGIN_SRC text\n[[file:a][guide]]\n#+END_SRC", "guide", -1},
	} {
		at, err := SourcePointCandidate([]byte(tc.source), tc.format, tc.run, len(tc.run), true)
		if tc.want < 0 {
			if err == nil {
				t.Fatal("literal link admitted")
			}
		} else if err != nil || at != tc.want {
			t.Fatalf("%+v: got %d %v", tc, at, err)
		}
	}
}

func TestAnnotationLinkSyntaxBoundaries(t *testing.T) {
	for _, tc := range []struct {
		format, source, run string
		want                int
	}{
		{"markdown", "See <https://example.org> now.", "https://example.org", len("See <https://example.org>")},
		{"markdown", "See https://example.org now.", "https://example.org", len("See https://example.org")},
		{"markdown", "See [the guide](notes.md) and [the guide](other.md).", "the guide", -1},
		{"markdown", "`[guide](notes.md)`", "guide", -1},
		{"org", "* WAIT Heading\n#+TODO: WAIT | FINISHED\n", "WAIT", -1},
	} {
		link := tc.run != "WAIT"
		at, err := SourcePointCandidate([]byte(tc.source), tc.format, tc.run, len(tc.run), link)
		if tc.want < 0 {
			if err == nil {
				t.Fatalf("unsafe: %+v at %d", tc, at)
			}
		} else if err != nil || at != tc.want {
			t.Fatalf("%+v: %d %v", tc, at, err)
		}
	}
}

func TestAfterLinkRejectsInteriorOffset(t *testing.T) {
	if _, err := SourcePointCandidate([]byte("See https://example.org now."), "markdown", "https://example.org", 4, true); err == nil {
		t.Fatal("after-link request admitted an interior URL offset")
	}
}
