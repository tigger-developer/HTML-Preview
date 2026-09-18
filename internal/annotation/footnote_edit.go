// ABOUTME: Locates native footnote definitions for editing without ownership requirements.
// ABOUTME: Preserves native IDs, attribution and unrelated bytes under revision checks.
package annotation

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// EditableFootnote is reconstructed from the current file, including ordinary notes.
type EditableFootnote struct {
	Label       string `json:"label"`
	Text        string `json:"text"`
	Revision    string `json:"revision"`
	Storage     string `json:"storage"`
	Owned       bool   `json:"owned"`
	Author      string `json:"author,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	attribution string
	definition  byteRange
	bodyStart   int
	indent      string
	note        Footnote
}

// EditableFootnotes recognizes named definitions outside literal blocks. Org
// definitions end at a heading, another definition or two consecutive blank
// lines; Markdown continuation lines have a tab or four spaces of indentation.
func EditableFootnotes(data []byte, format, storage string) []EditableFootnote {
	lines := sourceLines(data)
	literal := literalContext{}
	notes := []EditableFootnote{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if literal.active() {
			literal.consume(line.text, format)
			continue
		}
		parts := definitionPattern(format).FindStringSubmatchIndex(line.text)
		if parts == nil {
			literal.consume(line.text, format)
			continue
		}
		label := line.text[parts[2]:parts[3]]
		note, next, owned, err := readFootnote(lines, i, format)
		if err != nil {
			continue
		}
		entry := EditableFootnote{Label: label, Storage: storage, Owned: owned, bodyStart: line.offset + parts[4], definition: byteRange{line.offset, line.end}, note: note}
		if owned {
			entry.definition = note.definition
			entry.Text = note.Event.Text
			entry.Author, entry.CreatedAt = note.Event.Author, note.Event.CreatedAt
		} else {
			next = ordinaryNoteEnd(lines, i, format)
			end := next
			for end > i+1 && strings.TrimSpace(lines[end-1].text) == "" {
				end--
			}
			entry.definition.end = lines[end-1].end
			entry.indent = ""
			if format != "org" {
				entry.indent = "    "
			}
			body := []string{line.text[parts[4]:parts[5]]}
			for j := i + 1; j < end; j++ {
				text := lines[j].text
				if format != "org" && strings.HasPrefix(text, "\t") {
					text = text[1:]
				} else {
					text = strings.TrimPrefix(text, entry.indent)
				}
				body = append(body, text)
			}
			entry.Text, entry.attribution, entry.Author, entry.CreatedAt = splitAttribution(strings.Join(body, "\n"))
		}
		entry.Revision = Digest(data[entry.definition.start:entry.definition.end])
		if len(notes) >= MaxEvents {
			return nil
		}
		notes = append(notes, entry)
		i = next - 1
	}
	// Duplicate labels are ambiguous even when a particular Pandoc reader picks one.
	counts := map[string]int{}
	for _, note := range notes {
		counts[strings.ToLower(note.Label)]++
	}
	unique := notes[:0]
	for _, note := range notes {
		if counts[strings.ToLower(note.Label)] == 1 {
			unique = append(unique, note)
		}
	}
	return unique
}

func ordinaryNoteEnd(lines []sourceLine, start int, format string) int {
	literal := literalContext{}
	blanks := 0
	for i := start + 1; i < len(lines); i++ {
		line := lines[i].text
		if literal.active() {
			literal.consume(strings.TrimPrefix(line, "    "), format)
			continue
		}
		if definitionPattern(format).MatchString(line) {
			return i
		}
		if strings.TrimSpace(line) == "" {
			blanks++
			if format == "org" && blanks == 2 {
				return i - 1
			}
			continue
		}
		if format == "org" {
			if strings.HasPrefix(line, "*") && strings.HasPrefix(strings.TrimLeft(line, "*"), " ") {
				return i
			}
		} else if !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t") {
			return i
		}
		blanks = 0
		literal.consume(strings.TrimPrefix(line, "    "), format)
	}
	return len(lines)
}

func validateFootnoteEdit(r Request) error {
	if (r.Action == "delete" && r.Text != "") || (r.Action != "delete" && !ValidText(r.Text)) {
		return fail("body_limit")
	}
	if r.Label == "" || len(r.Label) > 256 || !utf8.ValidString(r.Label) || strings.ContainsAny(r.Label, "[]\x00\r\n\t ") || !ValidID(r.OperationID) || !ValidID(r.AnnotationID) || !ValidID(r.ComposerID) || r.Sequence < 1 || r.Kind != "" {
		return fail("invalid_event")
	}
	if r.Target.Type != "footnote" || (r.Target.Run != "embedded" && r.Target.Run != "sidecar") {
		return fail("invalid_event")
	}
	for _, value := range []string{r.Revision, r.SourceRevision, r.BodyRevision, r.Target.Exact} {
		if len(value) != 64 {
			return fail("invalid_event")
		}
		if _, err := hex.DecodeString(value); err != nil {
			return fail("invalid_event")
		}
	}
	return nil
}

func patchEditableFootnote(data []byte, format string, note EditableFootnote, text, operation, now, author string) ([]byte, error) {
	var encoded []byte
	if note.Owned {
		event := note.note.Event
		event.Text, event.OperationID, event.RecordedAt = text, operation, now
		event.Author, event.CreatedAt = author, now
		event.Sequence++
		var err error
		encoded, err = encodeReadableFootnote(format, event, note.Label, Parse(data, format).Ending, true)
		if err != nil {
			return nil, err
		}
	} else {
		ending := "\n"
		if bytes.Contains(data[note.definition.start:note.definition.end], []byte("\r\n")) {
			ending = "\r\n"
		}
		bodyText := text
		if note.attribution != "" {
			bodyText += "\n\n" + refreshAttribution(note.attribution, author, now)
		}
		body := strings.Split(bodyText, "\n")
		encoded = append(encoded, data[note.definition.start:note.bodyStart]...)
		for i, line := range body {
			if i > 0 {
				encoded = append(encoded, []byte(ending)...)
				if line != "" {
					encoded = append(encoded, []byte(note.indent)...)
				}
			}
			encoded = append(encoded, []byte(line)...)
		}
		if data[note.definition.end-1] == '\n' {
			encoded = append(encoded, []byte(ending)...)
		}
		// Refuse input that would break out of the current native definition.
		parsed := EditableFootnotes(encoded, format, note.Storage)
		if len(parsed) != 1 || parsed[0].Label != note.Label || parsed[0].Text != text || parsed[0].definition.end != len(encoded) {
			return nil, fail("invalid_footnote")
		}
	}
	return applySourcePatches(data, []sourcePatch{{note.definition, encoded}})
}

// editFootnote shares the normal writer lock and atomic replacement. The source
// label and definition digest are sufficient across browser/service restarts.
func (w *Writer) editFootnote(ctx context.Context, loc Location, expected os.FileInfo, author string, r Request) (Replacement, error) {
	if ValidateName(author) != nil {
		return Replacement{}, fail("invalid_event")
	}
	if err := validateFootnoteEdit(r); err != nil {
		return Replacement{}, err
	}
	if err := w.begin(loc.Path); err != nil {
		return Replacement{}, err
	}
	defer w.end(loc.Path)
	snap, err := Read(loc)
	if err != nil {
		return Replacement{}, err
	}
	if expected == nil || !os.SameFile(expected, snap.SourceInfo) {
		return Replacement{}, fail("source_replaced")
	}
	destination, err := Destination(loc, snap)
	if err != nil {
		return Replacement{}, err
	}
	storage := r.Target.Run
	if storage == "embedded" && destination != "embedded" {
		return Replacement{}, fail("read_only_footnote")
	}
	data, format := snap.RawSource, loc.Format
	if storage == "sidecar" {
		if !snap.SideExists {
			return Replacement{}, fail("footnote_conflict")
		}
		data, format = snap.RawSidecar, "org"
	}
	var note *EditableFootnote
	for _, item := range EditableFootnotes(data, format, storage) {
		if item.Label == r.Label {
			n := item
			note = &n
			break
		}
	}
	if note == nil {
		if r.Action == "delete" && !footnoteLabelPresent(data, format, r.Label) {
			if err := w.syncReplacement(ctx, loc, snap, storage); err != nil {
				return Replacement{}, err
			}
			w.retireDeletedComposer(loc.Path, r.Label, storage)
			return editReceipt(snap, snap, r, storage, format, true), nil
		}
		return Replacement{}, fail("footnote_conflict")
	}
	// Equality makes a lost acknowledgement retry harmless without an event log.
	if r.Action != "delete" && note.Text == r.Text {
		if err := w.syncReplacement(ctx, loc, snap, storage); err != nil {
			return Replacement{}, err
		}
		return editReceipt(snap, snap, r, storage, format, true), nil
	}
	if note.Revision != r.Target.Exact {
		return Replacement{}, fail("footnote_conflict")
	}
	if snap.Revision != r.Revision || snap.SourceRevision != r.SourceRevision {
		return Replacement{}, fail("stale_source")
	}
	if r.Action == "delete" {
		data, err = patchDeletedFootnote(data, format, *note)
	} else {
		data, err = patchEditableFootnote(data, format, *note, r.Text, r.OperationID, w.now().UTC().Format(time.RFC3339Nano), author)
	}
	if err != nil {
		return Replacement{}, err
	}
	if Parse(data, format).Reason != "" {
		return Replacement{}, fail("invalid_footnote")
	}
	updated, err := w.replaceFile(ctx, loc, snap, storage, data)
	if err != nil {
		// Preserve the new inode after a published replacement whose sync failed.
		current, readErr := Read(loc)
		written := current.RawSource
		if storage == "sidecar" {
			written = current.RawSidecar
		}
		if readErr == nil && current.Reason == "" && bytes.Equal(written, data) {
			w.rememberReplacement(loc.Path, snap.SourceInfo, current.SourceInfo)
			return editReceipt(snap, current, r, storage, format, false), err
		}
		return Replacement{}, err
	}
	w.rememberReplacement(loc.Path, snap.SourceInfo, updated.SourceInfo)
	if r.Action == "delete" {
		w.retireDeletedComposer(loc.Path, r.Label, storage)
	}
	return editReceipt(snap, updated, r, storage, format, false), nil
}

func editReceipt(before, after Snapshot, r Request, storage, format string, retry bool) Replacement {
	event := Event{AnnotationID: r.AnnotationID, Sequence: r.Sequence}
	result := Replacement{Receipt: receipt(after, event, r.BodyRevision, storage), PreviousInfo: before.SourceInfo, SourceInfo: after.SourceInfo, Retry: retry}
	data := after.RawSource
	if storage == "sidecar" {
		data = after.RawSidecar
	}
	for _, note := range EditableFootnotes(data, format, storage) {
		if note.Label == r.Label {
			result.Receipt.FootnoteRevision = note.Revision
			break
		}
	}
	return result
}

// MarkFootnotes adds temporary conversion-only tokens to native definitions.
// Pandoc's repeated references then carry the same source label in each endnote.
func MarkFootnotes(data []byte, format, prefix string) ([]byte, map[string]string, error) {
	data = Parse(data, format).Rendered
	labels := map[string]string{}
	var patches []sourcePatch
	for i, note := range EditableFootnotes(data, format, "embedded") {
		marker := prefix + strconv.Itoa(i) + "Q"
		labels[marker] = note.Label
		at, end := note.bodyStart, note.bodyStart
		indent := ""
		if format != "org" {
			indent = "    "
		}
		// Put the marker in its own leading paragraph. Keeping the body on
		// its native continuation line preserves lists and literal blocks.
		for end < note.definition.end {
			lineEnd := bytes.IndexByte(data[end:note.definition.end], '\n')
			if lineEnd < 0 {
				break
			}
			lineEnd += end + 1
			if len(bytes.TrimSpace(data[end:lineEnd])) != 0 {
				break
			}
			end = lineEnd
		}
		if end != at {
			indent = ""
		} // Existing continuation indentation remains.
		patches = append(patches, sourcePatch{byteRange{at, end}, []byte(marker + "\n\n" + indent)})

	}
	result, err := applySourcePatches(data, patches)
	return result, labels, err
}
