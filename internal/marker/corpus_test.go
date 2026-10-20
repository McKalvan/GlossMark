// @gloss verifies gmi:MarkerGrammar, gmi:DeclarationAnchor scope=file

package marker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type expMarker struct {
	Line        int               `json:"line"`
	Proposed    bool              `json:"proposed"`
	Relation    string            `json:"relation"`
	Targets     []string          `json:"targets"`
	Keys        map[string]string `json:"keys"`
	Scope       string            `json:"scope"`
	Symbol      string            `json:"symbol"`
	Sugar       bool              `json:"sugar"`
	Diagnostics []string          `json:"diagnostics"`
}

type expDiag struct {
	Line int    `json:"line"`
	Code string `json:"code"`
}

func TestCorpus(t *testing.T) {
	root := filepath.Join("..", "..", "corpus")
	b, err := os.ReadFile(filepath.Join(root, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	var expDiags map[string][]expDiag
	if d, ok := raw["_diagnostics"]; ok {
		if err := json.Unmarshal(d, &expDiags); err != nil {
			t.Fatal(err)
		}
		delete(raw, "_diagnostics")
	}
	// Every file in corpus/files must be listed (zero false markers rule).
	entries, _ := os.ReadDir(filepath.Join(root, "testdata"))
	for _, e := range entries {
		rel := "testdata/" + e.Name()
		if _, ok := raw[rel]; !ok {
			style, ok := StyleFor(e.Name(), nil)
			if !ok {
				continue // comment-less file (config.json) relies on its sidecar
			}
			f, _ := os.Open(filepath.Join(root, rel))
			found, _, _ := Scan(f, style)
			f.Close()
			if len(found) != 0 {
				t.Errorf("%s: not in expected.json but %d markers found", rel, len(found))
			}
		}
	}
	names := make([]string, 0, len(raw))
	for k := range raw {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, rel := range names {
		var exp []expMarker
		if err := json.Unmarshal(raw[rel], &exp); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		path := filepath.Join(root, rel)
		style, ok := StyleFor(path, nil)
		if !ok {
			t.Errorf("%s: no comment style", rel)
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		found, diags, lines := Scan(f, style)
		f.Close()
		if len(found) != len(exp) {
			t.Errorf("%s: expected %d markers, found %d: %+v", rel, len(exp), len(found), summarize(found))
			continue
		}
		anchors := make([]Anchor, len(found))
		for i, fd := range found {
			anchors[i] = Resolve(rel, fd, lines, style)
		}
		Disambiguate(rel, lines, anchors)
		for i, fd := range found {
			e := exp[i]
			m := fd.Marker
			if m.Line != e.Line || m.Proposed != e.Proposed || m.Relation != e.Relation || !reflect.DeepEqual(m.Targets, e.Targets) || m.Sugar != e.Sugar {
				t.Errorf("%s: marker %d: got line=%d proposed=%v rel=%s targets=%v sugar=%v, want %+v", rel, i, m.Line, m.Proposed, m.Relation, m.Targets, m.Sugar, e)
			}
			if e.Keys == nil {
				e.Keys = map[string]string{}
			}
			got := m.Keys
			if got == nil {
				got = map[string]string{}
			}
			if !reflect.DeepEqual(got, e.Keys) {
				t.Errorf("%s:%d: keys got %v want %v", rel, m.Line, got, e.Keys)
			}
			a := anchors[i]
			if a.Scope != e.Scope || a.Symbol != e.Symbol {
				t.Errorf("%s:%d: anchor got scope=%s symbol=%q want scope=%s symbol=%q (kind=%s lines %d-%d)", rel, m.Line, a.Scope, a.Symbol, e.Scope, e.Symbol, a.Kind, a.StartLine, a.EndLine)
			}
			if a.Fingerprint == "" {
				t.Errorf("%s:%d: empty fingerprint", rel, m.Line)
			}
		}
		if want, ok := expDiags[rel]; ok {
			var got []expDiag
			for _, d := range diags {
				got = append(got, expDiag{Line: d.Line, Code: d.Code})
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s: diagnostics got %+v want %+v", rel, got, want)
			}
		} else if len(diags) > 0 {
			t.Errorf("%s: unexpected diagnostics: %v", rel, diags)
		}
	}
}

func summarize(found []Found) []string {
	var out []string
	for _, f := range found {
		out = append(out, f.Marker.Raw)
	}
	return out
}

func TestReferenceRegexAgreesOnPlainMarkers(t *testing.T) {
	cases := []string{
		"@gloss implements impl:StallionProduction",
		"@gloss? represents zoo:Mare conf=0.72",
		"@gloss implements impl:HorseFactoryService scope=file",
		"@gloss produces zoo:Foal, zoo:FoalBorn",
		"@gloss implements .AuthorizeHandler",
	}
	for _, c := range cases {
		m, d := ParseLine(c, 1, 1)
		if m == nil || len(d) != 0 {
			t.Errorf("%q: parse failed: %v", c, d)
		}
	}
}

func TestIsNewTarget(t *testing.T) {
	if !IsNewTarget("impl:NEW:Gelding") || IsNewTarget("impl:Gelding") || IsNewTarget(".NEW") {
		t.Fatal("IsNewTarget wrong")
	}
	if StripNew("impl:NEW:Gelding") != "impl:Gelding" {
		t.Fatal("StripNew wrong")
	}
}

// TestDeclarationsKotlinConst: suggest's detector names a Kotlin constant,
// not its "val" keyword (issue #131).
func TestDeclarationsKotlinConst(t *testing.T) {
	lines := []string{"const val FILING_WINDOW_DAYS = 30", "private const val LIMIT = 9", "var counter = 0"}
	style, _ := StyleFor("a.kt", nil)
	var got []string
	for _, d := range Declarations("a.kt", lines, style) {
		got = append(got, d.Symbol)
	}
	if want := []string{"FILING_WINDOW_DAYS", "LIMIT", "counter"}; !reflect.DeepEqual(got, want) {
		t.Errorf("symbols = %v, want %v", got, want)
	}
}
