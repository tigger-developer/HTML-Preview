// ABOUTME: Embeds presentation sources, font bytes, and distribution notices.
// ABOUTME: Runtime asset lookup is independent of executable and source paths.
package htmlpreview

import "embed"

// Assets is the immutable build-time asset bundle.
//
//go:embed assets/fonts assets/web assets/pandoc LICENSE THIRD_PARTY_NOTICES.md
var Assets embed.FS
