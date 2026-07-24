package render

import (
	"strings"
	"testing"
)

func TestScoreFont(t *testing.T) {
	cases := []struct {
		name         string
		wantSelected bool
	}{
		{"JetBrainsMonoNerdFontMono-Regular.ttf", true},
		{"HackNerdFont-Regular.ttf", true},
		{"SymbolsNerdFontMono-Regular.ttf", true},
		{"DejaVuSansMono.ttf", false},                     // not a Nerd Font
		{"FiraCode-Regular.otf", false},                   // not a Nerd Font
		{"JetBrainsMonoNerdFont-Bold.ttf", false},         // wrong weight
		{"JetBrainsMonoNerdFont-Italic.ttf", false},       // italic
		{"JetBrainsMonoNerdFontPropo-Regular.ttf", false}, // proportional, not monospace
		{"JetBrainsMonoNerdFontMono-Regular.png", false},  // not a font file
	}
	for _, tc := range cases {
		selected := scoreFont(strings.ToLower(tc.name)) > 0
		if selected != tc.wantSelected {
			t.Errorf("scoreFont(%q) selected=%v, want %v", tc.name, selected, tc.wantSelected)
		}
	}

	// A preferred family's Mono variant should outrank a generic Nerd Font.
	preferred := scoreFont("jetbrainsmononerdfontmono-regular.ttf")
	generic := scoreFont("somethingnerdfont-regular.ttf")
	if preferred <= generic {
		t.Errorf("preferred mono nerd font (%d) should outrank a generic one (%d)", preferred, generic)
	}
}
