// ABOUTME: Selects available local HTTP previews through authenticated private registration.
// ABOUTME: Preflights entry responses before browser handoff and classifies fallback narrowly.
package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type httpPreviews struct {
	urls             map[string]string
	sources, outputs int64
	invalid          bool
}

type serviceConnection struct {
	client *http.Client
	status serviceStatus
}

type readerError struct{ cause error }

func (e *readerError) Error() string { return e.cause.Error() }
func (e *readerError) Unwrap() error { return e.cause }

func connectService(ctx context.Context, runtime string) (*serviceConnection, error) {
	client, err := controlClient(runtime)
	if err != nil {
		if unavailableService(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("service discovery: %w", err)
	}
	status, err := serviceDiscovery(ctx, client)
	if err != nil {
		client.CloseIdleConnections()
		if unavailableService(err) {
			return nil, nil
		}
		return nil, err
	}
	return &serviceConnection{client: client, status: status}, nil
}

func controlClient(runtime string) (*http.Client, error) {
	if err := checkRuntime(runtime, false); err != nil {
		return nil, err
	}
	path := filepath.Join(runtime, "control.sock")
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if st.Mode()&os.ModeSocket == 0 || !ownedByUser(st) || st.Mode().Perm() != 0600 {
		return nil, errors.New("unsafe control socket")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
		if err != nil {
			return nil, err
		}
		unix, ok := conn.(*net.UnixConn)
		if !ok {
			return nil, errors.Join(errors.New("control is not a Unix socket"), conn.Close())
		}
		if err = sameUserPeer(unix); err != nil {
			return nil, errors.Join(err, conn.Close())
		}
		return conn, nil
	}}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func unavailableService(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded)
}

func serviceDiscovery(ctx context.Context, client *http.Client) (serviceStatus, error) {
	var status serviceStatus
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://control/v1/status", nil)
	if err != nil {
		return status, err
	}
	data, code, err := controlResponse(client, req)
	if err != nil {
		return status, err
	}
	if code != 200 || decodeControl(data, &status) != nil || !status.Ready || status.Protocol != 1 || status.FormatContract != 1 || status.Instance == "" || len(status.InputFormats) == 0 || len(status.InputFormats) > 256 {
		return status, errors.New("incompatible service protocol; restart or upgrade the service")
	}
	u, err := url.Parse(status.Origin)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return status, errors.New("invalid service origin")
	}
	for _, reader := range status.InputFormats {
		if len(reader) > 128 || !readerSelectionPattern.MatchString(reader) || readerBase(reader) != reader {
			return status, errors.New("invalid service reader catalogue")
		}
	}
	return status, nil
}

func controlResponse(client *http.Client, request *http.Request) ([]byte, int, error) {
	resp, err := client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil {
		return nil, resp.StatusCode, errors.Join(readErr, closeErr)
	}
	if len(data) > 256*1024 || resp.Header.Get("Content-Type") != "application/json" {
		return nil, resp.StatusCode, errors.New("invalid control representation")
	}
	return data, resp.StatusCode, nil
}

func prepareHTTP(ctx context.Context, sources []sourceContext, cfg config, connection *serviceConnection, log *console) (httpPreviews, []sourceContext, error) {
	prepared := httpPreviews{urls: make(map[string]string)}
	if connection == nil {
		return prepared, sources, nil
	}
	client, status := connection.client, connection.status
	controlCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request := previewRegistration{FormatContract: 1, From: cfg.from, Settings: wireSettings(cfg)}
	for _, src := range sources {
		request.Paths = append(request.Paths, src.logical)
	}
	data, err := json.Marshal(annotationRegistration{previewRegistration: request, DisplayName: &cfg.displayName})
	if err != nil || len(data) > 65536 {
		return prepared, nil, errors.New("service registration exceeds request limit")
	}
	req, err := http.NewRequestWithContext(controlCtx, http.MethodPost, "http://control/v1/annotation-previews", bytes.NewReader(data))
	if err != nil {
		return prepared, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	data, code, err := controlResponse(client, req)
	if err == nil && code == 404 {
		log.notice("annotations unavailable in this service; using read-only preview")
		data, err = json.Marshal(request)
		if err != nil {
			return prepared, nil, err
		}
		req, err = http.NewRequestWithContext(controlCtx, http.MethodPost, "http://control/v1/previews", bytes.NewReader(data))
		if err != nil {
			return prepared, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		data, code, err = controlResponse(client, req)
	}
	if err != nil {
		return prepared, nil, errors.New("service registration failed")
	}
	var response registrationResponse
	if code == 400 {
		var failure struct {
			Error string `json:"error"`
		}
		if decodeControl(data, &failure) == nil && failure.Error == "invalid_reader" {
			return prepared, nil, &readerError{errors.New("invalid or unavailable --from reader; use --list-input-formats")}
		}
	}
	if code != 200 || decodeControl(data, &response) != nil || response.Protocol != 1 || len(response.Results) != len(sources) {
		return prepared, nil, errors.New("invalid service registration response")
	}
	return preflightHTTP(ctx, sources, cfg, status, response, prepared, log)
}

func wireSettings(cfg config) previewSettings {
	source, output, total := cfg.sourceBytes, min(cfg.outputBytes, 50*1024*1024), cfg.totalBytes
	var folding *foldingOverride
	if cfg.folding != (foldingOverride{}) {
		folding = &cfg.folding
	}
	return previewSettings{Folding: folding, TOC: &cfg.toc, TOCDepth: &cfg.tocDepth, SourceBytes: &source, TotalBytes: &total, OutputBytes: &output, Deadline: min(cfg.deadline, time.Minute).String(), Root: cfg.root}
}

func preflightHTTP(ctx context.Context, sources []sourceContext, cfg config, status serviceStatus, response registrationResponse, prepared httpPreviews, log *console) (httpPreviews, []sourceContext, error) {
	var fallback []sourceContext
	serviceGone := false
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 70 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	for i, result := range response.Results {
		src := sources[i]
		if result.Path != src.logical || (result.Error != "" && result.URL != "") {
			return prepared, nil, errors.New("service registration identity mismatch")
		}
		if result.Error == "outside_root" {
			fallback = append(fallback, src)
			continue
		}
		if result.Error != "" {
			log.warn("source %q: service refused input", src.logical)
			prepared.invalid = true
			continue
		}
		if !strings.HasPrefix(result.URL, status.Origin+"/") {
			return prepared, nil, errors.New("service returned invalid preview origin")
		}
		if serviceGone {
			fallback = append(fallback, src)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, result.URL, nil)
		if err != nil {
			return prepared, nil, errors.New("invalid preview URL")
		}
		resp, err := client.Do(req)
		if err != nil {
			if !publicServiceAbsent(ctx, client, status) {
				return prepared, nil, errors.New("service entry preflight failed")
			}
			// Reprepare successful entries too: none of this batch has opened yet.
			// Earlier conversion failures stay failures, never successful fallback.
			for _, earlier := range sources[:i] {
				if prepared.urls[earlier.key] != "" {
					fallback = append(fallback, earlier)
				}
			}
			clear(prepared.urls)
			prepared.sources, prepared.outputs = 0, 0
			fallback = append(fallback, src)
			serviceGone = true
			log.warn("service became unavailable before handoff; using file preview")
			continue
		}
		closeErr := resp.Body.Close()
		if closeErr != nil {
			return prepared, nil, errors.New("service entry response failed")
		}
		if resp.StatusCode != 200 {
			log.warn("source %q: service conversion status %d", src.logical, resp.StatusCode)
			prepared.invalid = true
			continue
		}
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") || resp.ContentLength < 0 {
			return prepared, nil, errors.New("invalid service preview representation")
		}
		input, err := snapshot(src, min(cfg.sourceBytes, cfg.totalBytes-prepared.sources))
		if err != nil {
			log.warn("source %q: source budget or snapshot failed", src.logical)
			prepared.invalid = true
			continue
		}
		if resp.ContentLength > cfg.outputBytes-prepared.outputs {
			return prepared, nil, errors.New("service batch output budget exceeded")
		}
		prepared.sources += int64(len(input))
		prepared.outputs += resp.ContentLength
		if resp.Header.Get("X-HTMLPreview-Omitted-HTML") == "1" {
			log.notice("%q: %s", src.logical, markdownHTMLWarning)
		}
		prepared.urls[src.key] = result.URL
	}
	return prepared, fallback, nil
}

// One bounded, token-free request distinguishes a disappeared endpoint from a
// failed document request. A responding but malformed/foreign service is not
// absence; callers must report that failure rather than change transports.
func publicServiceAbsent(ctx context.Context, client *http.Client, status serviceStatus) bool {
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probe, http.MethodGet, status.Origin+"/_health", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return ctx.Err() == nil && unavailableService(err)
	}
	// Any HTTP response establishes reachability. Its contents cannot authorize
	// fallback, even if the instance changed or the response reports an error.
	if err := resp.Body.Close(); err != nil {
		return false
	}
	return false
}

func openHTTP(ctx context.Context, sources []sourceContext, prepared httpPreviews, desktop *desktop, log *console) int {
	code := 0
	if prepared.invalid {
		code = 1
	}
	for _, src := range sources {
		if target := prepared.urls[src.key]; target != "" {
			log.print("%s", target)
			if err := desktop.open(ctx, target); err != nil {
				log.warn("browser handoff for %q failed", src.logical)
				code = 1
			}
		}
	}
	if len(prepared.urls) == 0 {
		return 1
	}
	return log.status(code)
}
