package main

import "testing"

// ZLG_OPACITY accepts percentages and fractions, reporting unusable values.
func TestParseOpacity(t *testing.T) {
	valid := map[string]float64{
		"20":    0.2,
		"0.2":   0.2,
		"90":    0.9,
		"0.9":   0.9,
		"1":     1,
		"1.0":   1,
		"100":   1,
		"0":     0,
		" 35 ":  0.35,
		"12.5":  0.125,
		"0.125": 0.125,
	}
	for raw, want := range valid {
		got, valid := parseOpacity(raw)
		if !valid {
			t.Errorf("parseOpacity(%q) reported invalid, want %v", raw, want)
			continue
		}
		if got != want {
			t.Errorf("parseOpacity(%q) = %v, want %v", raw, got, want)
		}
	}
	for _, raw := range []string{"", "abc", "20%", "-1", "101", "1e9", "0.2.3"} {
		if got, valid := parseOpacity(raw); valid {
			t.Errorf("parseOpacity(%q) = %v, want it reported invalid", raw, got)
		}
	}
}

// An unusable ZLG_OPACITY leaves the overlay opaque.
func TestMenuOptions_Opacity(t *testing.T) {
	t.Setenv("ZLG_OPACITY", "20")
	if got := menuOptions().Opacity; got != 0.2 {
		t.Errorf("Opacity = %v for ZLG_OPACITY=20, want 0.2", got)
	}
	t.Setenv("ZLG_OPACITY", "0.2")
	if got := menuOptions().Opacity; got != 0.2 {
		t.Errorf("Opacity = %v for ZLG_OPACITY=0.2, want 0.2", got)
	}
	t.Setenv("ZLG_OPACITY", "not-a-number")
	if got := menuOptions().Opacity; got != 0 {
		t.Errorf("Opacity = %v for an unusable value, want 0 (opaque)", got)
	}
}

// The remaining env knobs reach menu.Options; unset values leave the defaults.
func TestMenuOptions_Flags(t *testing.T) {
	for _, name := range []string{"ZLG_OPACITY", "ZLG_NO_ANIM", "ZLG_DEBUG", "ZLG_FONT"} {
		t.Setenv(name, "")
	}
	opts := menuOptions()
	if opts.NoAnim || opts.Debug || opts.FontPath != "" || opts.Opacity != 0 {
		t.Errorf("unset environment should leave the defaults, got %+v", opts)
	}
	if opts.AppID != "zinc.launcher" {
		t.Errorf("AppID = %q, want zinc.launcher (compositors match window rules on it)", opts.AppID)
	}
	t.Setenv("ZLG_NO_ANIM", "1")
	t.Setenv("ZLG_DEBUG", "1")
	t.Setenv("ZLG_FONT", "/usr/share/fonts/x.ttf")
	opts = menuOptions()
	if !opts.NoAnim || !opts.Debug || opts.FontPath != "/usr/share/fonts/x.ttf" {
		t.Errorf("env knobs did not reach the options, got %+v", opts)
	}
}
