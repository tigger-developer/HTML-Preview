// ABOUTME: Adapts document-scoped annotation state and mutations to the local service.
// ABOUTME: Keeps read capabilities separate from author-bound write and composer grants.
package preview

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tigger-developer/HTML-Preview/internal/annotation"
)

type annotationGrant struct {
	token    string
	location annotation.Location
	source   os.FileInfo
	author   string
}

type annotationView struct {
	annotation.Event
	Status string `json:"status"`
	Closed bool   `json:"closed"`
}

type annotationState struct {
	Footnotes      []annotation.EditableFootnote `json:"footnotes"`
	Protocol       int                           `json:"protocol"`
	Revision       string                        `json:"revision"`
	SourceRevision string                        `json:"source_revision"`
	BodyRevision   string                        `json:"body_revision"`
	Events         []annotationView              `json:"events"`
	Storage        string                        `json:"storage"`
	Writable       bool                          `json:"writable"`
	Reason         string                        `json:"reason"`
	WriteToken     string                        `json:"write_token,omitempty"`
	Labels         []string                      `json:"footnote_labels"`
}

type annotationPoll struct {
	state    annotationState
	snapshot annotation.Snapshot
	at       time.Time
	cost     int
}

// Polls share a bounded one-second source snapshot; writes always bypass it.
func (s *previewService) polledAnnotations(ctx context.Context, cap *readCapability, src sourceContext) (annotationState, error) {
	key := fmt.Sprintf("%s\x00%s\x00%s\x00%t/%d/%d/%d/%d/%d", cap.root, src.canonical, src.input.key(), cap.settings.toc, cap.settings.tocDepth, cap.settings.sourceBytes, cap.settings.totalBytes, cap.settings.outputBytes, cap.settings.deadline)
	s.annotationPollMu.Lock()
	defer s.annotationPollMu.Unlock()
	item, exists := s.annotationPolls[key]
	if !exists || time.Since(item.at) >= time.Second {
		state, snap, _, err := s.readAnnotationDocument(ctx, cap, src)
		if err != nil {
			return state, err
		}
		item = annotationPoll{state: state, snapshot: snap, at: time.Now(), cost: 3*(len(snap.RawSource)+len(snap.RawSidecar)) + len(state.Events)*512}
		s.annotationPolls[key] = item
		cost := 0
		for _, entry := range s.annotationPolls {
			cost += entry.cost
		}
		for len(s.annotationPolls) > 50 || cost > 100*1024*1024 {
			oldest := ""
			var at time.Time
			for name, entry := range s.annotationPolls {
				if oldest == "" || entry.at.Before(at) {
					oldest, at = name, entry.at
				}
			}
			cost -= s.annotationPolls[oldest].cost
			delete(s.annotationPolls, oldest)
		}
	}
	state := item.state
	err := s.authorizeAnnotations(&state, cap, src, item.snapshot)
	return state, err
}

func annotationFormat(src sourceContext) string {
	ext := strings.ToLower(filepath.Ext(src.canonical))
	if ext == ".org" && src.input.kind == "org" {
		return "org"
	}
	base := readerBase(src.input.reader)
	if strings.Contains(" .md .markdown .mdown .mkd .mkdn .commonmark ", " "+ext+" ") && (strings.HasPrefix(base, "markdown") || strings.HasPrefix(base, "commonmark") || base == "gfm") {
		return "markdown"
	}
	return ""
}

func (s *previewService) annotationSource(r *http.Request) (*readCapability, sourceContext, error) {
	parts, err := requestSegments(r.URL.EscapedPath())
	if err != nil || len(parts) < 4 || parts[0] != "_annotations" || (parts[1] != "v1" && parts[1] != "v2") || len(r.RequestURI) > 16*1024 {
		return nil, sourceContext{}, errors.New("invalid annotation path")
	}
	s.mu.Lock()
	cap := s.capabilities[parts[2]]
	s.mu.Unlock()
	if cap == nil {
		return nil, sourceContext{}, errors.New("unknown read capability")
	}
	from, status := s.requestFormat(r)
	if status != 0 {
		return nil, sourceContext{}, errors.New("invalid reader")
	}
	path := filepath.Join(append([]string{cap.logicalRoot}, parts[3:]...)...)
	if len(path) > 4096 {
		return nil, sourceContext{}, errors.New("path limit")
	}
	src, err := identifyFormat(path, cap.root, from, s.base.formats)
	if err != nil {
		return nil, src, err
	}
	base, err := os.Stat(cap.root)
	if err != nil || src.device != fileDevice(base) || !contained(cap.logicalRoot, src.logical) || annotationFormat(src) == "" {
		return nil, src, errors.New("annotation target unavailable")
	}
	return cap, src, nil
}

func (s *previewService) serveAnnotations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, HEAD, POST")
		annotationError(w, r, 405, "method_not_allowed")
		return
	}
	cap, src, err := s.annotationSource(r)
	if err != nil {
		annotationError(w, r, 404, "target_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if r.Method == http.MethodPost {
		s.appendAnnotation(w, r, cap, src)
		return
	}
	state, err := s.polledAnnotations(ctx, cap, src)
	if err != nil {
		annotationFailure(w, r, err)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/_annotations/v2/") {
		response := map[string]any{"protocol": 2, "revision": state.Revision, "source_revision": state.SourceRevision, "body_revision": state.BodyRevision, "comments": state.Events, "storage": state.Storage, "writable": state.Writable, "reason": state.Reason, "footnote_labels": state.Labels, "footnotes": state.Footnotes}
		if state.WriteToken != "" {
			response["write_token"] = state.WriteToken
		}
		annotationResponse(w, r, 200, response)
	} else {
		annotationResponse(w, r, 200, state)
	}
}

func (s *previewService) readAnnotationDocument(ctx context.Context, cap *readCapability, src sourceContext) (annotationState, annotation.Snapshot, *httpPage, error) {
	state := annotationState{Protocol: 1, Events: []annotationView{}}
	loc := annotation.Location{Root: cap.root, Path: src.canonical, Format: annotationFormat(src), Limit: min(cap.settings.sourceBytes, cap.settings.totalBytes)}
	snap, err := annotation.Read(loc)
	if err != nil {
		return state, snap, nil, err
	}
	page, status := s.currentPage(ctx, cap, src)
	if status != 200 {
		return state, snap, nil, &annotation.Failure{Code: "render_unavailable"}
	}
	if page.annotationSourceRevision != snap.SourceRevision || page.revision != annotation.Digest(snap.RawSource) {
		return state, snap, page, &annotation.Failure{Code: "stale_source"}
	}
	state.Revision, state.SourceRevision, state.BodyRevision = snap.Revision, snap.SourceRevision, annotation.Digest([]byte(page.bodyText))
	state.Reason = snap.Reason
	state.Footnotes = append(annotation.EditableFootnotes(snap.RawSource, loc.Format, "embedded"), annotation.EditableFootnotes(snap.RawSidecar, "org", "sidecar")...)
	labels := make(map[string]bool)
	for label := range snap.Embedded.Labels {
		labels[label] = true
	}
	for label := range snap.Sidecar.Labels {
		labels[label] = true
	}
	state.Labels = make([]string, 0, len(labels))
	for label := range labels {
		state.Labels = append(state.Labels, label)
	}
	sort.Strings(state.Labels)
	for _, event := range snap.Events {
		_, match := annotation.Resolve(event.Target, page.bodyText, page.headingSpans)
		if event.Schema == 2 {
			match = "unplaced"
			for _, note := range snap.Embedded.Notes {
				if note.Event.AnnotationID == event.AnnotationID && note.Located() {
					match = "resolved"
				}
			}
		}
		if located, virtual := page.annotationLocations[event.AnnotationID]; virtual {
			match = "unplaced"
			if located {
				match = "resolved"
			}
		}
		state.Events = append(state.Events, annotationView{event, match, event.Kind == "close"})
	}
	return state, snap, page, nil
}

func (s *previewService) authorizeAnnotations(state *annotationState, cap *readCapability, src sourceContext, snap annotation.Snapshot) error {
	loc := annotation.Location{Root: cap.root, Path: src.canonical, Format: annotationFormat(src), Limit: min(cap.settings.sourceBytes, cap.settings.totalBytes)}
	if !cap.settings.annotations {
		state.Reason = "read_only_preview"
		return nil
	}
	if cap.settings.displayName == "" {
		state.Reason = "missing_identity"
		return nil
	}
	if state.Reason != "" {
		return nil
	}
	var err error
	state.Storage, err = annotation.Destination(loc, snap)
	if err != nil {
		state.Reason = annotationErrorCode(err)
		return nil
	}
	key := cap.token + "\x00" + src.canonical
	s.mu.Lock()
	defer s.mu.Unlock()
	grant := s.annotationGrants[key]
	if grant == nil {
		if len(s.annotationGrants) >= 256 {
			return &annotation.Failure{Code: "grant_capacity"}
		}
		token, err := randomCapability()
		if err != nil {
			return err
		}
		grant = &annotationGrant{token, loc, snap.SourceInfo, cap.settings.displayName}
		s.annotationGrants[key] = grant
	}
	state.Writable, state.WriteToken = true, grant.token
	return nil
}

func (s *previewService) appendAnnotation(w http.ResponseWriter, r *http.Request, cap *readCapability, src sourceContext) {
	if !cap.settings.annotations || cap.settings.displayName == "" || len(r.Header.Values("Origin")) != 1 || r.Header.Get("Origin") != s.origin {
		annotationError(w, r, 403, "write_refused")
		return
	}
	key := cap.token + "\x00" + src.canonical
	s.mu.Lock()
	storedGrant := s.annotationGrants[key]
	var grant annotationGrant
	if storedGrant != nil {
		grant = *storedGrant
	}
	s.mu.Unlock()
	if storedGrant == nil || len(r.Header.Values("X-HTMLPreview-Annotation-Token")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-HTMLPreview-Annotation-Token")), []byte(grant.token)) != 1 {
		annotationError(w, r, 403, "write_refused")
		return
	}
	secret := r.Header.Get("X-HTMLPreview-Composer-Token")
	decoded, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil || len(decoded) != 32 || len(r.Header.Values("X-HTMLPreview-Composer-Token")) != 1 {
		annotationError(w, r, 403, "composer_refused")
		return
	}
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		annotationError(w, r, 400, "invalid_content_type")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
	if err != nil {
		annotationError(w, r, 413, "body_limit")
		return
	}
	var request annotation.Request
	if annotation.Decode(data, &request) != nil {
		annotationError(w, r, 400, "invalid_event")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/_annotations/v1/") {
		annotationError(w, r, 409, "annotation_upgrade_required")
		return
	}
	s.writeCurrentAnnotation(w, r, cap, src, grant, secret, request)
}

func annotationErrorCode(err error) string {
	var failure *annotation.Failure
	if errors.As(err, &failure) {
		return failure.Code
	}
	if os.IsPermission(err) {
		return "permission_denied"
	}
	if os.IsNotExist(err) {
		return "target_unavailable"
	}
	return "storage_unavailable"
}

func annotationFailure(w http.ResponseWriter, r *http.Request, err error) {
	code := annotationErrorCode(err)
	status := 503
	switch code {
	case "invalid_event", "invalid_source", "invalid_label", "invalid_selector":
		status = 400
	case "target_unavailable":
		status = 404
	case "store_limit", "body_limit":
		status = 413
	case "busy":
		status = 429
	case "footnote_conflict", "read_only_footnote", "invalid_footnote", "stale_source", "stale_body", "source_replaced", "source_changed", "store_changed", "corrupt_store", "foreign_sidecar", "unsafe_source", "unsafe_sidecar", "unsupported_store", "operation_conflict", "composer_conflict", "sequence_conflict", "closed_comment", "label_conflict", "point_unmappable", "storage_metadata_unsupported", "target_unresolved":
		status = 409
	}
	annotationError(w, r, status, code)
}

func (s *previewService) writeCurrentAnnotation(w http.ResponseWriter, r *http.Request, cap *readCapability, src sourceContext, grant annotationGrant, secret string, request annotation.Request) {
	if err := request.ValidateCurrent(); err != nil {
		annotationFailure(w, r, err)
		return
	}
	verify := func(ctx context.Context, snap annotation.Snapshot, target *annotation.Target) (int, error) {
		base, status := s.currentPage(ctx, cap, src)
		if status != 200 {
			return -1, &annotation.Failure{Code: "render_unavailable"}
		}
		if base.revision != annotation.Digest(snap.RawSource) || annotation.Digest([]byte(base.bodyText)) != request.BodyRevision {
			return -1, &annotation.Failure{Code: "stale_body"}
		}
		if target.Type == "point" && target.HeadingID != "" && base.explicitIDs[target.HeadingID] == "" {
			return -1, &annotation.Failure{Code: "invalid_selector"}
		}
		if target.Type == "text" {
			resolved, match := annotation.Resolve(*target, base.bodyText, base.headingSpans)
			if match != "resolved" {
				return -1, &annotation.Failure{Code: "point_unmappable"}
			}
			*target = annotation.Target{Type: "point", Position: resolved.End, Run: resolved.Exact, RunOffset: utf8.RuneCountInString(resolved.Exact)}
		}
		at, err := annotation.SourcePointCandidate(snap.RawSource, annotationFormat(src), target.Run, target.RunOffset)
		if err != nil {
			return -1, err
		}
		id, err := annotation.NewID()
		if err != nil {
			return -1, err
		}
		marker := "HPPOINT" + strings.ReplaceAll(id, "-", "")
		input := append(append(append([]byte(nil), snap.RawSource[:at]...), []byte(marker)...), snap.RawSource[at:]...)
		probeCap := *cap
		probeCap.settings.annotations = false
		probe, status := s.buildHTTP(ctx, &probeCap, src, input)
		if status != 200 {
			return -1, &annotation.Failure{Code: "point_unmappable"}
		}
		index := strings.Index(probe.bodyText, marker)
		if index < 0 || strings.Count(probe.bodyText, marker) != 1 || utf8.RuneCountInString(probe.bodyText[:index]) != target.Position || strings.Replace(probe.bodyText, marker, "", 1) != base.bodyText {
			return -1, &annotation.Failure{Code: "point_unmappable"}
		}
		body := []rune(base.bodyText)
		target.BodyRevision = request.BodyRevision
		target.Prefix = string(body[max(0, target.Position-64):target.Position])
		target.Suffix = string(body[target.Position:min(len(body), target.Position+64)])
		target.HeadingID = ""
		smallest := len(body) + 1
		for key, span := range base.headingSpans {
			if span.Start <= target.Position && target.Position <= span.End && (span.End-span.Start < smallest || span.End-span.Start == smallest && key < target.HeadingID) {
				smallest = span.End - span.Start
				target.HeadingID = key
			}
		}
		return at, nil
	}
	result, err := s.annotationWriter.Replace(r.Context(), grant.location, grant.source, grant.author, secret, request, verify)
	if result.PreviousInfo != nil && result.SourceInfo != nil {
		s.mu.Lock()
		for _, current := range s.annotationGrants {
			if current.location.Path == src.canonical && os.SameFile(current.source, result.PreviousInfo) {
				current.source = result.SourceInfo
			}
		}
		s.mu.Unlock()
	}
	s.annotationPollMu.Lock()
	for key := range s.annotationPolls {
		if strings.Contains(key, "\x00"+src.canonical+"\x00") {
			delete(s.annotationPolls, key)
		}
	}
	s.annotationPollMu.Unlock()
	if err != nil {
		annotationFailure(w, r, err)
		return
	}
	status := 201
	if result.Retry {
		status = 200
	}
	annotationResponse(w, r, status, result.Receipt)
}

func annotationError(w http.ResponseWriter, r *http.Request, status int, code string) {
	if status == 429 || status == 503 {
		w.Header().Set("Retry-After", "1")
	}
	annotationResponse(w, r, status, map[string]string{"error": code})
}

func annotationResponse(w http.ResponseWriter, r *http.Request, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil || len(data) > annotation.MaxStore+65536 {
		publicResponse(w, r, 503, "application/json", []byte(`{"error":"response_limit"}`))
		return
	}
	publicResponse(w, r, status, "application/json", data)
}
