// ABOUTME: Resolves Org file searches against a freshly rendered target catalogue.
// ABOUTME: Keeps lookup parameters separate from paths and redirects only unique anchors.
package preview

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func renderSettingsIdentity(c config) string {
	return fmt.Sprintf("%s\x00%t/%d/%d/%d/%d/%d", c.root, c.toc, c.tocDepth, c.sourceBytes, c.totalBytes, c.outputBytes, c.deadline)
}

func (s *previewService) knownOrgID(ctx context.Context, cap *readCapability, current *page, id string) string {
	matches := make(map[string]bool)
	for _, anchor := range current.orgIDs[id] {
		matches[s.documentURL(cap, current.source.logical, "", "", anchor)] = true
	}
	s.mu.Lock()
	var candidates []*httpPage
	for _, page := range s.cache {
		if len(page.orgIDs[id]) > 0 && page.source.logical != current.source.logical && renderSettingsIdentity(page.cap.settings) == renderSettingsIdentity(cap.settings) {
			candidates = append(candidates, page)
		}
	}
	s.mu.Unlock()
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return ""
		}
		src, err := s.localAssetSource(cap, candidate.source.logical)
		if err != nil || src.canonical != candidate.source.canonical || src.root != candidate.source.root {
			continue
		}
		data, err := snapshot(src, min(cap.settings.sourceBytes, cap.settings.totalBytes))
		if err != nil || digestText(data) != candidate.revision || !s.dependenciesCurrent(candidate, cap) {
			continue
		}
		for _, anchor := range candidate.orgIDs[id] {
			matches[s.documentURL(candidate.cap, src.logical, "", "", anchor)] = true
		}
	}
	if len(matches) == 1 {
		for target := range matches {
			return target
		}
	}
	return ""
}

type orgLookup struct{ search, fragment string }

func catalogueAnchor(custom, headings map[string][]string, search string) string {
	var ids []string
	if id, ok := strings.CutPrefix(search, "#"); ok {
		ids = custom[id]
	} else if title, ok := strings.CutPrefix(search, "*"); ok {
		ids = headings[strings.Join(strings.Fields(title), " ")]
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
