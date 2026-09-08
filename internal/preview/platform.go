// ABOUTME: Selects the desktop adapter without modifying system associations.
// ABOUTME: Browser-facing URL generation stays separate from source admission.
package preview

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	bundle "github.com/tigger-developer/HTML-Preview"
)

type desktop struct {
	host               Host
	opener             string
	wsl                bool
	translator, distro string
	urls               map[string]string
}

func selectDesktop(host Host, env []string) (*desktop, error) {
	d := &desktop{host: host, urls: make(map[string]string)}
	switch host.OS {
	case "darwin":
		d.opener = "/usr/bin/open"
	case "linux":
		for _, entry := range env {
			if value, ok := strings.CutPrefix(entry, "WSL_DISTRO_NAME="); ok && value != "" {
				d.wsl = true
				d.distro = value
			}
		}
		var err error
		if !d.wsl {
			kernel, kernelErr := host.Kernel()
			if kernelErr != nil {
				return nil, fmt.Errorf("identify Linux desktop: %w", kernelErr)
			}
			d.wsl = strings.Contains(strings.ToLower(kernel), "microsoft")
		}
		name := "xdg-open"
		if d.wsl {
			name = "powershell.exe"
			d.translator, err = host.LookPath("wslpath")
			if err != nil {
				return nil, fmt.Errorf("WSL requires wslpath: %w", err)
			}
		}
		d.opener, err = host.LookPath(name)
		if err != nil {
			return nil, fmt.Errorf("desktop requires %s: %w", name, err)
		}
	default:
		return nil, fmt.Errorf("unsupported operating system %q", host.OS)
	}
	return d, nil
}
func (d *desktop) fileURL(ctx context.Context, path string) (string, error) {
	if !d.wsl {
		return (&url.URL{Scheme: "file", Path: path}).String(), nil
	}
	if value, ok := d.urls[path]; ok {
		return value, nil
	}
	mapped, err := d.translate(ctx, "-w", path)
	if err != nil {
		return "", err
	}
	u, err := windowsURL(mapped, d.distro)
	if err != nil {
		return "", err
	}
	roundtrip, err := d.translate(ctx, "-u", mapped)
	if err != nil {
		return "", err
	}
	if roundtrip != filepath.Clean(path) {
		return "", errors.New("WSL path translation does not round-trip")
	}
	d.urls[path] = u.String()
	return u.String(), nil
}
func (d *desktop) open(ctx context.Context, path string) error {
	args := []string{path}
	var input []byte
	if d.wsl {
		script, err := bundle.Assets.ReadFile("assets/platform/open.ps1")
		if err != nil {
			return err
		}
		args = []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", string(script)}
		input = []byte(path)
	}
	_, err := helper(ctx, d.host, d.opener, args, input)
	return err
}

func (d *desktop) translate(ctx context.Context, direction, path string) (string, error) {
	data, err := helper(ctx, d.host, d.translator, []string{direction, path}, nil)
	if err != nil {
		return "", fmt.Errorf("wslpath %s: %w", direction, err)
	}
	value := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("malformed wslpath output")
	}
	return value, nil
}

func (d *desktop) prepare(ctx context.Context, path string) error {
	if !d.wsl {
		return nil
	}
	mapped, err := d.translate(ctx, "-w", path)
	if err != nil {
		return err
	}
	parts := strings.Split(strings.TrimPrefix(mapped, `\\`), `\`)
	if !strings.HasPrefix(mapped, `\\`) || len(parts) < 3 || (!strings.EqualFold(parts[0], "wsl.localhost") && !strings.EqualFold(parts[0], "wsl$")) {
		return errors.New("WSL requires a private Linux temporary directory, not Windows-mounted output")
	}
	if d.distro != "" && d.distro != parts[1] {
		return errors.New("WSL_DISTRO_NAME disagrees with the temporary-directory share")
	}
	d.distro = parts[1]
	_, err = d.fileURL(ctx, path)
	return err
}

func windowsURL(path, distro string) (*url.URL, error) {
	if strings.Contains(path, "/") {
		return nil, errors.New("mixed Windows path separators")
	}
	var components []string
	u := &url.URL{Scheme: "file"}
	if strings.HasPrefix(path, `\\`) {
		parts := strings.Split(path[2:], `\`)
		if len(parts) < 3 || (!strings.EqualFold(parts[0], "wsl.localhost") && !strings.EqualFold(parts[0], "wsl$")) || distro == "" || parts[1] != distro {
			return nil, errors.New("Windows share is not the current WSL distribution")
		}
		u.Host = parts[0]
		components = parts[1:]
		u.Path = "/" + strings.Join(components, "/")
	} else {
		if len(path) < 3 || !((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) || path[1] != ':' || path[2] != '\\' {
			return nil, errors.New("Windows path is not an absolute drive or allowed WSL share")
		}
		components = strings.Split(path[3:], `\`)
		u.Path = "/" + path[:2] + "/" + strings.Join(components, "/")
	}
	for _, component := range components {
		if err := windowsComponent(component); err != nil {
			return nil, err
		}
	}
	return u, nil
}
func windowsComponent(value string) error {
	if value == "" || strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return errors.New("empty or trailing-dot/space Windows component")
	}
	for _, r := range value {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return errors.New("unrepresentable Windows path component")
		}
	}
	base, _, _ := strings.Cut(strings.ToUpper(value), ".")
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return errors.New("reserved Windows path component")
	}
	return nil
}
