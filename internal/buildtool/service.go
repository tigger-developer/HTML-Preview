// ABOUTME: Packages service examples and expands fixed installation paths.
// ABOUTME: Writes only package artefacts; activation and user configuration remain operator-owned.
package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

func serviceTemplate(goos string) (string, error) {
	switch goos {
	case "darwin":
		return "org.htmlpreview.agent.plist", nil
	case "linux":
		return "htmlpreview.service", nil
	}
	return "", errors.New("unsupported service platform")
}

func serviceFiles(goos string) (map[string]string, error) {
	name, err := serviceTemplate(goos)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"config.example.yaml":       "packaging/service/config.example.yaml",
		"SERVICE.md":                "docs/SERVICE.md",
		"ANNOTATIONS.md":            "docs/ANNOTATIONS.md",
		"service/" + name + ".tmpl": "packaging/service/" + name + ".tmpl",
	}, nil
}

func installService(share, executable, goos string) error {
	files, err := serviceFiles(goos)
	if err != nil {
		return err
	}
	for name, source := range files {
		if err := copyFile(source, filepath.Join(share, name), 0644); err != nil {
			return err
		}
	}
	base, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	config := filepath.Join(base, ".config/htmlpreview/config.yaml")
	path := serviceSearchPath(installedPandoc())
	data, err := renderService(goos, executable, config, path)
	if err != nil {
		return err
	}
	name, err := serviceTemplate(goos)
	if err != nil {
		return err
	}
	return writePackage(filepath.Join(share, "service", name), data, 0644)
}

func renderService(goos, executable, config, path string) ([]byte, error) {
	var logPath string
	if goos == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		logPath = filepath.Join(home, "Library/Logs/htmlpreview/service.log")
	}
	return renderServiceLog(goos, executable, config, path, logPath)
}

func renderServiceLog(goos, executable, config, path, logPath string) ([]byte, error) {

	name, err := serviceTemplate(goos)
	if err != nil {
		return nil, err
	}
	values := struct{ Executable, Config, Path, Log string }{executable, config, path, logPath}
	for _, value := range []string{executable, config, path, logPath} {
		if strings.ContainsAny(value, "\x00\r\n") {
			return nil, errors.New("service installation paths cannot contain NUL or newlines")
		}
	}
	if goos == "darwin" {
		for _, field := range []*string{&values.Executable, &values.Config, &values.Path, &values.Log} {
			var escaped bytes.Buffer
			if err := xml.EscapeText(&escaped, []byte(*field)); err != nil {
				return nil, err
			}
			*field = escaped.String()
		}
	} else {
		values.Executable = unitQuote(strings.ReplaceAll(executable, "$", "$$"))
		values.Config = unitQuote("HTMLPREVIEW_CONFIG=" + config)
		values.Path = unitQuote("PATH=" + path)
	}
	t, err := template.ParseFiles(filepath.Join("packaging/service", name+".tmpl"))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err = t.Execute(&output, values); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// systemd accepts C-style quoting; percent specifiers must stay literal.
func unitQuote(value string) string { return strconv.Quote(strings.ReplaceAll(value, "%", "%%")) }

// Pandoc is optional; retain its discovered directory for specialist readers.
func installedPandoc() string {
	path, err := exec.LookPath("pandoc")
	if err != nil {
		return ""
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return ""
	}
	return path
}
func serviceSearchPath(pandoc string) string {
	path := "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	if pandoc != "" {
		path = filepath.Dir(pandoc) + ":" + path
	}
	return path
}
