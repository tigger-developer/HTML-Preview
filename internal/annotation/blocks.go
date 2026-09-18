// ABOUTME: Marks potential native annotation boundaries in a temporary source copy.
// ABOUTME: Pandoc output must verify each candidate before it becomes an interactive block.
package annotation

import (
	"fmt"
	"strings"
)

// BlockBoundary belongs to one exact source revision, not to client-provided offsets.
type BlockBoundary struct {
	Offset     int
	AfterBlock bool
	Following  bool
}

// MarkBlocks proposes line-end boundaries. The renderer admits only markers at
// the end of supported semantic blocks with unchanged surrounding structure.
func MarkBlocks(data []byte, format, token string) ([]byte, map[string]BlockBoundary, error) {
	lines := sourceLines(data)
	var excluded []byteRange
	for _, note := range EditableFootnotes(data, format, "embedded") {
		excluded = append(excluded, note.definition)
	}
	states := orgTaskStates(data)
	candidates := make(map[string]BlockBoundary)
	var patches []sourcePatch
	var definitions strings.Builder
	used := make(map[int]bool)
	ending := Parse(data, format).Ending
	add := func(offset int, after, code bool) {
		if used[offset] {
			return
		}
		used[offset] = true
		key := fmt.Sprintf("%s%dQ", token, len(candidates))
		candidates[key] = BlockBoundary{offset, after, code}
		marker := footnoteReference(format, key)
		if after {
			marker = ending + ending + marker + ending
		}
		patches = append(patches, sourcePatch{byteRange{offset, offset}, []byte(marker)})
		definitions.WriteString(ending + ending + footnoteReference(format, key))
		if format != "org" {
			definitions.WriteString(":")
		}
		definitions.WriteString(" " + key + ending)
	}

	literal := literalContext{}
	drawer, frontmatter := false, false
	for i, line := range lines {
		trim := strings.TrimSpace(line.text)
		if inRanges(line.offset, excluded) {
			continue
		}
		if format != "org" && i == 0 && trim == "---" {
			frontmatter = true
			continue
		}
		if frontmatter {
			if trim == "---" || trim == "..." {
				frontmatter = false
			}
			continue
		}
		was := literal
		literal.consume(line.text, format)
		if was.active() {
			code := was.org == "SRC" || was.org == "EXAMPLE" || was.fence != ""
			if literal.active() || !code || len(line.text) != len(strings.TrimLeft(line.text, " \t")) {
				continue
			}
			offset, after := afterBlockBoundary(lines, i, format)
			add(offset, after, true)
			continue
		}
		if literal.active() {
			continue
		}
		if format == "org" && drawerNameForAnnotation(trim) {
			drawer = !strings.EqualFold(trim, ":END:")
			continue
		}
		if drawer || trim == "" || strings.HasPrefix(trim, "|") || strings.HasPrefix(trim, ">") || strings.HasPrefix(trim, "<") || strings.HasPrefix(trim, ":") {
			continue
		}
		start, end, heading := headingProse(line.text, format, states)
		if strings.HasPrefix(trim, "#") && !heading {
			continue
		}
		if format != "org" && (strings.HasPrefix(line.text, "    ") || strings.HasPrefix(line.text, "\t")) {
			continue
		}
		if heading && format != "org" {
			offset, after := afterBlockBoundary(lines, i, format)
			add(offset, after, true)
			continue
		}
		if !heading {
			end = len(strings.TrimRight(line.text, " \t"))
		} else if start >= end {
			continue
		}
		if format != "org" && heading {
			if at := strings.LastIndex(line.text[:end], " {#"); at >= 0 && strings.HasSuffix(line.text[:end], "}") {
				end = at
			}
		}
		// Preserve hard line breaks and structural punctuation; unsupported lines
		// simply receive no interactive target.
		if strings.HasSuffix(line.text, "  ") || strings.HasSuffix(line.text, "\\") || strings.Trim(trim, "-=_* ") == "" {
			continue
		}
		offset := line.offset + end
		add(offset, false, false)
	}
	patches = append(patches, sourcePatch{byteRange{len(data), len(data)}, []byte(definitions.String())})
	marked, err := applySourcePatches(data, patches)
	if err != nil {
		return nil, nil, err
	}
	return marked, candidates, nil
}

func drawerNameForAnnotation(s string) bool {
	return len(s) > 2 && s[0] == ':' && s[len(s)-1] == ':' && !strings.ContainsAny(s, " \t")
}

// The separate-reference convention preserves Markdown automatic heading IDs
// and keeps references outside literal source blocks.
func afterBlockBoundary(lines []sourceLine, i int, format string) (int, bool) {
	next := i + 1
	for next < len(lines) && strings.TrimSpace(lines[next].text) == "" {
		next++
	}
	if next < len(lines) && strings.HasPrefix(lines[next].text, "Annotations: ") {
		tail := strings.TrimPrefix(lines[next].text, "Annotations: ")
		if strings.TrimSpace(referencePattern(format).ReplaceAllString(tail, "")) == "" {
			return lines[next].offset + len(strings.TrimRight(lines[next].text, " \t")), false
		}
	}
	return lines[i].offset + len(lines[i].text), true
}
