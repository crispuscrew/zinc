package schema

import "testing"

// Every one of these was measured against OVMF. They matter because the failure mode is
// silent: a size the emulated display cannot describe does not error, it comes up at
// 1280x800 with nothing logged, which reads as "the setting did nothing".
func TestGuestDisplay_MatchesWhatTheFirmwareAccepts(t *testing.T) {
	cases := []struct {
		width, height int
		ok            bool
		note          string
	}{
		{1920, 1080, true, "the common case"},
		{2560, 1440, true, "fits the display's default 16 MiB"},
		{2560, 1600, true, "still under 16 MiB"},
		{3200, 1800, true, "needs more memory, clock still fits at 75 Hz"},
		{3840, 2160, true, "4K: needs both a bigger framebuffer and a slower EDID clock"},
		{3840, 2400, true, "taller than 4K, both sides under the field width"},
		{4096, 2160, false, "4096 does not fit the EDID's 12-bit active-pixel field"},
		{5120, 2880, false, "5K: too wide, whatever the clock"},
		{7680, 4320, false, "8K: no standard refresh rate keeps the clock in range"},
	}
	for _, tc := range cases {
		mode, ok := GuestDisplay(tc.width, tc.height)
		if ok != tc.ok {
			t.Errorf("GuestDisplay(%d, %d) ok = %v, want %v (%s)", tc.width, tc.height, ok, tc.ok, tc.note)
			continue
		}
		if !ok {
			continue
		}
		if need := int64(tc.width) * int64(tc.height) * 4; mode.VideoMemBytes < need {
			t.Errorf("%dx%d got %d bytes of video memory, needs at least %d",
				tc.width, tc.height, mode.VideoMemBytes, need)
		}
		if mode.VideoMemBytes&(mode.VideoMemBytes-1) != 0 {
			t.Errorf("%dx%d video memory %d is not a power of two", tc.width, tc.height, mode.VideoMemBytes)
		}
		if !edidClockFits(mode.RefreshMilliHz, tc.width, tc.height) {
			t.Errorf("%dx%d chose %d mHz, whose EDID clock does not fit", tc.width, tc.height, mode.RefreshMilliHz)
		}
	}
}

// 4K is the case that drove this: at QEMU's default 75 Hz its pixel clock overflows the
// EDID's 16-bit field, so the rate has to come down for the mode to exist at all.
func TestGuestDisplay_SlowsTheEdidClockOnlyWhenItMust(t *testing.T) {
	full, _ := GuestDisplay(1920, 1080)
	if full.RefreshMilliHz != guestDisplayRefreshRates[0] {
		t.Errorf("1920x1080 chose %d mHz; a mode that fits at the highest rate should keep it", full.RefreshMilliHz)
	}
	uhd, ok := GuestDisplay(3840, 2160)
	if !ok {
		t.Fatal("4K should be supported")
	}
	if uhd.RefreshMilliHz >= full.RefreshMilliHz {
		t.Errorf("4K chose %d mHz; it cannot be described at %d", uhd.RefreshMilliHz, full.RefreshMilliHz)
	}
}
