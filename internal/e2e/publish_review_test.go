// @gloss verifies gmi:Publisher scope=file
// @gloss verifies gmi:ReviewCommands scope=file

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/model"
)

// TestPublish: gloss publish writes one directory per owned namespace. A core
// namespace gets every concept in ontology.jsonld; an implementation
// namespace gets only its published concepts there, plus a graph.jsonl of
// those concepts and every boundary, with no private structure. Each
// directory carries the source file, so an importer can point source: at it.
func TestPublish(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hf")
	copyTree(t, example(t, "horse-factory"), root)
	r := gloss(t, root, "", nil, "publish", "-o", "out")
	if r.code != 0 {
		t.Fatalf("gloss publish: %d %s", r.code, r.stderr)
	}
	for _, want := range []string{"gloss publish: impl: (implementation 0.3.0) → out/impl", "gloss publish: zoo: (core 1.0.0) → out/zoo"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("publish output lacks %q:\n%s", want, r.stdout)
		}
	}
	_, g, _, _ := build(t, root)

	type item struct {
		ID          string `json:"@id"`
		Kind        string `json:"gm:kind"`
		Tier        string `json:"gm:tier"`
		Doc         string `json:"doc"`
		PublishedAs string `json:"published-as"`
	}
	type ontology struct {
		ID      string `json:"@id"`
		Prefix  string `json:"gm:prefix"`
		Tier    string `json:"gm:tier"`
		Version string `json:"gm:version"`
		Graph   []item `json:"@graph"`
	}
	read := func(ns string) (ontology, map[string]item) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, "out", ns, "ontology.jsonld"))
		if err != nil {
			t.Fatal(err)
		}
		var o ontology
		if err := json.Unmarshal(b, &o); err != nil {
			t.Fatalf("%s/ontology.jsonld: %v", ns, err)
		}
		byID := map[string]item{}
		for _, it := range o.Graph {
			byID[it.ID] = it
		}
		return o, byID
	}

	// Core: every concept, with its kind and doc.
	zoo, zooItems := read("zoo")
	if zoo.ID != "https://onto.acme.dev/zoo/" || zoo.Prefix != "zoo" || zoo.Tier != model.TierCore || zoo.Version != "1.0.0" {
		t.Errorf("zoo namespace header: %+v", zoo)
	}
	var want, got []string
	for _, n := range g.NodesOf(model.NodeConcept) {
		if n.NS == "zoo" {
			want = append(want, n.CURIE)
		}
	}
	for id := range zooItems {
		got = append(got, id)
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("zoo ontology.jsonld concepts:\n got %v\nwant %v", got, want)
	}
	if m := zooItems["zoo:Mare"]; m.Kind != "entity" || m.Doc != "A female horse of breeding age." {
		t.Errorf("zoo:Mare: %+v", m)
	}
	if _, err := os.Stat(filepath.Join(root, "out", "zoo", "graph.jsonl")); err == nil {
		t.Error("a core namespace publishes no graph.jsonl")
	}

	// Implementation: only the published concept crosses the seam.
	impl, implItems := read("impl")
	if impl.Tier != model.TierImpl || impl.Version != "0.3.0" {
		t.Errorf("impl namespace header: %+v", impl)
	}
	if len(implItems) != 1 || implItems["impl:FoalingPipelineThroughput"].PublishedAs != "zoo:ProductionCapacity" {
		t.Errorf("impl ontology.jsonld must hold only the published impl:FoalingPipelineThroughput: %+v", implItems)
	}

	f, err := os.Open(filepath.Join(root, "out", "impl", "graph.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sub, err := graph.ReadJSONL(f)
	if err != nil {
		t.Fatalf("impl/graph.jsonl: %v", err)
	}
	if sub.NodeByCurie("impl:FoalingPipelineThroughput") == nil {
		t.Error("impl/graph.jsonl lacks the published concept")
	}
	for _, b := range g.NodesOf(model.NodeBoundary) {
		if sub.NodeByCurie(b.CURIE) == nil {
			t.Errorf("impl/graph.jsonl lacks boundary %s", b.CURIE)
		}
	}
	for _, n := range sub.NodesOf(model.NodeConcept) {
		if n.NS == "impl" && n.Published == "" {
			t.Errorf("impl/graph.jsonl leaks the unpublished %s", n.CURIE)
		}
	}
	for _, n := range sub.NodesOf(model.NodeArtifact) {
		t.Errorf("impl/graph.jsonl leaks artifact %s", n.Path)
	}
	for _, e := range sub.Edges() {
		if sub.Node(e.S) == nil || sub.Node(e.O) == nil {
			t.Errorf("impl/graph.jsonl edge %s %s %s has an endpoint outside the file", e.S, e.P, e.O)
		}
	}

	// The source files ride along, byte for byte.
	for _, ns := range []string{"zoo", "impl"} {
		src := readFile(t, filepath.Join(root, "ontology", ns+".gloss.yaml"))
		if pub := readFile(t, filepath.Join(root, "out", ns, ns+".gloss.yaml")); pub != src {
			t.Errorf("out/%s/%s.gloss.yaml differs from the source", ns, ns)
		}
	}

	// An importer can point source: at a published directory.
	consumer := filepath.Join(t.TempDir(), "stable")
	writeFile(t, filepath.Join(consumer, "glossmark.yaml"), "owns:\n  st: { iri: https://x/st/, tier: core }\nimports:\n  zoo: { iri: https://onto.acme.dev/zoo/, version: \"^1.0\", source: "+filepath.ToSlash(filepath.Join(root, "out", "zoo"))+" }\n")
	writeFile(t, filepath.Join(consumer, "ontology", "st.gloss.yaml"), "concepts:\n  Stable: { kind: entity, doc: Where a mare is kept. }\n")
	writeFile(t, filepath.Join(consumer, "mare.py"), "# @gloss represents zoo:Mare\nclass Mare: pass\n")
	if r := gloss(t, consumer, "", nil, "lock"); r.code != 0 {
		t.Fatalf("gloss lock on the importer: %d %s %s", r.code, r.stdout, r.stderr)
	}
	if r := gloss(t, consumer, "", nil, "check"); r.code != 0 || !strings.Contains(r.stdout+r.stderr, "gloss check: 0 errors, 0 warnings") {
		t.Errorf("an importer of the published zoo directory should check clean: %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestReviewCommands drives gloss accept, reject and status the way a
// reviewer does, and checks the source files and the ledger after each.
// TestLedgerTransitions covers what gloss build records when a human edits
// the marker by hand; this covers the commands.
func TestReviewCommands(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) { writeFile(t, filepath.Join(root, rel), content) }
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Thing: { kind: entity }\n  Doing: { kind: process }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Doer: { kind: process, realizes: z:Doing }\n  Undoer: { kind: process, realizes: z:Doing }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\ndefault-namespace: i\n")
	write("a.py", "# @gloss? implements .Doer conf=0.9\ndef go(x): pass\n")
	write("b.py", "# @gloss? implements .Undoer conf=0.9\ndef stop(x): pass\n")
	write("c.py", "# @gloss? represents z:Thing conf=0.9\nclass Thing: pass\n")
	agent := []string{"GLOSS_ACTOR=agent:bot"}
	alice := []string{"GLOSS_ACTOR=human:alice"}
	bob := []string{"GLOSS_ACTOR=human:bob"}
	run := func(env []string, args ...string) run {
		t.Helper()
		return gloss(t, root, "", env, args...)
	}
	lgPath := filepath.Join(root, ".glossmark", "ledger.jsonl")
	entry := func(path string) model.LedgerEntry {
		t.Helper()
		lg, err := ledger.Open(lgPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range lg.Entries {
			if e.Path == path {
				return *e
			}
		}
		t.Fatalf("no ledger entry for %s", path)
		return model.LedgerEntry{}
	}

	if r := run(agent, "build", "-q"); r.code != 0 {
		t.Fatalf("gloss build: %d %s", r.code, r.stderr)
	}
	r := run(nil, "status")
	if r.code != 0 || !strings.Contains(r.stdout, "proposed (3)") {
		t.Fatalf("status should list three proposals:\n%s%s", r.stdout, r.stderr)
	}
	for _, p := range []string{"a.py", "b.py", "c.py"} {
		if !strings.Contains(r.stdout, p) {
			t.Errorf("status lacks %s:\n%s", p, r.stdout)
		}
	}
	if !strings.Contains(r.stdout, "by agent:bot") || !strings.Contains(r.stdout, "[SOFT]") {
		t.Errorf("status should name the author and the gate:\n%s", r.stdout)
	}

	// A dry run changes nothing.
	if r := run(alice, "accept", "-n", "a.py:1"); r.code != 0 || !strings.Contains(r.stdout, "would accept a.py:1 implements") {
		t.Errorf("accept -n: %d %s %s", r.code, r.stdout, r.stderr)
	}
	if got := readFile(t, filepath.Join(root, "a.py")); !strings.HasPrefix(got, "# @gloss? implements") {
		t.Errorf("accept -n must not edit the file:\n%s", got)
	}
	if e := entry("a.py"); e.Status != model.StatusProposed {
		t.Errorf("accept -n must not touch the ledger: %+v", e)
	}

	// Accept flips the ? and records the reviewer; the author is kept.
	if r := run(alice, "accept", "a.py:1"); r.code != 0 || !strings.Contains(r.stdout, "1 link accepted by human:alice") {
		t.Fatalf("accept: %d %s %s", r.code, r.stdout, r.stderr)
	}
	if got := readFile(t, filepath.Join(root, "a.py")); got != "# @gloss implements .Doer conf=0.9\ndef go(x): pass\n" {
		t.Errorf("accept should only drop the ?:\n%s", got)
	}
	if e := entry("a.py"); e.Status != model.StatusAccepted || e.ReviewedBy != "human:alice" || e.Actor != "agent:bot" || e.Gate == "" {
		t.Errorf("ledger after accept: %+v", e)
	}
	if r := run(alice, "accept", "a.py:1"); r.code != 0 || !strings.Contains(r.stderr, "is not proposed; nothing to accept") {
		t.Errorf("accepting an accepted link should say so: %d %s %s", r.code, r.stdout, r.stderr)
	}

	// Reject needs a reason, records it, and keeps the marker unless asked.
	if r := run(bob, "reject", "b.py:1"); r.code != 2 || !strings.Contains(r.stderr, "--reason is required") {
		t.Errorf("reject without a reason: %d %s", r.code, r.stderr)
	}
	if e := entry("b.py"); e.Status != model.StatusProposed {
		t.Errorf("a refused reject must not touch the ledger: %+v", e)
	}
	if r := run(bob, "reject", entry("b.py").MarkerID, "--reason", "Undoer is the wrong process"); r.code != 0 {
		t.Fatalf("reject by id: %d %s", r.code, r.stderr)
	}
	if e := entry("b.py"); e.Status != model.StatusRejected || e.ReviewedBy != "human:bob" || e.Rationale != "Undoer is the wrong process" {
		t.Errorf("ledger after reject: %+v", e)
	}
	if got := readFile(t, filepath.Join(root, "b.py")); !strings.HasPrefix(got, "# @gloss? implements .Undoer") {
		t.Errorf("reject without --remove keeps the marker:\n%s", got)
	}
	if r := run(bob, "reject", "c.py:1", "--reason", "not a data shape", "--remove"); r.code != 0 {
		t.Fatalf("reject --remove: %d %s", r.code, r.stderr)
	}
	if got := readFile(t, filepath.Join(root, "c.py")); got != "class Thing: pass\n" {
		t.Errorf("reject --remove should delete the marker line:\n%s", got)
	}
	if e := entry("c.py"); e.Status != model.StatusRejected || e.Rationale != "not a data shape" {
		t.Errorf("ledger after reject --remove: %+v", e)
	}

	// The decisions survive the agent's next build.
	if r := run(agent, "build", "-q"); r.code != 0 {
		t.Fatalf("gloss build: %d %s", r.code, r.stderr)
	}
	if e := entry("a.py"); e.Status != model.StatusAccepted || e.ReviewedBy != "human:alice" {
		t.Errorf("acceptance lost on rebuild: %+v", e)
	}
	if e := entry("b.py"); e.Status != model.StatusRejected || e.ReviewedBy != "human:bob" {
		t.Errorf("rejection lost on rebuild: %+v", e)
	}
	r = run(nil, "status")
	if strings.Contains(r.stdout, "proposed (") || !strings.Contains(r.stdout, "rejected (") || !strings.Contains(r.stdout, "b.py") || strings.Contains(r.stdout, "a.py") {
		t.Errorf("status after review should list only the rejections:\n%s", r.stdout)
	}

	// Concepts: propose, accept by CURIE, and reject with --remove.
	if r := run(agent, "propose", "concept", "i:Helper", "--kind", "process", "--realizes", "z:Doing", "--doc", "Helps the doer."); r.code != 0 {
		t.Fatalf("propose concept: %d %s", r.code, r.stderr)
	}
	if r := run(agent, "propose", "concept", "i:Spare", "--kind", "process", "--realizes", "z:Doing", "--doc", "Not needed."); r.code != 0 {
		t.Fatalf("propose concept: %d %s", r.code, r.stderr)
	}
	if r := run(alice, "accept", "--concept", "i:Helper"); r.code != 0 || !strings.Contains(r.stdout, "1 concept accepted by human:alice") {
		t.Fatalf("accept --concept: %d %s %s", r.code, r.stdout, r.stderr)
	}
	if r := run(bob, "reject", "--concept", "i:Spare", "--reason", "duplicate of i:Undoer", "--remove"); r.code != 0 {
		t.Fatalf("reject --concept: %d %s", r.code, r.stderr)
	}
	onto := readFile(t, filepath.Join(root, "ontology", "i.gloss.yaml"))
	helper := onto[strings.Index(onto, "Helper"):]
	if i := strings.Index(helper, "\n  "); i > 0 {
		helper = helper[:i]
	}
	if strings.Contains(helper, "proposed") || strings.Contains(onto, "Spare") {
		t.Errorf("after review i:Helper is plain and i:Spare is gone:\n%s", onto)
	}
	lg, err := ledger.Open(lgPath)
	if err != nil {
		t.Fatal(err)
	}
	if e := lg.Entry("concept:i:Helper"); e == nil || e.Status != model.StatusAccepted || e.ReviewedBy != "human:alice" {
		t.Errorf("ledger concept:i:Helper: %+v", e)
	}
	if e := lg.Entry("concept:i:Spare"); e == nil || e.Status != model.StatusRejected || e.Rationale != "duplicate of i:Undoer" {
		t.Errorf("ledger concept:i:Spare: %+v", e)
	}
}
