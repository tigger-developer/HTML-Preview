// ABOUTME: Manages the checkout's Linux systemd user service on explicit request.
// ABOUTME: Keeps service definition changes and manager activation inside one boundary.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

func managedService(stop bool) error {
	return managedServiceFor(runtime.GOOS, stop, launchAgent, linuxUserService)
}

func managedServiceFor(goos string, stop bool, darwin, linux func(bool) error) error {
	switch goos {
	case "darwin":
		return darwin(stop)
	case "linux":
		return linux(stop)
	default:
		return errors.New("make service and service-stop require launchd on macOS or a systemd user manager on Linux")
	}
}

func linuxUserService(stop bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if stop {
		return systemdService(true, home, "", "", "", serviceManagerCommand)
	}
	executable, config, pandoc, err := serviceInputs(home)
	if err != nil {
		return err
	}
	return systemdService(false, home, executable, config, pandoc, serviceManagerCommand)
}

func systemdService(stop bool, home, executable, config, pandoc string, run func(string, ...string) error) error {
	if !filepath.IsAbs(home) {
		return errors.New("systemd user-service home must be absolute")
	}
	unit := filepath.Join(home, ".config/systemd/user/htmlpreview.service")
	if stop {
		if err := run("systemctl", "--user", "stop", "htmlpreview.service"); err != nil {
			return fmt.Errorf("stop HTMLPreview systemd user service: %w", err)
		}
		return nil
	}
	for _, path := range []string{executable, config} {
		if !filepath.IsAbs(path) {
			return errors.New("systemd executable and config paths must be absolute")
		}
	}
	info, err := os.Stat(config)
	if err != nil {
		return fmt.Errorf("service config %q: %w; create it with explicit serve.roots before make service", config, err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("service config must be a regular file")
	}
	data, err := renderServiceLog("linux", executable, config, serviceSearchPath(pandoc), "")
	if err != nil {
		return err
	}
	if err := replaceSystemdUnit(unit, data); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", "htmlpreview.service"},
		{"--user", "restart", "htmlpreview.service"},
	} {
		if err := run("systemctl", args...); err != nil {
			return fmt.Errorf("systemctl %v: %w", args, err)
		}
	}
	return nil
}

func replaceSystemdUnit(path string, data []byte) (err error) {
	if info, statErr := os.Lstat(path); statErr == nil {
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return errors.New("systemd unit destination must be a regular file or symlink")
		}
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return statErr
	}
	// #nosec G301 -- systemd reads this user-owned public configuration directory.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".htmlpreview-service-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if removeErr := os.Remove(tmpPath); !errors.Is(removeErr, fs.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return err
	}
	written, writeErr := tmp.Write(data)
	if writeErr != nil {
		_ = tmp.Close()
		return writeErr
	}
	if written != len(data) {
		_ = tmp.Close()
		return errors.New("short write preparing systemd user unit")
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace systemd user unit: %w", err)
	}
	return nil
}
