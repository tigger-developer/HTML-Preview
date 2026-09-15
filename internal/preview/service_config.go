// ABOUTME: Selects explicit or discovered configuration, with startup-directory fallback.
// ABOUTME: Keeps YAML syntax, filesystem ownership and canonical root selection explicit.
package preview

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

type serviceConfig struct {
	roots   []string
	folding foldingOverride
	runtime string
}

func serviceSettings(cfg config) (serviceConfig, error) {
	var result serviceConfig
	var err error
	if cfg.configPath != "" {
		result, err = readServiceConfiguration(cfg.configPath)
	} else {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return result, cwdErr
		}
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return result, homeErr
		}
		result, err = discoverServiceConfiguration(cwd, home)
	}
	if err != nil {
		return result, err
	}
	result.runtime = cfg.runtimePath
	if result.runtime == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return result, err
		}
		result.runtime = filepath.Join(base, "htmlpreview", "runtime")
	}
	if !filepath.IsAbs(result.runtime) {
		return result, errors.New("runtime directory must be absolute")
	}
	return result, nil
}

func discoverServiceConfiguration(cwd, home string) (serviceConfig, error) {
	for _, path := range []string{filepath.Join(cwd, "config.yaml"), filepath.Join(home, ".config/htmlpreview/config.yaml")} {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return serviceConfig{}, err
		}
		return readServiceConfiguration(path)
	}
	root, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return serviceConfig{}, err
	}
	return serviceConfig{roots: []string{root}}, nil
}

func readServiceConfiguration(path string) (serviceConfig, error) {
	if !filepath.IsAbs(path) {
		return serviceConfig{}, errors.New("configuration path must be absolute")
	}
	data, err := readConfiguration(path)
	if err != nil {
		return serviceConfig{}, err
	}
	return decodeServiceConfiguration(data)
}

func readConfiguration(path string) (data []byte, err error) {
	// #nosec G304 -- The explicit configuration is bounded and checked through its opened handle.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || !ownedByUser(st) || st.Mode().Perm()&0022 != 0 {
		return nil, errors.New("configuration must be a user-owned regular file without group/other writes")
	}
	if st.Size() > 65536 {
		return nil, errors.New("configuration exceeds 64 KiB")
	}
	data, err = io.ReadAll(io.LimitReader(f, 65537))
	if len(data) > 65536 {
		return nil, errors.New("configuration exceeds 64 KiB")
	}
	return data, err
}

func ownedByUser(st os.FileInfo) bool {
	info, ok := st.Sys().(*syscall.Stat_t)
	return ok && int64(info.Uid) == int64(os.Geteuid())
}

func decodeServiceConfiguration(data []byte) (serviceConfig, error) {
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return serviceConfig{}, fmt.Errorf("configuration YAML: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return serviceConfig{}, errors.New("configuration requires exactly one YAML document")
	}
	if err := safeYAML(&doc, 0); err != nil {
		return serviceConfig{}, err
	}
	if len(doc.Content) != 1 {
		return serviceConfig{}, errors.New("configuration requires a mapping")
	}
	fields, err := yamlFields(doc.Content[0], "version", "serve", "folding")
	if err != nil {
		return serviceConfig{}, err
	}
	version := fields["version"]
	if version == nil || version.Kind != yaml.ScalarNode || version.Tag != "!!int" || version.Value != "1" {
		return serviceConfig{}, errors.New("configuration version must be integer 1")
	}
	folding, err := decodeFolding(fields["folding"])
	if err != nil {
		return serviceConfig{}, err
	}
	result := serviceConfig{folding: folding}
	if fields["serve"] == nil {
		return result, nil
	}
	serve, err := yamlFields(fields["serve"], "roots")
	if err != nil {
		return serviceConfig{}, err
	}
	roots := serve["roots"]
	if roots == nil {
		return result, nil
	}
	if roots.Kind != yaml.SequenceNode || len(roots.Content) > 32 {
		return serviceConfig{}, errors.New("configuration roots must be a list of at most 32 directories")
	}
	result.roots, err = canonicalRoots(roots.Content)
	return result, err
}

func safeYAML(node *yaml.Node, depth int) error {
	if depth > 8 || node.Kind == yaml.AliasNode || node.Anchor != "" || node.Style&yaml.TaggedStyle != 0 {
		return errors.New("configuration rejects aliases, tags and excessive nesting")
	}
	for _, child := range node.Content {
		if err := safeYAML(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func yamlFields(node *yaml.Node, allowed ...string) (map[string]*yaml.Node, error) {
	if node.Kind != yaml.MappingNode {
		return nil, errors.New("configuration field must be a mapping")
	}
	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		known := false
		for _, name := range allowed {
			if key.Value == name {
				known = true
			}
		}
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !known || fields[key.Value] != nil {
			return nil, errors.New("configuration has an unknown or duplicate field")
		}
		fields[key.Value] = node.Content[i+1]
	}
	return fields, nil
}

func canonicalRoots(nodes []*yaml.Node) ([]string, error) {
	var roots []string
	seen := make(map[string]bool)
	for _, n := range nodes {
		if n.Kind != yaml.ScalarNode || n.Tag != "!!str" || len(n.Value) > 4096 || !utf8.ValidString(n.Value) {
			return nil, errors.New("configuration roots require absolute UTF-8 directory paths up to 4096 bytes")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path, err := configuredRootPath(n.Value, home)
		if err != nil {
			return nil, err
		}
		root, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("configuration root: %w", err)
		}
		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			return nil, errors.New("configuration root must be an existing directory")
		}
		if !seen[root] {
			roots = append(roots, root)
			seen[root] = true
		}
	}
	sort.Slice(roots, func(i, j int) bool { return len(roots[i]) > len(roots[j]) })
	return roots, nil
}

// Expand home notation and the explicitly supported TMPDIR variable without a shell.
func configuredRootPath(path, home string) (string, error) {
	for _, prefix := range []string{"$TMPDIR", "${TMPDIR}"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			temp := os.Getenv("TMPDIR")
			if !filepath.IsAbs(temp) {
				return "", errors.New("configuration TMPDIR must be set to an absolute path")
			}
			path = filepath.Join(temp, strings.TrimPrefix(strings.TrimPrefix(path, prefix), "/"))
			break
		}
	}

	if path == "~" || strings.HasPrefix(path, "~/") {
		if !filepath.IsAbs(home) {
			return "", errors.New("configuration home must be absolute")
		}
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("configuration roots require absolute paths, ~/ paths or $TMPDIR")
	}
	return path, nil
}
