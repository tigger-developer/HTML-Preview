// ABOUTME: Reconstructs native sidecar notes from readable context and definitions.
// ABOUTME: Keeps reference placement verifiable after browser or service restarts.
package annotation

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func sidecarContext(data []byte, label string) (Target, byteRange, bool) {
	suffix := footnoteReference("org", label)
	for _, line := range sourceLines(data) {
		if !strings.HasPrefix(line.text, "Context: ") || !strings.HasSuffix(line.text, suffix) {
			continue
		}
		context := strings.TrimSuffix(strings.TrimPrefix(line.text, "Context: "), suffix)
		target := Target{Type: "document", Prefix: context}
		if strings.Count(context, " | ") == 1 {
			target.Prefix, target.Suffix, _ = strings.Cut(context, " | ")
			target.Prefix = strings.ReplaceAll(target.Prefix, "\\[", "[")
			target.Suffix = strings.ReplaceAll(target.Suffix, "\\[", "[")
			target.Type = "point"
		}
		return target, byteRange{line.offset, line.end}, true
	}
	return Target{}, byteRange{}, false
}

func validNativeSidecar(data []byte) bool {
	var spans []byteRange
	for _, note := range EditableFootnotes(data, "org", "sidecar") {
		_, span, ok := sidecarContext(data, note.Label)
		if !ok {
			return false
		}
		spans = append(spans, note.definition, span)
	}
	return len(spans) > 0 && strings.TrimSpace(string(omitRanges(data, spans))) == ""
}

func nativeSidecarEvents(data []byte) []Event {
	var events []Event
	for _, note := range EditableFootnotes(data, "org", "sidecar") {
		if note.Owned {
			continue
		}
		target, _, ok := sidecarContext(data, note.Label)
		if !ok {
			continue
		}
		sum := sha256.Sum256([]byte(note.Label))
		id := fmt.Sprintf("%x-%x-%x-%x-%x", sum[:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
		events = append(events, Event{Schema: 2, AnnotationID: id, OperationID: id, Sequence: 1, Kind: "close", Label: note.Label, Text: note.Text, Author: note.Author, CreatedAt: note.CreatedAt, RecordedAt: note.CreatedAt, Target: target})
	}
	return events
}
