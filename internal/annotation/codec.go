// ABOUTME: Extracts exact owned annotation tails without rewriting authored bytes.
// ABOUTME: Shares event framing across Org and Markdown while respecting literal contexts.
package annotation

import (
	"bytes"
	"errors"
	"sort"
	"strings"
)

const headerMarker = "htmlpreview-annotations:v1"
const eventMarker = "htmlpreview-annotation-event:v1"

type Store struct {
	Source    []byte
	Header    *Header
	Events    []Event
	Reason    string
	Safe      bool
	Ending    string
	TailBytes int
}

type sourceLine struct {
	text        string
	offset, end int
}

func sourceLines(data []byte) []sourceLine {
	var lines []sourceLine
	for offset := 0; offset < len(data); {
		end := bytes.IndexByte(data[offset:], '\n')
		if end < 0 {
			end = len(data)
		} else {
			end += offset + 1
		}
		lines = append(lines, sourceLine{strings.TrimSuffix(strings.TrimSuffix(string(data[offset:end]), "\n"), "\r"), offset, end})
		offset = end
	}
	return lines
}

func Parse(data []byte, format string) Store {
	crlf := bytes.Count(data, []byte("\r\n"))
	lf := bytes.Count(data, []byte("\n")) - crlf
	ending := "\n"
	if crlf > lf {
		ending = "\r\n"
	}
	store := Store{Source: data, Safe: true, Ending: ending}
	lines := sourceLines(data)
	literal := literalContext{}
	for i, line := range lines {
		if literal.active() {
			literal.consume(line.text, format)
			continue
		}
		if ownedStart(lines, i, format) {
			start := line.offset
			boundaryEnding := "\n"
			if bytes.HasSuffix(data[line.offset:line.end], []byte("\r\n")) {
				boundaryEnding = "\r\n"
			}
			separator := []byte(boundaryEnding + boundaryEnding)
			if start >= len(separator) && bytes.Equal(data[start-len(separator):start], separator) {
				start -= len(separator)
			} else {
				store.Reason = "corrupt_store"
			}
			store.Source, store.TailBytes = data[:start], len(data)-start
			parseTail(&store, lines, i, format)
			return store
		}
		literal.consume(line.text, format)
	}
	store.Safe = !literal.active()
	return store
}

func ownedStart(lines []sourceLine, i int, format string) bool {
	line := lines[i].text
	if format == "org" {
		return line == "#+begin_comment" && i+1 < len(lines) && strings.HasPrefix(lines[i+1].text, "htmlpreview-annotation")
	}
	return strings.HasPrefix(line, "<!-- htmlpreview-annotation")
}

type literalContext struct {
	org, fence string
	comment    bool
}

func (c literalContext) active() bool { return c.org != "" || c.fence != "" || c.comment }

func (c *literalContext) consume(line, format string) {
	trim := strings.TrimSpace(line)
	if format == "org" {
		upper := strings.ToUpper(trim)
		if c.org != "" {
			if upper == "#+END_"+c.org {
				c.org = ""
			}
			return
		}
		if strings.HasPrefix(upper, "#+BEGIN_") {
			fields := strings.Fields(strings.TrimPrefix(upper, "#+BEGIN_"))
			if len(fields) > 0 {
				c.org = fields[0]
			}
		}
		return
	}
	if c.comment {
		if strings.Contains(line, "-->") {
			c.comment = false
		}
		return
	}
	if c.fence != "" {
		if strings.HasPrefix(trim, c.fence) && strings.Trim(trim, string(c.fence[0])) == "" {
			c.fence = ""
		}
		return
	}
	if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(trim, ">") {
		return
	}
	for _, marker := range []string{"```", "~~~"} {
		if strings.HasPrefix(trim, marker) {
			count := 0
			for count < len(trim) && trim[count] == marker[0] {
				count++
			}
			c.fence = trim[:count]
			return
		}
	}
	if start := strings.Index(line, "<!--"); start >= 0 && !strings.Contains(line[start+4:], "-->") {
		c.comment = true
	}
}

func parseTail(store *Store, lines []sourceLine, start int, format string) {
	for i := start; i < len(lines); {
		if lines[i].text == "" {
			i++
			continue
		}
		marker, payload, next, ok := readFrame(lines, i, format)
		if !ok {
			store.Reason = "corrupt_store"
			return
		}
		if lines[next-1].end-lines[i].offset > MaxFrame {
			store.Reason = "store_limit"
			return
		}
		switch marker {
		case headerMarker:
			var h Header
			if store.Header != nil || Decode([]byte(payload), &h) != nil || !HeaderValid(h) {
				store.Reason = "corrupt_store"
				return
			}
			store.Header = &h
		case eventMarker:
			var e Event
			if store.Header == nil || Decode([]byte(payload), &e) != nil || e.Validate() != nil {
				store.Reason = "corrupt_store"
				return
			}
			store.Events = append(store.Events, e)
		default:
			store.Reason = "unsupported_store"
			return
		}
		i = next
	}
	if store.Header == nil {
		store.Reason = "corrupt_store"
	}
	if store.TailBytes > MaxStore || len(store.Events) > MaxEvents {
		store.Reason = "store_limit"
	}
	if _, _, err := Project(store.Events); err != nil {
		store.Reason = "corrupt_store"
	}
}

func readFrame(lines []sourceLine, i int, format string) (string, string, int, bool) {
	if format == "org" {
		if i+3 >= len(lines) || lines[i].text != "#+begin_comment" || lines[i+3].text != "#+end_comment" {
			return "", "", i, false
		}
		return lines[i+1].text, lines[i+2].text, i + 4, true
	}
	if i+2 >= len(lines) || !strings.HasPrefix(lines[i].text, "<!-- ") || lines[i+2].text != "-->" {
		return "", "", i, false
	}
	return strings.TrimPrefix(lines[i].text, "<!-- "), lines[i+1].text, i + 3, true
}

func AppendBytes(store Store, header *Header, event Event, format string) ([]byte, error) {
	if store.Reason != "" || !store.Safe || event.Validate() != nil {
		return nil, errors.New("unsafe annotation store")
	}
	ending := store.Ending
	if ending == "" {
		ending = "\n"
	}
	var output []byte
	if store.Header == nil {
		if header == nil || !HeaderValid(*header) {
			return nil, errors.New("invalid annotation header")
		}
		frame, err := Frame(format, headerMarker, header, ending)
		if err != nil {
			return nil, err
		}
		output = append([]byte(ending+ending), frame...)
	}
	frame, err := Frame(format, eventMarker, event, ending)
	if err != nil {
		return nil, err
	}
	output = append(output, frame...)
	if store.TailBytes+len(output) > MaxStore || len(store.Events) >= MaxEvents {
		return nil, fail("store_limit")
	}
	return output, nil
}

// Project validates sequence history before exposing the latest state of each comment.
func Project(events []Event) ([]Event, map[string]Event, error) {
	operations := make(map[string]Event)
	grouped := make(map[string][]Event)
	var failure error
	for _, event := range events {
		if old, exists := operations[event.OperationID]; exists {
			if old != event {
				failure = errors.New("conflicting operation")
			}
			continue
		}
		operations[event.OperationID] = event
		grouped[event.AnnotationID] = append(grouped[event.AnnotationID], event)
	}
	latest := make([]Event, 0, len(grouped))
	for _, history := range grouped {
		sort.Slice(history, func(i, j int) bool { return history[i].Sequence < history[j].Sequence })
		var previous *Event
		for i := range history {
			if err := ValidateNext(previous, history[i]); err != nil {
				failure = err
				break
			}
			previous = &history[i]
		}
		if previous != nil {
			latest = append(latest, *previous)
		}
	}
	sort.Slice(latest, func(i, j int) bool {
		if latest[i].CreatedAt == latest[j].CreatedAt {
			return latest[i].AnnotationID < latest[j].AnnotationID
		}
		return latest[i].CreatedAt < latest[j].CreatedAt
	})
	return latest, operations, failure
}
