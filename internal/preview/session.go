// ABOUTME: Coordinates private preview sessions and their retained output.
// ABOUTME: Keeps publication separate from conversion and browser handoff.
package preview

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/net/html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	bundle "github.com/tigger-developer/HTML-Preview"
)

type session struct {
	path, pandoc     string
	host             Host
	desktop          *desktop
	cfg              config
	log              *console
	pages            []*page
	byKey            map[string]*page
	used, sourceUsed int64
	status           int
}

func execute(ctx context.Context, files, env []string, cfg config, host Host, log *console) (code int) {
	sources, invalid, err := explicitSources(files, cfg, log)
	if err != nil {
		log.warn("%v", err)
		return 2
	}
	pandoc, err := host.LookPath("pandoc")
	if err != nil {
		log.warn("install Pandoc 3.9.0.2 or a later 3.9 patch: %v", err)
		return 1
	}
	if err := checkPandoc(ctx, host, pandoc); err != nil {
		log.warn("Pandoc compatibility: %v", err)
		return 1
	}
	desktop, err := selectDesktop(host, env)
	if err != nil {
		log.warn("%v", err)
		return 1
	}
	if len(sources) == 0 {
		return 1
	}
	path, err := host.Temp()
	if err != nil {
		log.warn("allocate private session: %v", err)
		return 1
	}
	s := &session{path: path, pandoc: pandoc, host: host, desktop: desktop, cfg: cfg, log: log, byKey: make(map[string]*page)}
	if invalid {
		s.status = 1
	}
	defer func() {
		if err := host.Remove(path); err != nil {
			log.warn("cleanup failed; remaining directory %q: %v", path, err)
			code = 1
		}
		code = log.status(code)
	}()
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0700 {
		log.warn("session is not owner-private: %q", path)
		return 1
	}
	conversion, cancel := context.WithTimeout(ctx, cfg.deadline)
	defer cancel()
	if err := desktop.prepare(conversion, path); err != nil {
		log.warn("temporary-directory translation: %v", err)
		return 1
	}
	if err := s.extract(); err != nil {
		log.warn("prepare session: %v", err)
		return 1
	}
	for _, src := range sources {
		s.admit(src)
	}
	for i := 0; i < len(s.pages); i++ {
		p := s.pages[i]
		s.convert(conversion, p)
		if cfg.links && p.ready && conversion.Err() == nil {
			s.discover(p)
		}
	}
	cancel()
	if ctx.Err() != nil {
		log.warn("preview cancelled before publication")
		return 1
	}
	publication, stop := context.WithTimeout(ctx, 5*time.Second)
	err = s.publish(publication)
	stop()
	if err != nil {
		log.warn("publication failed: %v", err)
		return 1
	}
	opened := 0
	for _, p := range s.pages {
		if !p.source.explicit || !p.ready {
			continue
		}
		log.print("%s", p.url)
		opened++
		if err := desktop.open(ctx, p.url); err != nil {
			log.warn("browser handoff for %q: %v", p.source.logical, err)
			s.status = 1
		}
	}
	if opened == 0 {
		return 1
	}
	if ctx.Err() != nil {
		return 1
	}
	if cfg.mode == "read" {
		log.warn("reading session %q; press Ctrl+C or send SIGTERM to remove its previews", path)
		<-ctx.Done()
	} else {
		timer := time.NewTimer(cfg.grace)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
	}
	return s.status
}

func explicitSources(files []string, cfg config, log *console) ([]sourceContext, bool, error) {
	root := cfg.root
	if root != "" {
		var err error
		root, err = filepath.Abs(root)
		if err != nil {
			return nil, false, err
		}
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return nil, false, fmt.Errorf("HTMLPREVIEW_ROOT: %w", err)
		}
		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			return nil, false, errors.New("HTMLPREVIEW_ROOT must be an existing directory")
		}
	}
	var result []sourceContext
	seen := make(map[string]bool)
	invalid := false
	for _, file := range files {
		src, err := identify(file, root)
		if err != nil {
			log.warn("source %q: %v", file, err)
			invalid = true
			continue
		}
		if !seen[src.key] {
			seen[src.key] = true
			src.explicit = true
			result = append(result, src)
		}
	}
	if int64(len(result)) > cfg.files {
		return nil, false, errors.New("explicit source contexts exceed HTMLPREVIEW_MAX_FILES")
	}
	return result, invalid, nil
}

func checkPandoc(ctx context.Context, host Host, path string) error {
	data, err := helper(ctx, host, path, []string{"--version"}, nil)
	if err != nil {
		return err
	}
	match := regexp.MustCompile(`(?m)^pandoc 3\.9\.(\d+)(?:\.(\d+))?(?:\r?\n|$)`).FindSubmatch(data)
	if len(match) == 0 {
		return errors.New("requires Pandoc 3.9.0.2 or a later 3.9 patch")
	}
	patch, _ := strconv.Atoi(string(match[1]))
	sub, _ := strconv.Atoi(string(match[2]))
	if patch == 0 && sub < 2 {
		return errors.New("Pandoc is older than 3.9.0.2")
	}
	if !regexp.MustCompile(`Scripting engine: Lua 5\.4(?:\r?\n|$)`).Match(data) {
		return errors.New("Pandoc must include Lua 5.4")
	}
	return nil
}

func (s *session) write(name string, data []byte) error {
	if int64(len(data)) > s.cfg.outputBytes-s.used {
		return errors.New("HTMLPREVIEW_MAX_OUTPUT_BYTES exhausted")
	}
	s.used += int64(len(data))
	path := filepath.Join(s.path, name)
	if err := s.host.Write(path, data); err != nil {
		return err
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.Mode().Perm() != 0600 {
		return errors.New("generated file is not owner-private")
	}
	return nil
}

func (s *session) extract() error {
	for _, name := range []string{"defaults.yaml", "fidelity.lua"} {
		data, err := bundle.Assets.ReadFile("assets/pandoc/" + name)
		if err != nil {
			return err
		}
		if err := s.write(name, data); err != nil {
			return err
		}
	}
	return nil
}
func (s *session) admit(src sourceContext) *page {
	p := &page{source: src, name: fmt.Sprintf("%04d.html", len(s.pages)+1)}
	s.pages = append(s.pages, p)
	s.byKey[src.key] = p
	return p
}

func (s *session) convert(ctx context.Context, p *page) {
	if err := ctx.Err(); err != nil {
		s.log.warn("source %q: conversion deadline or cancellation", p.source.logical)
		if p.source.explicit {
			s.status = 1
		}
		return
	}
	limit := min(s.cfg.sourceBytes, s.cfg.totalBytes-s.sourceUsed)
	data, err := snapshot(p.source, limit)
	if err == nil {
		s.sourceUsed += int64(len(data))
		err = s.render(ctx, p, data)
	}
	if err != nil {
		s.log.warn("source %q: %v", p.source.logical, err)
		if p.source.explicit {
			s.status = 1
		}
		return
	}
	p.ready = true
}

func (s *session) publish(ctx context.Context) error {
	for _, p := range s.pages {
		if !p.ready {
			continue
		}
		var err error
		p.url, err = s.desktop.fileURL(ctx, filepath.Join(s.path, p.name))
		if err != nil {
			return err
		}
	}
	outputs, err := s.planOutput(ctx)
	if err != nil {
		return err
	}
	for _, p := range s.pages {
		if !p.ready {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.write(p.name, outputs[p]); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (s *session) planOutput(ctx context.Context) (map[*page][]byte, error) {
	original := make(map[*page]*html.Node)
	for _, p := range s.pages {
		if p.ready {
			original[p] = p.dom
		}
	}
	for {
		outputs := make(map[*page][]byte)
		remaining := s.cfg.outputBytes - s.used
		retry := false
		for i, p := range s.pages {
			if !p.ready {
				continue
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			p.dom = cloneTree(original[p])
			if err := s.resolve(ctx, p); err != nil {
				return nil, err
			}
			data, err := s.document(p)
			if err != nil {
				return nil, err
			}
			if int64(len(data)) > remaining {
				for _, skipped := range s.pages[i:] {
					if skipped.ready {
						skipped.ready = false
						s.log.warn("source %q: HTMLPREVIEW_MAX_OUTPUT_BYTES exhausted", skipped.source.logical)
						if skipped.source.explicit {
							s.status = 1
						}
					}
				}
				retry = true
				break
			}
			outputs[p] = data
			remaining -= int64(len(data))
		}
		if !retry {
			return outputs, nil
		}
	}
}

func cloneTree(n *html.Node) *html.Node {
	copy := &html.Node{Type: n.Type, DataAtom: n.DataAtom, Data: n.Data, Namespace: n.Namespace, Attr: append([]html.Attribute(nil), n.Attr...)}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		copy.AppendChild(cloneTree(child))
	}
	return copy
}
