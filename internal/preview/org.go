// ABOUTME: Preserves Org values that the Pandoc reader discards.
// ABOUTME: Tracks literal blocks and drawers before producing collision-free markers.
package preview

import (
	"crypto/sha256"
	"fmt"
	"html"
	"regexp"
	"strings"
)

type orgHeading struct {
	marker, id, custom, visibility, title string
	level                                 int
}
type preservation struct {
	text, startup string
	fragments     map[string]string
	heads         []*orgHeading
	warnings      []string
}

func preserveOrg(data []byte) preservation {
	source := strings.ReplaceAll(string(data), "\r\n", "\n")
	prefix := fmt.Sprintf("HTMLPREVIEW_%x", sha256.Sum256(data))
	for strings.Contains(source, prefix) {
		prefix += "X"
	}
	p := preservation{startup: "showall", fragments: make(map[string]string)}
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
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)
		upper := strings.ToUpper(trim)
		if strings.HasPrefix(upper, "#+BEGIN_SRC") || strings.HasPrefix(upper, "#+BEGIN_EXAMPLE") {
			kind := "SRC"
			if strings.HasPrefix(upper, "#+BEGIN_EXAMPLE") {
				kind = "EXAMPLE"
			}
			var literal strings.Builder
			for i++; i < len(lines); i++ {
				if strings.EqualFold(strings.TrimSpace(lines[i]), "#+END_"+kind) {
					break
				}
				literal.WriteString(lines[i])
			}
			out.WriteString(marker("<pre><code>" + html.EscapeString(literal.String()) + "</code></pre>"))
			continue
		}
		if block != "" {
			out.WriteString(line)
			if strings.EqualFold(trim, "#+END_"+block) {
				block = ""
			}
			continue
		}
		if strings.HasPrefix(upper, "#+BEGIN_") {
			fields := strings.Fields(strings.TrimPrefix(upper, "#+BEGIN_"))
			if len(fields) > 0 {
				block = fields[0]
			}
			out.WriteString(line)
			continue
		}
		if strings.HasPrefix(trim, "# ") || trim == "#" {
			out.WriteString(line)
			continue
		}
		if strings.HasPrefix(upper, "#+INCLUDE:") || strings.HasPrefix(upper, "#+SETUPFILE:") {
			out.WriteString(marker("<pre class=\"org-metadata\">" + html.EscapeString(line) + "</pre>"))
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
			out.WriteString(marker("<pre class=\"org-metadata\">" + html.EscapeString(line) + "</pre>"))
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
			out.WriteString(marker("<pre class=\"org-metadata\">" + html.EscapeString(line) + "</pre>"))
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
			body.WriteString(line)
			for i++; i < len(lines); i++ {
				body.WriteString(lines[i])
				if strings.EqualFold(strings.TrimSpace(lines[i]), ":END:") {
					break
				}
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
			out.WriteString(marker("<details open class=\"org-metadata\"><summary>:" + html.EscapeString(name) + ":</summary><pre>" + html.EscapeString(body.String()) + "</pre></details>"))
			continue
		}
		if strings.HasPrefix(upper, "SCHEDULED:") || strings.HasPrefix(upper, "DEADLINE:") || strings.HasPrefix(upper, "CLOSED:") {
			out.WriteString(marker("<pre class=\"org-metadata\">" + html.EscapeString(line) + "</pre>"))
			continue
		}
		out.WriteString(line)
	}
	p.text = out.String()
	return p
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
