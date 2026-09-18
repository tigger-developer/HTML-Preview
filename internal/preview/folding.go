// ABOUTME: Defines optional initial folding choices shared by file and service previews.
// ABOUTME: Keeps YAML and private registration validation on the same open/closed vocabulary.
package preview

import (
	"errors"
	"fmt"
	"go.yaml.in/yaml/v3"
)

type foldingOverride struct {
	Headers headerFolding `json:"headers,omitempty"`
	Drawers string        `json:"drawers,omitempty"`
	Default string        `json:"default,omitempty"`
}

type headerFolding struct {
	Default string `json:"default,omitempty"`
	Todo    string `json:"todo,omitempty"`
	Done    string `json:"done,omitempty"`
}

func (f foldingOverride) validate() error {
	for _, value := range []string{f.Headers.Default, f.Headers.Todo, f.Headers.Done, f.Drawers, f.Default} {
		if value != "" && value != "open" && value != "closed" {
			return errors.New("folding overrides require open or closed")
		}
	}
	return nil
}

func decodeFolding(node *yaml.Node) (foldingOverride, error) {
	var result foldingOverride
	if node == nil {
		return result, nil
	}
	fields, err := yamlFields(node, "override")
	if err != nil {
		return result, err
	}
	if fields["override"] == nil {
		return result, nil
	}
	values, err := yamlFields(fields["override"], "headers", "drawers", "default")
	if err != nil {
		return result, err
	}
	if node := values["headers"]; node != nil {
		if node.Kind != yaml.MappingNode {
			return result, errors.New("folding.override.headers must be a mapping; replace headers: open with headers: {default: open} (or closed)")
		}
		headers, err := yamlFields(node, "default", "todo", "done")
		if err != nil {
			return result, fmt.Errorf("folding.override.headers: %w", err)
		}
		if err := decodeFoldChoices(headers, map[string]*string{"default": &result.Headers.Default, "todo": &result.Headers.Todo, "done": &result.Headers.Done}, "folding.override.headers"); err != nil {
			return result, err
		}
	}
	if err := decodeFoldChoices(values, map[string]*string{"drawers": &result.Drawers, "default": &result.Default}, "folding.override"); err != nil {
		return result, err
	}
	return result, result.validate()
}

func decodeFoldChoices(values map[string]*yaml.Node, targets map[string]*string, path string) error {
	for name, target := range targets {
		if value := values[name]; value != nil {
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || (value.Value != "open" && value.Value != "closed") {
				return fmt.Errorf("%s.%s requires open or closed", path, name)
			}
			*target = value.Value
		}
	}
	return nil
}
