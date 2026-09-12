// ABOUTME: Verifies WSL HTTP selection through the existing desktop process boundary.
// ABOUTME: Simulates the bridge result only; native Windows compatibility remains a user qualification.
package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRT006_1_WSLHealthControlsHTTPFallback(t *testing.T) {
	s := startTestService(t, NativeHost())
	status, err := serviceDiscovery(t.Context(), s.control)
	if err != nil {
		t.Fatal(err)
	}
	file := source(t, s.root, "entry.org", "* Windows reading")
	for _, outcome := range []string{"matching", "wrong-instance", "unavailable", "malformed"} {
		t.Run(outcome, func(t *testing.T) {
			host := NativeHost()
			host.OS = "linux"
			host.Kernel = func() (string, error) { return "microsoft", nil }
			look, actual := host.LookPath, host.Execute
			host.LookPath = func(name string) (string, error) {
				if name == "powershell.exe" || name == "wslpath" {
					return "/tools/" + name, nil
				}
				return look(name)
			}
			probes := 0
			var opened string
			host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
				switch filepath.Base(cmd.Path) {
				case "wslpath":
					if cmd.Args[0] == "-w" {
						return []byte(`\\wsl.localhost\Ubuntu` + strings.ReplaceAll(cmd.Args[1], "/", `\`)), nil
					}
					return []byte(strings.ReplaceAll(strings.TrimPrefix(cmd.Args[1], `\\wsl.localhost\Ubuntu`), `\`, "/")), nil
				case "powershell.exe":
					if string(cmd.Input) == s.origin+"/_health" {
						probes++
						deadline, ok := ctx.Deadline()
						if !ok || time.Until(deadline) > 5*time.Second {
							return nil, errors.New("unbounded health probe")
						}
						switch outcome {
						case "unavailable":
							return nil, context.DeadlineExceeded
						case "malformed":
							return []byte("not JSON"), nil
						case "wrong-instance":
							return []byte(`{"protocol":1,"instance":"other","ready":true}`), nil
						}
						return json.Marshal(map[string]any{"protocol": 1, "instance": status.Instance, "ready": true})
					}
					opened = string(cmd.Input)
					return nil, nil
				}
				return actual(ctx, cmd)
			}
			cfg, err := settings([]string{"HTMLPREVIEW_GRACE=100ms"})
			if err != nil {
				t.Fatal(err)
			}
			cfg.runtimePath = s.runtime
			var diagnostics bytes.Buffer
			code := execute(t.Context(), []string{file}, []string{"WSL_DISTRO_NAME=Ubuntu"}, cfg, host, &console{out: io.Discard, diagnostics: &diagnostics})
			prefix := "file://wsl.localhost/Ubuntu/"
			if outcome == "matching" {
				prefix = s.origin + "/"
			}
			if code != 0 || probes != 1 || !strings.HasPrefix(opened, prefix) {
				t.Fatalf("WSL selection: code=%d probes=%d expected=%s diagnostics=%s", code, probes, prefix, diagnostics.String())
			}
		})
	}
}
