// ABOUTME: Verifies LaunchAgent activation with temporary paths and a captured manager boundary.
// ABOUTME: Ensures XML values remain literal and stop leaves the plist available for restart.
package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLaunchAgentLifecycle(t *testing.T) {
	t.Chdir("../..")
	home := t.TempDir()
	cfg := filepath.Join(home, "config & spaces.yaml")
	if err := os.WriteFile(cfg, []byte("version: 1\nserve: {roots: []}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(home, "bin & tools/htmlpreview")
	plist := filepath.Join(home, "Library/LaunchAgents/org.htmlpreview.agent.plist")
	var calls [][]string
	run := func(args ...string) error { calls = append(calls, args); return nil }
	if err := startLaunchAgent(home, exe, cfg, "/opt/pandoc/bin/pandoc", run); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(plist)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"bin &amp; tools/htmlpreview", "config &amp; spaces.yaml", "/opt/pandoc/bin", "<string>--serve</string>"} {
		if !bytes.Contains(data, []byte(value)) {
			t.Fatalf("plist missing literal %q", value)
		}
	}
	if err := stopLaunchAgent(home, run); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, [][]string{{"load", plist}, {"unload", plist}}) {
		t.Fatalf("manager calls: %v", calls)
	}
	after, err := os.ReadFile(plist)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatal("stop changed plist")
	}
	sentinel := errors.New("manager unavailable")
	if err := startLaunchAgent(home, exe, cfg, "/opt/pandoc/bin/pandoc", func(...string) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("lost manager failure: %v", err)
	}
	calls = nil
	if err := startLaunchAgent(home, exe, filepath.Join(home, "missing.yaml"), "/opt/pandoc/bin/pandoc", run); err == nil || !strings.Contains(err.Error(), "config") || len(calls) != 0 {
		t.Fatalf("missing config activation: %v %v", err, calls)
	}
}
