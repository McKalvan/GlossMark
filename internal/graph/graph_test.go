// @gloss verifies gmi:CanonicalGraph scope=file

package graph

import (
	"bytes"
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
