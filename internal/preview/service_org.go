// ABOUTME: Resolves Org file searches against a freshly rendered target catalogue.
// ABOUTME: Keeps lookup parameters separate from paths and redirects only unique anchors.
package preview

import (
	"net/http"
	"net/url"
	"strings"
)

type orgLookup struct{ search, fragment string }

func orgAnchor(p *page, search string) string {
	var ids []string
	if id, ok := strings.CutPrefix(search, "#"); ok {
		ids = p.ids[id]
	} else if title, ok := strings.CutPrefix(search, "*"); ok {
		ids = p.headings[strings.Join(strings.Fields(title), " ")]
	}
	if len(ids) == 1 {
		return ids[0]
	}
	return ""
}

func (s *previewService) serveOrgLookup(w http.ResponseWriter, r *http.Request, cap *readCapability) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query["path"]) != 1 || len(query["search"]) != 1 {
		publicError(w, r, 400, "Org lookup requires one path and search")
		return
	}
	path, search := query.Get("path"), query.Get("search")
	if len(path) == 0 || len(path) > 4096 || len(search) == 0 || len(search) > 4096 {
		publicError(w, r, 400, "Org lookup exceeds bounds")
		return
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if !validPathSegment(part) {
			publicError(w, r, 400, "Invalid Org lookup path")
			return
		}
	}
	lookup := &orgLookup{search: search}
	s.serveDocument(w, r, cap, parts, lookup)
}
