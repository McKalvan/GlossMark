package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// workspace copies an example repository into a temp dir so a CLI test can
// write to it.
func workspace(t *testing.T, example string) string {
	t.Helper()
	src := filepath.Join("..", "..", "examples", example)
	dst := t.TempDir()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// gloss runs one CLI invocation and returns its exit code and stdout.
func gloss(t *testing.T, args ...string) (int, string) {
	t.Helper()
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	code := run(args)
	w.Close()
	os.Stdout = old
	return code, <-done
}

func TestParseFlagsInterspersed(t *testing.T) {
	cases := []struct {
		args []string
		pos  []string
		ns   string
		dry  bool
	}{
		{[]string{"--from", "md", "a.md", "--namespace", "lib"}, []string{"a.md"}, "lib", false},
		{[]string{"a.md", "--namespace", "lib", "--dry-run"}, []string{"a.md"}, "lib", true},
		{[]string{"--namespace=lib", "a.md", "b.md", "--dry-run"}, []string{"a.md", "b.md"}, "lib", true},
		{[]string{"a.md", "--", "--namespace", "x"}, []string{"a.md", "--namespace", "x"}, "", false},
		{nil, nil, "", false},
	}
	for _, c := range cases {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.String("from", "", "")
		ns := fs.String("namespace", "", "")
		dry := fs.Bool("dry-run", false, "")
		if err := parseFlags(fs, c.args); err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		if got := fs.Args(); !(len(got) == 0 && len(c.pos) == 0) && !reflect.DeepEqual(got, c.pos) {
			t.Errorf("%v: positionals %v, want %v", c.args, got, c.pos)
		}
		if *ns != c.ns || *dry != c.dry {
			t.Errorf("%v: namespace=%q dry=%v, want %q %v", c.args, *ns, *dry, c.ns, c.dry)
		}
	}
}

// Issue #122: the QUICKSTART order, flags after the file, parses.
func TestImportFlagsAfterFile(t *testing.T) {
	root := workspace(t, "horse-factory")
	md := filepath.Join(root, "glossary.md")
	os.WriteFile(md, []byte("# Glossary\n\n## Paddock\n\n**Def.** A fenced field where horses graze.\n"), 0o644)
	code, out := gloss(t, "import", "--from", "markdown-glossary", md, "--namespace", "zoo", "--dry-run", "--root", root)
	if code != 0 || !strings.Contains(out, "1 added") {
		t.Fatalf("import exit %d, out %q", code, out)
	}
}

// Issue #29: --version and -v are aliases of the version subcommand.
func TestVersionFlags(t *testing.T) {
	_, want := gloss(t, "version")
	for _, a := range []string{"--version", "-v"} {
		code, out := gloss(t, a)
		if code != 0 || out != want || out == "" {
			t.Errorf("gloss %s: exit %d, out %q, want %q", a, code, out, want)
		}
	}
}

// Issue #120: propose --from-new honours the entry flags when one concept is
// lifted (horse-factory carries one impl:NEW:Gelding marker).
func TestProposeFromNewEntryFlags(t *testing.T) {
	root := workspace(t, "horse-factory")
	if code, _ := gloss(t, "propose", "--from-new", "--realizes", "zoo:Breeding", "--aliases", "gelding, castration", "--doc", "Castrating a colt.", "--root", root); code != 0 {
		t.Fatalf("propose --from-new: exit %d", code)
	}
	b, _ := os.ReadFile(filepath.Join(root, "ontology", "impl.gloss.yaml"))
	_, entry, _ := strings.Cut(string(b), "  Gelding:")
	for _, want := range []string{"realizes: zoo:Breeding", "gelding", "castration", "kind: process", "proposed: true"} {
		if !strings.Contains(entry, want) {
			t.Errorf("Gelding entry %q lacks %q", entry, want)
		}
	}
}

// Issue #119: the reviewer's reason survives a marker note= in its own field
// and reaches the PROV export.
func TestRejectReasonBesideNote(t *testing.T) {
	root := workspace(t, "horse-factory")
	sel := "services/horse-factory/breeding/engine.py:22" // the impl:NEW:Gelding proposal, which carries note=
	if code, _ := gloss(t, "reject", sel, "--reason", "gelding is not a step we run", "--root", root); code != 0 {
		t.Fatalf("reject: exit %d", code)
	}
	b, _ := os.ReadFile(filepath.Join(root, ".glossmark", "ledger.jsonl"))
	var row string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "impl:NEW:Gelding") {
			row = l
		}
	}
	if !strings.Contains(row, `"review_reason":"gelding is not a step we run"`) || !strings.Contains(row, `"rationale":"castration step has no concept yet"`) {
		t.Errorf("ledger row = %s", row)
	}
	prov := filepath.Join(root, "prov.jsonld")
	if code, _ := gloss(t, "export", "--prov", prov, "--root", root); code != 0 {
		t.Fatalf("export --prov: exit %d", code)
	}
	if b, _ := os.ReadFile(prov); !strings.Contains(string(b), `"gm:reviewReason": "gelding is not a step we run"`) {
		t.Errorf("PROV export lacks the reason")
	}
}

// tiny writes a minimal two-tier workspace plus the given files.
func tiny(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	base := map[string]string{
		"glossmark.yaml":        "owns:\n  z: { iri: https://x/z/, file: ontology/z.gloss.yaml, tier: core }\n  i: { iri: https://x/i/, file: ontology/i.gloss.yaml, tier: implementation }\n",
		"ontology/z.gloss.yaml": "namespace: z\nconcepts:\n  Doing: { kind: process }\n",
		"ontology/i.gloss.yaml": "namespace: i\nconcepts:\n  Doer: { kind: process, realizes: z:Doing }\n",
		".gloss.yaml":           "boundary: o:b\nlevel: container\ndefault-namespace: i\n",
	}
	for k, v := range files {
		base[k] = v
	}
	for rel, content := range base {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// ledgerStatus returns the ledger status of every row whose line contains sub.
func ledgerStatus(t *testing.T, root, sub string) []string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(root, ".glossmark", "ledger.jsonl"))
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, sub) {
			_, s, _ := strings.Cut(l, `"status":"`)
			s, _, _ = strings.Cut(s, `"`)
			out = append(out, s)
		}
	}
	return out
}

// Issue #42: accepting a scope=file proposal does not stale it, and accept
// re-confirms a link a real edit made stale.
func TestAcceptFileScopeAndReconfirm(t *testing.T) {
	root := tiny(t, map[string]string{"tests/test_doer.py": "# @gloss? verifies .Doer scope=file\ndef test_doer(): pass\n"})
	gloss(t, "build", "-q", "--root", root)
	if code, _ := gloss(t, "accept", "--all", "--root", root); code != 0 {
		t.Fatalf("accept --all: exit %d", code)
	}
	gloss(t, "build", "-q", "--root", root)
	if st := ledgerStatus(t, root, "verifies"); len(st) != 1 || st[0] != "accepted" {
		t.Fatalf("after accept --all and build: %v, want [accepted]", st)
	}
	os.WriteFile(filepath.Join(root, "tests/test_doer.py"), []byte("# @gloss verifies .Doer scope=file\ndef test_doer(): assert True\n"), 0o644)
	gloss(t, "build", "-q", "--root", root)
	if st := ledgerStatus(t, root, "verifies"); len(st) != 1 || st[0] != "stale" {
		t.Fatalf("after a real edit: %v, want [stale]", st)
	}
	if code, _ := gloss(t, "accept", "tests/test_doer.py:1", "--root", root); code != 0 {
		t.Fatalf("accept on a stale link: exit %d", code)
	}
	gloss(t, "build", "-q", "--root", root)
	if st := ledgerStatus(t, root, "verifies"); len(st) != 1 || st[0] != "accepted" {
		t.Fatalf("after re-confirm: %v, want [accepted]", st)
	}
}

// Issue #70: two identical proposals on one symbol share an id; check flags
// the second, and one accept flips both lines.
func TestDuplicateProposals(t *testing.T) {
	dup := "# @gloss? implements .Doer conf=0.9\n"
	root := tiny(t, map[string]string{"doer.py": dup + dup + "def do(): pass\n"})
	p, err := buildGraph(root, false)
	if err != nil {
		t.Fatal(err)
	}
	gm132 := false
	for _, d := range p.Diags {
		if d.Code == "GM132" && d.Line == 2 {
			gm132 = true
		}
	}
	if !gm132 {
		t.Errorf("no GM132 on line 2: %v", p.Diags)
	}
	if code, _ := gloss(t, "accept", "doer.py:1", "--root", root); code != 0 {
		t.Fatalf("accept: exit %d", code)
	}
	b, _ := os.ReadFile(filepath.Join(root, "doer.py"))
	if strings.Contains(string(b), "@gloss?") {
		t.Errorf("a proposal is left after accept:\n%s", b)
	}
	if st := ledgerStatus(t, root, "implements"); len(st) != 1 || st[0] != "accepted" {
		t.Errorf("ledger = %v, want [accepted]", st)
	}
}

// Issue #62: a rejected proposal adds no edge, stays rejected when its line
// is deleted, and is not suggested again.
func TestRejectedProposalSticks(t *testing.T) {
	files := map[string]string{"doer.py": "def doer(): pass\n"}
	root := tiny(t, files)
	_, out := gloss(t, "suggest", "--format", "text", "--root", root)
	if !strings.Contains(out, "implements .Doer") {
		t.Fatalf("suggest does not propose the link to begin with: %q", out)
	}
	gloss(t, "suggest", "--write", "--root", root)
	gloss(t, "build", "-q", "--root", root)
	if code, _ := gloss(t, "reject", "doer.py:1", "--reason", "not the mechanism", "--root", root); code != 0 {
		t.Fatalf("reject: exit %d", code)
	}
	p, err := buildGraph(root, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range p.Graph.Edges() {
		if e.Prov != nil && e.Prov.Path == "doer.py" && e.P == "implements" {
			t.Errorf("rejected link still in the graph: %+v", e)
		}
	}
	if code, _ := gloss(t, "reject", "doer.py:1", "--reason", "not the mechanism", "--remove", "--root", root); code != 0 {
		t.Fatalf("reject --remove: exit %d", code)
	}
	gloss(t, "build", "-q", "--root", root)
	if st := ledgerStatus(t, root, "implements"); len(st) != 1 || st[0] != "rejected" {
		t.Errorf("after reject --remove and build: %v, want [rejected]", st)
	}
	if _, out := gloss(t, "suggest", "--format", "text", "--root", root); strings.Contains(out, "implements .Doer") {
		t.Errorf("suggest proposes the rejected link again: %q", out)
	}
}

// addConcept inserts ontology lines right after the "concepts:" key.
func addConcept(t *testing.T, root, file, lines string) {
	t.Helper()
	p := filepath.Join(root, "ontology", file)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(b), "concepts:\n", "concepts:\n"+lines+"\n", 1)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Issue #121: accept --concept applies the GM320 rule, so a proposed concept
// that is part-of one that realizes is accepted; one that stands for nothing
// is still refused.
func TestAcceptConceptPartOf(t *testing.T) {
	root := workspace(t, "horse-factory")
	addConcept(t, root, "impl.gloss.yaml", "  MareSelection: { kind: process, part-of: HorseBuilder, proposed: true }\n  Loose: { kind: process, proposed: true }")
	if code, _ := gloss(t, "accept", "--concept", "impl:MareSelection", "--root", root); code != 0 {
		t.Fatalf("accept --concept part-of: exit %d", code)
	}
	b, _ := os.ReadFile(filepath.Join(root, "ontology", "impl.gloss.yaml"))
	if strings.Contains(string(b), "MareSelection: { kind: process, part-of: HorseBuilder, proposed: true }") {
		t.Errorf("MareSelection still proposed:\n%s", b)
	}
	if code, _ := gloss(t, "accept", "--concept", "impl:Loose", "--root", root); code == 0 {
		t.Errorf("accept --concept on a concept that stands for nothing succeeded")
	}
	// Issue #122: flags after the CURIE, as QUICKSTART writes it.
	if code, _ := gloss(t, "reject", "--concept", "impl:Loose", "--reason", "not a thing", "--remove", "--root", root); code != 0 {
		t.Fatalf("reject --concept <curie> --reason ... --remove: exit %d", code)
	}
	b, _ = os.ReadFile(filepath.Join(root, "ontology", "impl.gloss.yaml"))
	if strings.Contains(string(b), "Loose:") {
		t.Errorf("reject --remove left the entry")
	}
}
