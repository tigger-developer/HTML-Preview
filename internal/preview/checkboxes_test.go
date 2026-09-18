// ABOUTME: Exercises passive checkbox rendering through complete document previews.
// ABOUTME: Protects state variants, spacing, alternative bullets and literal exclusions.
package preview

import (
	"strings"
	"testing"
)

func TestCheckboxRendering(t *testing.T) {
	for _, format := range []string{"org", "md"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			body := "- [ ] Empty\n- [X] Upper\n- [x] Lower\n- [-] Dash\n- [/] Slash\n\n+ [X] Plus\n\n1. [ ] Ordered\n\n"
			if format == "org" {
				body += "  * [/] Star\n\n[ ] Standalone\n\n- =[/]= Literal inline\n\n#+BEGIN_SRC text\n- [-] Literal block\n#+END_SRC\n"
			} else {
				body += "* [/] Star\n\n[ ] Standalone\n\n- `[/]` Literal inline\n\n```text\n- [-] Literal block\n```\n"
			}
			result := run(t, dir, nil, source(t, dir, "tasks."+format, body))
			success(t, result, 1)
			main := documentNode(t, result.pages[0], "hp-document")
			want := []string{"unchecked", "checked", "checked", "partial", "partial", "checked", "unchecked", "partial"}
			count := 0
			for _, span := range nodes(main, "span") {
				if !hasClass(span, "hp-checkbox") {
					continue
				}
				if count >= len(want) || !hasClass(span, "hp-checkbox-"+want[count]) {
					t.Fatalf("incorrect checkbox state %d: %s", count, attr(span, "class"))
				}
				if attr(span, "role") != "img" || attr(span, "aria-label") == "" {
					t.Fatal("checkbox lacks an accessible state label")
				}
				if span.NextSibling == nil || !strings.HasPrefix(span.NextSibling.Data, " ") {
					t.Fatal("missing space between checkbox and item text")
				}
				count++
			}
			if count != len(want) {
				t.Fatalf("want %d checkboxes, got %d", len(want), count)
			}
			if len(nodes(main, "input")) != 0 || len(nodes(main, "label")) != 0 {
				t.Fatal("passive task indicators retained interactive inputs or obsolete labels")
			}
			if !strings.Contains(textOf(main), "[ ] Standalone") || !strings.Contains(textOf(main), "- [-] Literal block") {
				t.Fatal("standalone or literal checkbox syntax was changed")
			}
		})
	}
}
