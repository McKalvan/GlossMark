// @gloss verifies gmi:CanonicalGraph scope=file

package graph

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/model"
)

func testGraph() *Graph {
	g := New(map[string]string{"lib": "https://onto.acme.dev/lib/"})
	for _, c := range []string{"Item", "Media", "Book", "DVD", "Laptop", "Periodical"} {
		iri, _ := g.Expand("lib:" + c)
		g.AddNode(&Node{NodeType: model.NodeConcept, ID: iri, CURIE: "lib:" + c, NS: "lib", Tier: "core", Kind: "entity"})
	}
	isa := func(a, b string) {
		s, _ := g.Expand("lib:" + a)
		o, _ := g.Expand("lib:" + b)
		g.AddEdge(&Edge{S: s, P: "is-a", O: o, Source: SourceDeclared})
	}
	isa("Media", "Item")
	isa("Book", "Media")
	isa("DVD", "Media")
	isa("Laptop", "Item")
	isa("Periodical", "Item")
	return g
}

func TestLCS(t *testing.T) {
	g := testGraph()
	id := func(c string) string { s, _ := g.Expand("lib:" + c); return s }
	got := g.LCS([]string{id("Book"), id("DVD")})
	if len(got) != 1 || g.Curie(got[0]) != "lib:Media" {
		t.Fatalf("LCS(Book,DVD) = %v", got)
	}
	got = g.LCS([]string{id("Book"), id("Laptop")})
	if len(got) != 1 || g.Curie(got[0]) != "lib:Item" {
		t.Fatalf("LCS(Book,Laptop) = %v", got)
	}
	got = g.LCS([]string{id("Book")})
	if len(got) != 1 || g.Curie(got[0]) != "lib:Book" {
		t.Fatalf("LCS(Book) = %v", got)
	}
	if !g.Subsumes(id("Item"), id("Book")) || g.Subsumes(id("Book"), id("Item")) {
		t.Fatal("Subsumes wrong")
	}
}

// TestSubsumer: a unique least common subsumer, several incomparable ones
// (two concepts sharing two parents' lines), or none.
func TestSubsumer(t *testing.T) {
	g := testGraph()
	id := func(c string) string { s, _ := g.Expand("lib:" + c); return s }
	for _, c := range []string{"Thing", "StoryKit", "LanguageKit", "Shelf"} {
		g.AddNode(&Node{NodeType: model.NodeConcept, ID: id(c), CURIE: "lib:" + c, NS: "lib"})
	}
	for _, e := range [][2]string{{"Thing", "Item"}, {"StoryKit", "Media"}, {"StoryKit", "Thing"}, {"LanguageKit", "Media"}, {"LanguageKit", "Thing"}} {
		g.AddEdge(&Edge{S: id(e[0]), P: "is-a", O: id(e[1]), Source: SourceDeclared})
	}
	if l, cand := g.Subsumer([]string{id("Book"), id("DVD")}); g.Curie(l) != "lib:Media" || cand != nil {
		t.Errorf("Subsumer(Book, DVD) = %q %v", l, cand)
	}
	if l, cand := g.Subsumer([]string{id("StoryKit"), id("LanguageKit")}); l != "" || len(cand) != 2 || g.Curie(cand[0]) != "lib:Media" || g.Curie(cand[1]) != "lib:Thing" {
		t.Errorf("Subsumer(StoryKit, LanguageKit) = %q %v, want no unique subsumer: Media, Thing", l, cand)
	}
	if l, cand := g.Subsumer([]string{id("Book"), id("Shelf")}); l != "" || cand != nil {
		t.Errorf("Subsumer(Book, Shelf) = %q %v, want none", l, cand)
	}
}

// TestLiftGroups (ADR 0026): a partial lift folds each maximal subset that
// shares a subsumer, leaves the rest, and declines a subset whose members
// could be lifted two ways.
func TestLiftGroups(t *testing.T) {
	g := testGraph()
	id := func(c string) string { s, _ := g.Expand("lib:" + c); return s }
	for _, c := range []string{"Claim", "MotorClaim", "FloodClaim", "Assessment", "Payout", "X", "Y", "A", "B", "C"} {
		g.AddNode(&Node{NodeType: model.NodeConcept, ID: id(c), CURIE: "lib:" + c, NS: "lib"})
	}
	for _, e := range [][2]string{{"MotorClaim", "Claim"}, {"FloodClaim", "Claim"}, {"A", "X"}, {"B", "X"}, {"B", "Y"}, {"C", "Y"}} {
		g.AddEdge(&Edge{S: id(e[0]), P: "is-a", O: id(e[1]), Source: SourceDeclared})
	}
	render := func(cs ...string) string {
		var ids []string
		for _, c := range cs {
			ids = append(ids, id(c))
		}
		var out []string
		for _, gr := range g.LiftGroups(ids) {
			var ms []string
			for _, m := range gr.Members {
				ms = append(ms, strings.TrimPrefix(g.Curie(m), "lib:"))
			}
			s := strings.Join(ms, "+")
			if gr.Subsumer != "" {
				s = strings.TrimPrefix(g.Curie(gr.Subsumer), "lib:") + "<" + s
			}
			for _, c := range gr.Candidates {
				s += "?" + strings.TrimPrefix(g.Curie(c), "lib:")
			}
			out = append(out, s)
		}
		return strings.Join(out, " ")
	}
	for in, want := range map[string]string{
		// The issue's set: no single subsumer, so the claims fold on their own.
		"Claim MotorClaim FloodClaim Assessment Payout": "Assessment Claim<Claim+FloodClaim+MotorClaim Payout",
		"MotorClaim FloodClaim Assessment":              "Assessment Claim<FloodClaim+MotorClaim",
		// A whole-set subsumer is one group, as before.
		"Book DVD Laptop": "Item<Book+DVD+Laptop",
		// B could join X or Y: neither subset is lifted.
		"A B C":        "A?X B?X?Y C?Y",
		"A B C Payout": "A?X B?X?Y C?Y Payout",
		"Payout":       "Payout",
	} {
		if got := render(strings.Fields(in)...); got != want {
			t.Errorf("LiftGroups(%s) = %q, want %q", in, got, want)
		}
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
		{"ontology/**", "ontology/lib.gloss.yaml", true},
		{"*.json", "a/b/c.json", true},
		{"**/test_*.py", "pkg/tests/test_foo.py", true},
		{"**/*_test.go", "a_test.go", true},
	}
	for _, c := range cases {
		if got := model.MatchGlob(c.p, c.path); got != c.want {
			t.Errorf("MatchGlob(%q,%q)=%v want %v", c.p, c.path, got, c.want)
		}
	}
	if !model.MatchCurie("circ:Loan*", "circ:LoanStore") || model.MatchCurie("circ:Loan*", "lib:Loan") || !model.MatchCurie("hw:*", "hw:X") {
		t.Fatal("MatchCurie wrong")
	}
}

func TestCycles(t *testing.T) {
	g := testGraph()
	id := func(c string) string { s, _ := g.Expand("lib:" + c); return s }
	isa := func(a, b string) { g.AddEdge(&Edge{S: id(a), P: "is-a", O: id(b), Source: SourceDeclared}) }
	isa("Item", "Book")             // Item > Media > Book > Item
	isa("Periodical", "Periodical") // self-loop
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
	if got := strings.Join(path, ">"); got != "lib:Book>lib:Media>lib:Item" {
		t.Errorf("cycle = %s, want lib:Book>lib:Media>lib:Item", got)
	}
	if len(cycles[1]) != 1 || g.Curie(cycles[1][0].From) != "lib:Periodical" {
		t.Errorf("self-loop = %+v", cycles[1])
	}
	// Closures stay finite and exclude the start.
	if anc := g.Ancestors(id("Book")); len(anc) != 2 {
		t.Errorf("Ancestors(Book) on a cycle = %v", anc)
	}
}
