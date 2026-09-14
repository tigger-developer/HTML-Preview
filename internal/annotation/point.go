// ABOUTME: Finds one bounded literal source candidate for an annotation insertion point.
// ABOUTME: Excludes structural and literal regions; the renderer must still prove the candidate.
package annotation

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

func SourcePointCandidate(data []byte, format, run string, runOffset int) (int, error) {
	if !utf8.ValidString(run) || len(run) > 8192 || runOffset < 0 || runOffset > utf8.RuneCountInString(run) || strings.ContainsRune(run, 0) {
		return -1, fail("point_unmappable")
	}
	masked := append([]byte(nil), data...)
	literal := literalContext{}
	definition, drawer := false, false
	for _, line := range sourceLines(data) {
		trim := strings.TrimSpace(line.text)
		blocked := literal.active()
		literal.consume(line.text, format)
		blocked = blocked || literal.active()
		if definitionPattern(format).MatchString(line.text) {
			definition = true
		} else if trim != "" && !strings.HasPrefix(line.text, " ") && !strings.HasPrefix(line.text, "\t") {
			definition = false
		}
		if format == "org" && strings.HasPrefix(trim, ":") && strings.HasSuffix(trim, ":") {
			blocked = true
			drawer = !strings.EqualFold(trim, ":END:")
		}
		blocked = blocked || definition || drawer || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, ">")
		if format == "org" {
			blocked = blocked || strings.HasPrefix(trim, "*")
		} else {
			blocked = blocked || strings.HasPrefix(line.text, "    ") || strings.HasPrefix(line.text, "\t")
		}
		if blocked {
			for i := line.offset; i < line.end; i++ {
				masked[i] = 0
			}
			continue
		}
		// A candidate inside code, a link's brackets/destination or raw HTML
		// must not become writable merely because its text occurs once.
		brackets, angle := 0, false
		for i, r := range line.text {
			if r == '[' || r == '(' {
				brackets++
			}
			if r == '<' {
				angle = true
			}
			if brackets > 0 || angle || inlineLiteral(line.text, i, format) {
				for j := 0; j < utf8.RuneLen(r); j++ {
					masked[line.offset+i+j] = 0
				}
			}
			if (r == ']' || r == ')') && brackets > 0 {
				brackets--
			}
			if r == '>' {
				angle = false
			}
		}
	}
	normalized, boundaries := normalizedBytePositions(string(masked))
	needle := Normalize(run)
	if needle == "" {
		return -1, fail("point_unmappable")
	}
	at := strings.Index(normalized, needle)
	if at < 0 || strings.Contains(normalized[at+1:], needle) {
		return -1, fail("point_unmappable")
	}
	prefix := string([]rune(run)[:runOffset])
	inside := utf8.RuneCountInString(Normalize(prefix))
	if len(prefix) > 0 && runOffset < utf8.RuneCountInString(run) {
		last, _ := utf8.DecodeLastRuneInString(prefix)
		if WhiteSpace(last) && inside > 0 {
			inside++
		}
	}
	scalar := utf8.RuneCountInString(normalized[:at]) + inside
	if scalar < 0 || scalar >= len(boundaries) {
		return -1, fail("point_unmappable")
	}
	return boundaries[scalar], nil
}

func normalizedBytePositions(text string) (string, []int) {
	var out bytes.Buffer
	var positions []int
	pending, whitespace, end := false, 0, 0
	for at, r := range text {
		if WhiteSpace(r) {
			if !pending {
				whitespace = at
			}
			pending = out.Len() > 0
			continue
		}
		if pending {
			positions = append(positions, whitespace)
			out.WriteByte(' ')
			pending = false
		}
		positions = append(positions, at)
		out.WriteRune(r)
		end = at + utf8.RuneLen(r)
	}
	positions = append(positions, end)
	return out.String(), positions
}
