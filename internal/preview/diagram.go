// ABOUTME: Renders explicit Mermaid and PlantUML code blocks into bounded PNG assets.
// ABOUTME: Keeps diagram source literal and denies PlantUML file and network access.
package preview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxDiagramSource = 256 << 10

func (s *session) diagramExecutable(name string) (string, error) {
	value, checked := s.mermaid, s.mermaidChecked
	if name == "plantuml" {
		value, checked = s.plantuml, s.plantChecked
	}
	if checked {
		if value == "" {
			return "", fmt.Errorf("%s renderer is unavailable", name)
		}
		return value, nil
	}
	path, err := s.host.LookPath(name)
	if name == "plantuml" {
		s.plantChecked, s.plantuml = true, path
	} else {
		s.mermaidChecked, s.mermaid = true, path
	}
	if err != nil {
		return "", fmt.Errorf("%s renderer is unavailable: %w", name, err)
	}
	return path, nil
}

func (s *session) renderDiagram(ctx context.Context, p *page, language, source string) (string, error) {
	if len(source) == 0 || len(source) > maxDiagramSource {
		return "", errors.New("diagram source exceeds 256 KiB limit")
	}
	if p.diagrams >= 32 {
		return "", errors.New("diagram count exceeds 32")
	}
	p.diagrams++
	path, err := s.diagramExecutable(map[string]string{"mermaid": "mmdc", "plantuml": "plantuml"}[language])
	if err != nil {
		return "", err
	}
	limit := min(maxAssetBytes, s.cfg.outputBytes-s.used)
	if limit <= 0 {
		return "", errors.New("diagram output budget exhausted")
	}
	var output []byte
	if language == "plantuml" {
		output, err = s.host.Execute(ctx, Command{Path: path, Args: []string{"-pipe", "-tpng", "-nometadata"}, Input: []byte(source), Dir: s.path, Limit: limit, Env: []string{"PLANTUML_SECURITY_PROFILE=SANDBOX", "JAVA_TOOL_OPTIONS=-Xmx256m"}})
	} else {
		target := filepath.Join(s.path, fmt.Sprintf("diagram-%04d.png", p.diagrams))
		output, err = s.host.Execute(ctx, Command{Path: path, Args: []string{"--input", "-", "--output", target, "--outputFormat", "png", "--configFile", filepath.Join(s.path, "mermaid.json")}, Input: []byte(source), Dir: s.path, Limit: 65536})
		if err == nil && rasterMIME(output) != "image/png" {
			output, err = readGeneratedPNG(target, limit)
		}
	}
	if err != nil {
		return "", fmt.Errorf("render %s: %w", language, err)
	}
	if rasterMIME(output) != "image/png" {
		return "", errors.New("diagram renderer returned invalid PNG data")
	}
	s.used += int64(len(output))
	result, err := rasterURL(output, "", "image/png")
	if err == nil && s.imageResolver != nil {
		result, err = s.imageResolver(p, "", "image/png", output)
	}
	if err == nil {
		if p.images == nil {
			p.images = make(map[string]string)
		}
		p.images["\x00"+result] = result
		p.images[filepath.Dir(p.source.logical)+"\x00"+result] = result
	}
	return result, err
}

func readGeneratedPNG(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("diagram output exceeds byte limit")
	}
	return data, nil
}
