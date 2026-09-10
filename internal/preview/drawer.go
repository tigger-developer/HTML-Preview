// ABOUTME: Presents drawer contents as property rows and preserved free text.
// ABOUTME: An embedded HTML template escapes every source-derived field.
package preview

import (
	"bytes"
	"html/template"
	"strings"
)

type drawerPart struct {
	Properties []metadataField
	Text       []string
}

func renderDrawer(layout *template.Template, name, body string) (string, error) {
	var parts []drawerPart
	for _, line := range strings.SplitAfter(body, "\n") {
		if line == "" {
			continue
		}
		key, value, propertyLine := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), ":"), ":")
		propertyLine = propertyLine && strings.HasPrefix(strings.TrimSpace(line), ":") && key != "" && !strings.ContainsAny(key, " \t")
		if len(parts) == 0 || (parts[len(parts)-1].Properties != nil) != propertyLine {
			parts = append(parts, drawerPart{})
		}
		part := &parts[len(parts)-1]
		if propertyLine {
			part.Properties = append(part.Properties, metadataField{Name: key, Value: strings.TrimSpace(value)})
		} else {
			part.Text = append(part.Text, line)
		}
	}
	var out bytes.Buffer
	err := layout.Execute(&out, struct {
		Name  string
		Parts []drawerPart
	}{Name: name, Parts: parts})
	return out.String(), err
}
