// ABOUTME: Reuses the native highlight palette for preserved Org code.
// ABOUTME: Keeps the Org writer independent of Markdown parsing.
package orgconvert

import "github.com/tigger-developer/HTML-Preview/internal/converthtml"

func highlight(text, language string) string { return converthtml.Highlight(text, language) }
