// Package icons resolves an app icon to a small decoded raster, in pure Go. An icon spec is
// either an absolute path to an image file or a freedesktop icon name (e.g. "firefox") that
// is looked up in the standard icon-theme directories. Only raster formats the standard
// library decodes are supported (PNG, JPEG, GIF) - there is no cgo-free SVG rasterizer, so
// SVG-only icons resolve to nil and the caller simply draws no icon. Resolution is best
// effort: any failure returns nil rather than an error.
package icons

import (
	"bytes"
	"image"
	_ "image/gif"  // register decoders for image.Decode
	_ "image/jpeg" // register decoders for image.Decode
	_ "image/png"  // register decoders for image.Decode
	"io"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
)

const (
	// maxIconBytes caps how much of an icon file is read; icons are tiny, and this stops a
	// partly-untrusted config pointing Icon at a huge file.
	maxIconBytes = 8 << 20
	// maxIconPixels rejects an image whose declared dimensions would allocate too much; a
	// crafted small file can declare enormous width x height and OOM the decoder otherwise.
	maxIconPixels = 4 << 20
)

// searchSizes are the icon-theme size buckets tried, best first (larger scales down cleanly).
var searchSizes = []string{"64x64", "48x48", "128x128", "96x96", "256x256", "32x32", "24x24", "16x16"}

// searchThemes are the icon themes tried; hicolor is the freedesktop fallback every theme
// inherits, so it is the most reliable.
var searchThemes = []string{"hicolor", "Adwaita", "breeze", "gnome", "Papirus"}

// Resolve returns the icon for spec scaled to size x size pixels, or nil if it cannot be found
// or decoded. spec is an absolute file path or a freedesktop icon name.
func Resolve(spec string, size int) *image.RGBA {
	if spec == "" || size <= 0 {
		return nil
	}
	path := spec
	if !filepath.IsAbs(spec) {
		path = lookup(spec)
	}
	if path == "" {
		return nil
	}
	source := decode(path)
	if source == nil {
		return nil
	}
	return scaleTo(source, size)
}

// lookup finds a PNG-or-other-raster icon file for a freedesktop icon name, searching the
// theme directories (apps category) then the flat pixmaps directories. The scalable (SVG)
// theme dirs are intentionally skipped.
func lookup(name string) string {
	for _, base := range iconBaseDirs() {
		for _, theme := range searchThemes {
			themeDir := filepath.Join(base, theme)
			if !isDir(themeDir) {
				continue // skip the theme's whole size x category grid when it is not installed
			}
			for _, size := range searchSizes {
				for _, category := range []string{"apps", "categories"} {
					candidate := filepath.Join(themeDir, size, category, name+".png")
					if isFile(candidate) {
						return candidate
					}
				}
			}
		}
	}
	for _, base := range pixmapDirs() {
		for _, ext := range []string{".png", ".jpg", ".gif"} {
			candidate := filepath.Join(base, name+ext)
			if isFile(candidate) {
				return candidate
			}
		}
	}
	return ""
}

// iconBaseDirs returns the icon-theme roots, per the freedesktop icon-theme spec: the user
// dirs first, then the system XDG_DATA_DIRS.
func iconBaseDirs() []string {
	var dirs []string
	if home := os.Getenv("HOME"); home != "" {
		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			dirs = append(dirs, filepath.Join(xdgData, "icons"))
		} else {
			dirs = append(dirs, filepath.Join(home, ".local", "share", "icons"))
		}
		dirs = append(dirs, filepath.Join(home, ".icons"))
	}
	for _, dataDir := range dataDirs() {
		dirs = append(dirs, filepath.Join(dataDir, "icons"))
	}
	return dirs
}

func pixmapDirs() []string {
	var dirs []string
	for _, dataDir := range dataDirs() {
		dirs = append(dirs, filepath.Join(dataDir, "pixmaps"))
	}
	return dirs
}

func dataDirs() []string {
	value := os.Getenv("XDG_DATA_DIRS")
	if value == "" {
		value = "/usr/local/share:/usr/share"
	}
	var dirs []string
	for _, dir := range strings.Split(value, ":") {
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// decode reads and decodes an icon file, bounded because the path comes from partly-untrusted
// app config: it requires a REGULAR file (so a FIFO cannot block the open, and dirs/devices
// are rejected), caps the bytes read, rejects images whose declared dimensions exceed the
// pixel budget (a small file can claim huge dimensions and OOM the decoder), and recovers from
// a decoder panic. Any of these returns nil, which the caller renders as no icon.
func decode(path string) (result image.Image) {
	defer func() {
		if recover() != nil {
			result = nil
		}
	}()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxIconBytes))
	if err != nil {
		return nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > maxIconPixels {
		return nil
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return decoded
}

// scaleTo returns source resized to size x size as a premultiplied RGBA (what the renderer
// blits), preserving the source's alpha so transparent icon backgrounds composite cleanly.
func scaleTo(source image.Image, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), source, source.Bounds(), xdraw.Over, nil)
	return dst
}
