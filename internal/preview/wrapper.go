// ABOUTME: Wraps source code and plaintext in private Org without granting it document semantics.
// ABOUTME: Keeps original clipboard text separate from normalized or formatted display text.
package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func validateText(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("source is not valid UTF-8")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return fmt.Errorf("source contains NUL; expected UTF-8 text")
	}
	return nil
}

func (s *session) preserveWrapper(p *page, data []byte, token string) (preservation, error) {
	display := strings.TrimPrefix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\ufeff")
	language := p.source.input.language
	if language == "json" {
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, []byte(display), "", "  "); err != nil {
			s.log.notice("%q: invalid JSON; displaying literal source", p.source.logical)
		} else {
			display = formatted.String()
		}
	}
	if language != "plaintext" && !s.cfg.formats.languages[language] {
		s.log.notice("%q: highlighting language %q unavailable; displaying plain code", p.source.logical, language)
		language = "plaintext"
	}
	// The stored wrapper uses a collision-checked metadata token; its visible
	// basename is restored as owned metadata, never parsed as Org directives.
	schema := "HTML-Preview version " + s.cfg.version
	var wrapper strings.Builder
	fmt.Fprintf(&wrapper, "#+TITLE: HTMLPREVIEW_TITLE_%s\n#+SCHEMA: %s\n\n", token, schema)
	if p.source.input.kind == "code" {
		wrapper.WriteString("* Source code\n")
	}
	fmt.Fprintf(&wrapper, "#+BEGIN_SRC %s\n", language)
	for _, line := range strings.SplitAfter(display, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#+") || strings.HasPrefix(trim, "*") {
			wrapper.WriteByte(',')
		}
		wrapper.WriteString(line)
	}
	if !strings.HasSuffix(display, "\n") && display != "" {
		wrapper.WriteByte('\n')
	}
	wrapper.WriteString("#+END_SRC\n")
	if err := s.write(p.name+".wrapper.org", []byte(wrapper.String())); err != nil {
		return preservation{}, err
	}
	preserved, err := preserveOrgCode([]byte(wrapper.String()), token, &display)
	if err != nil {
		return preserved, err
	}
	preserved.title = filepath.Base(p.source.logical)
	preserved.frontmatter = []metadataField{{"TITLE", preserved.title}, {"SCHEMA", schema}}
	p.copyOriginal = append([]byte{}, data...)
	return preserved, nil
}
