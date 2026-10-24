// @gloss verifies gmi:PlanRecord scope=file

package load

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPlanRecord: gloss reads .glossmark/plan.jsonl and rejects each
// malformed line with its line number (GM124), keeping the valid ones; an
// unsupported format in the header is reported too (#200).
func TestPlanRecord(t *testing.T) {
	root := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(ConfigName, "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  A: { kind: process }\n")
	const ok = `"increment":"https://x/1","actor":"agent:a"`
	write(StateDir+"/"+PlanName, `{"type":"plan","format":2,"generator":"x"}
{"type":"brief",`+ok+`,"date":"2026-10-24","answers":{"scope":"one"}}
{"type":"decision",`+ok+`,"subject":"z:A","decision":"added","reason":"Why."}

{"type":"decision",`+ok+`,"subject":"z:A","decision":"guessed","reason":"Why."}
{"type":"deviation",`+ok+`,"subject":"z:A","disposition":"absorbed","reason":"Why."}
{"type":"deviation",`+ok+`,"subject":"z:A","reason":"Why."}
{"type":"brief","increment":"https://x/1","date":"2026-10-24","answers":{"a":"b"}}
{"type":"brief",`+ok+`,"date":"24 Oct","answers":{"a":"b"}}
{"type":"note",`+ok+`}
{"type":"decision",`+ok+`,"subject":"z:A","decision":"added","reason":"Why.","extra":1}
not json
`)
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var lines []int
	for _, d := range ws.Diags {
		if d.Code == "GM124" {
			if d.Severity != "error" || d.Path != ".glossmark/plan.jsonl" {
				t.Errorf("%+v", d)
			}
			lines = append(lines, d.Line)
		}
	}
	want := []int{1, 5, 7, 8, 9, 10, 11, 12}
	if len(lines) != len(want) {
		t.Fatalf("GM124 at %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("GM124 at %v, want %v", lines, want)
		}
	}
	var types []string
	for _, l := range ws.Plan {
		types = append(types, l.Type)
	}
	if len(types) != 3 || types[0] != "brief" || types[1] != "decision" || types[2] != "deviation" || ws.Plan[2].Line != 6 {
		t.Errorf("loaded %v (%+v)", types, ws.Plan)
	}
}
