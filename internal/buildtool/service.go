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
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	pandoc, err := exec.LookPath("pandoc")
	if err != nil {
		return errors.New("service template requires installed Pandoc; install Pandoc and rerun make install")
	}
	pandoc, err = filepath.Abs(pandoc)
	if err != nil {
		return err
	}
	config := filepath.Join(base, "htmlpreview/config.yaml")
	path := filepath.Dir(pandoc) + ":/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
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
	name, err := serviceTemplate(goos)
	if err != nil {
		return nil, err
	}
	values := struct{ Executable, Config, Path string }{executable, config, path}
	for _, value := range []string{executable, config, path} {
		if strings.ContainsAny(value, "\x00\r\n") {
			return nil, errors.New("service installation paths cannot contain NUL or newlines")
		}
	}
	if goos == "darwin" {
		for _, field := range []*string{&values.Executable, &values.Config, &values.Path} {
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
