//go:build htmlpreview_test_desktop

// ABOUTME: Captures browser handoff only in the installed-command regression.
// ABOUTME: The explicit test build tag excludes this adapter from ordinary builds.
package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tigger-developer/HTML-Preview/internal/preview"
)

func hostForCommand() preview.Host {
	host := preview.NativeHost()
	actual := host.Execute
	host.Execute = func(ctx context.Context, cmd preview.Command) ([]byte, error) {
		tool := filepath.Base(cmd.Path)
		if tool != "open" && tool != "xdg-open" && tool != "powershell.exe" {
			return actual(ctx, cmd)
		}
		value := cmd.Args[0]
		if tool == "powershell.exe" {
			value = string(cmd.Input)
		}
		u, err := url.Parse(value)
		if err != nil {
			return nil, err
		}
		if u.Scheme == "http" {
			client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
			defer client.CloseIdleConnections()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
			if err != nil {
				return nil, err
			}
			response, err := client.Do(request)
			if err != nil {
				return nil, err
			}
			data, readErr := io.ReadAll(io.LimitReader(response.Body, 50*1024*1024+1))
			closeErr := response.Body.Close()
			if readErr != nil || closeErr != nil {
				return nil, errors.Join(readErr, closeErr)
			}
			if response.StatusCode != 200 {
				return nil, errors.New("captured HTTP preview failed")
			}
			return nil, os.WriteFile(filepath.Join(os.Getenv("PREVIEW_TEST_CAPTURE"), "http-preview.html"), data, 0600)
		}
		path := u.Path
		if u.Host != "" {
			_, path, _ = strings.Cut(strings.TrimPrefix(path, "/"), "/")
			path = "/" + path
		}
		// #nosec G304 -- This test-only adapter reads the page just published for desktop handoff.
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return nil, os.WriteFile(filepath.Join(os.Getenv("PREVIEW_TEST_CAPTURE"), filepath.Base(u.Path)), data, 0600)
	}
	return host
}
