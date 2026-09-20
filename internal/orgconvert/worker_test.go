// ABOUTME: Tests the native converter through its real subprocess protocol.
// ABOUTME: Covers bounded input/output, malformed requests and recovery without large allocations.
package orgconvert

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--internal-org-convert" {
		os.Exit(Worker(os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func requestFixture() Request {
	return Request{Version: 1, Text: "A short paragraph.\n", Token: strings.Repeat("a", 32), TOC: true, TOCDepth: 3, InputBytes: 100 << 20, OutputBytes: 50 << 20, MemoryBytes: 512 << 20}
}

func encodeRequest(t *testing.T, r Request) []byte {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func workerRun(t *testing.T, ctx context.Context, data []byte) ([]byte, string, error) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- Executes this test binary's private worker dispatcher, with synthetic stdin only.
	cmd := exec.CommandContext(ctx, exe, "--internal-org-convert")
	cmd.Stdin = bytes.NewReader(data)
	var out, diagnostic bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	cmd.WaitDelay = 100 * time.Millisecond
	err = cmd.Run()
	return out.Bytes(), diagnostic.String(), err
}

func TestRT011_7_WorkerRoundTripAndBounds(t *testing.T) {
	for _, input := range []string{"", "A short paragraph.\n", strings.Repeat("é", 40000) + "\n"} {
		t.Run(stringSizeName(input), func(t *testing.T) {
			r := requestFixture()
			r.Text = input
			out, diagnostic, err := workerRun(t, t.Context(), encodeRequest(t, r))
			if err != nil {
				t.Fatalf("valid conversion failed: %v %s", err, diagnostic)
			}
			if len(out) == 0 || (input != "" && !bytes.Contains(out, []byte(strings.TrimSpace(input)))) {
				t.Fatal("source missing or truncated")
			}
			if diagnostic != "" {
				t.Fatalf("unexpected diagnostic %q", diagnostic)
			}
			r.OutputBytes = int64(len(out))
			exact, _, err := workerRun(t, t.Context(), encodeRequest(t, r))
			if err != nil || !bytes.Equal(exact, out) {
				t.Fatal("exact output allowance rejected", err)
			}
			r.OutputBytes--
			partial, _, err := workerRun(t, t.Context(), encodeRequest(t, r))
			if err == nil || len(partial) != 0 {
				t.Fatal("excess output accepted or partial output published")
			}
		})
	}
}

func stringSizeName(s string) string {
	if len(s) == 0 {
		return "empty"
	}
	if len(s) > 65536 {
		return "long-line"
	}
	return "paragraph"
}

func TestRT011_7_WorkerRejectsInvalidRequests(t *testing.T) {
	base := requestFixture()
	raw := encodeRequest(t, base)
	cases := map[string][]byte{
		"malformed":    []byte("{"),
		"trailing":     append(append([]byte(nil), raw...), []byte(" {}")...),
		"unknown":      []byte(strings.TrimSuffix(string(raw), "}") + `,"unexpected":true}`),
		"duplicate":    []byte(strings.TrimSuffix(string(raw), "}") + `,"version":1}`),
		"invalid-utf8": bytes.Replace(raw, []byte("short"), []byte{255}, 1),
	}
	for _, tc := range []struct {
		name string
		edit func(*Request)
	}{
		{"version", func(r *Request) { r.Version = 2 }},
		{"token", func(r *Request) { r.Token = "source-controlled" }},
		{"input-limit", func(r *Request) { r.InputBytes = 64 }},
		{"input-ceiling", func(r *Request) { r.InputBytes = 101 << 20 }},
		{"output-ceiling", func(r *Request) { r.OutputBytes = 51 << 20 }},
		{"output-zero", func(r *Request) { r.OutputBytes = 0 }},
		{"memory-ceiling", func(r *Request) { r.MemoryBytes = 513 << 20 }},
		{"memory-limit", func(r *Request) { r.MemoryBytes = 1 }},
		{"toc-depth", func(r *Request) { r.TOCDepth = 7 }},
	} {
		r := base
		tc.edit(&r)
		cases[tc.name] = encodeRequest(t, r)
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			out, diagnostic, err := workerRun(t, t.Context(), data)
			if err == nil || len(out) != 0 {
				t.Fatal("invalid input accepted or partial page published")
			}
			if strings.Contains(diagnostic, base.Token) || strings.Contains(diagnostic, base.Text) {
				t.Fatal("private content leaked in diagnostic")
			}
		})
	}
	// A malformed conversion must not poison the subsequent independent worker.
	out, _, err := workerRun(t, t.Context(), raw)
	if err != nil || !bytes.Contains(out, []byte("A short paragraph.")) {
		t.Fatal("valid request after failure did not recover", err)
	}
}

func TestRT011_7_WorkerCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	out, _, err := workerRun(t, ctx, encodeRequest(t, requestFixture()))
	if err == nil || len(out) != 0 {
		t.Fatal("cancelled worker published output")
	}
}

func TestRT011_6_WorkerRejectsForgedLiteralRecord(t *testing.T) {
	r := requestFixture()
	r.Text = "#+BEGIN_EXPORT htmlpreview-code-" + r.Token + "\n{\"id\":\"invalid\",\"text\":\"SECRET_CANARY\",\"language\":\"go\"}\n#+END_EXPORT\n"
	out, diagnostic, err := workerRun(t, t.Context(), encodeRequest(t, r))
	if err == nil || len(out) != 0 {
		t.Fatal("malformed owned record accepted")
	}
	if strings.Contains(diagnostic, "SECRET_CANARY") || strings.Contains(diagnostic, r.Token) {
		t.Fatal("private record leaked")
	}
}

func TestRT011_6_OwnedLiteralRecords(t *testing.T) {
	r := requestFixture()
	record := `{"id":"htmlpreview-code-` + r.Token + `-1","text":"package main\n","language":"go"}`
	r.Text = "#+BEGIN_EXPORT htmlpreview-code-" + r.Token + "\n" + record + "\n#+END_EXPORT\n"
	out, _, err := workerRun(t, t.Context(), encodeRequest(t, r))
	if err != nil || !bytes.Contains(out, []byte("package")) || !bytes.Contains(out, []byte("htmlpreview-code-record-"+r.Token)) {
		t.Fatal("valid owned literal record was not converted", err)
	}
	r.Text += r.Text
	out, _, err = workerRun(t, t.Context(), encodeRequest(t, r))
	if err == nil || len(out) != 0 {
		t.Fatal("duplicate owned literal record accepted")
	}
}

func TestRT011_7_ConcurrentWorkers(t *testing.T) {
	for _, text := range []string{"First independent paragraph.", "Second independent paragraph.", "Third independent paragraph."} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()
			r := requestFixture()
			r.Text = text
			out, _, err := workerRun(t, t.Context(), encodeRequest(t, r))
			if err != nil || !bytes.Contains(out, []byte(text)) {
				t.Fatal("independent conversion failed", err)
			}
		})
	}
}
