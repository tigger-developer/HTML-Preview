// ABOUTME: Enforces public HTTP authority, capability and encoded-path boundaries.
// ABOUTME: Returns bounded passive representations with uniform privacy headers.
package preview

import (
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func responsePrivacy(w http.ResponseWriter) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
}

func (s *previewService) serveHTTP(w http.ResponseWriter, r *http.Request) {
	responsePrivacy(w)
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	authority := strings.TrimPrefix(s.origin, "http://")
	if err != nil || net.ParseIP(peer) == nil || !net.ParseIP(peer).IsLoopback() || r.Host != authority || (r.URL.IsAbs() && (r.URL.Host != authority || r.URL.Scheme != "http")) || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.origin) || strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		publicError(w, r, 403, "Request authority refused")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		publicError(w, r, 405, "Method not allowed")
		return
	}
	if len(r.RequestURI) > 16*1024 {
		publicError(w, r, 413, "URL exceeds limit")
		return
	}
	if r.URL.Path == "/_health" {
		data := []byte(fmt.Sprintf(`{"protocol":1,"instance":%q,"ready":%t}`, s.instance, s.ctx.Err() == nil))
		status := 200
		if s.ctx.Err() != nil {
			status = 503
		}
		publicResponse(w, r, status, "application/json", data)
		return
	}
	parts, err := requestSegments(r.URL.EscapedPath())
	if err != nil {
		publicError(w, r, 400, "Malformed document path")
		return
	}
	if len(parts) < 2 {
		publicError(w, r, 404, "Not found")
		return
	}
	s.mu.Lock()
	cap := s.capabilities[parts[0]]
	s.mu.Unlock()
	if cap == nil {
		publicError(w, r, 404, "Not found")
		return
	}
	parts = parts[1:]
	if parts[0] == "_file" {
		parts = parts[1:]
	} else if parts[0] == "_org" && len(parts) == 1 {
		s.serveOrgLookup(w, r, cap)
		return
	}
	if len(parts) == 0 {
		publicError(w, r, 404, "Not found")
		return
	}
	s.serveDocument(w, r, cap, parts, nil)
}

func (s *previewService) serveDocument(w http.ResponseWriter, r *http.Request, cap *readCapability, parts []string, lookup *orgLookup) {
	from, status := s.requestFormat(r)
	if status != 0 {
		publicError(w, r, status, "Invalid or unavailable reader selection")
		return
	}
	path := filepath.Join(append([]string{cap.logicalRoot}, parts...)...)
	if len(path) > 4096 {
		publicError(w, r, 400, "Document path exceeds limit")
		return
	}
	src, err := identifyFormat(path, cap.root, from, s.base.formats)
	if err != nil {
		publicError(w, r, sourceHTTPStatus(err), "Document unavailable")
		return
	}
	rootInfo, err := os.Stat(cap.root)
	if err != nil || !contained(cap.logicalRoot, src.logical) || src.device != fileDevice(rootInfo) {
		publicError(w, r, 403, "Document outside authorized filesystem")
		return
	}
	data, status := s.renderHTTP(r.Context(), cap, src, lookup)
	if status != 200 {
		publicError(w, r, status, "Preview could not be rendered")
		return
	}
	if lookup != nil && lookup.fragment != "" {
		query := r.URL.Query()
		query.Del("path")
		query.Del("search")
		w.Header().Set("Location", s.documentURL(cap, src.logical, "", query.Encode(), lookup.fragment))
		publicResponse(w, r, 303, "text/html; charset=utf-8", nil)
		return
	}
	publicResponse(w, r, 200, "text/html; charset=utf-8", data)
}

func fileDevice(info os.FileInfo) string {
	if info == nil {
		return ""
	}
	return deviceOf(info)
}

func requestSegments(path string) ([]string, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("absolute path required")
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil || !validPathSegment(decoded) {
			return nil, errors.New("invalid path segment")
		}
		parts[i] = decoded
	}
	return parts, nil
}

func validPathSegment(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\\x00") && utf8.ValidString(value)
}

func (s *previewService) requestFormat(r *http.Request) (string, int) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return "", 400
	}
	values, present := query["htmlpreview-format"]
	if !present {
		return "", 0
	}
	if len(values) != 1 || len(values[0]) == 0 || len(values[0]) > 256 || !readerSelectionPattern.MatchString(values[0]) {
		return "", 400
	}
	if err := s.base.formats.validateSelection(r.Context(), s.host, s.pandoc, values[0]); err != nil {
		return "", 415
	}
	return values[0], 0
}

func sourceHTTPStatus(err error) int {
	if os.IsNotExist(err) {
		return 404
	}
	return 403
}

func publicError(w http.ResponseWriter, r *http.Request, status int, message string) {
	publicResponse(w, r, status, "text/html; charset=utf-8", []byte("<!doctype html><meta charset=utf-8><title>Preview unavailable</title><p>"+html.EscapeString(message)+"</p>"))
}

func publicResponse(w http.ResponseWriter, r *http.Request, status int, kind string, data []byte) {
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		if _, err := w.Write(data); err != nil {
			return
		}
	} // No retry or further body is sent after a disconnected reader.
}
