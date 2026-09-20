// ABOUTME: Scans linked code and the pinned upstream identity of the local Org parser.
// ABOUTME: Rejects stale scan manifests and keeps all generated scanner inputs temporary.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type dependencyVersion struct {
	Path    string
	Version string
	Replace *dependencyVersion
}

func moduleVersions(dir string) (map[string]dependencyVersion, error) {
	cmd := exec.Command("go", "list", "-mod=readonly", "-m", "-json", "all")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("read locked dependency graph: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	result := make(map[string]dependencyVersion)
	for {
		var module dependencyVersion
		if err := decoder.Decode(&module); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		result[module.Path] = module
	}
	return result, nil
}

func vulnerabilityCheck() (err error) {
	if err = command(nil, "govulncheck", "./..."); err != nil {
		return err
	}
	data, err := os.ReadFile("third_party/go-org/provenance.json")
	if err != nil {
		return err
	}
	var provenance struct {
		Module   string `json:"module"`
		Version  string `json:"version"`
		Revision string `json:"revision"`
	}
	if err = json.Unmarshal(data, &provenance); err != nil || provenance.Module == "" || provenance.Version == "" || len(provenance.Revision) != 40 {
		return errors.New("invalid Org parser provenance")
	}
	actual, err := moduleVersions(".")
	if err != nil {
		return err
	}
	if actual[provenance.Module].Version != provenance.Version {
		return errors.New("Org parser provenance differs from application version")
	}
	dir, err := os.MkdirTemp("", "htmlpreview-org-advisory-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	for _, name := range []string{"go.mod", "go.sum", "main.go"} {
		// #nosec G304 -- Only these three tracked scan manifest files are read.
		data, readErr := os.ReadFile(filepath.Join("third_party/go-org/advisory", name))
		if readErr != nil {
			return readErr
		}
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return err
		}
	}
	upstream, err := moduleVersions(dir)
	if err != nil {
		return err
	}
	if upstream[provenance.Module].Version != provenance.Version || upstream[provenance.Module].Replace != nil {
		return errors.New("upstream Org advisory identity is missing or replaced")
	}
	for path, module := range actual {
		if pinned, ok := upstream[path]; ok && pinned.Version != module.Version {
			return fmt.Errorf("stale Org advisory dependency %s", path)
		}
	}
	// The scanner receives immutable manifests; advisory retrieval failure remains failure.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "govulncheck", "-scan=module")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=readonly")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("upstream Org advisory scan: %w", err)
	}
	return nil
}
