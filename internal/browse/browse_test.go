// @gloss verifies gmi:GraphBrowser scope=file

package browse

import (
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func testGraph() *graph.Graph {
	g := graph.New(map[string]string{"lib": "https://x/lib/", "o": "https://x/o/"})
	add := func(t model.NodeType, curie, kind string) string {
		iri, _ := g.Expand(curie)
		g.AddNode(&graph.Node{NodeType: t, ID: iri, CURIE: curie, Kind: kind, Doc: curie + " doc"})
		return iri
	}
	book := add(model.NodeConcept, "lib:Book", "entity")
	media := add(model.NodeConcept, "lib:Media", "entity")
	b := add(model.NodeBoundary, "o:catalog", "container")
	file := graph.ArtifactPrefix + "src/book.py"
	g.AddNode(&graph.Node{NodeType: model.NodeArtifact, ID: file, Path: "src/book.py", Kind: "file"})
	g.AddEdge(&graph.Edge{S: book, P: "is-a", O: media, Source: graph.SourceDeclared})
	g.AddEdge(&graph.Edge{S: file, P: "represents", O: book, Source: graph.SourceInline, Prov: &graph.Prov{Path: "src/book.py", Line: 3, Proposed: true}})
	g.AddEdge(&graph.Edge{S: file, P: "part-of", O: b, Source: graph.SourceDerived})
	g.AddEdge(&graph.Edge{S: b, P: "holds", O: book, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: "lift"}})
	g.AddEdge(&graph.Edge{S: b, P: "holds", O: book, Source: graph.SourceDeclared})
	return g
}

func TestLookup(t *testing.T) {
	g := testGraph()
	rep := &rollup.Report{Findings: []rollup.Finding{{Kind: "untested", Concept: "lib:Book", Msg: "lib:Book is untested"}, {Kind: "overlap", Concept: "lib:Book"}, {Kind: "gap", Concept: "lib:Media"}}}
	n, err := Lookup(g, rep, "lib:Book")
	if err != nil {
		t.Fatal(err)
	}
	if n.Ref != "lib:Book" || n.Type != "concept" || n.Doc != "lib:Book doc" {
		t.Errorf("node: %+v", n)
	}
	if len(n.Out) != 1 || n.Out[0].Relation != "is-a" || n.Out[0].Nodes[0].Ref != "lib:Media" {
		t.Errorf("out: %+v", n.Out)
	}
	if len(n.In) != 2 || n.In[0].Relation != "holds" || n.In[1].Relation != "represents" {
		t.Fatalf("in: %+v", n.In)
	}
	if h := n.In[0].Nodes; len(h) != 1 || h[0].Source != graph.SourceDeclared {
		t.Errorf("one row per neighbour, the declared edge before the derived one: %+v", h)
	}
	if r := n.In[1].Nodes[0]; r.Ref != "src/book.py" || !r.Proposed || r.Line != 3 {
		t.Errorf("represents: %+v", r)
	}
	if len(n.Findings) != 1 || n.Findings[0].Kind != "untested" {
		t.Errorf("findings (overlap and other concepts' left out): %+v", n.Findings)
	}
	a, err := Lookup(g, nil, "./src/book.py")
	if err != nil || a.Type != "artifact" || a.Ref != "src/book.py" || len(a.Out) != 2 {
		t.Errorf("artifact by path: %+v %v", a, err)
	}
	sym := graph.ArtifactPrefix + "src/media.py#Media"
	g.AddNode(&graph.Node{NodeType: model.NodeArtifact, ID: sym, Path: "src/media.py", Kind: "class", Line: 4})
	if a, err := Lookup(g, nil, "src/media.py"); err != nil || a.Ref != "src/media.py#Media" {
		t.Errorf("a file with one declaration: %+v %v", a, err)
	}
	g.AddNode(&graph.Node{NodeType: model.NodeArtifact, ID: sym + "2", Path: "src/media.py", Kind: "class", Line: 9})
	if a, err := Lookup(g, nil, "src/media.py"); err != nil || a.Type != "file" || len(a.Out) != 1 || len(a.Out[0].Nodes) != 2 {
		t.Errorf("a file with two declarations: %+v %v", a, err)
	}
	if _, err := Lookup(g, nil, "lib:Nope"); err == nil || !strings.Contains(err.Error(), "no node") {
		t.Errorf("unknown ref: %v", err)
	}
}

func TestIndex(t *testing.T) {
	idx := Index(testGraph())
	var refs []string
	for _, nb := range idx {
		refs = append(refs, nb.Ref)
	}
	if got := strings.Join(refs, " "); got != "o:catalog lib:Book lib:Media" {
		t.Errorf("index: %s", got)
	}
}
