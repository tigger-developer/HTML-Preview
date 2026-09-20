// ABOUTME: Exercises configured annotation limits through YAML and service HTTP.
// ABOUTME: Checks Unicode boundaries, native writes, sidecars and retained existing notes.
package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tigger-developer/HTML-Preview/internal/annotation"
)

func TestRT013_3_AnnotationConfiguration(t *testing.T) {
	for _, value := range []string{"1", "4000", "5000", "16384", "0", "-1", "16385", "2.5", `"4000"`, "null", "true", "[4000]"} {
		t.Run(value, func(t *testing.T) {
			path := source(t, t.TempDir(), "config.yaml", "version: 1\nannotations:\n  max-chars: "+value+"\n")
			cfg, err := serviceSettings(config{configPath: path, runtimePath: t.TempDir()})
			valid := value == "1" || value == "4000" || value == "5000" || value == "16384"
			if (err == nil) != valid {
				t.Fatalf("max-chars=%s valid=%v error=%v", value, valid, err)
			}
			if valid && fmt.Sprint(cfg.maxChars) != value {
				t.Fatalf("selected limit=%d, want %s", cfg.maxChars, value)
			}
		})
	}
	for _, body := range []string{"max-chars: 10\n  max-chars: 20", "max_characters: 4000", "unknown: 4000"} {
		path := source(t, t.TempDir(), "config.yaml", "version: 1\nannotations:\n  "+body+"\n")
		if _, err := serviceSettings(config{configPath: path, runtimePath: t.TempDir()}); err == nil {
			t.Fatalf("accepted invalid annotation fields: %s", body)
		}
	}
}

func TestRT013_3_DefaultAnnotationLimits(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "default.org", "A passage.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	_, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if state["max_chars"] != float64(4000) || state["max_bytes"] != float64(16384) {
		t.Fatalf("default limits: chars=%v bytes=%v", state["max_chars"], state["max_bytes"])
	}
}

func TestRT013_3_ConfiguredAnnotationWrites(t *testing.T) {
	s := startConfiguredTestService(t, NativeHost(), io.Discard, "annotations:\n  max-chars: 5000\n")
	for _, format := range []string{"org", "md", "sidecar.md"} {
		t.Run(format, func(t *testing.T) {
			path := source(t, s.root, "configured."+format, "A passage.\n")
			if format == "sidecar.md" {
				if os.Geteuid() == 0 {
					t.Skip("requires permission enforcement")
				}
				if err := os.Chmod(path, 0400); err != nil {
					t.Fatal(err)
				}
			}
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			_, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if state["max_chars"] != float64(5000) || state["max_bytes"] != float64(16384) {
				t.Fatal("service did not advertise configured limits")
			}
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			_, _, body := responseAsset(t, s, "GET", pageURL)
			block := assertClickableAnnotationText(t, documentNode(t, parseHTTPDocument(t, body), "hp-document"), "A passage.")
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "action": "upsert", "label": "reviewer-001", "target": map[string]any{"type": "point", "block_id": block}, "text": strings.Repeat("é", 5000)}
			for _, key := range []string{"revision", "source_revision", "body_revision"} {
				request[key] = state[key]
			}
			if status, result := annotationJSON(t, s, "POST", endpoint, request, headers); status != 201 {
				t.Fatalf("configured create status=%d response=%v", status, result)
			}
			_, state = annotationJSON(t, s, "GET", endpoint, nil, nil)
			note := state["footnotes"].([]any)[0].(map[string]any)
			if note["text"] != request["text"] {
				t.Fatal("created note changed text")
			}
			for _, key := range []string{"revision", "source_revision", "body_revision"} {
				request[key] = state[key]
			}
			request["target"] = map[string]any{"type": "footnote", "exact": note["revision"], "run": note["storage"]}
			request["action"], request["sequence"] = "edit", 2
			for index, text := range []string{strings.Repeat("a", 5001), strings.Repeat("😀", 4097)} {
				request["text"], request["operation_id"] = text, fmt.Sprintf("10000000-0000-4000-8000-%012d", index+2)
				// #nosec G304 -- path is the synthetic source created in this test's temporary root.
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if status, result := annotationJSON(t, s, "POST", endpoint, request, headers); status != 413 {
					t.Fatalf("overflow status=%d response=%v", status, result)
				}
				// #nosec G304 -- reread that same temporary fixture after rejection.
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("rejected write changed source")
				}
				_, afterState := annotationJSON(t, s, "GET", endpoint, nil, nil)
				if afterState["revision"] != state["revision"] {
					t.Fatal("rejected write changed store")
				}
			}
			request["operation_id"], request["text"] = "10000000-0000-4000-8000-000000000009", strings.Repeat("😀", 4096)
			if status, result := annotationJSON(t, s, "POST", endpoint, request, headers); status != 201 {
				t.Fatalf("UTF-8 boundary edit status=%d response=%v", status, result)
			}
			_, state = annotationJSON(t, s, "GET", endpoint, nil, nil)
			if state["footnotes"].([]any)[0].(map[string]any)["text"] != request["text"] {
				t.Fatal("UTF-8 edit changed text")
			}
		})
	}
}

func TestRT013_3_GlobalAnnotationConfiguration(t *testing.T) {
	cwd, home := t.TempDir(), t.TempDir()
	source(t, home, ".config/htmlpreview/config.yaml", "version: 1\nannotations: {max-chars: 5000}\n")
	if cfg, err := discoverServiceConfiguration(cwd, home); err != nil || cfg.maxChars != 5000 {
		t.Fatal("global annotation configuration:", cfg.maxChars, err)
	}
	// A selected local file keeps first-found precedence, including its errors.
	source(t, cwd, "config.yaml", "version: 1\nannotations: {max-chars: 0}\n")
	if _, err := discoverServiceConfiguration(cwd, home); err == nil {
		t.Fatal("invalid local configuration fell through to global")
	}
}

func TestRT013_3_EscapedTextAtMaximum(t *testing.T) {
	s := startConfiguredTestService(t, NativeHost(), io.Discard, "annotations: {max-chars: 16384}\n")
	path := source(t, s.root, "maximum.org", "Body[fn:existing].\n\n[fn:existing] Initial text\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	_, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	note := state["footnotes"].([]any)[0].(map[string]any)
	headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "action": "edit", "label": "existing", "target": map[string]any{"type": "footnote", "exact": note["revision"], "run": note["storage"]}, "text": json.RawMessage(`"` + strings.Repeat(`\u0061`, 16384) + `"`)}
	for _, key := range []string{"revision", "source_revision", "body_revision"} {
		request[key] = state[key]
	}
	if status, result := annotationJSON(t, s, "POST", endpoint, request, headers); status != 201 {
		t.Fatal("maximum text with JSON escapes rejected", status, result)
	}
	_, state = annotationJSON(t, s, "GET", endpoint, nil, nil)
	if state["footnotes"].([]any)[0].(map[string]any)["text"] != strings.Repeat("a", 16384) {
		t.Fatal("maximum text changed during write")
	}
}

func TestRT013_3_LowerLimitKeepsExistingNotes(t *testing.T) {
	s := startConfiguredTestService(t, NativeHost(), io.Discard, "annotations: {max-chars: 20}\n")
	for _, format := range []string{"org", "markdown"} {
		ref, definition := "[fn:existing]", "[fn:existing] "
		if format == "markdown" {
			ref, definition = "[^existing]", "[^existing]: "
		}
		path := source(t, s.root, "existing."+format, "Keep this"+ref+".\n\n"+definition+strings.Repeat("a", 5000)+"\n")
		endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
		_, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
		if state["writable"] != true {
			t.Fatal("lower policy made existing notes unreadable", state["reason"])
		}
		note := state["footnotes"].([]any)[0].(map[string]any)
		if len(note["text"].(string)) != 5000 {
			t.Fatal("existing note was truncated")
		}
		headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
		request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "action": "edit", "label": "existing", "target": map[string]any{"type": "footnote", "exact": note["revision"], "run": note["storage"]}, "text": strings.Repeat("a", 21)}
		for _, key := range []string{"revision", "source_revision", "body_revision"} {
			request[key] = state[key]
		}
		if status, result := annotationJSON(t, s, "POST", endpoint, request, headers); status != 413 {
			t.Fatal("lower write policy not enforced", status, result)
		}
		request["max_chars"] = 5000
		if status, result := annotationJSON(t, s, "POST", endpoint, request, headers); status != 400 {
			t.Fatal("request can forge policy", status, result)
		}
		delete(request, "max_chars")
		request["action"], request["text"] = "delete", ""
		if status, result := annotationJSON(t, s, "POST", endpoint, request, headers); status != 201 {
			t.Fatal("lower policy prevented deletion", status, result)
		}
		snap, err := annotation.Read(annotation.Location{Root: s.root, Path: path, Format: format})
		if err != nil || len(annotation.EditableFootnotes(snap.RawSource, format, "embedded")) != 0 || !strings.Contains(string(snap.RawSource), "Keep this.") {
			t.Fatal("deletion changed unrelated source", err)
		}
	}
}
