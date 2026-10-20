// @gloss verifies gmi:CanonicalGraph scope=file

package graph

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/model"
)

func testGraph() *Graph {
	g := New(map[string]string{"zoo": "https://onto.acme.dev/zoo/"})
	for _, c := range []string{"Animal", "Horse", "Mare", "Stallion", "Donkey", "Cow"} {
		iri, _ := g.Expand("zoo:" + c)
		g.AddNode(&Node{NodeType: model.NodeConcept, ID: iri, CURIE: "zoo:" + c, NS: "zoo", Tier: "core", Kind: "entity"})
	}
	isa := func(a, b string) {
		s, _ := g.Expand("zoo:" + a)
		o, _ := g.Expand("zoo:" + b)
		g.AddEdge(&Edge{S: s, P: "is-a", O: o, Source: SourceDeclared})
	}
	isa("Horse", "Animal")
	isa("Mare", "Horse")
	isa("Stallion", "Horse")
	isa("Donkey", "Animal")
	isa("Cow", "Animal")
	return g
}

func TestLCS(t *testing.T) {
	g := testGraph()
	id := func(c string) string { s, _ := g.Expand("zoo:" + c); return s }
	got := g.LCS([]string{id("Mare"), id("Stallion")})
	if len(got) != 1 || g.Curie(got[0]) != "zoo:Horse" {
		t.Fatalf("LCS(Mare,Stallion) = %v", got)
	}
	got = g.LCS([]string{id("Mare"), id("Donkey")})
	if len(got) != 1 || g.Curie(got[0]) != "zoo:Animal" {
		t.Fatalf("LCS(Mare,Donkey) = %v", got)
	}
	got = g.LCS([]string{id("Mare")})
	if len(got) != 1 || g.Curie(got[0]) != "zoo:Mare" {
		t.Fatalf("LCS(Mare) = %v", got)
	}
	if !g.Subsumes(id("Animal"), id("Mare")) || g.Subsumes(id("Mare"), id("Animal")) {
		t.Fatal("Subsumes wrong")
	}
}

// TestSubsumer: a unique least common subsumer, several incomparable ones
// (two concepts sharing two parents' lines), or none.
func TestSubsumer(t *testing.T) {
	g := testGraph()
	id := func(c string) string { s, _ := g.Expand("zoo:" + c); return s }
	for _, c := range []string{"Pet", "Pony", "Mule", "Rock"} {
		g.AddNode(&Node{NodeType: model.NodeConcept, ID: id(c), CURIE: "zoo:" + c, NS: "zoo"})
	}
	for _, e := range [][2]string{{"Pet", "Animal"}, {"Pony", "Horse"}, {"Pony", "Pet"}, {"Mule", "Horse"}, {"Mule", "Pet"}} {
		g.AddEdge(&Edge{S: id(e[0]), P: "is-a", O: id(e[1]), Source: SourceDeclared})
	}
	if l, cand := g.Subsumer([]string{id("Mare"), id("Stallion")}); g.Curie(l) != "zoo:Horse" || cand != nil {
		t.Errorf("Subsumer(Mare, Stallion) = %q %v", l, cand)
	}
	if l, cand := g.Subsumer([]string{id("Pony"), id("Mule")}); l != "" || len(cand) != 2 || g.Curie(cand[0]) != "zoo:Horse" || g.Curie(cand[1]) != "zoo:Pet" {
		t.Errorf("Subsumer(Pony, Mule) = %q %v, want no unique subsumer: Horse, Pet", l, cand)
	}
	if l, cand := g.Subsumer([]string{id("Mare"), id("Rock")}); l != "" || cand != nil {
		t.Errorf("Subsumer(Mare, Rock) = %q %v, want none", l, cand)
	}
}

func TestRoundTrip(t *testing.T) {
	g := testGraph()
	var buf bytes.Buffer
	if err := g.WriteJSONL(&buf); err != nil {
		t.Fatal(err)
	}
	first := buf.String()
	g2, err := ReadJSONL(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var buf2 bytes.Buffer
	g2.WriteJSONL(&buf2)
	if first != buf2.String() {
		t.Fatalf("round trip differs:\n%s\n---\n%s", first, buf2.String())
	}
	if len(g2.Nodes()) != 6 || len(g2.Edges()) != 5 {
		t.Fatalf("stats %v", g2.Stats())
	}
}

func TestGlob(t *testing.T) {
	cases := []struct {
		p, path string
		want    bool
	}{
		{"gen/**", "gen/a/b.go", true},
		{"gen/**", "src/gen/a.go", false},
		{"**/.gloss.yaml", "services/x/.gloss.yaml", true},
		{"**/.gloss.yaml", ".gloss.yaml", true},
		{"ontology/**", "ontology/zoo.gloss.yaml", true},
		{"*.json", "a/b/c.json", true},
		{"**/test_*.py", "pkg/tests/test_foo.py", true},
		{"**/*_test.go", "a_test.go", true},
	}
	for _, c := range cases {
		if got := model.MatchGlob(c.p, c.path); got != c.want {
			t.Errorf("MatchGlob(%q,%q)=%v want %v", c.p, c.path, got, c.want)
		}
	}
	if !model.MatchCurie("impl:Foal*", "impl:FoalRegistry") || model.MatchCurie("impl:Foal*", "zoo:Foal") || !model.MatchCurie("pa:*", "pa:X") {
		t.Fatal("MatchCurie wrong")
	}
}

func TestCycles(t *testing.T) {
	g := testGraph()
	id := func(c string) string { s, _ := g.Expand("zoo:" + c); return s }
	isa := func(a, b string) { g.AddEdge(&Edge{S: id(a), P: "is-a", O: id(b), Source: SourceDeclared}) }
	isa("Animal", "Mare") // Animal > Horse > Mare > Animal
	isa("Cow", "Cow")     // self-loop
	var all []string
	for _, n := range g.NodesOf(model.NodeConcept) {
		all = append(all, n.ID)
	}
	next := func(x string) []Step {
		var out []Step
		for _, e := range g.Out(x, "is-a") {
			out = append(out, Step{From: x, To: e.O, Edge: e})
		}
		return out
	}
	cycles := g.Cycles(all, next)
	if len(cycles) != 2 {
		t.Fatalf("want 2 cycles, got %d: %+v", len(cycles), cycles)
	}
	var path []string
	for _, s := range cycles[0] {
		path = append(path, g.Curie(s.From))
	}
	if got := strings.Join(path, ">"); got != "zoo:Animal>zoo:Mare>zoo:Horse" {
		t.Errorf("cycle = %s, want zoo:Animal>zoo:Mare>zoo:Horse", got)
	}
	if len(cycles[1]) != 1 || g.Curie(cycles[1][0].From) != "zoo:Cow" {
		t.Errorf("self-loop = %+v", cycles[1])
	}
	// Closures stay finite and exclude the start.
	if anc := g.Ancestors(id("Mare")); len(anc) != 2 {
		t.Errorf("Ancestors(Mare) on a cycle = %v", anc)
	}
}
