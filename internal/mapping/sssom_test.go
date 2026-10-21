// @gloss verifies gmi:MappingSets scope=file

package mapping

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func codes(s *Set) []string {
	var out []string
	for _, p := range s.Problems {
		out = append(out, p.Code+" "+p.Msg)
	}
	return out
}

const embedded = "# curie_map:\n" +
	"#   a: https://x/a/\n" +
	"#   b: https://x/b/\n" +
	"# mapping_set_id: https://x/sets/ab\n" +
	"# mapping_set_version: 1.2.0\n" +
	"# license: https://creativecommons.org/publicdomain/zero/1.0/\n" +
	"# creator_id: [team:ab]\n" +
	"# mapping_tool: hand\n" +
	"subject_id\tpredicate_id\tobject_id\tmapping_justification\tauthor_id\tcomment\n" +
	"a:One\tskos:exactMatch\tb:Uno\tsemapv:ManualMappingCuration\thuman:x|agent:y\t\"tab\there, \"\"quoted\"\"\n and a newline\"\n" +
	"a:Two\tskos:closeMatch\tb:Dos\tsemapv:LexicalMatching\tagent:a\\|b\t\n"

// Embedded metadata, RFC 4180 quoting across lines, multi-values with
// escapes and set-level propagation all read as SSSOM defines them, and a
// save reads back to the same rows.
func TestSSSOMRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(write(t, dir, "ab.sssom.tsv", embedded))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Problems) > 0 {
		t.Fatalf("problems: %v", codes(s))
	}
	if s.ID != "https://x/sets/ab" || s.Version != "1.2.0" || !reflect.DeepEqual(s.Creators, []string{"team:ab"}) || len(s.Rows) != 2 {
		t.Fatalf("set = %+v", s)
	}
	r := s.Rows[0]
	if got := r.Get("comment"); got != "tab\there, \"quoted\"\n and a newline" {
		t.Errorf("quoted comment = %q", got)
	}
	if got := r.Multi("author_id"); !reflect.DeepEqual(got, []string{"human:x", "agent:y"}) {
		t.Errorf("author_id = %v", got)
	}
	if got := s.Rows[1].Multi("author_id"); !reflect.DeepEqual(got, []string{"agent:a|b"}) {
		t.Errorf("escaped pipe: author_id = %v", got)
	}
	if r.Get("mapping_tool") != "hand" || s.Rows[1].Get("mapping_tool") != "hand" {
		t.Errorf("mapping_tool did not propagate: %v", r.Cells)
	}
	if s.Rows[1].Line != 12 {
		t.Errorf("second row line = %d, want 12 (the quoted newline spans a line)", s.Rows[1].Line)
	}
	if iri, declared, ok := s.Expand("a:One", nil); !ok || !declared || iri != "https://x/a/One" {
		t.Errorf("Expand a:One = %s %v %v", iri, declared, ok)
	}
	if iri, declared, ok := s.Expand("c:Three", map[string]string{"c": "https://x/c/"}); !ok || declared || iri != "https://x/c/Three" {
		t.Errorf("Expand c:Three from the fallback = %s %v %v", iri, declared, ok)
	}

	out := filepath.Join(dir, "out.sssom.tsv")
	if err := s.Save(out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if strings.Contains(string(b), "\thand\t") {
		t.Errorf("save wrote a propagated value into rows:\n%s", b)
	}
	s2, err := Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Problems) > 0 {
		t.Fatalf("problems after save: %v", codes(s2))
	}
	for i := range s.Rows {
		if !reflect.DeepEqual(s.Rows[i].Cells, s2.Rows[i].Cells) {
			t.Errorf("row %d changed in the round trip:\n%v\n%v", i, s.Rows[i].Cells, s2.Rows[i].Cells)
		}
	}
	if s2.Version != "1.2.0" || s2.ID != s.ID {
		t.Errorf("metadata changed in the round trip: %+v", s2)
	}
}

// External metadata: a set without a # block reads its .sssom.yml sibling.
func TestSSSOMExternalMetadata(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ab.sssom.yml", "curie_map:\n  a: https://x/a/\n  b: https://x/b/\nmapping_set_id: https://x/sets/ab\nlicense: https://x/license\n")
	s, err := Load(write(t, dir, "ab.sssom.tsv", "subject_id\tpredicate_id\tobject_id\tmapping_justification\na:One\tskos:exactMatch\tb:Uno\tsemapv:ManualMappingCuration\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Problems) > 0 || s.External == "" || s.ID != "https://x/sets/ab" || len(s.Rows) != 1 {
		t.Errorf("external metadata: %v %+v", codes(s), s)
	}
	s, _ = Load(write(t, dir, "none.sssom.tsv", "subject_id\tpredicate_id\tobject_id\tmapping_justification\n"))
	if len(s.Problems) == 0 || !strings.Contains(codes(s)[0], "no metadata block") {
		t.Errorf("a set with no metadata at all: %v", codes(s))
	}
}

// What SSSOM forbids is a GM700 problem at its line.
func TestSSSOMProblems(t *testing.T) {
	dir := t.TempDir()
	cases := []struct{ name, body, want string }{
		{"bom", "\xEF\xBB\xBF# mapping_set_id: x\n# license: y\nsubject_id\tpredicate_id\tobject_id\tmapping_justification\n", "byte order mark"},
		{"no id", "# license: y\nsubject_id\tpredicate_id\tobject_id\tmapping_justification\n", "mapping_set_id is required"},
		{"builtin", "# mapping_set_id: x\n# license: y\n# curie_map:\n#   skos: https://wrong/\nsubject_id\tpredicate_id\tobject_id\tmapping_justification\n", "built-in prefix skos"},
		{"header", "# mapping_set_id: x\n# license: y\nsubject_id\tpredicate_id\tobject_id\n", "header must include"},
		{"unknown column", "# mapping_set_id: x\n# license: y\nsubject_id\tpredicate_id\tobject_id\tmapping_justification\tmy_col\n", "unknown column \"my_col\""},
		{"width", "# mapping_set_id: x\n# license: y\nsubject_id\tpredicate_id\tobject_id\tmapping_justification\na:A\tskos:exactMatch\tb:B\n", "row has 3 values"},
		{"modifier", "# mapping_set_id: x\n# license: y\nsubject_id\tpredicate_id\tpredicate_modifier\tobject_id\tmapping_justification\na:A\tskos:exactMatch\tNo\tb:B\tsemapv:ManualMappingCuration\n", "predicate_modifier must be Not"},
		{"confidence", "# mapping_set_id: x\n# license: y\nsubject_id\tpredicate_id\tobject_id\tmapping_justification\tconfidence\na:A\tskos:exactMatch\tb:B\tsemapv:ManualMappingCuration\t1.5\n", "confidence must be a number"},
		{"review date", "# mapping_set_id: x\n# license: y\nsubject_id\tpredicate_id\tobject_id\tmapping_justification\treview_date\na:A\tskos:exactMatch\tb:B\tsemapv:ManualMappingCuration\t2026-01-01\n", "review_date requires reviewer_id"},
		{"justification", "# mapping_set_id: x\n# license: y\nsubject_id\tpredicate_id\tobject_id\tmapping_justification\na:A\tskos:exactMatch\tb:B\tby hand\n", "not a semapv term"},
	}
	for _, c := range cases {
		s, err := Load(write(t, dir, strings.ReplaceAll(c.name, " ", "-")+".sssom.tsv", c.body))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range s.Problems {
			if p.Code == "GM700" && strings.Contains(p.Msg, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: want a GM700 %q, got %v", c.name, c.want, codes(s))
		}
	}
}

func TestMultiEscapes(t *testing.T) {
	for _, vs := range [][]string{{"a"}, {"a", "b"}, {"a|b", `c\d`}, {`\`, "|"}} {
		if got := SplitMulti(JoinMulti(vs)); !reflect.DeepEqual(got, vs) {
			t.Errorf("SplitMulti(JoinMulti(%q)) = %q", vs, got)
		}
	}
}
