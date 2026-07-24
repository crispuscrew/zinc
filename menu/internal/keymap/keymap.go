// Package keymap turns a Wayland wl_keyboard keycode into a character or a named key, for
// the menu's picker. Wayland delivers the raw Linux evdev keycode (KEY_A = 30, KEY_ENTER = 28,
// ...); this maps it with a fixed US-QWERTY layout plus Shift.
//
// Honest limitation: this is a US-layout fallback, not a full xkb interpreter. It reads
// the keycode directly rather than the layout the compositor advertises, so a non-US
// layout types US characters. Correct per-layout mapping (parsing the wl_keyboard keymap)
// is future work; a launcher's queries are short app names, so the fallback is usable, and
// the navigation/control keys below are layout-independent.
package keymap

// Special names a non-printable key the picker acts on. None means "a printable rune".
type Special uint8

const (
	None Special = iota
	Enter
	Escape
	Backspace
	Up
	Down
	PageUp
	PageDown
	Home
	End
)

// Key is one decoded keypress: either a printable Rune (Special == None) or a named
// Special (Rune == 0). Decode never sets both.
type Key struct {
	Rune    rune
	Special Special
}

// Printable reports whether the key is a character to insert into the query.
func (key Key) Printable() bool { return key.Special == None && key.Rune != 0 }

// evdev keycodes (from linux/input-event-codes.h) for the non-printable keys.
const (
	codeEscape    = 1
	codeBackspace = 14
	codeEnter     = 28
	codeKPEnter   = 96
	codeHome      = 102
	codePageUp    = 104
	codeEnd       = 107
	codePageDown  = 109
	codeUp        = 103
	codeDown      = 108
)

// printable maps an evdev keycode to its {unshifted, shifted} US-QWERTY runes.
var printable = map[uint32][2]rune{
	2: {'1', '!'}, 3: {'2', '@'}, 4: {'3', '#'}, 5: {'4', '$'}, 6: {'5', '%'},
	7: {'6', '^'}, 8: {'7', '&'}, 9: {'8', '*'}, 10: {'9', '('}, 11: {'0', ')'},
	12: {'-', '_'}, 13: {'=', '+'},
	16: {'q', 'Q'}, 17: {'w', 'W'}, 18: {'e', 'E'}, 19: {'r', 'R'}, 20: {'t', 'T'},
	21: {'y', 'Y'}, 22: {'u', 'U'}, 23: {'i', 'I'}, 24: {'o', 'O'}, 25: {'p', 'P'},
	26: {'[', '{'}, 27: {']', '}'},
	30: {'a', 'A'}, 31: {'s', 'S'}, 32: {'d', 'D'}, 33: {'f', 'F'}, 34: {'g', 'G'},
	35: {'h', 'H'}, 36: {'j', 'J'}, 37: {'k', 'K'}, 38: {'l', 'L'},
	39: {';', ':'}, 40: {'\'', '"'}, 41: {'`', '~'}, 43: {'\\', '|'},
	44: {'z', 'Z'}, 45: {'x', 'X'}, 46: {'c', 'C'}, 47: {'v', 'V'}, 48: {'b', 'B'},
	49: {'n', 'N'}, 50: {'m', 'M'},
	51: {',', '<'}, 52: {'.', '>'}, 53: {'/', '?'},
	57: {' ', ' '},
}

// Decode maps an evdev keycode (as delivered by wl_keyboard.key) and the Shift state to a
// Key. An unmapped keycode yields the zero Key (Special None, Rune 0), which Printable
// reports false for, so the caller ignores it.
func Decode(keycode uint32, shift bool) Key {
	switch keycode {
	case codeEnter, codeKPEnter:
		return Key{Special: Enter}
	case codeEscape:
		return Key{Special: Escape}
	case codeBackspace:
		return Key{Special: Backspace}
	case codeUp:
		return Key{Special: Up}
	case codeDown:
		return Key{Special: Down}
	case codePageUp:
		return Key{Special: PageUp}
	case codePageDown:
		return Key{Special: PageDown}
	case codeHome:
		return Key{Special: Home}
	case codeEnd:
		return Key{Special: End}
	}
	if pair, ok := printable[keycode]; ok {
		if shift {
			return Key{Rune: pair[1]}
		}
		return Key{Rune: pair[0]}
	}
	return Key{}
}
