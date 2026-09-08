// ABOUTME: Starts the local document preview command with native OS boundaries.
// ABOUTME: Build identity is supplied by the repository build targets.
package main

import (
	"github.com/tigger-developer/HTML-Preview/internal/preview"
	"os"
)

var version = "dev"
var revision = "unknown"

func main() {
	os.Exit(preview.Main(os.Args[1:], os.Environ(), os.Stdout, os.Stderr, version, revision, preview.NativeHost()))
}
