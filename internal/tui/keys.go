// @gloss implements gmi:ReviewTerminal scope=file

package tui

import "unicode/utf8"

// Key is one keystroke: a printable rune, or a named key.
type Key struct {
	Rune rune
	Name string // up, down, left, right, pgup, pgdn, home, end, enter, esc, backspace, tab, ctrl-c, ctrl-d, ctrl-u
}

func (k Key) String() string {
	if k.Name != "" {
		return k.Name
	}
	return string(k.Rune)
}

// R is the key for a rune; N for a named key.
func R(r rune) Key      { return Key{Rune: r} }
func N(name string) Key { return Key{Name: name} }

var escSeqs = map[string]string{
	"[A": "up", "[B": "down", "[C": "right", "[D": "left", "OA": "up", "OB": "down", "OC": "right", "OD": "left",
	"[5~": "pgup", "[6~": "pgdn", "[H": "home", "[F": "end", "[1~": "home", "[4~": "end", "OH": "home", "OF": "end",
	"[3~": "delete", "[Z": "backtab",
}

// Decode splits what one read returned into keys. An escape sequence the
// table does not know is dropped rather than typed.
func Decode(b []byte) []Key {
	var out []Key
	for len(b) > 0 {
		c := b[0]
		switch {
		case c == 0x1b:
			if len(b) == 1 {
				return append(out, N("esc"))
			}
			if b[1] != '[' && b[1] != 'O' {
				out = append(out, N("esc"))
				b = b[1:]
				continue
			}
			// CSI/SS3: parameters, then one final byte in @..~.
			j := 2
			for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
				j++
			}
			if j < len(b) {
				j++
			}
			if name, ok := escSeqs[string(b[1:j])]; ok {
				out = append(out, N(name))
			}
			b = b[j:]
		case c == '\r' || c == '\n':
			out = append(out, N("enter"))
			b = b[1:]
		case c == 0x7f || c == 0x08:
			out = append(out, N("backspace"))
			b = b[1:]
		case c == '\t':
			out = append(out, N("tab"))
			b = b[1:]
		case c == 0x03:
			out = append(out, N("ctrl-c"))
			b = b[1:]
		case c == 0x04:
			out = append(out, N("ctrl-d"))
			b = b[1:]
		case c == 0x15:
			out = append(out, N("ctrl-u"))
			b = b[1:]
		case c < 0x20:
			b = b[1:]
		default:
			r, n := utf8.DecodeRune(b)
			out = append(out, R(r))
			b = b[n:]
		}
	}
	return out
}
