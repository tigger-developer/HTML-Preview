// ABOUTME: Starts the local document preview command with native OS boundaries.
// ABOUTME: Build identity is supplied by the repository build targets.
package main

import (
	"github.com/tigger-developer/HTML-Preview/internal/orgconvert"
	"github.com/tigger-developer/HTML-Preview/internal/preview"
	"os"
)

var version = "dev"
var revision = "unknown"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--internal-org-convert" {
		os.Exit(orgconvert.Worker(os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(preview.Main(os.Args[1:], os.Environ(), os.Stdout, os.Stderr, version, revision, hostForCommand()))
}
