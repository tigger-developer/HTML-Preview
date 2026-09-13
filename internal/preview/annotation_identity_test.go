// ABOUTME: Verifies display-name capture at the command settings boundary.
// ABOUTME: Protects explicit precedence and strict invalid-name failure.
package preview

import (
	"strings"
	"testing"
)

func TestRT007_2_DisplayNameCapture(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     []string
		want    string
		invalid bool
	}{
		{"override", []string{"USER=daemon", "HTMLPREVIEW_USER_DISPLAY_NAME=  Taḋg  "}, "Taḋg", false},
		{"fallback", []string{"USER= reviewer ", "HTMLPREVIEW_USER_DISPLAY_NAME= "}, "reviewer", false},
		{"missing", nil, "", false},
		{"controls", []string{"USER=reviewer", "HTMLPREVIEW_USER_DISPLAY_NAME=bad\nname"}, "", true},
		{"oversized", []string{"USER=" + strings.Repeat("x", 129)}, "", true},
		{"invalid utf8", []string{"USER=bad\xff"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := settings(tc.env)
			if (err != nil) != tc.invalid || (err == nil && cfg.displayName != tc.want) {
				t.Fatalf("name=%q err=%v", cfg.displayName, err)
			}
		})
	}
}
