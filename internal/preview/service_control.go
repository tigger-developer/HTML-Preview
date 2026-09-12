// ABOUTME: Validates private preview registration and immutable read capabilities.
// ABOUTME: Separates reader choices and rendering settings from filesystem grants.
package preview

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type serviceStatus struct {
	Protocol       int      `json:"protocol"`
	Instance       string   `json:"instance"`
	Origin         string   `json:"origin"`
	Ready          bool     `json:"ready"`
	FormatContract int      `json:"format_contract"`
	InputFormats   []string `json:"input_formats"`
}

type previewRegistration struct {
	Paths          []string        `json:"paths"`
	Settings       previewSettings `json:"settings"`
	From           string          `json:"from,omitempty"`
	FormatContract int             `json:"format_contract"`
}

type previewSettings struct {
	TOC         *bool  `json:"toc,omitempty"`
	TOCDepth    *int   `json:"toc_depth,omitempty"`
	SourceBytes *int64 `json:"source_bytes,omitempty"`
	TotalBytes  *int64 `json:"total_source_bytes,omitempty"`
	OutputBytes *int64 `json:"output_bytes,omitempty"`
	Deadline    string `json:"deadline,omitempty"`
	Root        string `json:"root,omitempty"`
}

type registrationResult struct {
	Path  string `json:"path"`
	URL   string `json:"url,omitempty"`
	Error string `json:"error,omitempty"`
}
type registrationResponse struct {
	Protocol int                  `json:"protocol"`
	Results  []registrationResult `json:"results"`
}

func (s *previewService) serveControl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/v1/status" && r.Method == http.MethodGet {
		readers := make([]string, 0, len(s.base.formats.readers))
		for reader := range s.base.formats.readers {
			readers = append(readers, reader)
		}
		sort.Strings(readers)
		writeControl(w, 200, serviceStatus{1, s.instance, s.origin, s.ctx.Err() == nil, 1, readers})
		return
	}
	if r.URL.Path != "/v1/previews" || r.Method != http.MethodPost {
		writeControl(w, 404, map[string]string{"error": "not_found"})
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
	if err != nil {
		writeControl(w, 413, map[string]string{"error": "body_limit"})
		return
	}
	var request previewRegistration
	if err = decodeControl(data, &request); err != nil || len(request.Paths) == 0 || int64(len(request.Paths)) > s.base.files {
		writeControl(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if request.FormatContract != 1 {
		writeControl(w, 409, map[string]string{"error": "format_contract"})
		return
	}
	cfg, err := request.Settings.apply(s.base)
	if err != nil {
		writeControl(w, 400, map[string]string{"error": "invalid_settings"})
		return
	}
	if request.From != "" {
		if err = s.base.formats.validateSelection(r.Context(), s.host, s.pandoc, request.From); err != nil {
			writeControl(w, 400, map[string]string{"error": "invalid_reader"})
			return
		}
	}
	response := registrationResponse{Protocol: 1}
	for _, path := range request.Paths {
		response.Results = append(response.Results, s.register(path, request.From, cfg))
	}
	writeControl(w, 200, response)
}

func writeControl(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil || len(data) > 256*1024 {
		http.Error(w, `{"error":"response_limit"}`, 503)
		return
	}
	w.WriteHeader(status)
	if _, err = w.Write(data); err != nil {
		return
	} // The disconnected control caller owns retry policy.
}

func decodeControl(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueJSON(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func uniqueJSON(decoder *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("JSON nesting limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return errors.New("null is not a control value")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return errors.New("duplicate or invalid JSON key")
			}
			seen[name] = true
		}
		if err := uniqueJSON(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func (settings previewSettings) apply(base config) (config, error) {
	c := base
	c.sourceBytes = min(c.sourceBytes, 10*1024*1024)
	c.outputBytes = min(c.outputBytes, 50*1024*1024)
	c.deadline = min(c.deadline, time.Minute)
	if settings.TOC != nil {
		c.toc = *settings.TOC
	}
	if settings.TOCDepth != nil {
		c.tocDepth = *settings.TOCDepth
	}
	if c.tocDepth < 1 || c.tocDepth > 6 {
		return c, errors.New("toc_depth outside 1..6")
	}
	for _, bound := range []struct {
		value  *int64
		target *int64
		max    int64
	}{{settings.SourceBytes, &c.sourceBytes, 10 * 1024 * 1024}, {settings.TotalBytes, &c.totalBytes, 50 * 1024 * 1024}, {settings.OutputBytes, &c.outputBytes, 50 * 1024 * 1024}} {
		if bound.value != nil {
			if *bound.value < 1 || *bound.value > bound.max {
				return c, errors.New("byte limit outside service ceiling")
			}
			*bound.target = *bound.value
		}
	}
	if settings.Deadline != "" {
		d, err := time.ParseDuration(settings.Deadline)
		if err != nil || d < 100*time.Millisecond || d > time.Minute {
			return c, errors.New("deadline outside service ceiling")
		}
		c.deadline = d
	}
	c.root = settings.Root
	if c.root != "" {
		if !filepath.IsAbs(c.root) {
			return c, errors.New("narrowing root must be absolute")
		}
		root, err := filepath.EvalSymlinks(c.root)
		if err != nil {
			return c, err
		}
		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			return c, errors.New("narrowing root must be a directory")
		}
		c.root = root
	}
	return c, nil
}

func (s *previewService) register(path, from string, cfg config) registrationResult {
	result := registrationResult{Path: path}
	if !filepath.IsAbs(path) || len(path) > 4096 {
		result.Error = "invalid_source"
		return result
	}
	src, err := identify(path, cfg.root)
	if err != nil {
		result.Error = "invalid_source"
		return result
	}
	root := s.effectiveRoot(src.logical, src.canonical, cfg.root)
	if root == "" {
		result.Error = "outside_root"
		return result
	}
	if _, err := s.base.formats.resolve(path, from); err != nil {
		result.Error = "invalid_source"
		return result
	}
	cap, err := s.capability(root, filepath.Dir(src.logical), cfg)
	if err != nil {
		result.Error = "capacity_exceeded"
		return result
	}
	result.URL = s.documentURL(cap, src.logical, from, "", "")
	return result
}

func (s *previewService) effectiveRoot(logical, canonical, narrow string) string {
	for _, root := range s.config.roots {
		if logicalRootFor(root, logical) == "" || !contained(root, canonical) {
			continue
		}
		if narrow != "" {
			if logicalRootFor(narrow, logical) == "" || !contained(narrow, canonical) {
				continue
			}
			if contained(root, narrow) {
				root = narrow
			}
		}
		return root
	}
	return ""
}

func (s *previewService) capability(root, parent string, cfg config) (*readCapability, error) {
	key := fmt.Sprintf("%s\x00%s\x00%s\x00%t/%d/%d/%d/%d/%d", root, parent, cfg.root, cfg.toc, cfg.tocDepth, cfg.sourceBytes, cfg.totalBytes, cfg.outputBytes, cfg.deadline)
	s.mu.Lock()
	defer s.mu.Unlock()
	if cap := s.contexts[key]; cap != nil {
		return cap, nil
	}
	if len(s.capabilities) >= 128 {
		return nil, errors.New("capability capacity exceeded")
	}
	token, err := randomCapability()
	if err != nil {
		return nil, err
	}
	cap := &readCapability{token: token, root: root, logicalRoot: logicalRootFor(root, filepath.Join(parent, "document")), parent: parent, settings: cfg}
	s.capabilities[cap.token] = cap
	s.contexts[key] = cap
	return cap, nil
}

func (s *previewService) documentURL(cap *readCapability, path, from, query, fragment string) string {
	rel, err := filepath.Rel(cap.logicalRoot, path)
	if err != nil || !contained(cap.logicalRoot, path) {
		return ""
	}
	rel = filepath.ToSlash(rel)
	first, _, _ := strings.Cut(rel, "/")
	if first == "_org" || first == "_file" || first == "_media" {
		rel = "_file/" + rel
	}
	u := url.URL{Path: "/" + cap.token + "/" + rel, RawQuery: query, Fragment: fragment}
	if from != "" {
		values := u.Query()
		values.Set("htmlpreview-format", from)
		u.RawQuery = values.Encode()
	}
	return s.origin + u.String()
}

// Resolve only ancestor directories to recognize a configured root's logical
// spelling (for example Darwin /var). Descendant symlinks still undergo the
// independent canonical target and rooted-handle checks.
func logicalRootFor(canonical, logical string) string {
	for parent := filepath.Dir(logical); ; parent = filepath.Dir(parent) {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil && resolved == canonical {
			return parent
		}
		if parent == filepath.Dir(parent) {
			return ""
		}
	}
}
