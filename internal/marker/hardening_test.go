package marker

import (
	"strings"
	"testing"
)

// Inputs the chaos sweep found lost or mis-read markers on.
func TestScanEdgeCases(t *testing.T) {
	for _, c := range []struct {
		name, path, src string
		targets         []string
	}{
		{"apostrophe before an HTML comment", "a.md", "# The patron's guide <!-- @gloss documents x:Foo -->\n", []string{"x:Foo"}},
		{"apostrophe before a YAML comment", "a.yaml", "description: Patron's loans  # @gloss represents x:Foo\n", []string{"x:Foo"}},
		{"Rust lifetime", "a.rs", "fn f(s: &'static str) {} // @gloss implements x:Foo\n", []string{"x:Foo"}},
		{"C++ digit separator", "a.cpp", "int n = 1'000; // @gloss implements x:Foo\n", []string{"x:Foo"}},
		{"a closed string still hides a token", "a.py", "s = '# @gloss implements x:No'\n", nil},
		{"lone CR line endings", "a.py", "# @gloss implements x:Bar\rdef bar():\r    pass\r# @gloss implements x:Foo\rdef foo():\r    pass\r", []string{"x:Bar", "x:Foo"}},
		{"SPDX form mid-sentence is prose", "a.py", "def f():\n    # To mark a file, write a header like Gloss-Implements: x:Foo\n    pass\n", nil},
		{"SPDX header", "a.py", "# Gloss-Implements: x:Foo\n", []string{"x:Foo"}},
	} {
		style, _ := StyleFor(c.path, nil)
		found, _, _ := Scan(strings.NewReader(c.src), style)
		var got []string
		for _, f := range found {
			got = append(got, f.Marker.Targets...)
		}
		if strings.Join(got, ",") != strings.Join(c.targets, ",") {
			t.Errorf("%s: got %v, want %v", c.name, got, c.targets)
		}
	}
}

func TestBOMKeepsHeaderScope(t *testing.T) {
	style, _ := StyleFor("a.py", nil)
	found, _, _ := Scan(strings.NewReader("\ufeff# @gloss implements x:Bar\n\nimport os\n"), style)
	if len(found) != 1 || !found[0].Header {
		t.Fatalf("a BOM made the header marker not a header: %+v", found)
	}
}

func TestConfNaNRejected(t *testing.T) {
	for _, c := range []string{"NaN", "nan", "NAN"} {
		m, d := ParseLine(" @gloss? implements x:Bar conf="+c, 1, 1)
		if m == nil || m.Keys["conf"] != "" || len(d) == 0 || d[0].Code != "GM006" {
			t.Errorf("conf=%s: marker %+v, diags %+v", c, m, d)
		}
	}
}
