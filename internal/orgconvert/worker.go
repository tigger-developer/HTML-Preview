// ABOUTME: Defines the private native Org conversion process boundary.
// ABOUTME: Test-admission scaffold; conversion is implemented after the test-code gate.
package orgconvert

import "io"

// Request contains only bounded converter data, never filesystem authority.
type Request struct {
	Version     int      `json:"version"`
	Text        string   `json:"text"`
	Token       string   `json:"token"`
	TOC         bool     `json:"toc"`
	TOCDepth    int      `json:"toc_depth"`
	InputBytes  int64    `json:"input_bytes"`
	OutputBytes int64    `json:"output_bytes"`
	MemoryBytes int64    `json:"memory_bytes"`
	ProbeLabels []string `json:"probe_labels,omitempty"`
}

// Worker handles one private request. Its placeholder permits behavioural RED.
func Worker(_ io.Reader, _ io.Writer, _ io.Writer) int { return 1 }
