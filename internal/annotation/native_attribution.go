// ABOUTME: Writes ordinary attributed footnotes without hidden autosave records.
// ABOUTME: Reads optional native attribution independently of application ownership.
package annotation

import (
	"regexp"
	"strings"
	"time"
)

func orgTimestamp(value string) string {
	for _, layout := range []string{"[2006-01-02 Mon 15:04]", "[2006-01-02 Mon]"} {
		if _, err := time.Parse(layout, value); err == nil {
			return value
		}
	}
	if date, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return date.In(time.Local).Format("[2006-01-02 Mon 15:04]")
	}
	return ""
}

func splitAttribution(text string) (body, attribution, author, created string) {
	at := strings.LastIndex(text, "\n")
	if at < 0 {
		return text, "", "", ""
	}
	line := strings.TrimSpace(text[at+1:])
	name, date, ok := strings.Cut(strings.TrimPrefix(line, "Author: "), "; Created: ")
	if !ok {
		name, date, ok = strings.Cut(strings.TrimPrefix(line, "Author: "), "; Edited: ")
	}
	if !ok || !strings.HasPrefix(line, "Author: ") || ValidateName(name) != nil || orgTimestamp(date) == "" {
		return text, "", "", ""
	}
	return strings.TrimRight(text[:at], "\n"), text[at+1:], name, date
}

func encodeReadableFootnote(format string, event Event, label, ending string, edited bool) ([]byte, error) {
	if !ValidText(event.Text) || (event.Author != "" && ValidateName(event.Author) != nil) {
		return nil, fail("invalid_event")
	}
	date := orgTimestamp(event.CreatedAt)
	if event.CreatedAt != "" && date == "" {
		return nil, fail("invalid_event")
	}
	definition := footnoteReference(format, label)
	indent := ""
	if format != "org" {
		definition += ":"
		indent = "    "
	}
	text := event.Text
	if event.Target.Type == "document" && event.Target.Prefix+event.Target.Suffix != "" {
		text += "\n\nUnplaced annotation. Context: " + event.Target.Prefix + " | " + event.Target.Suffix
	}
	body := text
	if event.Author != "" && date != "" {
		kind := "Created"
		if edited {
			kind = "Edited"
		}
		body += "\n\nAuthor: " + event.Author + "; " + kind + ": " + date
	}
	lines := strings.Split(body, "\n")
	result := definition + " " + lines[0] + ending
	for _, line := range lines[1:] {
		if line != "" {
			result += indent + line
		}
		result += ending
	}
	if len(result) > MaxFrame {
		return nil, fail("body_limit")
	}
	encoded := []byte(result)
	notes := EditableFootnotes(encoded, format, "embedded")
	if len(notes) != 1 || notes[0].Label != label || notes[0].Text != text || notes[0].definition.end != len(encoded) || !Parse(encoded, format).Safe {
		return nil, fail("invalid_footnote")
	}
	return encoded, nil
}

// Old frames are decoded for compatibility, then emitted as ordinary current
// definitions on an authorized save. They are never written back as frames.
func readableFootnotes(data []byte, format, editedLabel string) ([]byte, error) {
	store := Parse(data, format)
	if store.Reason != "" {
		return nil, fail(store.Reason)
	}
	var patches []sourcePatch
	for _, note := range store.Notes {
		encoded, err := encodeReadableFootnote(format, note.Event, note.Label, store.Ending, note.Label == editedLabel)
		if err != nil {
			return nil, err
		}
		patches = append(patches, sourcePatch{note.definition, encoded})
	}
	return applySourcePatches(data, patches)
}

// Creation-session bookkeeping is transient. Restore its last verified value
// only in the working buffer so the existing exact reference patcher can run.
func restoreCurrentFrame(data []byte, format string, header Header, current *currentSave) ([]byte, error) {
	if current == nil || current.event.Text == "" {
		return data, nil
	}
	for _, note := range EditableFootnotes(data, format, "embedded") {
		if note.Label != current.event.Label {
			continue
		}
		if note.Revision != current.definitionRevision {
			return nil, fail("footnote_conflict")
		}
		encoded, err := encodeFootnote(format, header, current.event, note.Label, Parse(data, format).Ending)
		if err != nil {
			return nil, err
		}
		return applySourcePatches(data, []sourcePatch{{note.definition, encoded}})
	}
	return nil, fail("footnote_conflict")
}

// Authored-body revisions exclude all native footnote definitions/references,
// independently of who wrote them. Full-store and per-definition digests still
// detect changes to notes themselves.
func FootnoteBodySource(data []byte, format string) []byte {
	var spans []byteRange
	ending := Parse(data, format).Ending
	for _, note := range EditableFootnotes(data, format, "embedded") {
		start := note.definition.start
		separator := ending + ending
		if start >= len(separator) && string(data[start-len(separator):start]) == separator {
			start -= len(separator)
		}
		spans = append(spans, byteRange{start, note.definition.end})
	}
	literal := literalContext{}
	for _, line := range sourceLines(data) {
		if inRanges(line.offset, spans) {
			continue
		}
		if literal.active() {
			literal.consume(line.text, format)
			continue
		}
		if literal.consume(line.text, format); literal.active() {
			continue
		}
		if format != "org" && (strings.HasPrefix(line.text, "    ") || strings.HasPrefix(line.text, "\t")) {
			continue
		}
		for _, match := range referencePattern(format).FindAllStringIndex(line.text, -1) {
			if !inlineLiteral(line.text, match[0], format) {
				spans = append(spans, byteRange{line.offset + match[0], line.offset + match[1]})
			}
		}
	}
	return omitRanges(data, spans)
}

func nativeReferences(data []byte, format string) map[string]int {
	spans := []byteRange{}
	for _, note := range EditableFootnotes(data, format, "embedded") {
		spans = append(spans, note.definition)
	}
	result := map[string]int{}
	literal := literalContext{}
	for _, line := range sourceLines(data) {
		if inRanges(line.offset, spans) {
			continue
		}
		if literal.active() {
			literal.consume(line.text, format)
			continue
		}
		if literal.consume(line.text, format); literal.active() {
			continue
		}
		if format != "org" && (strings.HasPrefix(line.text, "    ") || strings.HasPrefix(line.text, "\t")) {
			continue
		}
		for _, match := range referencePattern(format).FindAllStringSubmatchIndex(line.text, -1) {
			if !inlineLiteral(line.text, match[0], format) {
				result[line.text[match[2]:match[3]]]++
			}
		}
	}
	return result
}

var nativeAttributionLine = regexp.MustCompile(`^Author:.*\[[0-9]{4}-[0-9]{2}-[0-9]{2} [A-Za-z]{3}(?: [0-9]{2}:[0-9]{2})?\]$`)

func refreshAttribution(line, author, now string) string {
	if !nativeAttributionLine.MatchString(strings.TrimSpace(line)) {
		return line
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	return indent + "Author: " + author + "; Edited: " + orgTimestamp(now)
}
