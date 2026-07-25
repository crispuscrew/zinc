package render

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
)

// fontSizePx is the pixel size of the rendered text (DPI is fixed at 72, so points == px).
const fontSizePx = 13

// The renderer uses one monospace face; its metrics (all in whole pixels) drive the layout.
var (
	face        font.Face
	faceAscent  int
	faceHeight  int
	faceAdvance int
	// LoadedFont describes the face in use, for diagnostics.
	LoadedFont string
)

func init() {
	applyFace(resolveFace(""))
}

// UseFont switches the renderer to the font file at path (.ttf/.otf); if path is empty or
// unusable it picks the best system Nerd Font, and failing that the built-in Go Mono. It is
// process-global: call it before rendering, and not while another overlay is live.
func UseFont(path string) {
	applyFace(resolveFace(path))
}

// resolveFace returns a face and a short description: the explicit path if it loads, else a
// system Nerd Font, else the bundled Go Mono.
func resolveFace(pathHint string) (font.Face, string) {
	if pathHint != "" {
		if built := faceFromFile(pathHint); built != nil {
			return built, filepath.Base(pathHint)
		}
	}
	if path := findNerdFont(); path != "" {
		if built := faceFromFile(path); built != nil {
			return built, filepath.Base(path) + " (system)"
		}
	}
	return builtinFace(), "Go Mono (builtin)"
}

func applyFace(newFace font.Face, description string) {
	face = newFace
	LoadedFont = description
	metrics := face.Metrics()
	faceAscent = metrics.Ascent.Round()
	faceHeight = metrics.Height.Round()
	if advance, ok := face.GlyphAdvance('M'); ok {
		faceAdvance = advance.Round()
	} else {
		faceAdvance = fontSizePx * 6 / 10
	}
	if faceAdvance < 1 {
		faceAdvance = 1 // never let a degenerate face divide-by-zero the layout math
	}
}

func faceFromFile(path string) font.Face {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return newFace(data)
}

// builtinFace is the antialiased Go Mono, falling back to the bundled bitmap font only if the
// embedded TTF ever fails to parse (it should not, so the fallback is just insurance).
func builtinFace() font.Face {
	if built := newFace(gomono.TTF); built != nil {
		return built
	}
	return basicfont.Face7x13
}

func newFace(ttf []byte) font.Face {
	parsed, err := opentype.Parse(ttf)
	if err != nil {
		return nil
	}
	built, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: fontSizePx, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil
	}
	return built
}

// preferredFamilies are well-known monospace coding families, best first, used to rank Nerd
// Font files when several are installed.
var preferredFamilies = []string{
	"jetbrainsmono", "firacode", "hack", "cascadiacode", "sourcecodepro",
	"iosevka", "meslo", "monaspace", "commitmono", "victormono", "ubuntumono",
	"inconsolata", "dejavusansmono", "liberationmono", "spacemono",
}

// findNerdFont walks the system font directories and returns the highest-scored Nerd Font
// file (a monospace, upright, regular weight), or "" if none is installed.
//
// It walks every directory to the end rather than stopping at the first good-enough file. A
// family's base and Mono variants sit side by side in one directory, with the base sorting
// first, so an early exit on score kept picking the base variant and never compared its Mono
// sibling. Font directories are small (a few hundred files) and this runs once per process,
// so the full walk costs single-digit milliseconds.
func findNerdFont() string {
	var best string
	bestScore := 0
	for _, dir := range fontDirs() {
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if score := scoreFont(strings.ToLower(entry.Name())); score > bestScore {
				bestScore, best = score, path
			}
			return nil
		})
	}
	return best
}

// scoreFont ranks a font filename: 0 for anything that is not an upright, regular-weight Nerd
// Font, and higher for the Mono (single-width) variants and the preferred coding families.
func scoreFont(name string) int {
	if !strings.Contains(name, "nerd") {
		return 0
	}
	if !strings.HasSuffix(name, ".ttf") && !strings.HasSuffix(name, ".otf") {
		return 0
	}
	// "symbols" excludes the standalone Symbols Nerd Font, which carries only the patched
	// glyph ranges and NO Latin letters - picking it as the UI text face renders blank tofu.
	for _, bad := range []string{"symbols", "italic", "oblique", "bold", "light", "thin", "medium", "semibold", "extrabold", "black", "propo"} {
		if strings.Contains(name, bad) {
			return 0
		}
	}
	score := 1
	compact := strings.ReplaceAll(name, " ", "")
	// Match the "NerdFontMono" variant token, not a bare "mono". A bare substring also matches
	// the family name of every monospace font (JetBrainsMono, DejaVuSansMono, ...), so it
	// scored the double-width base variant as if it were the single-width one - which then
	// broke the renderer's fixed-advance layout on any Nerd Font glyph.
	if strings.Contains(compact, "nerdfontmono") {
		score += 3 // the Mono Nerd Font variant keeps patched glyphs single-width
	}
	if strings.Contains(name, "regular") {
		score++
	}
	for index, family := range preferredFamilies {
		if strings.Contains(compact, family) {
			score += len(preferredFamilies) - index + 4
			break
		}
	}
	return score
}

func fontDirs() []string {
	var dirs []string
	if home := os.Getenv("HOME"); home != "" {
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			dirs = append(dirs, filepath.Join(xdg, "fonts"))
		}
		dirs = append(dirs, filepath.Join(home, ".local", "share", "fonts"), filepath.Join(home, ".fonts"))
	}
	value := os.Getenv("XDG_DATA_DIRS")
	if value == "" {
		value = "/usr/local/share:/usr/share"
	}
	for _, base := range strings.Split(value, ":") {
		if base != "" {
			dirs = append(dirs, filepath.Join(base, "fonts"))
		}
	}
	dirs = append(dirs, "/usr/share/fonts", "/usr/local/share/fonts")
	return dedup(dirs)
}

func dedup(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
