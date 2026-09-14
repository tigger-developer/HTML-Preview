// ABOUTME: Imports validated legacy values into native records during an authorized save.
// ABOUTME: Replaces only the selected destination's owned log in the same transaction.
package annotation

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

func migrateCurrent(ctx context.Context, snap Snapshot, data []byte, format, reserved string, position int, sidecar bool, verify PointVerifier) ([]byte, int, error) {
	legacy := parseLegacy(data, format)
	if legacy.Reason != "" {
		return nil, position, fail(legacy.Reason)
	}
	if legacy.TailBytes == 0 {
		return data, position, nil
	}
	owned := make(map[string]bool)
	for _, event := range legacy.Events {
		owned[event.AnnotationID] = true
	}
	labels := map[string]bool{strings.ToLower(reserved): true}
	for label := range snap.Embedded.Labels {
		labels[label] = true
	}
	for label := range snap.Sidecar.Labels {
		labels[label] = true
	}
	var patches []sourcePatch
	var definitions []byte
	shift := 0
	for _, old := range snap.Events {
		if !owned[old.AnnotationID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, position, err
		}
		event := old
		event.Schema, event.ComposerID = 2, ""
		label := event.Label
		if label == "" {
			// Stable IDs keep imports deterministic without deriving identity
			// from a display name or adding a normalization dependency.
			base := "annotation-" + event.AnnotationID
			label = base
			for n := 1; labels[strings.ToLower(label)]; n++ {
				label = base + "-" + strconv.Itoa(n)
			}
		}
		labels[strings.ToLower(label)] = true
		event.Label = label
		at := -1
		if event.Target.Type == "text" && verify != nil {
			var err error
			at, err = verify(ctx, snap, &event.Target)
			if err != nil {
				var failure *Failure
				if !errors.As(err, &failure) || failure.Code != "point_unmappable" {
					return nil, position, err
				}
			}
		}
		if at < 0 {
			before := []rune(old.Target.Prefix + old.Target.Exact)
			event.Target = Target{Type: "document", Prefix: string(before[max(0, len(before)-64):]), Suffix: old.Target.Suffix, HeadingID: old.Target.HeadingID}
		}
		encoded, err := encodeFootnote(format, *snap.Header, event, label, legacy.Ending)
		if err != nil {
			return nil, position, err
		}
		definitions = append(definitions, []byte(legacy.Ending+legacy.Ending)...)
		if sidecar {
			context := strings.NewReplacer("[", "\\[", "\r", " ", "\n", " ").Replace(event.Target.Prefix + " | " + event.Target.Suffix)
			definitions = append(definitions, []byte("Context: "+context+footnoteReference(format, label)+legacy.Ending+legacy.Ending)...)
		} else if at >= 0 {
			if at > len(legacy.Source) {
				return nil, position, fail("point_unmappable")
			}
			token := []byte(footnoteReference(format, label))
			patches = append(patches, sourcePatch{byteRange{at, at}, token})
			if at <= position {
				shift += len(token)
			}
		}
		definitions = append(definitions, encoded...)
	}
	patches = append(patches, sourcePatch{byteRange{len(legacy.Source), len(data)}, definitions})
	updated, err := applySourcePatches(data, patches)
	return updated, position + shift, err
}
