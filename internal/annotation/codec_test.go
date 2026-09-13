// ABOUTME: Checks owned annotation framing through persisted byte streams.
// ABOUTME: Protects literal examples, exact prefixes and append-history validation.
package annotation

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func fixtureEvent() Event {
	return Event{Schema: 1, OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Kind: "draft", Author: "Reviewer", CreatedAt: "2026-09-13T12:00:00Z", RecordedAt: "2026-09-13T12:00:00Z", Target: Target{Type: "document"}, Text: "Literal --> and #+end_comment\n<script>"}
}

func TestRT007_3_FrameRoundTripAndLiteralRecognition(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		for _, prefix := range []string{"", "Body", "\xef\xbb\xbfBody\r\n", "Body\n\n"} {
			header := Header{Schema: 1, DocumentID: "40000000-0000-4000-8000-000000000001", SourceFormat: format}
			store := Parse([]byte(prefix), format)
			addition, err := AppendBytes(store, &header, fixtureEvent(), format)
			if err != nil {
				t.Fatal(err)
			}
			all := append([]byte(prefix), addition...)
			parsed := Parse(all, format)
			if parsed.Reason != "" || !bytes.Equal(parsed.Source, []byte(prefix)) || len(parsed.Events) != 1 || parsed.Events[0].Text != fixtureEvent().Text {
				t.Fatalf("%s prefix=%q result=%+v", format, prefix, parsed)
			}
			if bytes.Contains(addition, []byte("<script>")) || bytes.Contains(addition, []byte("Literal -->")) {
				t.Fatal("raw delimiter in frame JSON")
			}
			for _, wrap := range [][2]string{{"#+BEGIN_SRC org\n", "\n#+END_SRC\n"}, {"```org\n", "\n```\n"}} {
				if (format == "org") != strings.HasPrefix(wrap[0], "#+") {
					continue
				}
				literal := []byte(wrap[0] + string(all) + wrap[1])
				if got := Parse(literal, format); got.Header != nil || !bytes.Equal(literal, got.Source) || got.Reason != "" {
					t.Fatal("literal frame interpreted as storage")
				}
			}
		}
	}
}

func TestRT007_3_MalformedOwnedTailAndUnsafeEOF(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		header := Header{Schema: 1, DocumentID: "40000000-0000-4000-8000-000000000001", SourceFormat: format}
		prefix := []byte("Body")
		addition, err := AppendBytes(Parse(prefix, format), &header, fixtureEvent(), format)
		if err != nil {
			t.Fatal(err)
		}
		for _, tail := range [][]byte{addition[:len(addition)-3], append(append([]byte{}, addition...), []byte("Authored after store")...)} {
			parsed := Parse(append(append([]byte{}, prefix...), tail...), format)
			if parsed.Reason == "" || !bytes.Equal(parsed.Source, prefix) {
				t.Fatalf("corrupt tail accepted or exposed: %+v", parsed)
			}
		}
	}
	for _, tc := range []struct{ format, body string }{{"org", "#+BEGIN_SRC go\nx"}, {"markdown", "```\nx"}, {"markdown", "<!-- open"}, {"org", "#+begin_comment\nordinary"}} {
		if Parse([]byte(tc.body), tc.format).Safe {
			t.Fatal("unsafe embedded insertion accepted")
		}
	}
}

func TestRT007_10_EscapedFrameLimit(t *testing.T) {
	event := fixtureEvent()
	event.Text = strings.Repeat("-", 4000)
	event.Target = Target{Type: "text", BodyRevision: strings.Repeat("a", 64), Exact: strings.Repeat("-", 8192), Start: 0, End: 8192}
	_, err := Frame("org", eventMarker, event, "\n")
	var failure *Failure
	if !errors.As(err, &failure) || failure.Code != "body_limit" {
		t.Fatalf("oversized escaped frame needs bounded client error: %v", err)
	}
}

func TestRT007_6_ProjectionRetainsValidEventsBeforeConflict(t *testing.T) {
	first := fixtureEvent()
	second := first
	second.OperationID = "10000000-0000-4000-8000-000000000002"
	second.Sequence = 3
	latest, _, err := Project([]Event{first, second})
	if err == nil || len(latest) != 1 || latest[0] != first {
		t.Fatalf("valid event lost on conflicting sequence: %v %#v", err, latest)
	}
}
