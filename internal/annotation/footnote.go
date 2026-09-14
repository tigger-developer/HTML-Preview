// ABOUTME: Reads owned current annotations from native Org and Markdown footnotes.
// ABOUTME: Keeps authored bytes and native rendering separate from hidden ownership metadata.
package annotation

import (
	"bytes"
	"errors"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const nativeMarker = "HTMLPREVIEW_ANNOTATION: 2"

type byteRange struct{ start, end int }

// Footnote records source positions, never a browser-provided file offset.
type Footnote struct {
	Label                string
	DocumentID           string
	Event                Event
	definition, metadata byteRange
	references           []byteRange
}

var labelPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var orgDefinition = regexp.MustCompile(`^\[fn:([^]\s]+)\][ \t]*(.*)$`)
var mdDefinition = regexp.MustCompile(`^\[\^([^]\s]+)\]:[ \t]*(.*)$`)
var orgReference = regexp.MustCompile(`\[fn:([^]\s]+)\]`)
var mdReference = regexp.MustCompile(`\[\^([^]\s]+)\]`)

func ValidLabel(label string) bool { return labelPattern.MatchString(label) }

// Parse retains the v1 decoder solely for reading and later explicit migration.
func Parse(data []byte, format string) Store {
	store := parseLegacy(data, format)
	store.Rendered = store.Source
	store.Labels = make(map[string]bool)
	if store.Reason != "" {
		return store
	}
	parseFootnotes(&store, format)
	return store
}

func definitionPattern(format string) *regexp.Regexp {
	if format == "org" {
		return orgDefinition
	}
	return mdDefinition
}

func referencePattern(format string) *regexp.Regexp {
	if format == "org" {
		return orgReference
	}
	return mdReference
}

func parseFootnotes(store *Store, format string) {
	data := store.Source
	lines := sourceLines(data)
	literal := literalContext{}
	var removed, hidden []byteRange
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if literal.active() {
			literal.consume(line.text, format)
			continue
		}
		parts := definitionPattern(format).FindStringSubmatch(line.text)
		if parts == nil {
			literal.consume(line.text, format)
			continue
		}
		store.Labels[strings.ToLower(parts[1])] = true
		note, next, owned, err := readFootnote(lines, i, format)
		if err != nil {
			store.Reason = "corrupt_store"
			return
		}
		if !owned {
			continue
		}
		if store.Header != nil && store.Header.DocumentID != note.DocumentID {
			store.Reason = "corrupt_store"
			return
		}
		store.Header = &Header{1, note.DocumentID, format}
		if len(store.Notes) >= MaxEvents {
			store.Reason = "store_limit"
			return
		}
		for _, old := range store.Notes {
			if strings.EqualFold(old.Label, note.Label) || old.Event.AnnotationID == note.Event.AnnotationID {
				store.Reason = "corrupt_store"
				return
			}
		}
		start := note.definition.start
		separator := []byte(store.Ending + store.Ending)
		if start >= len(separator) && bytes.Equal(data[start-len(separator):start], separator) {
			start -= len(separator)
		}
		removed = append(removed, byteRange{start, note.definition.end})
		hidden = append(hidden, note.metadata)
		store.Notes = append(store.Notes, note)
		store.Events = append(store.Events, note.Event)
		store.TailBytes += note.definition.end - start
		i = next - 1
	}
	// References are recognized only outside block and inline literal regions.
	literal = literalContext{}
	for _, line := range lines {
		if inRanges(line.offset, removed) {
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
			if inlineLiteral(line.text, match[0], format) {
				continue
			}
			label := line.text[match[2]:match[3]]
			store.Labels[strings.ToLower(label)] = true
			for n := range store.Notes {
				if label == store.Notes[n].Label {
					r := byteRange{line.offset + match[0], line.offset + match[1]}
					store.Notes[n].references = append(store.Notes[n].references, r)
					removed = append(removed, r)
				}
			}
		}
	}
	store.Source = omitRanges(data, removed)
	store.Rendered = omitRanges(data, hidden)
	if store.TailBytes > MaxStore {
		store.Reason = "store_limit"
	}
}

func inRanges(at int, ranges []byteRange) bool {
	for _, r := range ranges {
		if at >= r.start && at < r.end {
			return true
		}
	}
	return false
}

func inlineLiteral(line string, at int, format string) bool {
	if at > 0 && line[at-1] == '\\' {
		return true
	}
	markers := "`"
	if format == "org" {
		markers = "~="
	}
	for _, marker := range markers {
		if strings.Count(line[:at], string(marker))%2 != 0 {
			return true
		}
	}
	return false
}

func omitRanges(data []byte, ranges []byteRange) []byte {
	// Source files are bounded; sorting once avoids repeated full-file replacements.
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	var out bytes.Buffer
	at := 0
	for _, r := range ranges {
		if r.start < at {
			continue
		}
		out.Write(data[at:r.start])
		at = r.end
	}
	out.Write(data[at:])
	return out.Bytes()
}

func readFootnote(lines []sourceLine, start int, format string) (Footnote, int, bool, error) {
	parts := definitionPattern(format).FindStringSubmatch(lines[start].text)
	note := Footnote{Label: parts[1], definition: byteRange{start: lines[start].offset}}
	indent, open, close := "  ", "#+BEGIN_COMMENT", "#+END_COMMENT"
	if format != "org" {
		indent, open, close = "    ", "<!--", "-->"
	}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i].text
		if definitionPattern(format).MatchString(line) || (line != "" && !strings.HasPrefix(line, indent)) {
			break
		}
		if strings.TrimSpace(line) != open || i+1 >= len(lines) || strings.TrimSpace(lines[i+1].text) != nativeMarker {
			continue
		}
		if !ValidLabel(note.Label) {
			return note, i, true, errors.New("invalid owned label")
		}
		fields := make(map[string]string)
		end := i + 2
		for ; end < len(lines) && strings.TrimSpace(lines[end].text) != close; end++ {
			key, value, ok := strings.Cut(strings.TrimSpace(lines[end].text), ": ")
			if !ok || fields[key] != "" {
				return note, end, true, errors.New("invalid metadata")
			}
			fields[key] = html.UnescapeString(value)
		}
		if end >= len(lines) {
			return note, end, true, errors.New("unclosed metadata")
		}
		event, header, err := nativeMetadata(fields, format)
		if err != nil {
			return note, end, true, err
		}
		note.DocumentID = header.DocumentID
		body := []string{parts[2]}
		for j := start + 1; j < i; j++ {
			body = append(body, strings.TrimPrefix(lines[j].text, indent))
		}
		text := strings.TrimRight(strings.Join(body, "\n"), "\n")
		attribution := "Author: " + event.Author + "; Created: " + event.CreatedAt
		if !strings.HasSuffix(text, "\n\n"+attribution) {
			return note, end, true, errors.New("missing attribution")
		}
		text = strings.TrimSuffix(text, "\n\n"+attribution)
		event.Text, err = decodeFootnoteText(text, format)
		if err != nil || !ValidText(event.Text) {
			return note, end, true, errors.New("invalid comment text")
		}
		note.Event = event
		note.metadata = byteRange{lines[i].offset, lines[end].end}
		note.definition.end = lines[end].end
		if note.definition.end-note.definition.start > MaxFrame {
			return note, end, true, errors.New("oversized note")
		}
		return note, end + 1, true, nil
	}
	return note, start + 1, false, nil
}

func nativeMetadata(fields map[string]string, format string) (Event, Header, error) {
	allowed := " DOCUMENT_ID UUID AUTHOR CREATED UPDATED STATE OPERATION REVISION POINT BEFORE AFTER HEADING_ID "
	for key := range fields {
		if !strings.Contains(allowed, " "+key+" ") {
			return Event{}, Header{}, errors.New("unknown metadata")
		}
	}
	for _, key := range strings.Fields("DOCUMENT_ID UUID AUTHOR CREATED UPDATED STATE OPERATION REVISION") {
		if fields[key] == "" {
			return Event{}, Header{}, errors.New("missing metadata")
		}
	}
	seq, err := strconv.Atoi(fields["REVISION"])
	if err != nil || seq < 1 || !ValidID(fields["DOCUMENT_ID"]) || !ValidID(fields["UUID"]) || !ValidID(fields["OPERATION"]) || ValidateName(fields["AUTHOR"]) != nil {
		return Event{}, Header{}, errors.New("invalid metadata value")
	}
	for _, key := range []string{"CREATED", "UPDATED"} {
		if _, err := time.Parse(time.RFC3339Nano, fields[key]); err != nil || !strings.HasSuffix(fields[key], "Z") {
			return Event{}, Header{}, errors.New("invalid timestamp")
		}
	}
	kind := "draft"
	if fields["STATE"] == "closed" {
		kind = "close"
	} else if fields["STATE"] != "draft" {
		return Event{}, Header{}, errors.New("invalid state")
	}
	target := Target{Type: "point"}
	if fields["POINT"] == "unplaced" {
		target.Type = "document"
	}
	return Event{Schema: 2, AnnotationID: fields["UUID"], OperationID: fields["OPERATION"], Author: fields["AUTHOR"], CreatedAt: fields["CREATED"], RecordedAt: fields["UPDATED"], Sequence: seq, Kind: kind, Target: target}, Header{1, fields["DOCUMENT_ID"], format}, nil
}

func decodeFootnoteText(text, format string) (string, error) {
	if strings.HasPrefix(text, "\n#+BEGIN_EXAMPLE\n") || strings.HasPrefix(text, "\n```") {
		text = text[1:]
	}
	if !utf8.ValidString(text) {
		return "", errors.New("invalid UTF-8")
	}
	if format == "org" && strings.HasPrefix(text, "#+BEGIN_EXAMPLE\n") {
		if !strings.HasSuffix(text, "\n#+END_EXAMPLE") {
			return "", errors.New("invalid literal body")
		}
		text = strings.TrimSuffix(strings.TrimPrefix(text, "#+BEGIN_EXAMPLE\n"), "\n#+END_EXAMPLE")
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, ",") {
				lines[i] = line[1:]
			}
		}
		return strings.Join(lines, "\n"), nil
	}
	if format != "org" && strings.HasPrefix(text, "```") {
		fence, body, ok := strings.Cut(text, "\n")
		if !ok || strings.Trim(fence, "`") != "" || !strings.HasSuffix(body, "\n"+fence) {
			return "", errors.New("invalid literal body")
		}
		return strings.TrimSuffix(body, "\n"+fence), nil
	}
	return text, nil
}
