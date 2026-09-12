// ABOUTME: Preserves passive authored CSS through a maintained grammar and token parser.
// ABOUTME: Decodes resource tokens and admits only bounded local raster URLs.
package preview

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

func (s *session) passiveCSS(p *page, input string, inline bool, base string) string {
	parser := css.NewParser(parse.NewInputString(input), inline)
	var out strings.Builder
	var allowed []bool
	suppressed := 0
	for {
		kind, _, data := parser.Next()
		if kind == css.ErrorGrammar {
			if !errors.Is(parser.Err(), io.EOF) {
				s.log.notice("%q: malformed stylesheet omitted", p.source.logical)
				return ""
			}
			break
		}
		switch kind {
		case css.BeginAtRuleGrammar, css.BeginRulesetGrammar:
			accept := suppressed == 0 && !inline
			var prelude strings.Builder
			for _, token := range parser.Values() {
				prelude.Write(token.Data)
			}
			cleanPrelude := prelude.String()
			if kind == css.BeginAtRuleGrammar {
				name := strings.ToLower(decodeCSS(string(data)))
				accept = accept && (name == "@media" || name == "@supports" || name == "@layer")
				if accept {
					cleanPrelude, accept = s.cssValue(p, cleanPrelude, base)
					if !accept {
						s.log.notice("%q: unsupported conditional CSS resource omitted", p.source.logical)
					}
				}
			}
			allowed = append(allowed, accept)
			if !accept {
				suppressed++
				continue
			}
			if kind == css.BeginAtRuleGrammar {
				out.Write(data)
				out.WriteByte(' ')
			}
			out.WriteString(cleanPrelude)
			out.WriteByte('{')
		case css.EndAtRuleGrammar, css.EndRulesetGrammar:
			if len(allowed) == 0 {
				return ""
			}
			last := allowed[len(allowed)-1]
			allowed = allowed[:len(allowed)-1]
			if last {
				out.WriteByte('}')
			} else {
				suppressed--
			}
		case css.DeclarationGrammar, css.CustomPropertyGrammar:
			if suppressed != 0 {
				continue
			}
			name := strings.ToLower(decodeCSS(string(data)))
			if name == "behavior" || name == "-moz-binding" || !css.IsIdent([]byte(name)) {
				continue
			}
			var value strings.Builder
			for _, token := range parser.Values() {
				value.Write(token.Data)
			}
			clean, ok := s.cssValue(p, value.String(), base)
			if !ok {
				s.log.notice("%q: unsupported CSS resource or function omitted", p.source.logical)
				continue
			}
			out.WriteString(name)
			out.WriteByte(':')
			out.WriteString(clean)
			out.WriteByte(';')
		case css.AtRuleGrammar:
			s.log.notice("%q: stylesheet import or unsupported at-rule omitted", p.source.logical)
		}
		if int64(out.Len()) > s.cfg.outputBytes-s.used {
			return ""
		}
	}
	// HTML raw-text elements must never acquire a closing tag from source CSS.
	return strings.ReplaceAll(out.String(), "<", "\\3c ")
}

// This allowlist excludes functions with implicit resource loading, including
// image-set's string URLs. Custom properties pass through the same token checks.
// #nosec G101 -- These are CSS function names, not credentials.
const passiveCSSFunctions = "rgb rgba hsl hsla hwb lab lch oklab oklch color color-mix light-dark calc min max clamp round mod rem abs sign pow sqrt hypot log exp sin cos tan asin acos atan atan2 var env linear-gradient radial-gradient conic-gradient repeating-linear-gradient repeating-radial-gradient repeating-conic-gradient translate translatex translatey translatez translate3d scale scalex scaley scalez scale3d rotate rotatex rotatey rotatez rotate3d skew skewx skewy matrix matrix3d perspective cubic-bezier steps fit-content minmax repeat counter counters attr"

func (s *session) cssValue(p *page, value, base string) (string, bool) {
	lexer := css.NewLexer(parse.NewInputString(value))
	var out strings.Builder
	for {
		kind, data := lexer.Next()
		if kind == css.ErrorToken {
			return out.String(), errors.Is(lexer.Err(), io.EOF)
		}
		decoded := decodeCSS(string(data))
		switch kind {
		case css.BadURLToken, css.BadStringToken, css.AtKeywordToken, css.LeftBraceToken, css.RightBraceToken, css.SemicolonToken:
			return "", false
		case css.URLToken:
			raw := string(data)
			open := strings.IndexByte(raw, '(')
			if open < 0 || !strings.HasSuffix(raw, ")") {
				return "", false
			}
			value := strings.TrimSpace(raw[open+1 : len(raw)-1])
			if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
				if value[len(value)-1] != value[0] {
					return "", false
				}
				value = value[1 : len(value)-1]
			}
			value = decodeCSS(value)
			resolved := resolveHTMLReference(base, value)
			image, err := s.imageURL(p, resolved, "")
			if err != nil {
				return "", false
			}
			out.WriteString("url(\"")
			out.WriteString(image)
			out.WriteString("\")")
		case css.FunctionToken:
			name := strings.ToLower(strings.TrimSuffix(decoded, "("))
			if name == "url" {
				// Escaped function names can be tokenized separately from URLToken.
				value, ok := cssURLArgument(lexer)
				if !ok {
					return "", false
				}
				image, err := s.imageURL(p, resolveHTMLReference(base, value), "")
				if err != nil {
					return "", false
				}
				out.WriteString("url(\"")
				out.WriteString(image)
				out.WriteString("\")")
			} else {
				if !strings.Contains(" "+passiveCSSFunctions+" ", " "+name+" ") {
					return "", false
				}
				out.WriteString(name)
				out.WriteByte('(')
			}
		case css.CommentToken:
			out.WriteByte(' ')
		default:
			out.Write(data)
		}
	}
}

func cssURLArgument(lexer *css.Lexer) (string, bool) {
	var out strings.Builder
	for {
		kind, data := lexer.Next()
		switch kind {
		case css.RightParenthesisToken:
			value := strings.TrimSpace(out.String())
			if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
				if value[len(value)-1] != value[0] {
					return "", false
				}
				return decodeCSS(value[1 : len(value)-1]), true
			}
			return decodeCSS(value), value != ""
		case css.ErrorToken, css.BadURLToken, css.BadStringToken, css.FunctionToken, css.URLToken, css.LeftParenthesisToken, css.LeftBraceToken, css.RightBraceToken, css.SemicolonToken:
			return "", false
		default:
			out.Write(data)
		}
	}
}

// CSS Syntax's escaped-code-point algorithm is applied to tokens, never used
// as a replacement for parsing a stylesheet. Invalid scalars become U+FFFD.
func decodeCSS(input string) string {
	var out strings.Builder
	for i := 0; i < len(input); i++ {
		if input[i] != '\\' || i+1 == len(input) {
			out.WriteByte(input[i])
			continue
		}
		i++
		start := i
		for i < len(input) && i-start < 6 && strings.ContainsRune("0123456789abcdefABCDEF", rune(input[i])) {
			i++
		}
		if i > start {
			value, err := strconv.ParseUint(input[start:i], 16, 32)
			if err != nil || value == 0 || !utf8.ValidRune(rune(value)) {
				out.WriteRune(utf8.RuneError)
			} else {
				out.WriteRune(rune(value))
			}
			if i < len(input) && strings.ContainsRune(" \t\r\n\f", rune(input[i])) {
				if input[i] == '\r' && i+1 < len(input) && input[i+1] == '\n' {
					i++
				}
			} else {
				i--
			}
		} else if input[i] == '\r' || input[i] == '\n' || input[i] == '\f' {
			if input[i] == '\r' && i+1 < len(input) && input[i+1] == '\n' {
				i++
			}
		} else {
			out.WriteByte(input[i])
		}
	}
	return out.String()
}
