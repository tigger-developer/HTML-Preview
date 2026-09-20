// ABOUTME: Highlights owned literal code using native lexers and the existing palette.
// ABOUTME: Verifies token text before publishing spans, falling back to escaped plaintext.
package orgconvert

import (
	"html"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

func highlight(text, language string) string {
	language = strings.ToLower(language)
	if language == "sh" || language == "shell" {
		language = "bash"
	}
	lexer := lexers.Get(language)
	if language == "" || lexer == nil {
		return html.EscapeString(text)
	}
	iterator, err := lexer.Tokenise(nil, text)
	if err != nil {
		return html.EscapeString(text)
	}
	var literal, out strings.Builder
	for token := iterator(); token != chroma.EOF; token = iterator() {
		literal.WriteString(token.Value)
		class := tokenClass(token.Type)
		escaped := html.EscapeString(token.Value)
		if class == "" {
			out.WriteString(escaped)
		} else {
			out.WriteString(`<span class="` + class + `">` + escaped + `</span>`)
		}
	}
	if literal.String() != text {
		return html.EscapeString(text)
	}
	return out.String()
}
func tokenClass(t chroma.TokenType) string {
	switch {
	case t == chroma.KeywordType:
		return "dt"
	case t.InCategory(chroma.Keyword):
		return "kw"
	case t.InSubCategory(chroma.NameFunction):
		return "fu"
	case t.InSubCategory(chroma.NameBuiltin):
		return "bu"
	case t.InSubCategory(chroma.LiteralString):
		return "st"
	case t.InSubCategory(chroma.LiteralNumber):
		return "dv"
	case t.InCategory(chroma.Comment):
		return "co"
	case t.InCategory(chroma.Operator):
		return "op"
	case t == chroma.NameAttribute:
		return "at"
	default:
		return ""
	}
}
