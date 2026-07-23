// Package theme resolves the launcher's color palette. It prefers the system appearance from
// the XDG desktop portal (org.freedesktop.appearance: the dark/light preference and the
// accent color) so zlg matches the rest of the desktop, and falls back to a built-in palette
// when no portal is reachable. It speaks D-Bus through the pure-Go godbus client, so zlg
// stays a static, cgo-free binary.
package theme

import (
	"context"
	"image/color"
	"time"

	"github.com/godbus/dbus/v5"
)

// Palette is the set of colors the renderer draws with.
type Palette struct {
	BG      color.RGBA // window background
	FG      color.RGBA // primary text (app names, the typed query)
	Dim     color.RGBA // secondary text (descriptions, the footer hint)
	Accent  color.RGBA // prompt, cursor, the selected-row marker
	SelBG   color.RGBA // selected-row background band
	SelFG   color.RGBA // selected-row text
	Running color.RGBA // running-app indicator dot
}

// Dark is the built-in dark palette (Catppuccin Mocha).
func Dark() Palette {
	return Palette{
		BG:      rgb(0x1e, 0x1e, 0x2e),
		FG:      rgb(0xcd, 0xd6, 0xf4),
		Dim:     rgb(0x7f, 0x84, 0x9c),
		Accent:  rgb(0x89, 0xb4, 0xfa),
		SelBG:   rgb(0x31, 0x32, 0x44),
		SelFG:   rgb(0xf5, 0xe0, 0xdc),
		Running: rgb(0xa6, 0xe3, 0xa1),
	}
}

// Light is the built-in light palette (Catppuccin Latte).
func Light() Palette {
	return Palette{
		BG:      rgb(0xef, 0xf1, 0xf5),
		FG:      rgb(0x4c, 0x4f, 0x69),
		Dim:     rgb(0x8c, 0x8f, 0xa1),
		Accent:  rgb(0x1e, 0x66, 0xf5),
		SelBG:   rgb(0xdc, 0xe0, 0xe8),
		SelFG:   rgb(0x4c, 0x4f, 0x69),
		Running: rgb(0x40, 0xa0, 0x2b),
	}
}

// Detect returns a palette from the system appearance, falling back to Dark(). It reads the
// portal's color-scheme (dark vs light base) and accent-color; anything missing, erroring, or
// slow just leaves the built-in value in place, so it never blocks the launcher for long.
func Detect() Palette {
	conn, err := dbus.SessionBus()
	if err != nil {
		return Dark()
	}
	// color-scheme: 1 = prefer dark, 2 = prefer light, 0 = no preference (treated as dark).
	pal := Dark()
	if scheme, ok := readUint(conn, "color-scheme"); ok && scheme == 2 {
		pal = Light()
	}
	if accent, ok := readAccent(conn); ok {
		pal.Accent = accent
	}
	return pal
}

// readSetting reads one org.freedesktop.appearance key from the Settings portal. It returns
// the unwrapped value, guarded by a short timeout so a missing or wedged portal can't stall
// startup.
func readSetting(conn *dbus.Conn, key string) (interface{}, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	obj := conn.Object("org.freedesktop.portal.Desktop", dbus.ObjectPath("/org/freedesktop/portal/desktop"))
	var out dbus.Variant
	err := obj.CallWithContext(ctx, "org.freedesktop.portal.Settings.Read", 0,
		"org.freedesktop.appearance", key).Store(&out)
	if err != nil {
		return nil, false
	}
	return out.Value(), true
}

func readUint(conn *dbus.Conn, key string) (uint32, bool) {
	value, ok := readSetting(conn, key)
	if !ok {
		return 0, false
	}
	switch number := unwrap(value).(type) {
	case uint32:
		return number, true
	case int32:
		return uint32(number), true
	case uint8:
		return uint32(number), true
	}
	return 0, false
}

func readAccent(conn *dbus.Conn) (color.RGBA, bool) {
	value, ok := readSetting(conn, "accent-color")
	if !ok {
		return color.RGBA{}, false
	}
	// accent-color is a (ddd) struct of doubles in 0..1, which godbus hands back as a slice.
	sequence, ok := unwrap(value).([]interface{})
	if !ok || len(sequence) != 3 {
		return color.RGBA{}, false
	}
	red, redOK := sequence[0].(float64)
	green, greenOK := sequence[1].(float64)
	blue, blueOK := sequence[2].(float64)
	if !redOK || !greenOK || !blueOK {
		return color.RGBA{}, false
	}
	// The portal returns (-1,-1,-1) when the user has set no accent color.
	if red < 0 || green < 0 || blue < 0 {
		return color.RGBA{}, false
	}
	return color.RGBA{channel(red), channel(green), channel(blue), 0xff}, true
}

// unwrap peels any nested D-Bus variants (the Settings portal double-wraps its return value)
// down to the concrete Go value.
func unwrap(value interface{}) interface{} {
	for {
		inner, ok := value.(dbus.Variant)
		if !ok {
			return value
		}
		value = inner.Value()
	}
}

func channel(fraction float64) uint8 {
	switch {
	case fraction <= 0:
		return 0
	case fraction >= 1:
		return 255
	default:
		return uint8(fraction*255 + 0.5)
	}
}

func rgb(red, green, blue uint8) color.RGBA { return color.RGBA{red, green, blue, 0xff} }
