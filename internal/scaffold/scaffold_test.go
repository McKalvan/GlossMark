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
	"github.com/mckalvan/glossmark/internal/model"
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

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
}

func planned(p *Plan) map[string]File {
	out := map[string]File{}
	for _, f := range p.Files {
		out[f.Path] = f
	}
	return out
}

// TestInitMonorepo (issue #35): a grouping directory is replaced by the
// projects in it, a directory of Kubernetes YAML or Avro schemas gets a
// manifest, and --boundary-prefix sets the boundary prefix.
func TestInitMonorepo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "clinic-booking")
	writeTree(t, root, map[string]string{
		"apps/web/package.json":                  "{}\n",
		"apps/web/app/page.tsx":                  "export default 1\n",
		"services/booking-api/go.mod":            "module x\n",
		"services/booking-api/main.go":           "package main\n",
		"services/reminders/pyproject.toml":      "[project]\n",
		"services/reminders/app/main.py":         "x = 1\n",
		"services/api/Dockerfile":                "FROM scratch\n",
		"apps/api/package.json":                  "{}\n",
		"deploy/booking-api.yaml":                "apiVersion: apps/v1\nkind: Deployment\n",
		"schemas/claim.avsc":                     "{}\n",
		"db/migrations/0001_init.sql":            "create table t();\n",
		"docs/glossary.md":                       "## Appointment\n",
		"config/enums.yaml":                      "Status: [OPEN]\n",
		"README.md":                              "hi\n",
		"services/reminders/node_modules/x.js":   "x\n",
		"apps/web/node_modules/dep/package.json": "{}\n",
	})
	p, err := Make(root, Options{BoundaryPrefix: "lk"})
	if err != nil {
		t.Fatal(err)
	}
	want := "apps/api,apps/web,config,db,deploy,schemas,services/api,services/booking-api,services/reminders"
	if got := strings.Join(p.Dirs, ","); got != want {
		t.Errorf("dirs = %s\nwant   %s", got, want)
	}
	files := planned(p)
	if f := files["apps/web/.gloss.yaml"]; !strings.Contains(f.Content, "boundary: lk:web\n") || !strings.Contains(f.Content, "part-of: lk:clinic-booking") {
		t.Errorf("apps/web manifest:\n%s", f.Content)
	}
	// Two projects named api: the second takes its path.
	if a, s := files["apps/api/.gloss.yaml"].Content, files["services/api/.gloss.yaml"].Content; !strings.Contains(a, "lk:api\n") || !strings.Contains(s, "lk:services-api\n") {
		t.Errorf("api manifests:\n%s\n%s", a, s)
	}
	if _, ok := files["apps/.gloss.yaml"]; ok {
		t.Error("a grouping directory got a manifest")
	}
	if p.Boundary != "lk" || !strings.Contains(p.String(), "boundaries lk:") {
		t.Errorf("boundary prefix %q", p.Boundary)
	}
	// The vocabulary files are offered to the importers (issue #135, L1).
	if v := strings.Join(p.Vocabulary, "\n"); !strings.Contains(v, "--from markdown-glossary docs/glossary.md --namespace cb") || !strings.Contains(v, "--from enums config/enums.yaml --namespace cb") {
		t.Errorf("vocabulary = %v", p.Vocabulary)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := extract.Build(ws)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range append(res.Diags, check.Run(ws, res.Graph)...) {
		if d.Severity == "error" {
			t.Errorf("%s", d)
		}
	}
}

// TestInitBelowGitRoot (issue #36): in a subdirectory of a git repository
// init writes no .github/ and prints the job with root: set.
func TestInitBelowGitRoot(t *testing.T) {
	top := t.TempDir()
	root := filepath.Join(top, "showcase", "clinic-booking")
	writeTree(t, top, map[string]string{".git/HEAD": "ref: refs/heads/main\n", "showcase/clinic-booking/src/a.go": "package a\n"})
	p, err := Make(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if f := planned(p)[".github/workflows/gloss.yml"]; f.Action != Skip {
		t.Errorf("workflow action = %s", f.Action)
	}
	if !strings.Contains(p.Workflow, "root: showcase/clinic-booking,") || !strings.Contains(p.String(), "uses: mckalvan/glossmark@") {
		t.Errorf("workflow snippet:\n%s", p.String())
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".github")); err == nil {
		t.Error("init wrote .github/ below the git root")
	}
	// At the git root the job is written.
	p2, _ := Make(top, Options{Name: "mono"})
	if f := planned(p2)[".github/workflows/gloss.yml"]; f.Action != Create || p2.Workflow != "" {
		t.Errorf("at the git root: %s %q", f.Action, p2.Workflow)
	}
}

// TestInitIRIs (issue #135, FF3): an --iri base gives every namespace its
// own IRI, so siblings initialised with the same base never share one.
func TestInitIRIs(t *testing.T) {
	cfg := func(name, core string) *model.Config {
		root := filepath.Join(t.TempDir(), name)
		os.MkdirAll(root, 0o755)
		p, err := Make(root, Options{Core: core, IRIBase: "https://onto.kestrel.example/"})
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Apply(); err != nil {
			t.Fatal(err)
		}
		c, err := model.LoadConfig(filepath.Join(root, "glossmark.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	a, b := cfg("alpha", "frt"), cfg("beta", "bc")
	seen := map[string]string{}
	for _, c := range []*model.Config{a, b} {
		for prefix, ns := range c.Owns {
			if other, ok := seen[ns.IRI]; ok {
				t.Errorf("%s: and %s: share %s", prefix, other, ns.IRI)
			}
			seen[ns.IRI] = prefix
		}
	}
	if a.Owns["frt"].IRI != "https://onto.kestrel.example/frt/" || a.RepoIRI != "https://onto.kestrel.example/alpha/" {
		t.Errorf("alpha: %s %s", a.Owns["frt"].IRI, a.RepoIRI)
	}
}

// TestInitImport (issue #135, FF2): --import makes the sibling's core
// namespace an import, writes no core tier file and keeps its own prefixes.
func TestInitImport(t *testing.T) {
	top := t.TempDir()
	writeTree(t, top, map[string]string{
		"shipment-core/glossmark.yaml":          "owns:\n  frt: { iri: https://onto.k.example/frt/, tier: core, version: 1.2.0 }\n  sci: { iri: https://onto.k.example/sci/, tier: implementation }\n",
		"shipment-core/ontology/frt.gloss.yaml": "namespace: frt\nversion: 1.2.0\nconcepts:\n  Shipment: { kind: entity, doc: \"A consignment.\" }\n",
		"dispatch/src/a.ts":                     "export const a = 1\n",
	})
	root := filepath.Join(top, "dispatch")
	p, err := Make(root, Options{Imports: []string{"../shipment-core"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Core != "frt" || p.Impl != "dii" || p.Boundary != "dib" || strings.Join(p.Imported, ",") != "frt" {
		t.Errorf("prefixes %s %s %s %v", p.Core, p.Impl, p.Boundary, p.Imported)
	}
	files := planned(p)
	if _, ok := files["ontology/frt.gloss.yaml"]; ok {
		t.Error("an importer writes the core tier file")
	}
	cfg := files["glossmark.yaml"].Content
	if strings.Contains(cfg, "tier: core") || !strings.Contains(cfg, `frt: { iri: https://onto.k.example/frt/, version: "^1.2.0", source: ../shipment-core/ontology/frt.gloss.yaml }`) {
		t.Errorf("glossmark.yaml:\n%s", cfg)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Owned["frt"] || ws.Resolved["frt"] == nil {
		t.Errorf("frt: owned=%v resolved=%v", ws.Owned["frt"], ws.Resolved["frt"])
	}
	if _, err := Make(root+"2", Options{Imports: []string{"../nowhere"}}); err == nil {
		t.Error("a missing sibling is accepted")
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
		// A new repository starts on the starter relation profile (issue
		// #261): its AGENTS.md snippet lists the starter relations only.
		if ws.Config.RelationProfile() != model.ProfileStarter {
			t.Errorf("%s: relations: %q, want starter", tc.name, ws.Config.Relations)
		}
		if s := agents.Snippet(ws, res.Graph, nil); !strings.Contains(s, "inline: `consumes`, `implements`, `produces`, `represents`, `verifies`. ") {
			t.Errorf("%s: snippet does not list the starter relations:\n%s", tc.name, s)
		}
		// Boundary prefixes come from repo-iri: no urn fallback.
		for _, n := range res.Graph.Nodes() {
			if strings.HasPrefix(n.ID, "urn:gloss:boundary:") {
				t.Errorf("%s: boundary %s fell back to a urn", tc.name, n.CURIE)
			}
		}
		// glossmark.yaml carries the commented suggest: block at its
		// defaults, no fixed soft-conf-threshold, and uncommented it loads
		// to the same rule (ADR 0024).
		cb, _ := os.ReadFile(filepath.Join(root, "glossmark.yaml"))
		if !strings.Contains(string(cb), "# suggest:") || !strings.Contains(string(cb), "#   auto-accept-precision: 0.9") || strings.Contains(string(cb), "soft-conf-threshold") {
			t.Errorf("%s: glossmark.yaml lacks the suggest: block:\n%s", tc.name, cb)
		}
		var un []string
		for _, l := range strings.Split(string(cb), "\n") {
			switch {
			case strings.HasPrefix(l, "# suggest:"):
				l = l[2:]
			case strings.HasPrefix(l, "#   ") && !strings.HasPrefix(strings.TrimSpace(l[1:]), "#"):
				l = " " + l[1:]
			}
			un = append(un, l)
		}
		os.WriteFile(filepath.Join(root, "glossmark.yaml"), []byte(strings.Join(un, "\n")), 0o644)
		if ws2, err := load.Load(root); err != nil {
			t.Errorf("%s: uncommented suggest: block: %v", tc.name, err)
		} else if a, d := ws2.Config.AutoAccept(), ws.Config.AutoAccept(); a.Target != d.Target || a.Threshold != d.Threshold || ws2.Config.SuggestMinConf() != 0.3 {
			t.Errorf("%s: uncommented block %+v, default %+v", tc.name, a, d)
		}
		os.WriteFile(filepath.Join(root, "glossmark.yaml"), cb, 0o644)
		rb, _ := os.ReadFile(filepath.Join(root, tc.runner))
		if !strings.Contains(string(rb), "gloss check") {
			t.Errorf("%s: %s lacks the gloss target:\n%s", tc.name, tc.runner, rb)
		}
		gi, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
		if strings.Contains(string(gi), "ledger") && !strings.Contains(string(gi), "ledger.jsonl are committed") {
			t.Errorf("the ledger must not be ignored:\n%s", gi)
		}
		// A second run changes nothing.
		p2, _ := Make(root, Options{})
		for _, f := range p2.Files {
			if f.Action == Create || (f.Action == Append && f.Path != ".gitignore") {
				t.Errorf("%s: second init would %s %s", tc.name, f.Action, f.Path)
			}
		}
	}
}

// TestInitUtilityDirs (issue #64, F-POOR-7): on the stripped opstool layout
// init plans no manifest for core/ and utils/ (shared code, no entry point)
// and lists them as not a boundary; common/ (a Maven project), web/ and
// sql/ keep theirs. A utility-named directory with an entry point, or a
// project inside a grouping directory, is still a boundary.
func TestInitUtilityDirs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "opstool")
	writeTree(t, root, map[string]string{
		"app.py":            "from core.Processor import Processor\n",
		"helpers.py":        "def fmt(x): return x\n",
		"requirements.txt":  "flask\n",
		"README.md":         "ops tool\n",
		"core/__init__.py":  "",
		"core/Processor.py": "class Processor: pass\n",
		"core/Manager.py":   "class Manager: pass\n",
		"utils/__init__.py": "",
		"utils/common.py":   "def a(): pass\n",
		"utils/misc.py":     "def b(): pass\n",
		"utils/db.py":       "def c(): pass\n",
		"common/pom.xml":    "<project/>\n",
		"common/src/main/java/com/acme/ops/common/Handler.java":     "class Handler {}\n",
		"common/src/main/java/com/acme/ops/common/StringUtils.java": "class StringUtils {}\n",
		"web/page2.php":         "<?php\n",
		"web/functions.php":     "<?php\n",
		"web/misc.js":           "x\n",
		"sql/tables.sql":        "create table tbl_rma();\n",
		"jobs/nightly.sh":       "#!/bin/sh\n",
		"tests/test_helpers.py": "x = 1\n",
	})
	p, err := Make(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(p.Dirs, ","), "common,sql,web"; got != want {
		t.Errorf("dirs = %s, want %s", got, want)
	}
	if got, want := strings.Join(p.Utility, ","), "core,utils"; got != want {
		t.Errorf("utility = %s, want %s", got, want)
	}
	files := planned(p)
	for _, d := range []string{"core", "utils"} {
		if _, ok := files[d+"/.gloss.yaml"]; ok {
			t.Errorf("%s/ got a manifest", d)
		}
		if !strings.Contains(p.String(), d+"/  (not a boundary") {
			t.Errorf("%s/ not listed as not a boundary:\n%s", d, p.String())
		}
	}

	// An entry point, a Dockerfile or a project file makes a utility-named
	// directory a deployable; inside a grouping the same rules apply.
	root2 := filepath.Join(t.TempDir(), "mono")
	writeTree(t, root2, map[string]string{
		"core/main.go":                  "package main\n",
		"lib/Dockerfile":                "FROM scratch\n",
		"lib/x.py":                      "x = 1\n",
		"shared/package.json":           "{}\n",
		"shared/index.ts":               "export {}\n",
		"helpers/server/routes.rb":      "x\n",
		"misc/a.py":                     "x = 1\n",
		"packages/common/package.json":  "{}\n",
		"packages/common/index.ts":      "export {}\n",
		"packages/utils/strings.ts":     "export {}\n",
		"packages/billing/package.json": "{}\n",
		"packages/billing/index.ts":     "export {}\n",
	})
	p2, err := Make(root2, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(p2.Dirs, ","), "core,helpers,lib,packages/billing,packages/common,shared"; got != want {
		t.Errorf("dirs = %s, want %s", got, want)
	}
	if got, want := strings.Join(p2.Utility, ","), "misc,packages/utils"; got != want {
		t.Errorf("utility = %s, want %s", got, want)
	}
}
