// ABOUTME: Activates or stops the checkout's macOS user LaunchAgent on explicit request.
// ABOUTME: Expands paths before launchd and never grants an incidental working directory.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func launchAgent(stop bool) error {
	if runtime.GOOS != "darwin" {
		return errors.New("make service and service-stop require macOS; see docs/SERVICE.md for Linux and WSL")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	run := func(args ...string) error { return command(nil, "launchctl", args...) }
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
	data, err := renderService("darwin", executable, config, filepath.Dir(pandoc)+":/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
	if err != nil {
		return err
	}
	if err = writePackage(plist, data, 0644); err != nil {
		return err
	}
	return run("load", plist)
}

func stopLaunchAgent(home string, run func(...string) error) error {
	plist, err := launchAgentPath(home)
	if err != nil {
		return err
	}
	return run("unload", plist)
}
