// ABOUTME: Serializes current native annotations and patches only their owned spans.
// ABOUTME: Leaves unrelated source, ordinary notes and previous authors' bytes intact.
package annotation

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type sourcePatch struct {
	byteRange
	text []byte
}

func footnoteReference(format, label string) string {
	if format == "org" {
		return "[fn:" + label + "]"
	}
	return "[^" + label + "]"
}

func metadataValue(value string) string {
	value = strings.NewReplacer("&", "&#38;", "<", "&#60;", ">", "&#62;").Replace(value)
	return strings.ReplaceAll(value, "--", "-&#45;")
}

func literalFootnoteBody(text, format string) string {
	// Plain letters/numbers and sentence punctuation need no markup escaping.
	unsafe := strings.ContainsAny(text, "[]<>*_`~=#\\&") || strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ") || strings.Contains(text, "\n")
	if !unsafe {
		return text
	}
	if format == "org" {
		lines := strings.Split(text, "\n")
		for i := range lines {
			// Prefix every line consistently; the native reader removes only
			// Org comma escapes, so plain lines need no comma.
			if strings.HasPrefix(lines[i], ",") || strings.HasPrefix(lines[i], "*") || strings.HasPrefix(strings.TrimLeft(lines[i], " \t"), "#+") {
				lines[i] = "," + lines[i]
			}
		}
		return "#+BEGIN_EXAMPLE\n" + strings.Join(lines, "\n") + "\n#+END_EXAMPLE"
	}
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return fence + "\n" + text + "\n" + fence
}

func encodeFootnote(format string, header Header, event Event, label, ending string) ([]byte, error) {
	if !ValidLabel(label) || !HeaderValid(header) || !ValidID(event.AnnotationID) || !ValidID(event.OperationID) || !ValidText(event.Text) || ValidateName(event.Author) != nil {
		return nil, fail("invalid_event")
	}
	state := "draft"
	if event.Kind == "close" {
		state = "closed"
	}
	meta := [][2]string{{"DOCUMENT_ID", header.DocumentID}, {"UUID", event.AnnotationID}, {"AUTHOR", event.Author}, {"CREATED", event.CreatedAt}, {"UPDATED", event.RecordedAt}, {"STATE", state}, {"OPERATION", event.OperationID}, {"REVISION", strconv.Itoa(event.Sequence)}}
	if event.Target.Type == "document" {
		meta = append(meta, [2]string{"POINT", "unplaced"})
	}
	indent, open, close := "  ", "#+BEGIN_COMMENT", "#+END_COMMENT"
	definition := footnoteReference(format, label)
	if format != "org" {
		indent, open, close, definition = "    ", "<!--", "-->", definition+":"
	}
	body := literalFootnoteBody(event.Text, format)
	// A literal block must start on its own line, not after the label.
	if strings.HasPrefix(body, "#+BEGIN_EXAMPLE") || strings.HasPrefix(body, "```") {
		body = "\n" + body
	}
	body += "\n\nAuthor: " + event.Author + "; Created: " + event.CreatedAt + "\n\n" + open + "\n" + nativeMarker + "\n"
	for _, pair := range meta {
		body += pair[0] + ": " + metadataValue(pair[1]) + "\n"
	}
	body += close
	lines := strings.Split(body, "\n")
	var result strings.Builder
	result.WriteString(definition + " " + lines[0] + ending)
	for _, line := range lines[1:] {
		if line != "" {
			result.WriteString(indent + line)
		}
		result.WriteString(ending)
	}
	if result.Len() > MaxFrame {
		return nil, fail("body_limit")
	}
	return []byte(result.String()), nil
}

// UpdateFootnote patches a verified raw byte position for a new note. Later
// updates find the existing reference through ownership, never a stale offset.
func UpdateFootnote(data []byte, format string, header Header, event Event, label string, position int) ([]byte, error) {
	store := Parse(data, format)
	if store.Reason != "" {
		return nil, fail(store.Reason)
	}
	if !store.Safe {
		return nil, fail("unsafe_source")
	}
	if !ValidLabel(label) {
		return nil, fail("invalid_label")
	}
	var existing *Footnote
	for i := range store.Notes {
		if store.Notes[i].Event.AnnotationID == event.AnnotationID {
			existing = &store.Notes[i]
		}
	}
	if store.Labels[strings.ToLower(label)] && (existing == nil || !strings.EqualFold(existing.Label, label)) {
		return nil, fail("label_conflict")
	}
	clearing := event.Text == ""
	if existing != nil && existing.Event.Kind == "close" {
		return nil, fail("closed_comment")
	}
	var encoded []byte
	var err error
	if !clearing {
		encoded, err = encodeFootnote(format, header, event, label, store.Ending)
		if err != nil {
			return nil, err
		}
	}
	var patches []sourcePatch
	if existing != nil {
		if len(existing.references) != 1 && existing.Event.Target.Type != "document" {
			return nil, fail("target_unresolved")
		}
		r := existing.definition
		if clearing {
			separator := []byte(store.Ending + store.Ending)
			if r.start >= len(separator) && bytes.Equal(data[r.start-len(separator):r.start], separator) {
				r.start -= len(separator)
			}
		}
		patches = append(patches, sourcePatch{r, encoded})
		for _, ref := range existing.references {
			var token []byte
			if !clearing {
				token = []byte(footnoteReference(format, label))
			}
			patches = append(patches, sourcePatch{ref, token})
		}
	} else {
		if clearing {
			return append([]byte(nil), data...), nil
		}
		if event.Target.Type != "document" {
			if position < 0 || position > len(data) {
				return nil, fail("point_unmappable")
			}
			patches = append(patches, sourcePatch{byteRange{position, position}, []byte(footnoteReference(format, label))})
		}
		patches = append(patches, sourcePatch{byteRange{len(data), len(data)}, append([]byte(store.Ending+store.Ending), encoded...)})
	}
	return applySourcePatches(data, patches)
}

func applySourcePatches(data []byte, patches []sourcePatch) ([]byte, error) {
	sort.SliceStable(patches, func(i, j int) bool { return patches[i].start < patches[j].start })
	var out bytes.Buffer
	at := 0
	for _, patch := range patches {
		if patch.start < at || patch.end < patch.start || patch.end > len(data) {
			return nil, fmt.Errorf("overlapping annotation patch")
		}
		out.Write(data[at:patch.start])
		out.Write(patch.text)
		at = patch.end
	}
	out.Write(data[at:])
	if out.Len() > MaxStore {
		return nil, fail("store_limit")
	}
	return out.Bytes(), nil
}
