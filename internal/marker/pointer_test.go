// @gloss verifies gmi:DeclarationAnchor scope=file

package marker

import "testing"

func TestPointerAnchor(t *testing.T) {
	src := []byte(`{
  "loan-days": {
    "BOOK": 21,
    "DVD": 7
  },
  "formats": ["BOOK", "DVD"],
  "a/b": { "~x": true },
  "note": "braces { ] in a \"string\""
}
`)
	cases := []struct {
		ptr        string
		ok         bool
		start, end int
	}{
		{"/loan-days", true, 2, 5},
		{"/loan-days/DVD", true, 4, 4},
		{"/formats/1", true, 6, 6},
		{"/a~1b/~0x", true, 7, 7},
		{"/note", true, 8, 8},
		{"/formats/EBOOK", false, 0, 0},
		{"/formats/2", false, 0, 0},
		{"/nope/9", false, 0, 0},
		{"formats", false, 0, 0},
		{"", false, 0, 0},
	}
	for _, c := range cases {
		a, ok := PointerAnchor(src, c.ptr, "range")
		if ok != c.ok || a.StartLine != c.start || a.EndLine != c.end {
			t.Errorf("%q: got ok=%v lines %d-%d, want ok=%v lines %d-%d", c.ptr, ok, a.StartLine, a.EndLine, c.ok, c.start, c.end)
		}
		if ok && (a.Symbol != c.ptr || a.Scope != "range" || a.Fingerprint == "") {
			t.Errorf("%q: anchor %+v", c.ptr, a)
		}
	}
	// The fingerprint covers the value only.
	a1, _ := PointerAnchor(src, "/loan-days/DVD", "range")
	a2, _ := PointerAnchor([]byte(`{"loan-days": {"BOOK": 21, "DVD": 14}}`), "/loan-days/DVD", "range")
	a3, _ := PointerAnchor([]byte(`{"loan-days": {"BOOK": 28, "DVD": 7}}`), "/loan-days/DVD", "range")
	if a1.Fingerprint == a2.Fingerprint || a1.Fingerprint != a3.Fingerprint {
		t.Errorf("fingerprints: %s %s %s", a1.Fingerprint, a2.Fingerprint, a3.Fingerprint)
	}
}
