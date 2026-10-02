// ABOUTME: Verifies service restart uses the native user service manager in order.
// ABOUTME: Prevents Make prerequisite scheduling from racing stop and start operations.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestRT017_1_ServiceRestartUsesNativeManager(t *testing.T) {
	manager := "systemctl"
	want := "--user stop htmlpreview.service\n--user start htmlpreview.service\n"
	if runtime.GOOS == "darwin" {
		manager = "launchctl"
		service := launchService()
		want = "stop " + service + "\nstart " + service + "\n"
	}

	dir := t.TempDir()
	capture := filepath.Join(dir, "manager.calls")
	script := filepath.Join(dir, manager)
	body := "#!/usr/bin/env bash\nset -eo pipefail\nprintf '%s\\n' \"$*\" >> \"$HTMLPREVIEW_MANAGER_CAPTURE\"\n"
	// #nosec G306 -- This test-owned manager fixture must be executable and is private to t.TempDir.
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTMLPREVIEW_MANAGER_CAPTURE", capture)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := task([]string{"service-restart"}); err != nil {
		t.Fatalf("service-restart: %v", err)
	}
	data, err := os.ReadFile(capture) // #nosec G304 -- capture is created beneath this test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.ReplaceAll(string(data), "\r\n", "\n"); got != want {
		t.Fatalf("manager calls = %q, want ordered stop/start %q", got, want)
	}
}

func TestServiceRestartManagerCommands(t *testing.T) {
	for _, tc := range []struct {
		name, goos string
		want       [][]string
	}{
		{
			name: "launchd",
			goos: "darwin",
			want: [][]string{
				{"launchctl", "stop", launchService()},
				{"launchctl", "start", launchService()},
			},
		},
		{
			name: "systemd",
			goos: "linux",
			want: [][]string{
				{"systemctl", "--user", "stop", "htmlpreview.service"},
				{"systemctl", "--user", "start", "htmlpreview.service"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			run := func(name string, args ...string) error {
				calls = append(calls, append([]string{name}, args...))
				return nil
			}
			if err := restartService(tc.goos, run); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, tc.want) {
				t.Fatalf("manager calls = %v, want %v", calls, tc.want)
			}
		})
	}
}

func TestServiceRestartReportsManagerFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failCall int
	}{
		{name: "stop", failCall: 1},
		{name: "start", failCall: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := errors.New("manager failure")
			calls := 0
			err := restartService("linux", func(string, ...string) error {
				calls++
				if calls == tc.failCall {
					return sentinel
				}
				return nil
			})
			if !errors.Is(err, sentinel) || calls != tc.failCall || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("%s failure = %v after %d calls", tc.name, err, calls)
			}
		})
	}
}

func TestServiceRestartRejectsUnsupportedPlatform(t *testing.T) {
	if err := restartService("windows", func(string, ...string) error { return nil }); err == nil {
		t.Fatal("unsupported platform accepted")
	}
}
