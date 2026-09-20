// ABOUTME: Discovers optional converters only when their reader is selected.
// ABOUTME: Keeps native previews independent of optional catalogue availability.
package preview

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const markdownHTMLWarning = "Embedded HTML omitted: This Markdown source contains HTML that is not rendered in this preview. Some content may be missing."

func nativeFormats() *formatCatalogue {
	return &formatCatalogue{readers: map[string]bool{"org": true, "markdown": true, "html": true}, languages: map[string]bool{}, listing: "html\nmarkdown\norg\n"}
}
func nativeReader(reader string) bool {
	return reader == "" || reader == "org" || reader == "html" || readerBase(reader) == "markdown"
}
func markdownSmartOff(selection string) bool {
	off := false
	for rest := strings.TrimPrefix(selection, "markdown"); rest != ""; {
		sign := rest[0]
		rest = rest[1:]
		end := strings.IndexAny(rest, "+-")
		if end < 0 {
			end = len(rest)
		}
		if rest[:end] == "smart" {
			off = sign == '-'
		}
		rest = rest[end:]
	}
	return off
}
func validateMarkdown(selection string) error {
	if len(selection) > 256 || !readerSelectionPattern.MatchString(selection) {
		return fmt.Errorf("invalid --from reader; maximum 256 ASCII characters")
	}
	for rest := strings.TrimPrefix(selection, "markdown"); rest != ""; {
		sign := rest[0]
		rest = rest[1:]
		end := strings.IndexAny(rest, "+-")
		if end < 0 {
			end = len(rest)
		}
		name := rest[:end]
		if name != "smart" && (name != "raw_html" || sign != '-') {
			return fmt.Errorf("unsupported Markdown qualifier %c%s; supported: +smart, -smart, -raw_html", sign, name)
		}
		rest = rest[end:]
	}
	return nil
}
func optionalFormats(ctx context.Context, host Host) (string, *formatCatalogue, error) {
	path, err := host.LookPath("pandoc")
	if err != nil {
		return "", nil, fmt.Errorf("this reader requires optional Pandoc 3.9.0.2 or a later 3.9 patch: %w", err)
	}
	if err = checkPandoc(ctx, host, path); err != nil {
		return "", nil, fmt.Errorf("Pandoc compatibility: %w", err)
	}
	c, err := discoverFormats(ctx, host, path)
	if err != nil {
		return "", nil, err
	}
	for reader := range nativeFormats().readers {
		c.readers[reader] = true
	}
	names := make([]string, 0, len(c.readers))
	for name := range c.readers {
		if len(name) > 128 {
			return "", nil, fmt.Errorf("reader name exceeds limit")
		}
		names = append(names, name)
	}
	if len(names) > 256 {
		return "", nil, fmt.Errorf("reader catalogue exceeds limit")
	}
	sort.Strings(names)
	c.listing = strings.Join(names, "\n") + "\n"
	return path, c, nil
}
func (s *previewService) optional(ctx context.Context) (string, *formatCatalogue, error) {
	s.optionalMu.Lock()
	defer s.optionalMu.Unlock()
	if s.optionalCatalogue != nil {
		return s.optionalPath, s.optionalCatalogue, nil
	}
	path, c, err := optionalFormats(ctx, s.host)
	if err == nil {
		s.optionalPath, s.optionalCatalogue = path, c
	}
	return path, c, err
}
func (s *previewService) selection(ctx context.Context, selection string) error {
	if nativeReader(selection) {
		return s.base.formats.validateSelection(ctx, s.host, "", selection)
	}
	path, c, err := s.optional(ctx)
	if err != nil {
		return err
	}
	return c.validateSelection(ctx, s.host, path, selection)
}
func (s *previewService) catalogueFor(ctx context.Context, path, selection string) (*formatCatalogue, error) {
	c := s.base.formats
	f, err := c.resolve(path, selection)
	if err == nil && nativeReader(f.reader) {
		return c, nil
	}
	_, c, err = s.optional(ctx)
	return c, err
}

func markdownFamily(reader string) bool {
	switch readerBase(reader) {
	case "markdown", "markdown_strict", "markdown_mmd", "markdown_phpextra", "markdown_github", "gfm", "commonmark", "commonmark_x":
		return true
	}
	return false
}
