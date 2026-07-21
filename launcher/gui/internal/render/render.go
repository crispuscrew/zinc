// Package render draws the picker into an in-memory RGBA image with a bundled bitmap font
// (pure Go, no cgo). Frame is a pure function of the model and the pixel size, so the whole
// look is unit-testable without a display: zlg's Wayland layer only has to blit the pixels
// Frame returns into a shared-memory buffer. Software rendering is ample for a small,
// redraw-on-keystroke launcher window.
package render

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/crispuscrew/zinc/launcher/gui/internal/picker"
)

// A dark palette that reads on any compositor background (the window is opaque).
var (
	colorBG      = color.RGBA{0x1e, 0x1e, 0x2e, 0xff}
	colorFG      = color.RGBA{0xcd, 0xd6, 0xf4, 0xff}
	colorPrompt  = color.RGBA{0x89, 0xb4, 0xfa, 0xff}
	colorDim     = color.RGBA{0x6c, 0x70, 0x86, 0xff}
	colorSelBG   = color.RGBA{0x45, 0x47, 0x5a, 0xff}
	colorSelFG   = color.RGBA{0xf5, 0xe0, 0xdc, 0xff}
	colorRunning = color.RGBA{0xa6, 0xe3, 0xa1, 0xff}
)

const (
	padX    = 8
	padY    = 6
	headerH = 26 // the prompt line's block height
	rowH    = 18
	dotSize = 6
)

var face = basicfont.Face7x13

// Frame renders the picker at width x height pixels into a fresh, fully opaque RGBA image.
// It never panics on an empty model or a tiny size (it clamps to at least one row).
func Frame(mdl *picker.Model, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	fill(img, img.Bounds(), colorBG)

	// Prompt line: "> " then the query, then a block cursor.
	drawText(img, padX, padY+face.Ascent, colorPrompt, "> ")
	queryX := padX + 2*face.Advance
	drawText(img, queryX, padY+face.Ascent, colorFG, mdl.Query())
	cursorX := queryX + runeLen(mdl.Query())*face.Advance
	fill(img, image.Rect(cursorX, padY, cursorX+face.Advance, padY+face.Height), colorFG)

	visible := mdl.Visible()
	if len(visible) == 0 {
		drawText(img, padX, headerH+padY+face.Ascent, colorDim, "no matches")
		return img
	}

	rows := (height - headerH) / rowH
	if rows < 1 {
		rows = 1
	}
	start := scrollStart(mdl.Cursor(), len(visible), rows)
	for offset := 0; offset < rows && start+offset < len(visible); offset++ {
		index := start + offset
		drawRow(img, width, headerH+offset*rowH, visible[index], index == mdl.Cursor())
	}
	return img
}

// drawRow renders one app row at pixel row-top y: an optional running dot, the name, and a
// dim description, with the selected row on a highlight band.
func drawRow(img *image.RGBA, width, top int, app picker.App, selected bool) {
	foreground := colorFG
	if selected {
		fill(img, image.Rect(0, top, width, top+rowH), colorSelBG)
		foreground = colorSelFG
	}
	if app.Running {
		dotTop := top + (rowH-dotSize)/2
		fill(img, image.Rect(padX, dotTop, padX+dotSize, dotTop+dotSize), colorRunning)
	}
	baseline := top + (rowH-face.Height)/2 + face.Ascent
	nameX := padX + 12
	drawText(img, nameX, baseline, foreground, app.Name)
	if app.Description != "" {
		descX := nameX + (runeLen(app.Name)+2)*face.Advance
		drawText(img, descX, baseline, colorDim, app.Description)
	}
}

// scrollStart returns the first visible row so the cursor stays on screen (it scrolls only
// once the cursor would fall past the last visible row).
func scrollStart(cursor, total, rows int) int {
	if total <= rows {
		return 0
	}
	start := 0
	if cursor >= rows {
		start = cursor - rows + 1
	}
	if start+rows > total {
		start = total - rows
	}
	if start < 0 {
		start = 0
	}
	return start
}

func drawText(img *image.RGBA, x, baseline int, col color.Color, text string) {
	drawer := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	drawer.DrawString(text)
}

func fill(img *image.RGBA, rect image.Rectangle, col color.Color) {
	draw.Draw(img, rect, image.NewUniform(col), image.Point{}, draw.Src)
}

func runeLen(text string) int { return len([]rune(text)) }
