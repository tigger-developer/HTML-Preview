// ABOUTME: Restarts an already registered user service through its native manager.
// ABOUTME: Keeps stop and start ordered without launching the preview server directly.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func restartService(goos string, run func(string, ...string) error) error {
	var manager string
	var stopArgs, startArgs []string
	switch goos {
	case "darwin":
		manager = "launchctl"
		stopArgs = []string{"stop", launchService()}
		startArgs = []string{"start", launchService()}
	case "linux":
		manager = "systemctl"
		stopArgs = []string{"--user", "stop", "htmlpreview.service"}
		startArgs = []string{"--user", "start", "htmlpreview.service"}
	default:
		return errors.New("service restart requires launchd on macOS or a systemd user service on Linux")
	}
	if err := run(manager, stopArgs...); err != nil {
		return fmt.Errorf("stop HTMLPreview service: %w", err)
	}
	if err := run(manager, startArgs...); err != nil {
		return fmt.Errorf("start HTMLPreview service: %w", err)
	}
	return nil
}

func serviceManagerCommand(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
