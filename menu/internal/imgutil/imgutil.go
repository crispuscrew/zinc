// Package imgutil is the menu's shared image helper: one hardened decode plus two scalers.
// The decode is bounded and panic-safe because the paths come from partly-untrusted config
// (an app's Icon, a wallpaper directory), so a hostile or corrupt file must never block, OOM,
// or crash the process. Square resizes to a fixed square (icons); Fit resizes to fit a box
// while preserving aspect ratio (thumbnails). The icons and thumbs packages share it so the
// decode-safety rules live in exactly one place.
package imgutil

import (
	"bytes"
	"image"
	_ "image/gif"  // register the GIF decoder for image.Decode
	_ "image/jpeg" // register the JPEG decoder for image.Decode
	_ "image/png"  // register the PNG decoder for image.Decode
	"io"
	"math"
	"os"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register the pure-Go WebP decoder (common for wallpapers)
)

// Decode reads and decodes an image file, bounded because the path may come from
// partly-untrusted config: it requires a REGULAR file (so a FIFO cannot block the open, and
// directories and devices are rejected), caps the bytes read at maxBytes, rejects an image
// whose declared dimensions exceed maxPixels (a small file can claim enormous width x height
// and OOM the decoder), and recovers from a decoder panic. Any failure returns nil, which
// callers render as no image.
func Decode(path string, maxBytes, maxPixels int64) (result image.Image) {
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
	data, err := io.ReadAll(io.LimitReader(file, maxBytes))
	if err != nil {
		return nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > maxPixels {
		return nil
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return decoded
}

// Square returns source resized to size x size as premultiplied RGBA, preserving alpha so
// transparent backgrounds composite cleanly. It is what icons draw, since they sit in a fixed
// square cell. It returns nil for a nil source or a non-positive size.
func Square(source image.Image, size int) *image.RGBA {
	if source == nil || size <= 0 {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), source, source.Bounds(), xdraw.Over, nil)
	return dst
}

// Fit returns source scaled to fit within boxW x boxH preserving aspect ratio (letterboxed,
// never cropped or stretched), as premultiplied RGBA sized to the fitted dimensions. It is
// what thumbnails draw, since their source aspect ratios vary. A source already smaller than
// the box is kept at native size rather than upscaled (upscaling a small image only blurs it).
// It returns nil for a nil source or a non-positive box.
func Fit(source image.Image, boxW, boxH int) *image.RGBA {
	if source == nil || boxW <= 0 || boxH <= 0 {
		return nil
	}
	srcW := source.Bounds().Dx()
	srcH := source.Bounds().Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil
	}
	scale := math.Min(float64(boxW)/float64(srcW), float64(boxH)/float64(srcH))
	if scale > 1 {
		scale = 1 // never upscale; show a small image at native size, centered by the caller
	}
	dstW := int(float64(srcW)*scale + 0.5)
	dstH := int(float64(srcH)*scale + 0.5)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), source, source.Bounds(), xdraw.Over, nil)
	return dst
}
