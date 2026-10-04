// ABOUTME: Verifies generated native services can resolve optional document tools.
// ABOUTME: Keeps converter directories ordered, bounded, and free of duplicates.
package main

import "testing"

func TestRT018_1_ServiceSearchPathIncludesSVGConverter(t *testing.T) {
	got := serviceSearchPathFor("/pandoc/bin/pandoc", "/svg/bin/rsvg-convert", "/svg/bin/rsvg-convert")
	want := "/pandoc/bin:/svg/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	if got != want {
		t.Fatalf("service PATH=%q, expected %q", got, want)
	}
}
