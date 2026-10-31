// @gloss verifies gmi:Stripper scope=file

package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestStrip (#263): after gloss strip, git diff shows only marker lines,
// the agents block and GlossMark files removed, excluded and generated
// files are untouched, and the export opens in SQLite and parses as one
// JSON-LD document. --dry-run writes nothing; --keep-ontology keeps the
// vocabulary.
func TestStrip(t *testing.T) {
	root, write, git := gitRepo(t)
	write("glossmark.yaml", "owns:\n  c: { iri: https://x.test/c/, tier: core }\n  ci: { iri: https://x.test/ci/, tier: implementation }\nexclude: [\"vendor/**\"]\n")
	write("ontology/c.gloss.yaml", "namespace: c\nversion: 1.0.0\nconcepts:\n  Patient: { kind: entity }\n  Booking: { kind: process }\n")
	write("ontology/ci.gloss.yaml", "namespace: ci\nversion: 1.0.0\nconcepts:\n  Booker: { kind: process, realizes: c:Booking }\n")
	write(".gloss.yaml", "boundary: o:clinic\nlevel: system\nrealizes: inherit\n")
	write("app/.gloss.yaml", "boundary: o:app\nlevel: container\npart-of: o:clinic\nrealizes: [ci:Booker]\ndefault-namespace: ci\ngenerated: [\"gen/**\"]\n")
	write("app/book.py", "# @gloss implements .Booker\ndef book(): pass\n\nx = 1  # @gloss? represents c:Patient conf=0.5\ns = '# @gloss implements .Booker'\n")
	write("app/gen/model.py", "# @gloss represents c:Patient\nclass Patient: pass\n")
	write("app/config.json", "{\"a\": 1}\n")
	write("app/config.json.gloss", "@gloss configures ci:Booker\n")
	write("app/index.html", "<!-- @gloss presents ci:Booker scope=file -->\n<p>hi</p>\n")
	write("db/init.sql", "-- @gloss represents c:Patient\ncreate table patients (id int);\n")
	write("vendor/lib.py", "# @gloss implements ci:Booker\n")
	write("AGENTS.md", "# Agents\n\nRead this.\n\n<!-- gloss:begin -->\n## GlossMark vocabulary\n<!-- gloss:end -->\n")
	write("README.md", "# Repo\n\nThe block sits between `<!-- gloss:begin -->` and `<!-- gloss:end -->`.\n")
	if out, code := capture(t, "build", "--root", root, "-q"); code != 0 {
		t.Fatalf("build: %d\n%s", code, out)
	}
	git("add", ".")
	git("commit", "-q", "-m", "marked")

	out, code := capture(t, "strip", "--root", root, "--dry-run")
	if code != 0 {
		t.Fatalf("dry run: %d\n%s", code, out)
	}
	for _, want := range []string{"edit   app/book.py: 2 markers", "edit   AGENTS.md: the gloss:begin/end block", "delete app/config.json.gloss", "delete .glossmark/", "delete ontology/c.gloss.yaml", "delete glossmark.yaml", "nothing written"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "vendor/") || strings.Contains(out, "app/gen/") || strings.Contains(out, "README.md") {
		t.Errorf("dry run touches excluded, generated or unaffected files:\n%s", out)
	}
	if st := gitText(t, root, "status", "--porcelain"); st != "" {
		t.Fatalf("dry run changed the tree:\n%s", st)
	}

	out, code = capture(t, "strip", "--root", root)
	if code != 0 {
		t.Fatalf("strip: %d\n%s", code, out)
	}
	if !strings.Contains(out, "4 markers removed from 3 files") {
		t.Errorf("summary:\n%s", out)
	}
	// git diff: only marker lines and the agents block are gone; the one
	// line added is the code a trailing marker sat after, without it.
	diff := gitText(t, root, "diff", "--", ".", ":!.glossmark", ":!*.gloss.yaml", ":!glossmark.yaml", ":!*.gloss")
	var removed []string
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "+++") || strings.HasPrefix(l, "---"):
		case strings.HasPrefix(l, "-") && !strings.Contains(l, "@gloss") && !strings.Contains(l, "gloss:") && strings.TrimSpace(l) != "-" && !strings.Contains(l, "GlossMark vocabulary"):
			t.Errorf("strip removed a non-marker line: %q", l)
		case strings.HasPrefix(l, "-"):
			removed = append(removed, l[1:])
		}
	}
	for _, l := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(l, "+") || strings.HasPrefix(l, "+++") {
			continue
		}
		ok := false
		for _, r := range removed {
			if strings.HasPrefix(r, l[1:]) && strings.Contains(r, "@gloss") {
				ok = true
			}
		}
		if !ok {
			t.Errorf("strip added a line: %q", l)
		}
	}
	for rel, want := range map[string]string{
		"app/book.py":      "def book(): pass\n\nx = 1\ns = '# @gloss implements .Booker'\n",
		"app/index.html":   "<p>hi</p>\n",
		"db/init.sql":      "create table patients (id int);\n",
		"AGENTS.md":        "# Agents\n\nRead this.\n",
		"app/gen/model.py": "# @gloss represents c:Patient\nclass Patient: pass\n",
		"vendor/lib.py":    "# @gloss implements ci:Booker\n",
		"README.md":        "# Repo\n\nThe block sits between `<!-- gloss:begin -->` and `<!-- gloss:end -->`.\n",
	} {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil || string(b) != want {
			t.Errorf("%s after strip:\n%s\nwant:\n%s (%v)", rel, b, want, err)
		}
	}
	for _, rel := range []string{".glossmark", "glossmark.yaml", "ontology", ".gloss.yaml", "app/.gloss.yaml", "app/config.json.gloss"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Errorf("%s still exists", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "app/config.json")); err != nil {
		t.Error("the sidecar's JSON file is gone")
	}
	// The export: SQLite with the closure views, one JSON-LD document, PROV-O.
	exp := filepath.Join(root, "glossmark-export")
	db, err := sql.Open("sqlite", filepath.Join(exp, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM realizes_closure").Scan(&n); err != nil || n == 0 {
		t.Errorf("graph.db realizes_closure: %d, %v", n, err)
	}
	var doc struct {
		Context map[string]any   `json:"@context"`
		Graph   []map[string]any `json:"@graph"`
	}
	b, err := os.ReadFile(filepath.Join(exp, "graph.jsonld"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &doc); err != nil || doc.Context["c"] != "https://x.test/c/" || len(doc.Graph) == 0 {
		t.Errorf("graph.jsonld: %v, context %v, %d records", err, doc.Context["c"], len(doc.Graph))
	}
	ids := 0
	for _, r := range doc.Graph {
		if _, ok := r["id"]; ok {
			ids++
		}
	}
	if ids == 0 {
		t.Error("graph.jsonld has no node with an id")
	}
	for _, f := range []string{"graph.jsonl", "context.jsonld", "ledger.jsonl", "ledger.prov.jsonld", "glossmark.yaml", "ontology/c.gloss.yaml", "ontology/ci.gloss.yaml", "manifests/.gloss.yaml", "manifests/app/.gloss.yaml"} {
		if _, err := os.Stat(filepath.Join(exp, f)); err != nil {
			t.Errorf("export lacks %s", f)
		}
	}
	b, _ = os.ReadFile(filepath.Join(exp, "ledger.prov.jsonld"))
	var prov map[string]any
	if err := json.Unmarshal(b, &prov); err != nil || prov["@context"] == nil {
		t.Errorf("ledger.prov.jsonld: %v", err)
	}

	// --keep-ontology on a fresh checkout keeps the vocabulary.
	git("checkout", "-q", ".")
	os.RemoveAll(exp)
	if out, code := capture(t, "strip", "--root", root, "--keep-ontology", "--export", "-"); code != 0 {
		t.Fatalf("keep: %d\n%s", code, out)
	}
	for _, rel := range []string{"glossmark.yaml", "ontology/c.gloss.yaml"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("--keep-ontology removed %s", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".gloss.yaml")); err == nil {
		t.Error("--keep-ontology kept the manifest")
	}
	if _, err := os.Stat(exp); err == nil {
		t.Error("--export - wrote an export")
	}
}

func gitText(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}
