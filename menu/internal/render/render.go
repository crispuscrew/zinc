// Package render draws the picker into an in-memory RGBA image with an antialiased monospace
// font (pure Go, no cgo; a system Nerd Font when one is installed, else the bundled Go Mono -
// see font.go). Frame is a pure function of the model, the palette, the per-frame view state,
// and the pixel size, so the whole look is unit-testable without a display: the menu's Wayland
// layer only has to blit the pixels Frame returns into a shared-memory buffer. Software
// rendering is ample for a small, redraw-on-keystroke menu window.
//
// Frame returns PREMULTIPLIED-alpha pixels (what Wayland surfaces expect), so translucency,
// the rounded corners, and the fade-in composite correctly over the desktop.
package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/crispuscrew/zinc/menu/internal/picker"
	"github.com/crispuscrew/zinc/menu/internal/theme"
)

const (
	marginX      = 18 // left/right inset
	marginY      = 16 // top inset for the prompt
	headerH      = 46 // prompt line plus the separator beneath it
	rowH         = 22
	footerH      = 30
	errorH       = 24 // launch-error banner, shown above the footer when present
	barW         = 3  // accent bar down the left of the selected/error row
	dotSize      = 6  // running-app indicator
	nameX        = marginX + dotSize + 8
	cornerRadius = 12
	descGap      = 2  // spaces between the name column and the description
	descColMax   = 18 // cap on the name-column width used to line descriptions up
	iconGap      = 8  // space between the icon column and the name
)

// IconSize is the pixel size icons are scaled to and drawn at; the caller pre-scales to this.
const IconSize = 16

// View is the transient, per-frame state the renderer needs beyond the model: the entrance
// fade (0..1), the steady-state background opacity (0..1), and an optional launch error to
// surface in a banner.
type View struct {
	Prompt  string
	Footer  string // the hint line at the bottom (default "up/down move   enter select   esc quit")
	Fade    float64
	Opacity float64
	Error   string
}

// Frame renders the picker at width x height pixels into a fresh RGBA image using pal and
// view. It never panics on an empty model or a tiny size (it clamps to at least one row).
func Frame(mdl *picker.Model, pal theme.Palette, view View, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	fill(img, img.Bounds(), pal.BG)

	// Prompt line: the (accent) prompt, the query, then a block cursor.
	prompt := view.Prompt
	if prompt == "" {
		prompt = "> "
	}
	promptBaseline := marginY + faceAscent
	drawText(img, marginX, promptBaseline, pal.Accent, prompt)
	queryX := marginX + runeLen(prompt)*faceAdvance
	drawText(img, queryX, promptBaseline, pal.FG, mdl.Query())
	cursorX := queryX + runeLen(mdl.Query())*faceAdvance
	fill(img, image.Rect(cursorX, marginY, cursorX+faceAdvance, marginY+faceHeight), pal.Accent)
	// Separator under the prompt (a subtle, theme-matched divider).
	fill(img, image.Rect(marginX, headerH-1, width-marginX, headerH), pal.SelBG)

	visible := mdl.Visible()
	listTop := headerH + 4
	listBottom := height - footerH
	if view.Error != "" {
		listBottom -= errorH
	}
	if listBottom < listTop+rowH {
		listBottom = listTop + rowH
	}
	rows := (listBottom - listTop) / rowH
	if rows < 1 {
		rows = 1
	}

	if len(visible) == 0 {
		drawText(img, marginX, listTop+faceAscent, pal.Dim, "no matches")
	} else {
		// Group into sections only when idle; typing flattens to the ranked list.
		grouping := mdl.Query() == "" && anyGroup(visible)
		display := buildRows(visible, grouping)
		descCol := descColumn(visible)
		iconCol := anyIcon(visible)
		start := scrollStart(rowForItem(display, mdl.Cursor()), len(display), rows)
		for offset := 0; offset < rows && start+offset < len(display); offset++ {
			row := display[start+offset]
			y := listTop + offset*rowH
			if row.isHeader {
				drawHeader(img, pal, y, row.header)
			} else {
				drawRow(img, pal, width, y, visible[row.item], descCol, iconCol, row.item == mdl.Cursor())
			}
		}
	}

	if view.Error != "" {
		drawError(img, pal, width, height, view.Error)
	}
	drawFooter(img, pal, width, height, view.Footer, len(visible))

	finish(img, view.Opacity, view.Fade)
	return img
}

// descColumn returns the column (in characters) at which descriptions line up: the longest
// visible name, capped, so short names share a tidy column and long names just push past it.
func descColumn(visible []picker.App) int {
	longest := 0
	for _, app := range visible {
		if n := runeLen(app.Name); n > longest {
			longest = n
		}
	}
	if longest > descColMax {
		longest = descColMax
	}
	return longest
}

// drawRow renders one app row at pixel row-top y: a highlight band and accent bar when
// selected, an icon (when the column is active) or a running dot, the name, and a dim
// description aligned to descCol. iconCol keeps the name column aligned across rows even for
// rows whose own icon is missing.
func drawRow(img *image.RGBA, pal theme.Palette, width, top int, app picker.App, descCol int, iconCol, selected bool) {
	nameColor := pal.FG
	if selected {
		fill(img, image.Rect(0, top, width, top+rowH), pal.SelBG)
		fill(img, image.Rect(0, top, barW, top+rowH), pal.Accent)
		nameColor = pal.SelFG
	}
	textX := nameX
	if iconCol {
		textX = marginX + IconSize + iconGap
		if app.Icon != nil {
			iconTop := top + (rowH-IconSize)/2
			draw.Draw(img, image.Rect(marginX, iconTop, marginX+IconSize, iconTop+IconSize), app.Icon, image.Point{}, draw.Over)
		}
		if app.Running {
			// a small badge on the icon cell's bottom-right corner
			dotX := marginX + IconSize - dotSize
			dotY := top + (rowH+IconSize)/2 - dotSize
			fill(img, image.Rect(dotX, dotY, dotX+dotSize, dotY+dotSize), pal.Running)
		}
	} else if app.Running {
		dotTop := top + (rowH-dotSize)/2
		fill(img, image.Rect(marginX, dotTop, marginX+dotSize, dotTop+dotSize), pal.Running)
	}
	baseline := top + (rowH-faceHeight)/2 + faceAscent
	drawText(img, textX, baseline, nameColor, app.Name)
	if app.Description != "" {
		column := descCol
		if n := runeLen(app.Name); n > column {
			column = n // a name past the column pushes its own description along
		}
		descX := textX + (column+descGap)*faceAdvance
		drawText(img, descX, baseline, pal.Dim, app.Description)
	}
}

// anyIcon reports whether any visible item has a resolved icon (so the icon column shows).
func anyIcon(visible []picker.App) bool {
	for _, app := range visible {
		if app.Icon != nil {
			return true
		}
	}
	return false
}

// displayRow is one row in the rendered list: either a section header or an item (an index
// into the visible slice).
type displayRow struct {
	header   string
	item     int
	isHeader bool
}

// buildRows expands the visible items into rendered rows, inserting a section header before
// each new group when grouping is on. Items sharing a group should be adjacent in visible (the
// caller orders them) so a group's header renders once.
func buildRows(visible []picker.App, grouping bool) []displayRow {
	if !grouping {
		rows := make([]displayRow, len(visible))
		for index := range visible {
			rows[index] = displayRow{item: index}
		}
		return rows
	}
	var rows []displayRow
	current := "\x00" // a sentinel no real group equals, so the first item always opens a header
	for index, app := range visible {
		if app.Group != current {
			current = app.Group
			name := app.Group
			if name == "" {
				name = "Other"
			}
			rows = append(rows, displayRow{header: name, isHeader: true})
		}
		rows = append(rows, displayRow{item: index})
	}
	return rows
}

// rowForItem returns the display-row index of a visible item, so scrolling can keep the
// selected item on screen.
func rowForItem(rows []displayRow, item int) int {
	for index, row := range rows {
		if !row.isHeader && row.item == item {
			return index
		}
	}
	return 0
}

// anyGroup reports whether any visible item carries a group.
func anyGroup(visible []picker.App) bool {
	for _, app := range visible {
		if app.Group != "" {
			return true
		}
	}
	return false
}

// drawHeader draws a section-header row: the group name in the accent color.
func drawHeader(img *image.RGBA, pal theme.Palette, top int, name string) {
	baseline := top + (rowH-faceHeight)/2 + faceAscent
	drawText(img, marginX, baseline, pal.Accent, name)
}

// drawError draws a banner just above the footer: an error-colored bar and message, so a
// failed launch is reported in the window instead of on the terminal.
func drawError(img *image.RGBA, pal theme.Palette, width, height int, message string) {
	top := height - footerH - errorH
	fill(img, image.Rect(0, top, width, top+errorH), pal.SelBG)
	fill(img, image.Rect(0, top, barW, top+errorH), pal.Error)
	baseline := top + (errorH-faceHeight)/2 + faceAscent
	maxChars := (width - nameX - marginX) / faceAdvance
	drawText(img, nameX, baseline, pal.Error, truncate(message, maxChars))
}

// drawFooter draws a divider and the hint line, with the match count right-aligned.
func drawFooter(img *image.RGBA, pal theme.Palette, width, height int, hint string, count int) {
	if hint == "" {
		hint = "up/down move   enter select   esc quit"
	}
	top := height - footerH
	fill(img, image.Rect(marginX, top, width-marginX, top+1), pal.SelBG)
	baseline := top + (footerH-faceHeight)/2 + faceAscent
	drawText(img, marginX, baseline, pal.Dim, hint)
	countText := fmt.Sprintf("%d shown", count)
	countX := width - marginX - runeLen(countText)*faceAdvance
	drawText(img, countX, baseline, pal.Dim, countText)
}

// finish turns the opaque drawing into the final surface: it rounds the corners and applies
// the steady-state opacity and the entrance fade, writing premultiplied alpha so the result
// composites correctly over whatever is behind the overlay.
func finish(img *image.RGBA, opacity, fade float64) {
	global := clamp01(opacity) * clamp01(fade)
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			alpha := global * cornerCoverage(x, y, width, height, cornerRadius)
			offset := img.PixOffset(x, y)
			img.Pix[offset+0] = scale(img.Pix[offset+0], alpha)
			img.Pix[offset+1] = scale(img.Pix[offset+1], alpha)
			img.Pix[offset+2] = scale(img.Pix[offset+2], alpha)
			img.Pix[offset+3] = scale(0xff, alpha)
		}
	}
}

// cornerCoverage returns how much of the pixel lies inside the rounded rectangle: 1 in the
// body, 0 outside a corner, and a fractional value on a corner's antialiased edge.
func cornerCoverage(x, y, width, height, radius int) float64 {
	if radius <= 0 {
		return 1
	}
	var centerX, centerY float64
	switch {
	case x < radius && y < radius:
		centerX, centerY = float64(radius), float64(radius)
	case x >= width-radius && y < radius:
		centerX, centerY = float64(width-radius), float64(radius)
	case x < radius && y >= height-radius:
		centerX, centerY = float64(radius), float64(height-radius)
	case x >= width-radius && y >= height-radius:
		centerX, centerY = float64(width-radius), float64(height-radius)
	default:
		return 1
	}
	deltaX := float64(x) + 0.5 - centerX
	deltaY := float64(y) + 0.5 - centerY
	coverage := float64(radius) - math.Sqrt(deltaX*deltaX+deltaY*deltaY) + 0.5
	return clamp01(coverage)
}

// scale multiplies an 8-bit channel by an alpha fraction (used to premultiply).
func scale(channel uint8, alpha float64) uint8 {
	return uint8(float64(channel)*alpha + 0.5)
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
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

func truncate(text string, maxChars int) string {
	runes := []rune(text)
	if maxChars < 0 {
		maxChars = 0
	}
	if len(runes) <= maxChars {
		return text
	}
	if maxChars <= 3 {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-3]) + "..."
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
