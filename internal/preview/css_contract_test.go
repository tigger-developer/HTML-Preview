// ABOUTME: Checks resource admission inside conditional authored CSS at the CLI boundary.
// ABOUTME: Keeps conditional layout while rejecting resource URLs hidden in rule preludes.
package preview

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRT008_6_ConditionalCSSResources(t *testing.T) {
	root := t.TempDir()
	png := rasterFixture(t)
	source(t, root, "pixel.png", string(png))
	input := `<style>
@supports (background: url(pixel.png)) {.local {color:navy}}
@supports (background: u\72l(https://example.invalid/probe)) {.remote {color:red}}
@supports (display:grid) {@media (min-width: 20rem) {.static {display:grid}}}
</style><p class="local static">Conditional layout</p>`
	r := run(t, root, nil, source(t, root, "conditional.html", input))
	success(t, r, 1)
	styles := ""
	for _, n := range nodes(r.pages[0], "style") {
		styles += textOf(n)
	}
	if strings.Contains(styles, "example.invalid") || strings.Contains(styles, ".remote") {
		t.Error("unadmitted conditional resource survived")
	}
	if !strings.Contains(styles, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png)) {
		t.Error("conditional raster did not retain its original path context")
	}
	if !strings.Contains(styles, ".local") || !strings.Contains(styles, ".static") || !strings.Contains(styles, "display:grid") {
		t.Error("permitted conditional layout was removed")
	}
}

func TestRT008_6_CSSQuotedRasterFilename(t *testing.T) {
	root := t.TempDir()
	png := rasterFixture(t)
	source(t, root, "'leading.png", string(png))
	input := `<style>.quoted {background-image:url("'leading.png");color:navy}</style><p class="quoted">Quoted filename</p>`
	r := run(t, root, nil, source(t, root, "quoted.html", input))
	success(t, r, 1)
	style := textOf(nodes(r.pages[0], "style")[0])
	if !strings.Contains(style, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png)) {
		t.Fatalf("quoted filename changed during CSS resource resolution: %s", r.stderr)
	}
}
