// ABOUTME: Supplies native footnote input for sidecar and legacy preview comments.
// ABOUTME: Keeps virtual references separate from source writes and requires rendered proof.
package annotation

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"
)

// VirtualNote identifies a private marker whose rendered position needs checking.
type VirtualNote struct {
	Marker, AnnotationID, Context string
	Position                      int
}

// PreviewFootnotes constructs conversion input only; callers retain the original
// source payload and revision. The selected converter renders ordinary and virtual notes together.
func PreviewFootnotes(ctx context.Context, snap Snapshot, format, body string, headings map[string]Span, prefix string) ([]byte, []VirtualNote, error) {
	if snap.Reason != "" {
		return nil, nil, nil
	}
	data := parseLegacy(snap.RawSource, format).Source
	store := Parse(data, format)
	labels := store.Labels
	owned := make(map[string]Footnote)
	for _, note := range store.Notes {
		owned[note.Event.AnnotationID] = note
	}
	var patches []sourcePatch
	var tail []byte
	var virtual []VirtualNote
	editedSidecars := make(map[string]bool)
	for _, note := range EditableFootnotes(snap.RawSidecar, "org", "sidecar") {
		editedSidecars[note.Label] = strings.Contains(note.attribution, "; Edited: ")
	}
	events := append(append([]Event{}, snap.Events...), nativeSidecarEvents(snap.RawSidecar)...)
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		note, exists := owned[event.AnnotationID]
		if exists && note.Located() && sameCurrentValue(note.Event, event) {
			continue
		}
		label := note.Label
		if !exists {
			label = event.Label
			if label == "" || labels[strings.ToLower(label)] {
				base := "annotation-" + event.AnnotationID
				label = base
				for n := 1; labels[strings.ToLower(label)]; n++ {
					label = base + "-" + strconv.Itoa(n)
				}
			}
			labels[strings.ToLower(label)] = true
		}
		encoded, err := encodeReadableFootnote(format, event, label, store.Ending, editedSidecars[event.Label])
		if err != nil {
			return nil, nil, err
		}
		if exists {
			patches = append(patches, sourcePatch{note.definition, encoded})
			if note.Located() {
				continue
			}
		} else {
			tail = append(tail, []byte(store.Ending+store.Ending)...)
			tail = append(tail, encoded...)
		}
		point, match := ResolvePoint(event.Target, body, headings)
		at := -1
		if match == "resolved" {
			run, offset := point.Prefix+point.Suffix, utf8.RuneCountInString(point.Prefix)
			if event.Target.Type == "text" {
				run, offset = event.Target.Exact, utf8.RuneCountInString(event.Target.Exact)
			}
			at, err = SourcePointCandidate(data, format, run, offset)
			if err != nil {
				at = -1
			}
		}
		marker := prefix + strconv.Itoa(len(virtual)) + "Z"
		position := point.Position
		if at < 0 {
			position = -1
		}
		virtual = append(virtual, VirtualNote{marker, event.AnnotationID, event.Target.Prefix + event.Target.Exact + " | " + event.Target.Suffix, position})
		reference := marker + footnoteReference(format, label)
		if at >= 0 {
			patches = append(patches, sourcePatch{byteRange{at, at}, []byte(reference)})
		} else {
			// This temporary reference makes the converter emit an otherwise unreferenced
			// definition. The caller removes it and marks that endnote unplaced.
			tail = append(tail, []byte(store.Ending+store.Ending+reference+store.Ending)...)
		}
	}
	patches = append(patches, sourcePatch{byteRange{len(data), len(data)}, tail})
	result, err := applySourcePatches(data, patches)
	if err != nil {
		return nil, nil, err
	}
	references := nativeReferences(result, format)
	for _, note := range EditableFootnotes(result, format, "embedded") {
		if references[note.Label] > 0 {
			continue
		}
		marker := prefix + strconv.Itoa(len(virtual)) + "Z"
		virtual = append(virtual, VirtualNote{marker, "native:" + note.Label, "", -1})
		result = append(result, []byte(store.Ending+store.Ending+marker+footnoteReference(format, note.Label)+store.Ending)...)
	}
	if len(result) > MaxStore {
		return nil, nil, fail("store_limit")
	}
	return result, virtual, nil

}

// ResolvePoint accepts a unique current context, never the nearest paragraph.
func ResolvePoint(target Target, body string, headings map[string]Span) (Target, string) {
	if target.Type == "text" {
		resolved, status := Resolve(target, body, headings)
		if status != "resolved" {
			return target, status
		}
		runes := []rune(body)
		return Target{Type: "point", Position: resolved.End, Prefix: string(runes[max(0, resolved.End-64):resolved.End]), Suffix: string(runes[resolved.End:min(len(runes), resolved.End+64)])}, "resolved"
	}
	if target.Type != "point" || target.Prefix+target.Suffix == "" {
		return target, "unplaced"
	}
	needle := target.Prefix + target.Suffix
	match, count := -1, 0
	for cursor := 0; cursor <= len(body); {
		rel := strings.Index(body[cursor:], needle)
		if rel < 0 {
			break
		}
		at := cursor + rel
		point := utf8.RuneCountInString(body[:at]) + utf8.RuneCountInString(target.Prefix)
		span, scoped := headings[target.HeadingID]
		if !scoped || point >= span.Start && point <= span.End {
			match, count = point, count+1
		}
		if count > 1 {
			return target, "unplaced"
		}
		_, width := utf8.DecodeRuneInString(body[at:])
		cursor = at + width
	}
	if count != 1 {
		return target, "unplaced"
	}
	target.Position = match
	return target, "resolved"
}
