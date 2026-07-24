package render

import (
	"path/filepath"
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
		{"SymbolsNerdFontMono-Regular.ttf", false},        // symbols-only: no letters, must be rejected
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

// With no Nerd Font installed and no usable explicit path, resolveFace falls back to the
// built-in Go Mono rather than returning a nil face.
func TestResolveFace_FallsBackToBuiltin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_DATA_DIRS", t.TempDir())

	face, desc := resolveFace("")
	if face == nil || desc != "Go Mono (builtin)" {
		t.Fatalf("empty path with no nerd font: face=%v desc=%q, want a builtin Go Mono", face != nil, desc)
	}
	face, desc = resolveFace(filepath.Join(t.TempDir(), "nope.ttf"))
	if face == nil || desc != "Go Mono (builtin)" {
		t.Fatalf("bad path: face=%v desc=%q, want a builtin Go Mono", face != nil, desc)
	}
}
