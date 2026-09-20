// ABOUTME: Verifies replacement of an existing user LaunchAgent through its manager boundary.
// ABOUTME: Exercises absent jobs, repeated activation and restoration after a failed replacement.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type launchExit int

func (e launchExit) Error() string { return fmt.Sprintf("launchctl exit %d", e) }
func (e launchExit) ExitCode() int { return int(e) }

func TestLaunchAgentReplacement(t *testing.T) {
	t.Chdir("../..")
	home := t.TempDir()
	config := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(config, []byte("version: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plist := filepath.Join(home, "Library/LaunchAgents/org.htmlpreview.agent.plist")
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	service := domain + "/org.htmlpreview.agent"
	loaded := false
	var calls [][]string
	run := func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		switch args[0] {
		case "bootout":
			if !loaded {
				return launchExit(3)
			}
			loaded = false
		case "enable":
		case "bootstrap":
			if loaded {
				return errors.New("job already loaded")
			}
			loaded = true
		default:
			return fmt.Errorf("unexpected launchctl operation %s", args[0])
		}
		return nil
	}
	for _, exe := range []string{"/old checkout/bin/htmlpreview", "/sdlc submodule/bin/htmlpreview", "/sdlc submodule/bin/htmlpreview"} {
		if err := startLaunchAgent(home, exe, config, "/opt/bin/pandoc", run); err != nil {
			t.Fatalf("activate %s: %v", exe, err)
		}
		data, err := os.ReadFile(plist) // #nosec G304 -- plist is created under this test's temporary home.
		if err != nil || !strings.Contains(string(data), exe) || !loaded {
			t.Fatalf("replacement not active: %s %v", data, err)
		}
	}
	if want := [][]string{{"bootout", service}, {"enable", service}, {"bootstrap", domain, plist}}; !reflect.DeepEqual(calls[:3], want) {
		t.Fatalf("calls %v", calls)
	}
	for range 2 {
		if err := stopLaunchAgent(home, run); err != nil {
			t.Fatal(err)
		}
	}
	if loaded {
		t.Fatal("job remained loaded")
	}
	if _, err := os.Stat(plist); err != nil {
		t.Fatal("stop removed plist", err)
	}
}

func TestLaunchAgentReplacementFailure(t *testing.T) {
	for _, failure := range []string{"bootout", "enable", "bootstrap"} {
		t.Run(failure, func(t *testing.T) {
			t.Chdir("../..")
			home := t.TempDir()
			config := filepath.Join(home, "config.yaml")
			if err := os.WriteFile(config, []byte("version: 1\n"), 0600); err != nil {
				t.Fatal(err)
			}
			plist := filepath.Join(home, "Library/LaunchAgents/org.htmlpreview.agent.plist")
			if err := os.MkdirAll(filepath.Dir(plist), 0700); err != nil {
				t.Fatal(err)
			}
			original := []byte("original plist\n")
			if err := os.WriteFile(plist, original, 0600); err != nil {
				t.Fatal(err)
			}
			sentinel := errors.New("manager denied replacement")
			loaded := true
			failed := false
			run := func(args ...string) error {
				if args[0] == failure && !failed {
					failed = true
					return sentinel
				}
				switch args[0] {
				case "bootout":
					loaded = false
				case "bootstrap":
					loaded = true
				case "enable":
				default:
					return errors.New("unexpected operation")
				}
				return nil
			}
			err := startLaunchAgent(home, "/new/bin/htmlpreview", config, "/bin/pandoc", run)
			if !errors.Is(err, sentinel) {
				t.Fatalf("lost failure: %v", err)
			}
			data, readErr := os.ReadFile(plist) // #nosec G304 -- plist is created under this test's temporary home.
			if readErr != nil || string(data) != string(original) || !loaded {
				t.Fatalf("old service not restored: loaded=%v data=%q error=%v", loaded, data, readErr)
			}
		})
	}
}

func TestLaunchAgentStopReportsManagerFailure(t *testing.T) {
	err := stopLaunchAgent(t.TempDir(), func(...string) error { return launchExit(5) })
	if err == nil {
		t.Fatal("unexpected manager failure ignored")
	}
}

func TestLaunchAgentReplacesPlistSymlink(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		t.Run(fmt.Sprint(dangling), func(t *testing.T) {
			t.Chdir("../..")
			home := t.TempDir()
			cfg := filepath.Join(home, "config.yaml")
			if err := os.WriteFile(cfg, []byte("version: 1\n"), 0600); err != nil {
				t.Fatal(err)
			}
			plist, _ := launchAgentPath(home)
			if err := os.MkdirAll(filepath.Dir(plist), 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(home, "previous.plist")
			if !dangling {
				if err := os.WriteFile(target, []byte("original target"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(target, plist); err != nil {
				t.Fatal(err)
			}
			run := func(args ...string) error {
				if args[0] == "bootout" {
					return launchExit(3)
				}
				return nil
			}
			if err := startLaunchAgent(home, "/new/bin/htmlpreview", cfg, "/bin/pandoc", run); err != nil {
				t.Fatal(err)
			}
			st, err := os.Lstat(plist)
			if err != nil || !st.Mode().IsRegular() {
				t.Fatalf("plist not replaced: %v", err)
			}
			if dangling {
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("dangling target changed: %v", err)
				}
			} else {
				b, err := os.ReadFile(target) // #nosec G304 -- target is this test's temporary sentinel.
				if err != nil || string(b) != "original target" {
					t.Fatalf("symlink target changed: %v", err)
				}
			}
		})
	}
}
