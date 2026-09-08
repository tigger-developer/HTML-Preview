// ABOUTME: Selects the desktop adapter without modifying system associations.
// ABOUTME: Browser-facing URL generation stays separate from source admission.
package preview

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
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
		kernel, err := host.Kernel()
		if err != nil {
			return nil, fmt.Errorf("identify Linux desktop: %w", err)
		}
		d.wsl = d.wsl || strings.Contains(strings.ToLower(kernel), "microsoft")
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
	return "", errors.New("WSL path translation is not implemented")
}
func (d *desktop) open(ctx context.Context, path string) error {
	_, err := helper(ctx, d.host, d.opener, []string{path}, nil)
	return err
}
