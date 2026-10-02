// ABOUTME: Verifies Linux service activation and stopping through systemd user units.
// ABOUTME: Exercises generated unit data, ordered manager calls and explicit failures.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRT017_2_SystemdServiceLifecycle(t *testing.T) {
	t.Chdir("../..")
	home := t.TempDir()
	config := filepath.Join(home, ".config/htmlpreview/config.yaml")
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("version: 1\nserve: {roots: []}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(home, "checkout/bin/htmlpreview")
	var calls [][]string
	run := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}

	if err := systemdService(false, home, executable, config, "/opt/pandoc/bin/pandoc", run); err != nil {
		t.Fatal(err)
	}
	unit := filepath.Join(home, ".config/systemd/user/htmlpreview.service")
	data, err := os.ReadFile(unit) // #nosec G304 -- unit is generated beneath this test's temporary home.
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{executable, "HTMLPREVIEW_CONFIG=" + config, "/opt/pandoc/bin"} {
		if !strings.Contains(string(data), value) {
			t.Fatalf("unit missing literal %q", value)
		}
	}
	wantStart := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "htmlpreview.service"},
		{"systemctl", "--user", "restart", "htmlpreview.service"},
	}
	if !reflect.DeepEqual(calls, wantStart) {
		t.Fatalf("start calls = %v, want %v", calls, wantStart)
	}

	calls = nil
	if err := systemdService(true, home, executable, config, "", run); err != nil {
		t.Fatal(err)
	}
	wantStop := [][]string{{"systemctl", "--user", "stop", "htmlpreview.service"}}
	if !reflect.DeepEqual(calls, wantStop) {
		t.Fatalf("stop calls = %v, want %v", calls, wantStop)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("stop removed unit: %v", err)
	}
}

func TestSystemdServiceRejectsInvalidInputsBeforeManagerCalls(t *testing.T) {
	t.Chdir("../..")
	home := t.TempDir()
	calls := 0
	err := systemdService(false, home, "/bin/htmlpreview", filepath.Join(home, "missing.yaml"), "", func(string, ...string) error {
		calls++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "config") || calls != 0 {
		t.Fatalf("invalid config = %v after %d calls", err, calls)
	}
}

func TestSystemdServiceReportsManagerFailures(t *testing.T) {
	t.Chdir("../..")
	home := t.TempDir()
	config := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(config, []byte("version: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("systemd unavailable")
	err := systemdService(false, home, "/bin/htmlpreview", config, "", func(string, ...string) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("manager failure lost: %v", err)
	}
}

func TestManagedServiceRoutesByPlatform(t *testing.T) {
	for _, tc := range []struct {
		goos, want string
	}{
		{goos: "darwin", want: "darwin"},
		{goos: "linux", want: "linux"},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			called := ""
			darwin := func(stop bool) error {
				called = "darwin"
				if !stop {
					t.Fatal("stop flag lost")
				}
				return nil
			}
			linux := func(stop bool) error {
				called = "linux"
				if !stop {
					t.Fatal("stop flag lost")
				}
				return nil
			}
			if err := managedServiceFor(tc.goos, true, darwin, linux); err != nil {
				t.Fatal(err)
			}
			if called != tc.want {
				t.Fatalf("route = %q, want %q", called, tc.want)
			}
		})
	}
	if err := managedServiceFor("windows", false, func(bool) error { return nil }, func(bool) error { return nil }); err == nil {
		t.Fatal("unsupported platform accepted")
	}
}

func TestSystemdServiceReplacesUnitSymlinkWithoutChangingTarget(t *testing.T) {
	t.Chdir("../..")
	home := t.TempDir()
	config := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(config, []byte("version: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	unit := filepath.Join(home, ".config/systemd/user/htmlpreview.service")
	if err := os.MkdirAll(filepath.Dir(unit), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "previous.service")
	if err := os.WriteFile(target, []byte("previous unit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, unit); err != nil {
		t.Fatal(err)
	}
	if err := systemdService(false, home, "/new/bin/htmlpreview", config, "", func(string, ...string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(unit)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("unit symlink not replaced: %v", err)
	}
	data, err := os.ReadFile(target) // #nosec G304 -- target is this test's temporary sentinel.
	if err != nil || string(data) != "previous unit\n" {
		t.Fatalf("symlink target changed: %q %v", data, err)
	}
}
