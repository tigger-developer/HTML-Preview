// ABOUTME: Preserves Org values that the Pandoc reader discards.
// ABOUTME: Tracks literal blocks and drawers before producing collision-free markers.
package preview

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"

	bundle "github.com/tigger-developer/HTML-Preview"
)

type orgHeading struct {
	marker, id, custom, visibility, title string
	level                                 int
}
type preservation struct {
	text, startup                 string
	title, subtitle, author, date string
	fragments                     map[string]string
	heads                         []*orgHeading
	warnings                      []string
	frontmatter                   []metadataField
}

type metadataField struct {
	Name, Value string
}

func preserveOrg(data []byte, token string) (preservation, error) {
	return preserveOrgCode(data, token, nil)
}

// A wrapper supplies its original display payload only after its generated
// boundaries have been parsed. Ordinary Org input never supplies this authority.
func preserveOrgCode(data []byte, token string, wrapperDisplay *string) (preservation, error) {
	source := strings.ReplaceAll(string(data), "\r\n", "\n")
	prefix := "HTMLPREVIEW_DRAWER_" + token
	p := preservation{startup: "showall", fragments: make(map[string]string)}
	drawerLayout, err := template.ParseFS(bundle.Assets, "assets/web/drawer.html")
	if err != nil {
		return p, fmt.Errorf("drawer template: %w", err)
	}
	var out strings.Builder
	marker := func(fragment string) string {
		key := fmt.Sprintf("%s_%d", prefix, len(p.fragments))
		p.fragments[key] = fragment
		return "\n#+begin_export html\n<p>" + key + "</p>\n#+end_export\n"
	}
	lines := strings.SplitAfter(source, "\n")
	var current *orgHeading
	keywords := map[string]bool{"TODO": true, "DONE": true}
	block := ""
	codeCount := 0
	preamble := true
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)
		upper := strings.ToUpper(trim)
		if block != "" {
			out.WriteString(line)
			if strings.EqualFold(trim, "#+END_"+block) {
				block = ""
			}
			continue
		}
		fields := strings.Fields(upper)
		directive := ""
		if len(fields) > 0 {
			directive = fields[0]
		}
		key, value, keyword := orgKeyword(trim)
		frontmatter := preamble && keyword
		if frontmatter {
			p.frontmatter = append(p.frontmatter, metadataField{key, value})
		}
		if frontmatter {
			switch key {
			case "TITLE":
				p.title = value
			case "SUBTITLE":
				p.subtitle = value
			case "AUTHOR":
				p.author = value
			case "DATE":
				p.date = value
			}
		} else if !keyword && trim != "" && !strings.HasPrefix(trim, "# ") && trim != "#" {
			preamble = false
		}
		if directive == "#+BEGIN_SRC" || directive == "#+BEGIN_EXAMPLE" {
			kind := "SRC"
			if directive == "#+BEGIN_EXAMPLE" {
				kind = "EXAMPLE"
			}
			var literal strings.Builder
			for i++; i < len(lines); i++ {
				if strings.EqualFold(strings.TrimSpace(lines[i]), "#+END_"+kind) {
					break
				}
				literal.WriteString(lines[i])
			}
			if kind == "SRC" {
				codeCount++
				if wrapperDisplay != nil {
					if codeCount != 1 {
						return p, fmt.Errorf("wrapper contains multiple code blocks")
					}
					literal.Reset()
					literal.WriteString(*wrapperDisplay)
				}
				language := ""
				if parts := strings.Fields(trim); len(parts) > 1 {
					language = parts[1]
				}
				// A string-only record cannot fail JSON encoding. Newlines and export
				// terminators are escaped within its single transport line.
				record, _ := json.Marshal(struct {
					ID       string `json:"id"`
					Text     string `json:"text"`
					Language string `json:"language"`
				}{
					ID: fmt.Sprintf("htmlpreview-code-%s-%d", token, codeCount), Text: literal.String(), Language: language,
				})
				out.WriteString("\n#+begin_export htmlpreview-code-" + token + "\n" + string(record) + "\n#+end_export\n")
			} else {
				out.WriteString(marker("<pre><code>" + html.EscapeString(literal.String()) + "</code></pre>"))
			}
			continue
		}
		if directive == "#+BEGIN_COMMENT" || directive == "#+BEGIN_EXPORT" {
			block = strings.TrimPrefix(directive, "#+BEGIN_")
			out.WriteString(line)
			continue
		}
		if strings.HasPrefix(trim, "# ") || trim == "#" {
			out.WriteString(line)
			continue
		}
		if strings.HasPrefix(upper, "#+INCLUDE:") || strings.HasPrefix(upper, "#+SETUPFILE:") {
			if !frontmatter {
				out.WriteString(marker("<pre class=\"org-metadata\">" + html.EscapeString(line) + "</pre>"))
			}
			p.warnings = append(p.warnings, "Org include/setup directive retained without expansion")
			continue
		}
		if strings.HasPrefix(upper, "#+STARTUP:") {
			value := strings.TrimSpace(trim[len("#+STARTUP:"):])
			p.startup = value
			if value != "overview" && value != "content" && value != "showall" {
				p.startup = "showall"
				p.warnings = append(p.warnings, "unknown STARTUP value; showing all content")
			}
			if !frontmatter {
				out.WriteString(marker("<pre class=\"org-metadata\">" + html.EscapeString(line) + "</pre>"))
			}
			continue
		}
		if strings.HasPrefix(upper, "#+TODO:") || strings.HasPrefix(upper, "#+SEQ_TODO:") || strings.HasPrefix(upper, "#+TYP_TODO:") {
			_, value, _ := strings.Cut(trim, ":")
			for _, word := range strings.Fields(value) {
				word, _, _ = strings.Cut(word, "(")
				if word != "|" {
					keywords[word] = true
				}
			}
			out.WriteString(line)
			if !frontmatter {
				out.WriteString(marker("<pre class=\"org-metadata\">" + html.EscapeString(line) + "</pre>"))
			}
			continue
		}
		if level, title, ok := orgTitle(line); ok {
			current = &orgHeading{level: level, title: normalizeHeading(title, keywords)}
			p.heads = append(p.heads, current)
			out.WriteString(line)
			current.marker = fmt.Sprintf("%s_HEAD_%d", prefix, len(p.heads))
			out.WriteString("\n#+begin_export html\n<p>" + current.marker + "</p>\n#+end_export\n")
			continue
		}
		if drawerName(trim) != "" && trim != ":END:" {
			name := drawerName(trim)
			var body strings.Builder
			for i++; i < len(lines); i++ {
				if strings.EqualFold(strings.TrimSpace(lines[i]), ":END:") {
					break
				}
				body.WriteString(lines[i])
				if name == "PROPERTIES" && current != nil {
					key, value, ok := property(lines[i])
					if ok {
						switch key {
						case "ID":
							current.id = value
						case "CUSTOM_ID":
							current.custom = value
						case "VISIBILITY":
							current.visibility = value
						}
					}
				}
			}
			fragment, err := renderDrawer(drawerLayout, name, body.String())
			if err != nil {
				return p, fmt.Errorf("render drawer: %w", err)
			}
			out.WriteString(marker(fragment))
			continue
		}
		if strings.HasPrefix(upper, "SCHEDULED:") || strings.HasPrefix(upper, "DEADLINE:") || strings.HasPrefix(upper, "CLOSED:") {
			out.WriteString(marker("<pre class=\"org-metadata\">" + html.EscapeString(line) + "</pre>"))
			continue
		}
		out.WriteString(line)
	}
	// Pandoc's Org reader otherwise turns headings beyond its H limit into lists.
	p.text = out.String() + "\n#+OPTIONS: H:100000\n"
	return p, nil
}

// Blocks and drawers are consumed before their contents reach this boundary.
func orgKeyword(line string) (string, string, bool) {
	if !strings.HasPrefix(line, "#+") {
		return "", "", false
	}
	name, value, ok := strings.Cut(line[2:], ":")
	if !ok || name == "" {
		return "", "", false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_') {
			return "", "", false
		}
	}
	return strings.ToUpper(name), strings.TrimSpace(value), true
}

func orgTitle(line string) (int, string, bool) {
	n := 0
	for n < len(line) && line[n] == '*' {
		n++
	}
	if n == 0 || n >= len(line) || line[n] != ' ' {
		return 0, "", false
	}
	return n, strings.TrimSpace(line[n+1:]), true
}
func drawerName(line string) string {
	if len(line) < 3 || line[0] != ':' || line[len(line)-1] != ':' {
		return ""
	}
	name := line[1 : len(line)-1]
	for _, r := range name {
		if !(r == '_' || r == '-' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return ""
		}
	}
	return strings.ToUpper(name)
}
func property(line string) (string, string, bool) {
	trim := strings.TrimSpace(line)
	if !strings.HasPrefix(trim, ":") {
		return "", "", false
	}
	key, value, ok := strings.Cut(trim[1:], ":")
	return strings.ToUpper(key), strings.TrimSpace(value), ok
}
func normalizeHeading(title string, keywords map[string]bool) string {
	words := strings.Fields(title)
	if len(words) > 0 && keywords[words[0]] {
		words = words[1:]
	}
	if len(words) > 0 && regexp.MustCompile(`^\[#[A-Za-z0-9]\]$`).MatchString(words[0]) {
		words = words[1:]
	}
	if len(words) > 0 && strings.HasPrefix(words[len(words)-1], ":") && strings.HasSuffix(words[len(words)-1], ":") {
		words = words[:len(words)-1]
	}
	return strings.Join(words, " ")
}
