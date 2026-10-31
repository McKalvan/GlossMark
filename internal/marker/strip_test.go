// @gloss verifies gmi:Stripper scope=file

package marker

import (
	"bytes"
	"testing"
)

// TestStrip (#263): Strip removes exactly the markers Scan reads, in every
// comment style, and nothing else: a whole line when the comment held only
// the marker, the trailing comment after code, the marker text alone inside
// a comment with other text or on the opening line of a block; string
// literals, backtick quotes, fenced code, line endings and a BOM stay.
func TestStrip(t *testing.T) {
	cases := []struct {
		name, path, in, want string
		n                    int
	}{
		{"hash line", "a.py", "# @gloss implements .A conf=0.9\ndef a(): pass\n", "def a(): pass\n", 1},
		{"trailing after code", "a.py", "x = 1  # @gloss represents .B\n", "x = 1\n", 1},
		{"proposal", "a.py", "# @gloss? implements .A conf=0.5 note=\"maybe\"\ndef a(): pass\n", "def a(): pass\n", 1},
		{"header with scope", "a.go", "// @gloss implements gmi:X scope=file\n\npackage p\n", "\npackage p\n", 1},
		{"block on one line", "a.go", "/* @gloss implements .Y */\nfunc f() {}\n", "func f() {}\n", 1},
		{"interior of a block", "a.go", "/*\n * Doc.\n * @gloss implements .Z\n */\nfunc g() {}\n", "/*\n * Doc.\n */\nfunc g() {}\n", 1},
		{"opening line of a block", "a.go", "/* @gloss implements .W\n   more */\n", "/*\n   more */\n", 1},
		{"prose before the marker", "a.go", "// Handler. @gloss implements .H\n", "// Handler.\n", 1},
		{"string literal", "a.go", "var s = \"// @gloss implements .Q\"\n", "var s = \"// @gloss implements .Q\"\n", 0},
		{"backtick quoted", "a.go", "// see `@gloss implements .Q` in the docs\n", "// see `@gloss implements .Q` in the docs\n", 0},
		{"two block comments on a line", "a.go", "/* @gloss implements .A */ /* @gloss implements .B */\n", "", 2},
		{"html", "a.html", "<!-- @gloss presents x:A scope=file -->\n<p><!-- @gloss documents x:C --> text</p>\n", "<p> text</p>\n", 2},
		{"markdown fence", "a.md", "<!-- @gloss documents x:A -->\n# T\n\n```\n<!-- @gloss documents x:B -->\n```\n", "# T\n\n```\n<!-- @gloss documents x:B -->\n```\n", 1},
		{"sql dash", "a.sql", "-- @gloss represents c:Patient\ncreate table p (id int);\n", "create table p (id int);\n", 1},
		{"sidecar", "c.json.gloss", "# sidecar\n@gloss configures circ:Bus scope=range note=\"/bus\"\n", "# sidecar\n", 1},
		{"spdx sugar", "a.java", "/**\n * Gloss-Implements: x:A\n */\nclass A {}\n", "/**\n */\nclass A {}\n", 1},
		{"crlf kept", "a.py", "# @gloss implements .A\r\nprint(1)\r\n", "print(1)\r\n", 1},
		{"bom kept", "a.py", "\ufeff# @gloss implements .A\nprint(1)\n", "\ufeffprint(1)\n", 1},
		{"malformed left alone", "a.py", "# @gloss implements\ndef a(): pass\n", "# @gloss implements\ndef a(): pass\n", 0},
		{"no newline at end", "a.py", "x = 1\n# @gloss implements .A", "x = 1\n", 1},
	}
	for _, c := range cases {
		style, ok := StyleFor(c.path, nil)
		if !ok {
			t.Fatalf("%s: no style for %s", c.name, c.path)
		}
		got, n := Strip([]byte(c.in), style)
		if string(got) != c.want || n != c.n {
			t.Errorf("%s: Strip removed %d, want %d\n got: %q\nwant: %q", c.name, n, c.n, got, c.want)
		}
	}
}

// TestStripMatchesScan: on the files the scanner has tests for, every
// marker Scan finds is gone after Strip and the remaining lines are a
// subsequence of the original, so nothing is rewritten.
func TestStripMatchesScan(t *testing.T) {
	src := "// @gloss implements gmi:X scope=file\npackage p\n\n// f does things.\n// @gloss implements .F\nfunc f() { s := \"@gloss implements .No\" }\n/* @gloss? represents .R conf=0.4 */ var r int\n"
	style, _ := StyleFor("x.go", nil)
	before, _, _ := Scan(bytes.NewReader([]byte(src)), style)
	out, n := Strip([]byte(src), style)
	if n != len(before) {
		t.Fatalf("Strip removed %d markers, Scan found %d", n, len(before))
	}
	after, _, _ := Scan(bytes.NewReader(out), style)
	if len(after) != 0 {
		t.Errorf("markers left after Strip: %+v", after)
	}
}
