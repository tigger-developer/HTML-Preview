// ABOUTME: Activates or stops the checkout's macOS user LaunchAgent on explicit request.
// ABOUTME: Expands paths before launchd and never grants an incidental working directory.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func launchAgent(stop bool) error {
	if runtime.GOOS != "darwin" {
		return errors.New("make service and service-stop require macOS; see docs/SERVICE.md for Linux and WSL")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	run := launchctlCommand
	if stop {
		return stopLaunchAgent(home, run)
	}
	executable, err := filepath.Abs("bin/htmlpreview")
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	pandoc, err := exec.LookPath("pandoc")
	if err != nil {
		return errors.New("make service requires Pandoc on PATH")
	}
	pandoc, err = filepath.Abs(pandoc)
	if err != nil {
		return err
	}
	config := os.Getenv("HTMLPREVIEW_CONFIG")
	if config == "" {
		config, err = filepath.Abs("config.yaml")
		if err != nil {
			return err
		}
		if _, err = os.Lstat(config); os.IsNotExist(err) {
			config = filepath.Join(home, ".config/htmlpreview/config.yaml")
		} else if err != nil {
			return err
		}
	}
	return startLaunchAgent(home, executable, config, pandoc, run)
}

func launchctlCommand(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "launchctl", args...).CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("launchctl %s: %w", args[0], ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("launchctl %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return nil
}

func launchAgentPath(home string) (string, error) {
	if !filepath.IsAbs(home) {
		return "", errors.New("LaunchAgent home must be absolute")
	}
	return filepath.Join(home, "Library/LaunchAgents/org.htmlpreview.agent.plist"), nil
}

func startLaunchAgent(home, executable, config, pandoc string, run func(...string) error) error {
	plist, err := launchAgentPath(home)
	if err != nil {
		return err
	}
	for _, path := range []string{executable, config, pandoc} {
		if !filepath.IsAbs(path) {
			return errors.New("LaunchAgent executable, config and Pandoc paths must be absolute")
		}
	}
	info, err := os.Stat(config)
	if err != nil {
		return fmt.Errorf("service config %q: %w; create it with explicit serve.roots before make service", config, err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("service config must be a regular file")
	}
	logPath := filepath.Join(home, "Library/Logs/htmlpreview/service.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return err
	}
	// Refuse symlinks rather than following another file when enabling diagnostics.
	if st, err := os.Lstat(logPath); err == nil && !st.Mode().IsRegular() {
		return errors.New("service log must be a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	// #nosec G304 -- Fixed user service log; refuse symlinks and blocking special files.
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	if err := errors.Join(logFile.Chmod(0600), logFile.Close()); err != nil {
		return err
	}
	data, err := renderServiceLog("darwin", executable, config, filepath.Dir(pandoc)+":/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin", logPath)

	if err != nil {
		return err
	}
	return replaceLaunchAgent(plist, data, run)
}

func stopLaunchAgent(home string, run func(...string) error) error {
	_, err := launchAgentPath(home)
	if err != nil {
		return err
	}
	_, err = bootoutLaunchAgent(run)
	return err
}
