// ABOUTME: Connects the Org parser to shared bounded worker supervision.
// ABOUTME: Retains the existing Org request and process entry-point contract.
package orgconvert

import (
	"github.com/tigger-developer/HTML-Preview/internal/convertworker"
	"io"
)

type Request = convertworker.Request

const (
	MaxInputBytes  = convertworker.MaxInputBytes
	MaxOutputBytes = convertworker.MaxOutputBytes
	MaxMemoryBytes = convertworker.MaxMemoryBytes
	LimitExit      = convertworker.LimitExit
)

func Worker(input io.Reader, output, diagnostic io.Writer) int {
	return convertworker.Run(input, output, diagnostic, convert)
}
func uniqueObject(data []byte) error { return convertworker.UniqueObject(data) }
