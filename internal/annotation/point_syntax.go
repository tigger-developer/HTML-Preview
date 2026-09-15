// ABOUTME: Identifies heading prose and complete native link boundaries for annotations.
// ABOUTME: Keeps source syntax and Org task metadata outside editable insertion regions.
package annotation

import (
	"strings"
	"unicode"
)

func headingProse(line, format string, states map[string]bool) (int, int, bool) {
	start := len(line) - len(strings.TrimLeft(line, " "))
	marker := byte('#')
	if format == "org" {
		marker = '*'
		if start != 0 {
			return 0, 0, false
		}
	}
	end := start
	for end < len(line) && line[end] == marker {
		end++
	}
	if end == start || end >= len(line) || line[end] != ' ' {
		return 0, 0, false
	}
	if format != "org" && (start > 3 || end-start > 6) {
		return 0, 0, false
	}
	start = end
	for start < len(line) && line[start] == ' ' {
		start++
	}
	end = len(strings.TrimRightFunc(line, unicode.IsSpace))
	if format == "org" {
		word := strings.Fields(line[start:])
		if len(word) > 0 && states[word[0]] {
			start += len(word[0])
			for start < end && line[start] == ' ' {
				start++
			}
		}
		if strings.HasPrefix(line[start:], "[#") {
			if at := strings.Index(line[start:], "]"); at >= 0 {
				start += at + 1
			}
		}
		if end > start && line[end-1] == ':' {
			at := strings.LastIndexAny(line[start:end], " \t")
			if at >= 0 && line[start+at+1] == ':' {
				end = start + at
			}
		}
	} else {
		// Optional closing ATX marker is not heading prose.
		at := end
		for at > start && line[at-1] == '#' {
			at--
		}
		if at < end && at > start && line[at-1] == ' ' {
			end = at - 1
		}
	}
	return start, end, true
}

func orgTaskStates(data []byte) map[string]bool {
	states := map[string]bool{"TODO": true, "DONE": true}
	for _, line := range sourceLines(data) {
		key, value, ok := strings.Cut(line.text, ":")
		if !ok {
			continue
		}
		switch strings.ToUpper(key) {
		case "#+TODO", "#+SEQ_TODO", "#+TYP_TODO":
			for _, word := range strings.Fields(value) {
				word, _, _ = strings.Cut(word, "(")
				if word != "|" {
					states[word] = true
				}
			}
		}
	}
	return states
}

// balancedEnd returns the byte immediately after a balanced delimiter pair.
func balancedEnd(s string, start int, open, close byte) int {
	depth := 0
	var quote byte
	for i := start; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if open == '(' && (s[i] == '"' || s[i] == '\'') {
			if quote == 0 {
				quote = s[i]
			} else if quote == s[i] {
				quote = 0
			}
			continue
		}
		if quote != 0 {
			continue
		}
		if s[i] == open {
			depth++
		}
		if s[i] == close {
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

func linkEnds(line, format, run string) []int {
	var ends []int
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if inlineLiteral(line, i, format) {
			continue
		}
		if line[i] == '<' && (strings.Contains(run, "://") || strings.Contains(run, "@")) {
			if close := strings.IndexByte(line[i:], '>'); close > 0 && line[i+1:i+close] == run {
				ends = append(ends, i+close+1)
				i += close
				continue
			}
		}
		if line[i] != '[' || i > 0 && line[i-1] == '!' {
			continue
		}
		end := balancedEnd(line, i, '[', ']')
		if end < 0 {
			continue
		}
		labelStart, labelEnd := i+1, end-1
		if format == "org" {
			if !strings.HasPrefix(line[i:], "[[") || !strings.HasSuffix(line[i:end], "]]") {
				continue
			}
			labelStart = i + 2
			labelEnd = end - 2
			if split := strings.Index(line[labelStart:labelEnd], "]["); split >= 0 {
				labelStart += split + 2
			}
		} else {
			if strings.HasPrefix(line[i:], "[^") {
				i = end - 1
				continue
			}
			if end < len(line) && (line[end] == '(' || line[end] == '[') {
				close := byte(')')
				if line[end] == '[' {
					close = ']'
				}
				suffix := balancedEnd(line, end, line[end], close)
				if suffix < 0 {
					continue
				}
				end = suffix
			}
		}
		if strings.Contains(Normalize(line[labelStart:labelEnd]), Normalize(run)) {
			ends = append(ends, end)
		}
		i = end - 1
	}
	return ends
}
