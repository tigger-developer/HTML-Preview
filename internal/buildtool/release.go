// ABOUTME: Creates versioned platform archives and a verified Homebrew formula.
// ABOUTME: Uses local archive URLs unless an existing remote location is supplied.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

func release() (err error) {
	v := os.Getenv("VERSION")
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(v) {
		return errors.New("release VERSION requires three non-negative decimal components, for example 0.1.0")
	}
	if len(v) > 100 {
		return errors.New("VERSION is too long")
	}
	base := os.Getenv("RELEASE_BASE_URL")
	if base != "" {
		u, err := url.Parse(base)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("RELEASE_BASE_URL must be an existing HTTP(S) directory URL without query or fragment")
		}
	}
	scratch, err := os.MkdirTemp("", "htmlpreview-release-")
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := os.RemoveAll(scratch); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()
	// #nosec G301 -- This directory contains distributable package metadata.
	if err := os.MkdirAll("dist/Formula", 0755); err != nil {
		return err
	}
	var manifest strings.Builder
	hashes := make(map[string]string)
	urls := make(map[string]string)
	for _, target := range []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"} {
		goos, arch, _ := strings.Cut(target, "-")
		binary := filepath.Join(scratch, "htmlpreview")
		if err := build(binary, goos, arch, v); err != nil {
			return err
		}
		name := "htmlpreview-" + v + "-" + target + ".tar.gz"
		path := filepath.Join("dist", name)
		if err := archive(path, binary, goos); err != nil {
			return err
		}
		// #nosec G304 -- The path is an archive generated from the validated version and fixed target list.
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		hash := hex.EncodeToString(digest[:])
		hashes[target] = hash
		fmt.Fprintf(&manifest, "%s  %s\n", hash, name)
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		location := (&url.URL{Scheme: "file", Path: absolute}).String()
		if base != "" {
			location, err = url.JoinPath(base, name)
			if err != nil {
				return err
			}
			if err := verifyRemote(location, hash); err != nil {
				return err
			}
		}
		urls[target] = location
	}
	// #nosec G306 -- Public archive checksums contain no document data.
	if err := os.WriteFile("dist/SHA256SUMS", []byte(manifest.String()), 0644); err != nil {
		return err
	}
	return formula(v, hashes, urls)
}

func archive(path, binary, goos string) (err error) {
	// #nosec G304 -- The caller constructs an archive path under dist from a validated version.
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	defer func() { err = errors.Join(err, tw.Close(), gz.Close(), f.Close()) }()
	files := licenceFiles()
	service, err := serviceFiles(goos)
	if err != nil {
		return err
	}
	for name, source := range service {
		files["share/htmlpreview/"+name] = source
	}
	files["htmlpreview"] = binary
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data, err := os.ReadFile(files[name])
		if err != nil {
			return err
		}
		mode := int64(0644)
		if name == "htmlpreview" {
			mode = 0755
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data))}); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func verifyRemote(location, want string) (err error) {
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Get(location)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("release archive unavailable: HTTP %d", response.StatusCode)
	}
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(response.Body, 104857601))
	if err != nil {
		return err
	}
	if count > 104857600 {
		return errors.New("remote archive exceeds verification limit")
	}
	if hex.EncodeToString(hash.Sum(nil)) != want {
		return errors.New("remote archive checksum differs from the built archive")
	}
	return nil
}

func formula(version string, hashes, urls map[string]string) error {
	data, err := os.ReadFile("packaging/htmlpreview.rb.tmpl")
	if err != nil {
		return err
	}
	t, err := template.New("formula").Parse(string(data))
	if err != nil {
		return err
	}
	values := struct{ Version, ArmURL, IntelURL, ArmHash, IntelHash string }{strconv.Quote(version), strconv.Quote(urls["darwin-arm64"]), strconv.Quote(urls["darwin-amd64"]), strconv.Quote(hashes["darwin-arm64"]), strconv.Quote(hashes["darwin-amd64"])}
	var output bytes.Buffer
	if err := t.Execute(&output, values); err != nil {
		return err
	}
	// #nosec G306 -- The generated formula is public package metadata.
	return os.WriteFile("dist/Formula/htmlpreview.rb", output.Bytes(), 0644)
}
