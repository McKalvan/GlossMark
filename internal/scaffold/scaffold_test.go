// @gloss verifies gmi:Scaffolder scope=file

package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func TestGuessPrefixes(t *testing.T) {
	for name, want := range map[string][2]string{"pfinance": {"pf", "pfi"}, "agent-arena": {"aa", "aai"}, "AgentArena": {"aa", "aai"}, "x": {"app", "appi"}} {
		c, i := GuessPrefixes(name)
		if c != want[0] || i != want[1] {
			t.Errorf("%s: %s/%s, want %v", name, c, i, want)
		}
	}
}

// TestInitThenCheck: on an empty Python and an empty TypeScript repository,
// init writes a workspace that loads and passes gloss check, never
// overwrites, and wires the task runner.
func TestInitThenCheck(t *testing.T) {
	for _, tc := range []struct{ name, src, runner string }{
		{"pfinance", "pfinance/importers/csv.py", "Makefile"},
		{"web-shop", "web/src/app.ts", "package.json"},
	} {
		root := filepath.Join(t.TempDir(), tc.name)
		write := func(rel, content string) {
			p := filepath.Join(root, rel)
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, []byte(content), 0o644)
		}
		write(tc.src, "x = 1\n")
		write("node_modules/dep/index.js", "x\n")
		if tc.runner == "Makefile" {
			write("Makefile", "all:\n\techo hi\n")
		} else {
			write("package.json", "{\n  \"scripts\": {\n    \"build\": \"tsc\"\n  }\n}\n")
		}
		write("README.md", "keep me\n")
		if tc.runner == "package.json" {
			// Existing agent files: AGENTS.md gets the rules block appended,
			// CLAUDE.md the import line.
			write("AGENTS.md", "# Web shop agents\nUse pnpm.")
			write("CLAUDE.md", "# Claude\nBe brief.")
		}
		p, err := Make(root, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Dirs) != 1 || strings.Contains(strings.Join(p.Dirs, ","), "node_modules") {
			t.Errorf("%s: source dirs = %v", tc.name, p.Dirs)
		}
		if err := p.Apply(); err != nil {
			t.Fatal(err)
		}
		ws, err := load.Load(root)
		if err != nil {
			t.Fatalf("%s: load: %v", tc.name, err)
		}
		res, err := extract.Build(ws)
		if err != nil {
			t.Fatal(err)
		}
		rollup.Derive(res.Graph, ws.Vocab)
		for _, d := range append(res.Diags, check.Run(ws, res.Graph)...) {
			t.Errorf("%s: %s", tc.name, d)
		}
		// Boundary prefixes come from repo-iri: no urn fallback.
		for _, n := range res.Graph.Nodes() {
			if strings.HasPrefix(n.ID, "urn:gloss:boundary:") {
				t.Errorf("%s: boundary %s fell back to a urn", tc.name, n.CURIE)
			}
		}
		rb, _ := os.ReadFile(filepath.Join(root, tc.runner))
		if !strings.Contains(string(rb), "gloss check") {
			t.Errorf("%s: %s lacks the gloss target:\n%s", tc.name, tc.runner, rb)
		}
		am, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
		if st, err := agents.CheckRules(string(am)); st != agents.RulesCurrent || err != nil {
			t.Errorf("%s: AGENTS.md lacks the current rules block:\n%s", tc.name, am)
		}
		cm, cmErr := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
		if tc.runner == "package.json" {
			if !strings.HasPrefix(string(am), "# Web shop agents\nUse pnpm.\n\n"+agents.RulesBegin) {
				t.Errorf("%s: existing AGENTS.md text must stay first:\n%s", tc.name, am)
			}
			if string(cm) != "# Claude\nBe brief.\n@AGENTS.md\n" {
				t.Errorf("%s: CLAUDE.md = %q", tc.name, cm)
			}
		} else if cmErr == nil {
			t.Errorf("%s: init must not create CLAUDE.md", tc.name)
		}
		gi, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
		if strings.Contains(string(gi), "ledger") && !strings.Contains(string(gi), "ledger.jsonl are committed") {
			t.Errorf("the ledger must not be ignored:\n%s", gi)
		}
		// A second run changes nothing, and a stale rules block is reported, not rewritten.
		p2, _ := Make(root, Options{})
		for _, f := range p2.Files {
			if f.Action == Create || (f.Action == Append && f.Path != ".gitignore") {
				t.Errorf("%s: second init would %s %s", tc.name, f.Action, f.Path)
			}
		}
		write("AGENTS.md", strings.Replace(string(am), "Leave it clean", "Leave it tidy", 1))
		p3, _ := Make(root, Options{})
		if s := p3.String(); !strings.Contains(s, "skip   AGENTS.md  (rules block out of date; gloss agents --rules --write AGENTS.md replaces it)") {
			t.Errorf("%s: plan for a stale block:\n%s", tc.name, s)
		}
	}
}
