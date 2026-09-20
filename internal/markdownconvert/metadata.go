// ABOUTME: Parses bounded YAML frontmatter without evaluating source-controlled tags.
// ABOUTME: Preserves the established Markdown metadata presentation separately from parsing.
package markdownconvert

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
	dom "golang.org/x/net/html"
)

func extractMetadata(input []byte, limit int64) ([]byte, *yaml.Node, error) {
	input = bytes.TrimPrefix(input, []byte("\ufeff"))
	lines := bytes.SplitAfter(input, []byte("\n"))
	if len(lines) == 0 || strings.TrimSpace(string(lines[0])) != "---" {
		return input, nil, nil
	}
	offset := len(lines[0])
	end := -1
	after := 0
	for _, line := range lines[1:] {
		trim := strings.TrimSpace(string(line))
		if trim == "---" || trim == "..." {
			end = offset
			after = offset + len(line)
			break
		}
		offset += len(line)
	}
	if end < 0 {
		return input, nil, nil
	}
	var root yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(input[len(lines[0]):end]))
	if err := decoder.Decode(&root); err != nil {
		return nil, nil, errors.New("invalid Markdown metadata")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, nil, errors.New("multiple metadata documents")
	}
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return nil, nil, errors.New("metadata must be a mapping")
	}
	budget := limit
	seen := make(map[*yaml.Node]bool)
	if err := validateMetadata(root.Content[0], 0, &budget, seen); err != nil {
		return nil, nil, err
	}
	return input[after:], root.Content[0], nil
}
func validateMetadata(n *yaml.Node, depth int, budget *int64, path map[*yaml.Node]bool) error {
	if n == nil || depth > 64 || path[n] {
		return errors.New("metadata nesting or alias cycle")
	}
	*budget -= int64(len(n.Value)) + 32
	if *budget < 0 {
		return errors.New("metadata expansion exceeds limit")
	}
	path[n] = true
	defer delete(path, n)
	if n.Kind == yaml.AliasNode {
		return validateMetadata(n.Alias, depth+1, budget, path)
	}
	if n.Kind == yaml.MappingNode {
		keys := make(map[string]bool)
		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode || keys[key.Value] {
				return errors.New("invalid or duplicate metadata key")
			}
			keys[key.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := validateMetadata(child, depth+1, budget, path); err != nil {
			return err
		}
	}
	return nil
}
func resolved(n *yaml.Node) *yaml.Node {
	for n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}
func metadataText(n *yaml.Node) string {
	n = resolved(n)
	if n.Kind == yaml.ScalarNode {
		return n.Value
	}
	var parts []string
	for _, c := range n.Content {
		parts = append(parts, metadataText(c))
	}
	return strings.Join(parts, " ")
}
func metadataList(n *yaml.Node) *dom.Node {
	n = resolved(n)
	if n.Kind == yaml.ScalarNode {
		p := node("p")
		p.AppendChild(textNode(n.Value))
		return p
	}
	list := node("dl")
	values := map[string]*yaml.Node{}
	if n.Kind == yaml.MappingNode {
		for i := 0; i < len(n.Content); i += 2 {
			values[n.Content[i].Value] = n.Content[i+1]
		}
	} else {
		for i, c := range n.Content {
			values[fmt.Sprint(i+1)] = c
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		term := node("dt")
		term.AppendChild(textNode(key))
		description := node("dd")
		description.AppendChild(metadataList(values[key]))
		list.AppendChild(term)
		list.AppendChild(description)
	}
	return list
}
func prependMetadata(body *dom.Node, metadata *yaml.Node, hasBody bool) error {
	if metadata == nil {
		return nil
	}
	if !hasBody {
		box := node("div", "class", "frontmatter")
		box.AppendChild(metadataList(metadata))
		body.InsertBefore(box, body.FirstChild)
		return nil
	}
	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(metadata.Content); i += 2 {
		fields[metadata.Content[i].Value] = metadata.Content[i+1]
	}
	before := body.FirstChild
	for _, key := range []string{"title", "author", "date", "lang"} {
		if value := fields[key]; value != nil {
			p := node("p")
			p.AppendChild(textNode(metadataText(value)))
			body.InsertBefore(p, before)
		}
	}
	return nil
}
