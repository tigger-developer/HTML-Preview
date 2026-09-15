// ABOUTME: Defines optional initial folding choices shared by file and service previews.
// ABOUTME: Keeps YAML and private registration validation on the same open/closed vocabulary.
package preview

import (
	"errors"
	"go.yaml.in/yaml/v3"
)

type foldingOverride struct {
	Headers string `json:"headers,omitempty"`
	Drawers string `json:"drawers,omitempty"`
	Default string `json:"default,omitempty"`
}

func (f foldingOverride) validate() error {
	for _, value := range []string{f.Headers, f.Drawers, f.Default} {
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
	for name, target := range map[string]*string{"headers": &result.Headers, "drawers": &result.Drawers, "default": &result.Default} {
		if value := values[name]; value != nil {
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || value.Value == "" {
				return result, errors.New("folding overrides require open or closed")
			}
			*target = value.Value
		}
	}
	return result, result.validate()
}
