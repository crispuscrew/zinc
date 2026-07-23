// Package render draws the picker into an in-memory RGBA image with a bundled bitmap font
// (pure Go, no cgo). Frame is a pure function of the model, the palette, and the pixel size,
// so the whole look is unit-testable without a display: zlg's Wayland layer only has to blit
// the pixels Frame returns into a shared-memory buffer. Software rendering is ample for a
// small, redraw-on-keystroke launcher window.
package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/crispuscrew/zinc/launcher/gui/internal/picker"
	"github.com/crispuscrew/zinc/launcher/gui/internal/theme"
)

const (
	marginX = 16 // left/right inset
	marginY = 14 // top inset for the prompt
	headerH = 44 // prompt line plus the separator beneath it
	rowH    = 22
	footerH = 30
	barW    = 3 // accent bar down the left of the selected row
	dotSize = 6 // running-app indicator
	nameX   = marginX + dotSize + 8
)

var face = basicfont.Face7x13

// Frame renders the picker at width x height pixels into a fresh, fully opaque RGBA image
// using pal. It never panics on an empty model or a tiny size (it clamps to at least one row).
func Frame(mdl *picker.Model, pal theme.Palette, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	fill(img, img.Bounds(), pal.BG)

	// Prompt line: an accent "> ", the query, then a block cursor.
	promptBaseline := marginY + face.Ascent
	drawText(img, marginX, promptBaseline, pal.Accent, ">")
	queryX := marginX + 2*face.Advance
	drawText(img, queryX, promptBaseline, pal.FG, mdl.Query())
	cursorX := queryX + runeLen(mdl.Query())*face.Advance
	fill(img, image.Rect(cursorX, marginY, cursorX+face.Advance, marginY+face.Height), pal.Accent)
	// Separator under the prompt (a subtle, theme-matched divider).
	fill(img, image.Rect(marginX, headerH-1, width-marginX, headerH), pal.SelBG)

	visible := mdl.Visible()
	listTop := headerH + 4
	listBottom := height - footerH
	if listBottom < listTop+rowH {
		listBottom = listTop + rowH
	}
	rows := (listBottom - listTop) / rowH
	if rows < 1 {
		rows = 1
	}

	if len(visible) == 0 {
		drawText(img, marginX, listTop+face.Ascent, pal.Dim, "no matches")
	} else {
		start := scrollStart(mdl.Cursor(), len(visible), rows)
		for offset := 0; offset < rows && start+offset < len(visible); offset++ {
			index := start + offset
			drawRow(img, pal, width, listTop+offset*rowH, visible[index], index == mdl.Cursor())
		}
	}

	drawFooter(img, pal, width, height, len(visible))
	return img
}

// drawRow renders one app row at pixel row-top y: a highlight band and accent bar when
// selected, an optional running dot, the name, and a dim description.
func drawRow(img *image.RGBA, pal theme.Palette, width, top int, app picker.App, selected bool) {
	nameColor := pal.FG
	if selected {
		fill(img, image.Rect(0, top, width, top+rowH), pal.SelBG)
		fill(img, image.Rect(0, top, barW, top+rowH), pal.Accent)
		nameColor = pal.SelFG
	}
	if app.Running {
		dotTop := top + (rowH-dotSize)/2
		fill(img, image.Rect(marginX, dotTop, marginX+dotSize, dotTop+dotSize), pal.Running)
	}
	baseline := top + (rowH-face.Height)/2 + face.Ascent
	drawText(img, nameX, baseline, nameColor, app.Name)
	if app.Description != "" {
		descX := nameX + (runeLen(app.Name)+2)*face.Advance
		drawText(img, descX, baseline, pal.Dim, app.Description)
	}
}

// drawFooter draws a divider and a hint line, with the match count right-aligned.
func drawFooter(img *image.RGBA, pal theme.Palette, width, height, count int) {
	top := height - footerH
	fill(img, image.Rect(marginX, top, width-marginX, top+1), pal.SelBG)
	baseline := top + (footerH-face.Height)/2 + face.Ascent
	drawText(img, marginX, baseline, pal.Dim, "up/down move   enter launch   esc quit")
	countText := fmt.Sprintf("%d shown", count)
	countX := width - marginX - runeLen(countText)*face.Advance
	drawText(img, countX, baseline, pal.Dim, countText)
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
