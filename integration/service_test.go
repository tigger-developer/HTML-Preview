// ABOUTME: Exercises the optional service through its executable and private/public HTTP interfaces.
// ABOUTME: Uses synthetic roots and temporary state without activating an installed service manager.
package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type serviceFixture struct {
	binary, root, runtime, config, origin string
	control, public                       *http.Client
	env                                   []string
}

func serviceBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "htmlpreview")
	// #nosec G204 -- Builds only this project's executable in test-owned storage.
	cmd := exec.Command("go", "build", "-tags=htmlpreview_test_desktop", "-o", binary, "../cmd/htmlpreview")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build service candidate: %v\n%s", err, output)
	}
	return binary
}

func startService(t *testing.T, binary string) *serviceFixture {
	t.Helper()
	// Keep the socket path below the native Unix-domain address limit.
	runtime, err := os.MkdirTemp("", "hp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(runtime); err != nil {
			t.Error(err)
		}
	})
	s := &serviceFixture{binary: binary, root: t.TempDir(), runtime: runtime}
	s.config = filepath.Join(t.TempDir(), "config.yaml")
	config := "version: 1\nserve:\n  roots: [" + strconv.Quote(s.root) + "]\n"
	if err := os.WriteFile(s.config, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	s.env = append(os.Environ(), "HTMLPREVIEW_CONFIG="+s.config, "HTMLPREVIEW_RUNTIME_DIR="+runtime)
	// #nosec G204 -- Starts the test-built executable with synthetic config and roots.
	cmd := exec.Command(binary, "--serve")
	cmd.Env = s.env
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		select {
		case <-done:
			return
		default:
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Error(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("service shutdown: %v\n%s", err, output.String())
			}
		case <-time.After(7 * time.Second):
			if err := cmd.Process.Kill(); err != nil {
				t.Error(err)
			}
			<-done
			t.Error("service exceeded shutdown deadline")
		}
	})
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(runtime, "control.sock"))
	}}
	s.control = &http.Client{Transport: transport, Timeout: 5 * time.Second}
	s.public = &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	t.Cleanup(transport.CloseIdleConnections)
	t.Cleanup(s.public.CloseIdleConnections)
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			done <- err
			t.Fatalf("service exited before readiness: %v\n%s", err, output.String())
		case <-deadline.C:
			t.Fatal("service readiness deadline exceeded")
		case <-tick.C:
			resp, err := s.control.Get("http://control/v1/status")
			if err != nil {
				continue
			}
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
			closeErr := resp.Body.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("read status: %v %v", readErr, closeErr)
			}
			var status struct {
				Protocol         int
				Instance, Origin string
				Ready            bool
				FormatContract   int      `json:"format_contract"`
				InputFormats     []string `json:"input_formats"`
			}
			if resp.StatusCode != 200 || json.Unmarshal(data, &status) != nil || status.Protocol != 1 || status.FormatContract != 1 || !status.Ready || status.Instance == "" || len(status.InputFormats) == 0 {
				t.Fatalf("invalid service status: status=%d body=%s", resp.StatusCode, data)
			}
			s.origin = status.Origin
			return s
		}
	}
}

func (s *serviceFixture) register(t *testing.T, path string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"paths": []string{path}, "settings": map[string]any{}, "format_contract": 1})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.control.Post("http://control/v1/previews", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("registration read: %v %v", readErr, closeErr)
	}
	var result struct {
		Protocol int
		Results  []struct{ Path, URL, Error string }
	}
	if resp.StatusCode != 200 || json.Unmarshal(data, &result) != nil || result.Protocol != 1 || len(result.Results) != 1 || result.Results[0].Path != path || result.Results[0].Error != "" || result.Results[0].URL == "" {
		t.Fatalf("registration failed: status=%d body=%s", resp.StatusCode, data)
	}
	return result.Results[0].URL
}

func serviceResponse(t *testing.T, client *http.Client, method, target string, headers map[string]string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		if key == "Host" {
			req.Host = value
		} else {
			req.Header.Set(key, value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024+1))
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("HTTP response: %v %v", readErr, closeErr)
	}
	return resp.StatusCode, resp.Header, data
}

func TestRT006_3_LoopbackAndCapabilityBoundary(t *testing.T) {
	s := startService(t, serviceBinary(t))
	u, err := url.Parse(s.origin)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" {
		t.Fatal("service must advertise literal IPv4 loopback HTTP")
	}
	file := filepath.Join(s.root, "secret.md")
	if err := os.WriteFile(file, []byte("# Authorized fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	target := s.register(t, file)
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")[0]
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(token) != 43 || len(decoded) != 32 {
		t.Fatal("capability does not carry 32 random bytes in unpadded base64url")
	}
	for _, tc := range []struct {
		name, method, target string
		headers              map[string]string
		status               int
	}{
		{"health", "GET", s.origin + "/_health", nil, 200},
		{"no token", "GET", s.origin + "/secret.md", nil, 404},
		{"unknown token", "GET", s.origin + "/" + strings.Repeat("a", 43) + "/secret.md", nil, 404},
		{"foreign host", "GET", target, map[string]string{"Host": "evil.example"}, 403},
		{"foreign origin", "GET", target, map[string]string{"Origin": "https://evil.example"}, 403},
		{"null origin", "GET", target, map[string]string{"Origin": "null"}, 403},
		{"cross site", "GET", target, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"public grant", "POST", s.origin + "/v1/previews", nil, 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, headers, body := serviceResponse(t, s.public, tc.method, tc.target, tc.headers)
			if status != tc.status || bytes.Contains(body, []byte("Authorized fixture")) || bytes.Contains(body, []byte(token)) {
				t.Fatalf("boundary status=%d expected=%d or source/credential disclosed", status, tc.status)
			}
			if headers.Get("Cache-Control") != "no-store" || headers.Get("Referrer-Policy") != "no-referrer" || headers.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(headers.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
				t.Fatal("missing response privacy headers")
			}
		})
	}
}

func TestRT006_1_AutomaticHTTPSelection(t *testing.T) {
	s := startService(t, serviceBinary(t))
	file := filepath.Join(s.root, "notes.org")
	if err := os.WriteFile(file, []byte("* Served notes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- Invokes this test's executable and source through the real CLI.
	cmd := exec.Command(s.binary, file)
	cmd.Env = append(s.env, "PREVIEW_TEST_CAPTURE="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("HTTP CLI handoff: %v\n%s", err, stderr.String())
	}
	target := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(target, s.origin+"/") || !strings.HasSuffix(target, "/notes.org") {
		t.Fatal("eligible CLI input did not select its service URL")
	}
	status, _, body := serviceResponse(t, s.public, "GET", target, nil)
	if status != 200 || !bytes.Contains(body, []byte("Served notes")) || !bytes.Contains(body, []byte(file)) {
		t.Fatal("HTTP output lost source identity or document content")
	}
}

func TestRT006_11_HTTPDoesNotRequireClientPandoc(t *testing.T) {
	s := startService(t, serviceBinary(t))
	for _, tc := range []struct {
		name, body, from string
		code             int
	}{
		{"notes.org", "* Service reader", "", 0},
		{"selected.data", "# Selected reader", "markdown+smart", 0},
		{"bad.data", "text", "markdown+not_a_real_extension", 2},
		{"unknown.data", "text", "not_a_reader", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(s.root, tc.name)
			if err := os.WriteFile(file, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{file}
			if tc.from != "" {
				args = append([]string{"--from=" + tc.from}, args...)
			}
			// #nosec G204 -- Test-built CLI and synthetic input; an empty PATH isolates client dependencies.
			cmd := exec.Command(s.binary, args...)
			bin := t.TempDir()
			// The test desktop adapter captures this executable; it must never run.
			// #nosec G306 -- Owner-only execute permission is required for LookPath in the isolated desktop fixture.
			if err := os.WriteFile(filepath.Join(bin, "xdg-open"), []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
				t.Fatal(err)
			}
			cmd.Env = append(s.env, "PATH="+bin, "PREVIEW_TEST_CAPTURE="+t.TempDir())
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			code := 0
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					code = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if code != tc.code {
				t.Fatalf("service-owned reader: status=%d expected=%d diagnostic=%s", code, tc.code, stderr.String())
			}
			if tc.code == 0 && !strings.HasPrefix(stdout.String(), s.origin+"/") {
				t.Fatal("client did not open HTTP without local Pandoc")
			}
			if tc.code != 0 && stdout.Len() != 0 {
				t.Fatal("invalid reader opened a browser")
			}
		})
	}
}

func TestRT006_1_MixedHTTPAndFileFallback(t *testing.T) {
	s := startService(t, serviceBinary(t))
	inside := filepath.Join(s.root, "inside.md")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	for path, body := range map[string]string{inside: "# Inside service", outside: "Outside literal"} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	capture := t.TempDir()
	// #nosec G204 -- Project executable and synthetic paths only.
	cmd := exec.Command(s.binary, inside, outside)
	cmd.Env = append(s.env, "PREVIEW_TEST_CAPTURE="+capture, "HTMLPREVIEW_GRACE=100ms")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("mixed preview: %v %s", err, stderr.String())
	}
	urls := strings.Fields(stdout.String())
	if len(urls) != 2 || !strings.HasPrefix(urls[0], s.origin+"/") || !strings.HasPrefix(urls[1], "file://") {
		t.Fatalf("mixed handoff lost order or transport: %d URLs", len(urls))
	}
	entries, err := os.ReadDir(capture)
	if err != nil || len(entries) != 2 {
		t.Fatal("both entry previews were not ready at handoff")
	}
}

func TestRT006_2_OutsideRootPrecedesServiceFormat(t *testing.T) {
	s := startService(t, serviceBinary(t))
	path := filepath.Join(t.TempDir(), "file.unknown")
	if err := os.WriteFile(path, []byte("unknown input"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"paths": []string{path}, "settings": map[string]any{}, "format_contract": 1})
	if err != nil {
		t.Fatal(err)
	}
	response, err := s.control.Post("http://control/v1/previews", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 65536))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatal("control read failed")
	}
	var result struct{ Results []struct{ Error string } }
	if response.StatusCode != 200 || json.Unmarshal(data, &result) != nil || len(result.Results) != 1 || result.Results[0].Error != "outside_root" {
		t.Fatalf("outside-root fallback suppressed by service reader: %s", data)
	}
}
