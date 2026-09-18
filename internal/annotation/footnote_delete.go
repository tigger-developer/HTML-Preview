// ABOUTME: Removes a native footnote and its references without rewriting the document.
// ABOUTME: Retains literal examples and unrelated bytes, including sibling footnotes.
package annotation

import "strings"

// An absent definition is a harmless retry only when no real occurrence remains.
// Include definitions here so duplicate labels cannot masquerade as absence.
func footnoteLabelPresent(data []byte, format, label string) bool {
	return len(deletionReferences(data, format, label, byteRange{})) != 0
}

func patchDeletedFootnote(data []byte, format string, note EditableFootnote) ([]byte, error) {
	patches := deletionReferences(data, format, note.Label, note.definition)
	if note.Storage == "sidecar" {
		if _, context, ok := sidecarContext(data, note.Label); ok {
			kept := patches[:0]
			for _, patch := range patches {
				if patch.start < context.start || patch.end > context.end {
					kept = append(kept, patch)
				}
			}
			patches = append(kept, sourcePatch{context, nil})
		}
	}
	patches = append(patches, sourcePatch{note.definition, nil})
	return applySourcePatches(data, patches)
}

func deletionReferences(data []byte, format, label string, omit byteRange) []sourcePatch {
	var patches []sourcePatch
	literal := literalContext{}
	notes := EditableFootnotes(data, format, "embedded")
	for _, line := range sourceLines(data) {
		if line.offset >= omit.start && line.offset < omit.end {
			continue
		}
		text := line.text
		if format != "org" {
			for _, note := range notes {
				if line.offset > note.definition.start && line.offset < note.definition.end {
					text = strings.TrimPrefix(strings.TrimPrefix(text, "    "), "\t")
					break
				}
			}
		}
		wasLiteral := literal.active()
		literal.consume(text, format)
		// Org's rendered containers can contain ordinary footnote references.
		if format == "org" && literal.org != "SRC" && literal.org != "EXAMPLE" && literal.org != "COMMENT" && literal.org != "EXPORT" {
			literal.org = ""
		}
		if wasLiteral || literal.active() || (format != "org" && (strings.HasPrefix(text, "    ") || strings.HasPrefix(text, "\t"))) {
			continue
		}
		if format == "org" && (strings.HasPrefix(strings.TrimSpace(line.text), "#") || strings.HasPrefix(strings.TrimSpace(line.text), ": ")) {
			continue
		}
		var matches []sourcePatch
		for _, match := range referencePattern(format).FindAllStringSubmatchIndex(line.text, -1) {
			if strings.EqualFold(line.text[match[2]:match[3]], label) && !deletionInlineLiteral(line.text, match[0], format) {
				matches = append(matches, sourcePatch{byteRange{line.offset + match[0], line.offset + match[1]}, nil})
			}
		}
		if len(matches) == 0 {
			continue
		}
		remaining := line.text
		for i := len(matches) - 1; i >= 0; i-- {
			span := matches[i].byteRange
			remaining = remaining[:span.start-line.offset] + remaining[span.end-line.offset:]
		}
		if strings.TrimSpace(remaining) == "Annotations:" {
			patches = append(patches, sourcePatch{byteRange{line.offset, line.end}, nil})
		} else {
			patches = append(patches, matches...)
		}
	}
	return patches
}

// Match paired code delimiters rather than counting individual backticks: a
// double-backtick span is just as literal as a single-backtick span.
func deletionInlineLiteral(line string, at int, format string) bool {
	if at > 0 && line[at-1] == '\\' {
		return true
	}
	if format == "org" {
		return inlineLiteral(line, at, format)
	}
	for i := 0; i < at; i++ {
		if strings.HasPrefix(line[i:], "<!--") {
			end := strings.Index(line[i+4:], "-->")
			if end < 0 || at < i+4+end+3 {
				return true
			}
			i += 4 + end + 2
			continue
		}
		if line[i] != '`' || (i > 0 && line[i-1] == '\\') {
			continue
		}
		end := i + 1
		for end < len(line) && line[end] == '`' {
			end++
		}
		for j := end; j < len(line); j++ {
			if line[j] != '`' {
				continue
			}
			close := j + 1
			for close < len(line) && line[close] == '`' {
				close++
			}
			if close-j == end-i {
				if at < close {
					return true
				}
				i = close - 1
				break
			}
			j = close - 1
		}
	}
	return false
}

func (w *Writer) retireDeletedComposer(path, label, storage string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for key, item := range w.composers {
		if strings.HasPrefix(key, path+"\x00") && item.storage == storage && item.current != nil && item.current.event.Label == label {
			item.current.event.Kind = "close"
		}
	}
}
