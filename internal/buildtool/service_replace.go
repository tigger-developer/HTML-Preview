// ABOUTME: Replaces the single user HTMLPreview launchd job without following stale plist links.
// ABOUTME: Preserves the prior plist for rollback and distinguishes absent jobs from manager failures.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func launchDomain() string  { return fmt.Sprintf("gui/%d", os.Getuid()) }
func launchService() string { return launchDomain() + "/org.htmlpreview.agent" }

func bootoutLaunchAgent(run func(...string) error) (bool, error) {
	err := run("bootout", launchService())
	if err == nil {
		return true, nil
	}
	var exit interface{ ExitCode() int }
	// launchctl reports ESRCH (3) when this exact service is not registered.
	if errors.As(err, &exit) && exit.ExitCode() == 3 {
		return false, nil
	}
	return false, fmt.Errorf("stop existing HTMLPreview service: %w", err)
}

func replaceLaunchAgent(plist string, data []byte, run func(...string) error) (err error) {
	info, statErr := os.Lstat(plist)
	exists := statErr == nil
	if exists && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return errors.New("LaunchAgent destination must be a regular file or symlink")
	}
	if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}
	// #nosec G301 -- launchd reads this user-owned public configuration directory.
	if err := os.MkdirAll(filepath.Dir(plist), 0755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(filepath.Dir(plist), ".htmlpreview-service-")
	if err != nil {
		return err
	}
	preserveBackup := false
	defer func() {
		if !preserveBackup {
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()
	candidate, backup := filepath.Join(dir, "candidate.plist"), filepath.Join(dir, "previous.plist")
	if err := writePackage(candidate, data, 0644); err != nil {
		return err
	}
	loaded, err := bootoutLaunchAgent(run)
	if err != nil {
		return err
	}
	moved := false
	if exists {
		if err = os.Rename(plist, backup); err != nil {
			if loaded {
				err = errors.Join(err, run("bootstrap", launchDomain(), plist))
			}
			return err
		}
		moved = true
	}
	attemptedBootstrap := false
	if err = os.Rename(candidate, plist); err == nil {
		err = run("enable", launchService())
		if err == nil {
			attemptedBootstrap = true
			err = run("bootstrap", launchDomain(), plist)
		}
	}
	if err == nil {
		return nil
	}
	failure := fmt.Errorf("activate replacement HTMLPreview service: %w", err)
	if attemptedBootstrap {
		if _, cleanupErr := bootoutLaunchAgent(run); cleanupErr != nil {
			preserveBackup = true
			return errors.Join(failure, fmt.Errorf("remove unsuccessful replacement (recovery files at %q): %w", dir, cleanupErr))
		}
	}
	var restoreErr error
	if moved {
		restoreErr = os.Rename(backup, plist)
	} else {
		restoreErr = os.Remove(plist)
		if os.IsNotExist(restoreErr) {
			restoreErr = nil
		}
	}
	if restoreErr != nil {
		preserveBackup = true
		return errors.Join(failure, fmt.Errorf("restore prior plist (recovery files retained at %q): %w", dir, restoreErr))
	}
	if loaded {
		if !exists {
			return errors.Join(failure, errors.New("previous job had no plist; cannot automatically restore it"))
		}
		if restoreErr = run("bootstrap", launchDomain(), plist); restoreErr != nil {
			return errors.Join(failure, fmt.Errorf("restart previous service: %w", restoreErr))
		}
	}
	return failure
}
