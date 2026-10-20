// @gloss implements gmi:DeclarationAnchor scope=file

package marker

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// PointerAnchor resolves a JSON Pointer (RFC 6901) written in a sidecar's
// note= against the JSON file the sidecar annotates, and anchors to the
// value's line range. The fingerprint covers the value itself, so editing it
// makes the link stale and editing the sidecar does not. ok is false when
// the pointer is malformed or names nothing in the document.
func PointerAnchor(src []byte, ptr, scope string) (Anchor, bool) {
	if !strings.HasPrefix(ptr, "/") {
		return Anchor{}, false
	}
	var path []string
	for _, tok := range strings.Split(ptr[1:], "/") {
		path = append(path, strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~"))
	}
	p := &jsonScan{b: src}
	p.ws()
	s, e, ok := p.find(path)
	if !ok {
		return Anchor{}, false
	}
	raw := string(src[s:e])
	start := 1 + bytes.Count(src[:s], []byte("\n"))
	end := start + strings.Count(raw, "\n")
	sig := norm(raw)
	return Anchor{Scope: scope, Symbol: ptr, Kind: "json-pointer", StartLine: start, EndLine: end, Signature: sig, Fingerprint: fp(sig)}, true
}

// jsonScan walks a JSON document without decoding it, to find byte offsets.
type jsonScan struct {
	b []byte
	i int
}

func (p *jsonScan) ws() {
	for p.i < len(p.b) && strings.IndexByte(" \t\r\n", p.b[p.i]) >= 0 {
		p.i++
	}
}

// find returns the offsets of the value at path, starting at the value under
// the cursor.
func (p *jsonScan) find(path []string) (int, int, bool) {
	if len(path) == 0 {
		s := p.i
		if !p.skip() {
			return 0, 0, false
		}
		return s, p.i, true
	}
	if p.i >= len(p.b) {
		return 0, 0, false
	}
	switch p.b[p.i] {
	case '{':
		p.i++
		for {
			p.ws()
			if p.i >= len(p.b) || p.b[p.i] != '"' {
				return 0, 0, false
			}
			ks := p.i
			if !p.skip() {
				return 0, 0, false
			}
			var key string
			if json.Unmarshal(p.b[ks:p.i], &key) != nil {
				return 0, 0, false
			}
			p.ws()
			if p.i >= len(p.b) || p.b[p.i] != ':' {
				return 0, 0, false
			}
			p.i++
			p.ws()
			if key == path[0] {
				return p.find(path[1:])
			}
			if !p.next() {
				return 0, 0, false
			}
		}
	case '[':
		want, err := strconv.Atoi(path[0])
		if err != nil || want < 0 || (len(path[0]) > 1 && path[0][0] == '0') {
			return 0, 0, false
		}
		p.i++
		for n := 0; ; n++ {
			p.ws()
			if n == want {
				return p.find(path[1:])
			}
			if !p.next() {
				return 0, 0, false
			}
		}
	}
	return 0, 0, false
}

// next skips the value under the cursor and the comma after it; false at
// the end of the enclosing object or array.
func (p *jsonScan) next() bool {
	if !p.skip() {
		return false
	}
	p.ws()
	if p.i < len(p.b) && p.b[p.i] == ',' {
		p.i++
		return true
	}
	return false
}

// skip moves past one value: a string, an object or array, or a literal.
func (p *jsonScan) skip() bool {
	if p.i >= len(p.b) {
		return false
	}
	switch c := p.b[p.i]; c {
	case '"':
		for p.i++; p.i < len(p.b); p.i++ {
			switch p.b[p.i] {
			case '\\':
				p.i++
			case '"':
				p.i++
				return true
			}
		}
		return false
	case '{', '[':
		depth := 0
		for p.i < len(p.b) {
			switch p.b[p.i] {
			case '"':
				if !p.skip() {
					return false
				}
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					p.i++
					return true
				}
			}
			p.i++
		}
		return false
	case '}', ']', ',', ':':
		return false
	}
	s := p.i
	for p.i < len(p.b) && strings.IndexByte(",}] \t\r\n", p.b[p.i]) < 0 {
		p.i++
	}
	return p.i > s
}

// FileAnchor anchors to a whole file, fingerprinting its content.
func FileAnchor(lines []string) Anchor { return fileAnchor(lines) }
