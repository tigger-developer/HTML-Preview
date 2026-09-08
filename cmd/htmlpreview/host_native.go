//go:build !htmlpreview_test_desktop

// ABOUTME: Binds the installed command to its native operating-system adapter.
// ABOUTME: Production builds expose no environment or flag for replacing it.
package main

import "github.com/tigger-developer/HTML-Preview/internal/preview"

func hostForCommand() preview.Host { return preview.NativeHost() }
