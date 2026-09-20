// ABOUTME: Distinguishes rendered Org footnote references from literal examples.
// ABOUTME: Keeps virtual unplaced entries only for genuinely unreferenced definitions.
package annotation

import (
	"strings"
	"testing"
)

func TestPreviewFootnotesOrgContainers(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		placed     bool
	}{
		{"quote", "#+BEGIN_QUOTE\nA[fn:note] word.\n#+END_QUOTE\n", true},
		{"indented-quote", "- Draft ::\n\n  #+begin_quote\n  A[fn:note] word.\n  #+end_quote\n", true},
		{"verse", "#+BEGIN_VERSE\nA[fn:note] line.\n#+END_VERSE\n", true},
		{"center", "#+BEGIN_CENTER\nA[fn:note] line.\n#+END_CENTER\n", true},
		{"source", "#+BEGIN_SRC text\nA[fn:note] example.\n#+END_SRC\n", false},
		{"example", "#+BEGIN_EXAMPLE\nA[fn:note] example.\n#+END_EXAMPLE\n", false},
		{"comment", "#+BEGIN_COMMENT\nA[fn:note] example.\n#+END_COMMENT\n", false},
		{"export", "#+BEGIN_EXPORT html\nA[fn:note] example.\n#+END_EXPORT\n", false},
		{"literal-in-quote", "#+BEGIN_QUOTE\n=literal [fn:note]=\n#+END_QUOTE\n", false},
		{"source-in-quote", "#+BEGIN_QUOTE\n#+BEGIN_SRC text\nA[fn:note] example.\n#+END_SRC\n#+END_QUOTE\n", false},
		{"unreferenced", "A paragraph.\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.body + "\n[fn:note] A synthetic note.\n")
			output, virtual, err := PreviewFootnotes(t.Context(), Snapshot{RawSource: input}, "org", "", nil, "PROBE")
			want := 1
			if tc.placed {
				want = 0
			}
			if err != nil || len(virtual) != want {
				t.Fatalf("virtual notes=%d want=%d error=%v", len(virtual), want, err)
			}
			if tc.placed && string(output) != string(input) {
				t.Fatal("placed note changed conversion input")
			}
			if !tc.placed && (!strings.Contains(string(output), "PROBE0Z[fn:note]") || virtual[0].Position != -1) {
				t.Fatal("unreferenced definition lost its unplaced entry")
			}
		})
	}
}
