// ABOUTME: Shares Unicode and DOM text fixtures with the native browser suite.
// ABOUTME: Verifies unique anchoring and explicit heading ranges through rendered DOM.
package annotation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestRT007_9_CanonicalSharedFixtures(t *testing.T) {
	data, err := os.ReadFile("../../testdata/annotation-text.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct{ Name, HTML, Text string }
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			dom, err := html.Parse(strings.NewReader(fixture.HTML))
			if err != nil {
				t.Fatal(err)
			}
			if got := CanonicalText(dom); got != fixture.Text {
				t.Fatalf("got %q want %q", got, fixture.Text)
			}
		})
	}
}

func TestRT007_9_HeadingCorroboration(t *testing.T) {
	dom, err := html.Parse(strings.NewReader(`<main><section id="one"><h2>One</h2><p>same quote</p></section><section id="two"><h2>Two</h2><p>same quote</p></section></main>`))
	if err != nil {
		t.Fatal(err)
	}
	text, spans := CanonicalDocument(dom, map[string]string{"source-one": "one", "source-two": "two"})
	target := Target{Type: "text", BodyRevision: strings.Repeat("a", 64), Exact: "same quote", Start: 0, End: 10, HeadingID: "source-two"}
	resolved, status := Resolve(target, text, spans)
	if status != "resolved" || resolved.Start != 19 {
		t.Fatalf("resolved=%+v status=%s spans=%+v", resolved, status, spans)
	}
	target.HeadingID = ""
	if _, status = Resolve(target, text, spans); status != "ambiguous" {
		t.Fatal("duplicate quote chose a match")
	}
	target.Prefix = "missing "
	if _, status = Resolve(target, text, spans); status != "missing" {
		t.Fatal("conflicting context discarded")
	}
}
