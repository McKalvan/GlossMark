// @gloss verifies gmi:RelationProfile scope=file

package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// profileRepo writes a workspace whose markers, manifest and ontology use
// starter and advanced relations, under the given relations: value.
func profileRepo(t *testing.T, relations string) string {
	t.Helper()
	root := t.TempDir()
	cfg := "owns:\n  lib: { iri: https://x/lib/, tier: core }\n"
	if relations != "" {
		cfg += "relations: " + relations + "\n"
	}
	for rel, content := range map[string]string{
		"glossmark.yaml":          cfg,
		"ontology/lib.gloss.yaml": "concepts:\n  Return: { kind: process }\n  Loan: { kind: entity }\n  Limit: { kind: constraint, constrains: Return }\n  Late: { kind: process, is-a: Return }\n",
		".gloss.yaml":             "boundary: o:b\nlevel: container\ndefault-namespace: lib\nholds: [lib:Loan]\n",
		"README.md":               "<!-- @gloss documents lib:Return scope=file -->\n# Library\n",
		"app.py":                  "# @gloss implements lib:Return\ndef a(): pass\n\n# @gloss presents lib:Loan\ndef b(): pass\n\n# @gloss represents lib:Loan\nclass Loan: pass\n\n# @gloss tests lib:Return\ndef test_a(): pass\n",
	} {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func buildRepo(t *testing.T, root string) (*load.Workspace, *graph.Graph, []string) {
	t.Helper()
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := extract.Build(ws)
	if err != nil {
		t.Fatal(err)
	}
	rollup.Derive(res.Graph, ws.Vocab)
	var hints []string
	for _, d := range check.Run(ws, res.Graph) {
		if d.Code == "GM250" {
			if d.Severity != "hint" {
				t.Errorf("GM250 is a %s, want a hint: %s", d.Severity, d)
			}
			hints = append(hints, d.Path+":"+d.Msg)
		} else if d.Severity == "error" {
			t.Errorf("unexpected error: %s", d)
		}
	}
	return ws, res.Graph, hints
}

// TestStarterProfile (issue #261): under relations: starter, check hints
// (GM250, never an error) at presents in a marker, holds in a manifest and
// constrains in an ontology file, naming the switch; implements,
// represents, tests (the verifies alias), documents and is-a pass. Unset and full
// hint at nothing, and the graph is the same under every value.
func TestStarterProfile(t *testing.T) {
	var graphs []string
	for _, relations := range []string{"starter", "", "full"} {
		ws, g, hints := buildRepo(t, profileRepo(t, relations))
		var edges []string
		for _, e := range g.Edges() {
			edges = append(edges, e.S+" "+e.P+" "+e.O)
		}
		graphs = append(graphs, strings.Join(edges, "\n"))
		all := strings.Join(hints, "\n")
		if relations != "starter" {
			if len(hints) > 0 {
				t.Errorf("relations: %q hints:\n%s", relations, all)
			}
			if s := agents.Snippet(ws, g, nil); !strings.Contains(s, "`presents`") || strings.Contains(s, "starter") {
				t.Errorf("relations: %q snippet does not list every inline relation:\n%s", relations, s)
			}
			continue
		}
		for _, want := range []string{
			"app.py:presents is outside the starter relation profile (implements, represents, verifies, documents, produces, consumes, part-of, is-a); set relations: full in glossmark.yaml",
			".gloss.yaml:holds is outside",
			"ontology/lib.gloss.yaml:constrains is outside",
		} {
			if !strings.Contains(all, want) {
				t.Errorf("missing %q in:\n%s", want, all)
			}
		}
		for _, no := range []string{"implements is", "represents is", "verifies is", "tests is", "documents is", "is-a is", "part-of is"} {
			if strings.Contains(all, no) {
				t.Errorf("a starter relation is hinted at: %q in:\n%s", no, all)
			}
		}
		if len(hints) != 3 {
			t.Errorf("want 3 hints, got %d:\n%s", len(hints), all)
		}
		s := agents.Snippet(ws, g, nil)
		line := ""
		for _, l := range strings.Split(s, "\n") {
			if strings.HasPrefix(l, "- Relations you may use inline:") {
				line = l
			}
		}
		if !strings.HasPrefix(line, "- Relations you may use inline: `consumes`, `documents`, `implements`, `produces`, `represents`, `verifies`. ") || !strings.Contains(line, "relations: full") {
			t.Errorf("starter snippet lists the wrong relations:\n%s", line)
		}
	}
	for i := 1; i < len(graphs); i++ {
		if graphs[i] != graphs[0] {
			t.Errorf("the relation profile changed the graph:\n%s\n---\n%s", graphs[0], graphs[i])
		}
	}
}
