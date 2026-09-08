// ABOUTME: Verifies native desktop selection and the WSL data/code boundary.
// ABOUTME: Platform doubles test policy, not Windows or browser compatibility.
package preview

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestRT001_17_DesktopSelection(t *testing.T) {
	for _, tc := range []struct {
		os, kernel, distro, opener string
		wsl                        bool
	}{
		{"darwin", "", "", "/usr/bin/open", false},
		{"linux", "6.8.0", "", "/tools/xdg-open", false},
		{"linux", "Microsoft", "", "/tools/powershell.exe", true},
		{"linux", "microsoft-standard-WSL2", "Ubuntu", "/tools/powershell.exe", true},
		{"linux", "generic", "Ubuntu", "/tools/powershell.exe", true},
	} {
		host := NativeHost()
		host.OS = tc.os
		host.Kernel = func() (string, error) { return tc.kernel, nil }
		host.LookPath = func(name string) (string, error) { return "/tools/" + name, nil }
		d, err := selectDesktop(host, []string{"WSL_DISTRO_NAME=" + tc.distro, "WAYLAND_DISPLAY=wayland-0"})
		if err != nil {
			t.Fatal(err)
		}
		if d.opener != tc.opener || d.wsl != tc.wsl {
			t.Fatalf("desktop %+v", d)
		}
	}
	host := NativeHost()
	host.OS = "linux"
	host.Kernel = func() (string, error) { return "microsoft", nil }
	host.LookPath = func(string) (string, error) { return "", errors.New("missing interoperation") }
	if _, err := selectDesktop(host, nil); err == nil {
		t.Fatal("WSL silently fell back")
	}
}

func TestRT001_17_WindowsURLs(t *testing.T) {
	for _, tc := range []struct {
		windows, want string
		valid         bool
	}{
		{`C:\Docs\Taḋg  %#.md`, `file:///C:/Docs/Ta%E1%B8%8Bg%20%20%25%23.md`, true},
		{`\\wsl.localhost\Ubuntu\home\reader\notes.md`, `file://wsl.localhost/Ubuntu/home/reader/notes.md`, true},
		{`\\wsl$\Ubuntu\home\reader\notes.md`, `file://wsl$/Ubuntu/home/reader/notes.md`, true},
		{`\\other\Ubuntu\x`, "", false},
		{`\\wsl.localhost\Other\x`, "", false},
		{`C:relative.md`, "", false},
		{`\\?\C:\Docs\x`, "", false},
		{`C:\Docs\CON.md`, "", false},
		{`C:\Docs\bad:name.md`, "", false},
		{`C:\Docs\trailing.`, "", false},
		{`C:\Docs\trailing `, "", false},
	} {
		t.Run(tc.windows, func(t *testing.T) {
			host := NativeHost()
			host.Execute = func(ctx context.Context, c Command) ([]byte, error) {
				if len(c.Args) != 2 {
					t.Fatal("translation argv")
				}
				if c.Args[0] == "-w" {
					return []byte(tc.windows + "\n"), nil
				}
				return []byte("/home/reader/notes.md\n"), nil
			}
			d := &desktop{host: host, wsl: true, translator: "wslpath", distro: "Ubuntu", urls: make(map[string]string)}
			got, err := d.fileURL(context.Background(), "/home/reader/notes.md")
			if tc.valid {
				if err != nil || got != tc.want {
					t.Fatalf("URL=%q err=%v", got, err)
				}
			} else if err == nil {
				t.Fatalf("unsafe path accepted %q", got)
			}
		})
	}
}

func TestRT001_17_PowerShellDataBoundary(t *testing.T) {
	var calls []Command
	host := NativeHost()
	host.Execute = func(ctx context.Context, c Command) ([]byte, error) { calls = append(calls, c); return nil, nil }
	d := &desktop{host: host, wsl: true, opener: "powershell.exe"}
	for _, p := range []string{"/Ubuntu/notes.md", "/Ubuntu/a '; $(touch SENTINEL) Taḋg.md"} {
		if err := d.open(context.Background(), (&url.URL{Scheme: "file", Host: "wsl.localhost", Path: p}).String()); err != nil {
			t.Fatal(err)
		}
	}
	if len(calls) != 2 {
		t.Fatal("handoff count")
	}
	a, b := calls[0], calls[1]
	if strings.Join(a.Args, "\x00") != strings.Join(b.Args, "\x00") || len(a.Args) != 5 || a.Args[3] != "-Command" {
		t.Fatalf("document enters command source: %+v", calls)
	}
	if len(a.Input) == 0 || string(a.Input) == string(b.Input) || !strings.Contains(a.Args[4], "[Console]::In.ReadToEnd()") || !strings.Contains(a.Args[4], "-FilePath") || strings.Contains(a.Args[4], "-Wait") {
		t.Fatal("PowerShell transport contract")
	}
	for _, c := range calls {
		if c.Limit != 65536 {
			t.Fatal("helper output not bounded")
		}
		for _, b := range c.Input {
			if b > 127 {
				t.Fatal("URL is not ASCII encoded")
			}
		}
	}
}
