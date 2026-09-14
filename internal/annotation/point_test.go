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
