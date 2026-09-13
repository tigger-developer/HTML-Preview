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
	"strings"
	"time"

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
	Protocol       int              `json:"protocol"`
	Revision       string           `json:"revision"`
	SourceRevision string           `json:"source_revision"`
	BodyRevision   string           `json:"body_revision"`
	Events         []annotationView `json:"events"`
	Storage        string           `json:"storage"`
	Writable       bool             `json:"writable"`
	Reason         string           `json:"reason"`
	WriteToken     string           `json:"write_token,omitempty"`
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
	if err != nil || len(parts) < 4 || parts[0] != "_annotations" || parts[1] != "v1" || len(r.RequestURI) > 16*1024 {
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
	annotationResponse(w, r, 200, state)
}

func (s *previewService) annotationDocument(ctx context.Context, cap *readCapability, src sourceContext) (annotationState, annotation.Snapshot, *httpPage, error) {
	state, snap, page, err := s.readAnnotationDocument(ctx, cap, src)
	if err == nil {
		err = s.authorizeAnnotations(&state, cap, src, snap)
	}
	return state, snap, page, err
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
	for _, event := range snap.Events {
		_, match := annotation.Resolve(event.Target, page.bodyText, page.headingSpans)
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
	grant := s.annotationGrants[key]
	s.mu.Unlock()
	if grant == nil || len(r.Header.Values("X-HTMLPreview-Annotation-Token")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-HTMLPreview-Annotation-Token")), []byte(grant.token)) != 1 {
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
	if err := request.Validate(); err != nil {
		annotationFailure(w, r, err)
		return
	}
	state, snap, page, err := s.annotationDocument(r.Context(), cap, src)
	if err != nil {
		annotationFailure(w, r, err)
		return
	}
	// An identical committed operation is checked by the writer before revisions.
	if _, exists := snap.Operations[request.OperationID]; !exists {
		if request.SourceRevision != state.SourceRevision {
			annotationError(w, r, 409, "stale_source")
			return
		}
		if request.BodyRevision != state.BodyRevision {
			annotationError(w, r, 409, "stale_body")
			return
		}
		if request.Target.HeadingID != "" && page.explicitIDs[request.Target.HeadingID] == "" {
			annotationError(w, r, 400, "invalid_selector")
			return
		}
		_, match := annotation.Resolve(request.Target, page.bodyText, page.headingSpans)
		if match != "resolved" {
			annotationError(w, r, 409, "target_"+match)
			return
		}
	}
	receipt, retry, err := s.annotationWriter.Append(r.Context(), grant.location, grant.source, grant.author, secret, request)
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
	if retry {
		status = 200
	}
	annotationResponse(w, r, status, receipt)
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
	case "invalid_event", "invalid_source":
		status = 400
	case "target_unavailable":
		status = 404
	case "store_limit", "body_limit":
		status = 413
	case "busy":
		status = 429
	case "stale_source", "stale_body", "source_replaced", "source_changed", "store_changed", "corrupt_store", "foreign_sidecar", "unsafe_source", "unsafe_sidecar", "unsupported_store", "operation_conflict", "composer_conflict", "sequence_conflict", "closed_comment":
		status = 409
	}
	annotationError(w, r, status, code)
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
