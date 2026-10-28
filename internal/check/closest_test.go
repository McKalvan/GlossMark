// @gloss verifies gmi:Linter scope=file

package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func TestClosest(t *testing.T) {
	cmds := []string{"check", "build", "drift", "diff", "init", "impact", "import"}
	for word, want := range map[string]string{
		"chek":    "check",
		"chekc":   "check",
		"CHECK":   "check",
		"drfit":   "drift",
		"xyz":     "",
		"imp":     "", // too short to tell impact from import
		"buildgo": "",
	} {
		if got := check.Closest(word, cmds); got != want {
			t.Errorf("Closest(%q) = %q, want %q", word, got, want)
		}
	}
	if d := check.Distance("Retrun", "Return"); d != 1 {
		t.Errorf("Distance(Retrun, Return) = %d, want 1 (one transposition)", d)
	}
}

// TestTypoSuggestions: GM200 names the relation a typo was meant to be
// instead of listing all of them, and GM215 names the concept, matching by
// name or alias within the namespace (#233).
func TestTypoSuggestions(t *testing.T) {
	root := t.TempDir()
	for rel, content := range map[string]string{
		"glossmark.yaml":          "owns:\n  lib: { iri: https://x/lib/, tier: core }\n",
		"ontology/lib.gloss.yaml": "concepts:\n  Return: { kind: process }\n  Loan: { kind: entity, aliases: [Borrowing] }\n",
		".gloss.yaml":             "boundary: o:b\nlevel: container\ndefault-namespace: lib\n",
		"app.py":                  "# @gloss implemnts lib:Return\ndef a(): pass\n\n# @gloss implements lib:Retrun\ndef b(): pass\n\n# @gloss represents lib:Borowing\ndef c(): pass\n\n# @gloss implements lib:Zebra\ndef d(): pass\n\n# @gloss frobnicates lib:Return\ndef e(): pass\n",
	} {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := extract.Build(ws)
	if err != nil {
		t.Fatal(err)
	}
	rollup.Derive(res.Graph, ws.Vocab)
	var msgs []string
	for _, d := range check.Run(ws, res.Graph) {
		if d.Code == "GM200" || d.Code == "GM215" {
			msgs = append(msgs, d.Code+" "+d.Msg)
		}
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{
		`GM200 unknown relation "implemnts"; did you mean "implements"?`,
		"GM215 dangling reference: lib:Retrun does not exist; did you mean lib:Return?",
		"GM215 dangling reference: lib:Borowing does not exist; did you mean lib:Loan?",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	for _, m := range msgs {
		switch {
		case strings.Contains(m, "implemnts") && strings.Contains(m, "takes one of"):
			t.Errorf("GM200 with a close match should not list every relation: %s", m)
		case strings.Contains(m, "frobnicates") && !strings.Contains(m, "takes one of"):
			t.Errorf("GM200 with no close match should list the relations: %s", m)
		case strings.Contains(m, "Zebra") && strings.Contains(m, "did you mean"):
			t.Errorf("GM215 with no close match should not guess: %s", m)
		}
	}
}
