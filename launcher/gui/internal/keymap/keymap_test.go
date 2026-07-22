package keymap

import "testing"

func TestDecode_LettersAndShift(t *testing.T) {
	if got := Decode(30, false); !got.Printable() || got.Rune != 'a' {
		t.Fatalf("KEY_A unshifted = %+v, want 'a'", got)
	}
	if got := Decode(30, true); !got.Printable() || got.Rune != 'A' {
		t.Fatalf("KEY_A shifted = %+v, want 'A'", got)
	}
}

func TestDecode_DigitsAndSymbols(t *testing.T) {
	if got := Decode(2, false); got.Rune != '1' {
		t.Fatalf("KEY_1 unshifted = %q, want '1'", got.Rune)
	}
	if got := Decode(2, true); got.Rune != '!' {
		t.Fatalf("KEY_1 shifted = %q, want '!'", got.Rune)
	}
	if got := Decode(57, false); got.Rune != ' ' {
		t.Fatalf("KEY_SPACE = %q, want space", got.Rune)
	}
}

func TestDecode_SpecialKeys(t *testing.T) {
	cases := map[uint32]Special{
		28:  Enter,
		96:  Enter, // keypad enter
		1:   Escape,
		14:  Backspace,
		103: Up,
		108: Down,
		104: PageUp,
		109: PageDown,
		102: Home,
		107: End,
	}
	for code, want := range cases {
		got := Decode(code, false)
		if got.Special != want {
			t.Fatalf("keycode %d = special %d, want %d", code, got.Special, want)
		}
		if got.Printable() {
			t.Fatalf("keycode %d should not be printable", code)
		}
	}
}

func TestDecode_UnmappedIsIgnored(t *testing.T) {
	got := Decode(255, false) // no such key in the table
	if got.Printable() || got.Special != None {
		t.Fatalf("unmapped keycode should be the zero Key, got %+v", got)
	}
}

// Shift must not corrupt a key that has no distinct shifted form (space stays space).
func TestDecode_ShiftOnSpace(t *testing.T) {
	if got := Decode(57, true); got.Rune != ' ' {
		t.Fatalf("shift+space = %q, want space", got.Rune)
	}
}

// Every printable entry decodes to its unshifted and shifted rune, guarding the whole
// table against a typo (not just the two codes spot-checked above).
func TestDecode_AllPrintablePairs(t *testing.T) {
	for code, pair := range printable {
		if got := Decode(code, false); got.Rune != pair[0] || !got.Printable() {
			t.Errorf("code %d unshifted = %q, want %q", code, got.Rune, pair[0])
		}
		if got := Decode(code, true); got.Rune != pair[1] || !got.Printable() {
			t.Errorf("code %d shifted = %q, want %q", code, got.Rune, pair[1])
		}
	}
}
