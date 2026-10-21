// @gloss verifies gmi:MappingCommands scope=file

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// zooFarm writes two sibling repositories: zoo owns zoo:, farm owns farm:
// and imports zoo:. It returns the farm root.
func zooFarm(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"zoo/glossmark.yaml":            "owns:\n  zoo: { iri: https://x/zoo/, file: ontology/zoo.gloss.yaml, tier: core, version: 1.0.0, owner: zoo-product }\n",
		"zoo/ontology/zoo.gloss.yaml":   "namespace: zoo\nversion: 1.0.0\nconcepts:\n  Animal: { kind: entity }\n  Horse: { kind: entity, is-a: Animal, doc: \"A domesticated equine.\" }\n  Foal: { kind: entity, is-a: Horse }\n  FoalBorn: { kind: event }\n  Stall: { kind: resource }\n",
		"zoo/.gloss.yaml":               "boundary: o:zoo\nlevel: container\n",
		"farm/glossmark.yaml":           "owns:\n  farm: { iri: https://x/farm/, file: ontology/farm.gloss.yaml, tier: core, version: 0.1.0, owner: farm-ops }\nimports:\n  zoo: { iri: https://x/zoo/, source: ../zoo/ontology/zoo.gloss.yaml }\n",
		"farm/ontology/farm.gloss.yaml": "namespace: farm\nversion: 0.1.0\nconcepts:\n  Equine: { kind: entity, doc: \"Any horse kept on the farm.\" }\n  Colt: { kind: entity, is-a: Equine }\n  FoalBirth: { kind: event }\n  Stall: { kind: resource }\n",
		"farm/.gloss.yaml":              "boundary: o:farm\nlevel: container\n",
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "farm")
}

func mapStatusJSON(t *testing.T, root string) map[string]string {
	t.Helper()
	code, out := gloss(t, "map", "status", "--format", "json", "--root", root)
	if code != 0 {
		t.Fatalf("map status: exit %d\n%s", code, out)
	}
	var sts []struct{ Subject, Predicate, Object, Status string }
	if err := json.Unmarshal([]byte(out), &sts); err != nil {
		t.Fatalf("map status json: %v\n%s", err, out)
	}
	m := map[string]string{}
	for _, s := range sts {
		m[s.Subject+" "+s.Predicate+" "+s.Object] = s.Status
	}
	return m
}

func setFile(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "mappings", "zoo-farm.sssom.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The mapping review loop end to end: init declares a set, propose writes a
// proposed row with the owned concept as subject, accept reviews it, a
// definition change makes it stale, and reject leaves a sticky negation.
func TestMapLifecycle(t *testing.T) {
	t.Setenv("GLOSS_ACTOR", "human:bob")
	root := zooFarm(t)
	if code, out := gloss(t, "map", "init", "zoo-farm", "--owner", "farm-ops", "--root", root); code != 0 || !strings.Contains(out, "mappings/zoo-farm.sssom.tsv") {
		t.Fatalf("map init: exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "glossmark.yaml")); !strings.Contains(string(b), "zoo-farm") {
		t.Fatalf("init did not declare the set:\n%s", b)
	}
	if code, _ := gloss(t, "map", "init", "zoo-farm", "--root", root); code == 0 {
		t.Errorf("init of a declared set succeeded")
	}

	// Object-first: the owned concept becomes the subject.
	code, out := gloss(t, "map", "propose", "zoo:Horse", "exact", "farm:Equine", "--comment", "same animal", "--root", root)
	if code != 0 || !strings.Contains(out, "farm:Equine skos:exactMatch zoo:Horse") {
		t.Fatalf("propose: exit %d\n%s", code, out)
	}
	// broad flips to narrow when swapped.
	if code, out := gloss(t, "map", "propose", "zoo:Foal", "broad", "farm:Colt", "--root", root); code != 0 || !strings.Contains(out, "farm:Colt skos:narrowMatch zoo:Foal") {
		t.Fatalf("propose broad swapped: exit %d\n%s", code, out)
	}
	if code, _ := gloss(t, "map", "propose", "farm:Equine", "exact", "zoo:Horse", "--root", root); code == 0 {
		t.Errorf("proposing a mapping already in the set succeeded")
	}
	if code, _ := gloss(t, "map", "propose", "farm:Equine", "exact", "zoo:Unicorn", "--root", root); code == 0 {
		t.Errorf("proposing a mapping to a missing concept succeeded")
	}
	if code, _ := gloss(t, "map", "propose", "farm:Equine", "sideways", "zoo:Horse", "--root", root); code == 0 {
		t.Errorf("an unknown predicate was accepted")
	}
	st := mapStatusJSON(t, root)
	if st["farm:Equine skos:exactMatch zoo:Horse"] != "proposed" || st["farm:Colt skos:narrowMatch zoo:Foal"] != "proposed" {
		t.Fatalf("status after propose: %v", st)
	}

	if code, out := gloss(t, "map", "accept", "farm:Equine", "exact", "zoo:Horse", "--root", root); code != 0 {
		t.Fatalf("accept: exit %d\n%s", code, out)
	}
	if f := setFile(t, root); !strings.Contains(f, "human:bob") {
		t.Errorf("accept did not record the reviewer:\n%s", f)
	}
	if st := mapStatusJSON(t, root); st["farm:Equine skos:exactMatch zoo:Horse"] != "accepted" {
		t.Fatalf("status after accept: %v", st)
	}

	// The imported endpoint's definition changes: the accepted mapping is stale.
	zoo := filepath.Join(filepath.Dir(root), "zoo", "ontology", "zoo.gloss.yaml")
	b, _ := os.ReadFile(zoo)
	os.WriteFile(zoo, []byte(strings.Replace(string(b), "A domesticated equine.", "A large hoofed mammal, wild or tame.", 1)), 0o644)
	if st := mapStatusJSON(t, root); st["farm:Equine skos:exactMatch zoo:Horse"] != "stale" {
		t.Fatalf("status after the endpoint changed: %v, want stale", st)
	}
	if _, out := gloss(t, "map", "status", "--root", root); !strings.Contains(out, "changed since review") {
		t.Errorf("text status does not say what changed:\n%s", out)
	}
	if code, _ := gloss(t, "map", "accept", "farm:Equine", "exact", "zoo:Horse", "--root", root); code != 0 {
		t.Fatalf("re-accept a stale mapping: exit %d", code)
	}
	if st := mapStatusJSON(t, root); st["farm:Equine skos:exactMatch zoo:Horse"] != "accepted" {
		t.Fatalf("status after re-accept: %v", st)
	}

	// Reject needs a reason; once rejected, proposing again needs --force.
	if code, _ := gloss(t, "map", "reject", "farm:Colt", "narrow", "zoo:Foal", "--root", root); code == 0 {
		t.Errorf("reject without --reason succeeded")
	}
	if code, out := gloss(t, "map", "reject", "farm:Colt", "narrow", "zoo:Foal", "--reason", "a colt is male; a foal is any young horse", "--root", root); code != 0 {
		t.Fatalf("reject: exit %d\n%s", code, out)
	}
	if f := setFile(t, root); !strings.Contains(f, "\tNot\t") {
		t.Errorf("reject did not write a negation row:\n%s", f)
	}
	if st := mapStatusJSON(t, root); st["farm:Colt skos:narrowMatch zoo:Foal"] != "rejected" {
		t.Fatalf("status after reject: %v", st)
	}
	if code, _ := gloss(t, "map", "propose", "farm:Colt", "narrow", "zoo:Foal", "--root", root); code == 0 {
		t.Errorf("re-proposing a rejected mapping without --force succeeded")
	}
	if code, _ := gloss(t, "map", "propose", "farm:Colt", "narrow", "zoo:Foal", "--force", "--root", root); code != 0 {
		t.Errorf("re-proposing a rejected mapping with --force failed")
	}
	if st := mapStatusJSON(t, root); st["farm:Colt skos:narrowMatch zoo:Foal"] != "proposed" {
		t.Errorf("status after a forced re-proposal: %v", st)
	}
}

// Repoint moves a mapping off a deprecated endpoint to its successor and
// leaves it proposed for review.
func TestMapRepoint(t *testing.T) {
	t.Setenv("GLOSS_ACTOR", "human:bob")
	root := zooFarm(t)
	gloss(t, "map", "init", "zoo-farm", "--owner", "farm-ops", "--root", root)
	gloss(t, "map", "propose", "farm:FoalBirth", "exact", "zoo:FoalBorn", "--root", root)
	gloss(t, "map", "accept", "farm:FoalBirth", "exact", "zoo:FoalBorn", "--root", root)
	if code, _ := gloss(t, "map", "repoint", "farm:FoalBirth", "exact", "zoo:FoalBorn", "--root", root); code == 0 {
		t.Errorf("repoint with no deprecated endpoint succeeded")
	}
	zoo := filepath.Join(filepath.Dir(root), "zoo", "ontology", "zoo.gloss.yaml")
	b, _ := os.ReadFile(zoo)
	s := strings.Replace(string(b), "FoalBorn: { kind: event }", "FoalBorn: { kind: event, deprecated: true, replaced-by: Delivered }\n  Delivered: { kind: event }", 1)
	os.WriteFile(zoo, []byte(s), 0o644)
	if _, out := gloss(t, "map", "status", "--root", root); !strings.Contains(out, "gloss map repoint") {
		t.Errorf("status does not suggest repoint:\n%s", out)
	}
	if code, out := gloss(t, "map", "repoint", "farm:FoalBirth", "exact", "zoo:FoalBorn", "--root", root); code != 0 || !strings.Contains(out, "zoo:Delivered") {
		t.Fatalf("repoint: exit %d\n%s", code, out)
	}
	st := mapStatusJSON(t, root)
	if st["farm:FoalBirth skos:exactMatch zoo:Delivered"] != "proposed" {
		t.Errorf("status after repoint: %v", st)
	}
	if _, ok := st["farm:FoalBirth skos:exactMatch zoo:FoalBorn"]; ok {
		t.Errorf("repoint left the old row: %v", st)
	}
}

// Suggest finds lexical matches with a kind check, --write proposes them,
// and export renders every set as JSON records and as Turtle.
func TestMapSuggestAndExport(t *testing.T) {
	t.Setenv("GLOSS_ACTOR", "agent:claude-code")
	root := zooFarm(t)
	gloss(t, "map", "init", "zoo-farm", "--owner", "farm-ops", "--root", root)
	code, out := gloss(t, "map", "suggest", "--format", "json", "--root", root)
	if code != 0 || !strings.Contains(out, "farm:Stall") || !strings.Contains(out, "zoo:Stall") {
		t.Fatalf("suggest: exit %d\n%s", code, out)
	}
	if code, out := gloss(t, "map", "suggest", "--write", "--root", root); code != 0 {
		t.Fatalf("suggest --write: exit %d\n%s", code, out)
	}
	if f := setFile(t, root); !strings.Contains(f, "farm:Stall\tskos:exactMatch\tzoo:Stall\tsemapv:LexicalMatching") {
		t.Errorf("suggest --write did not add a lexical row:\n%s", f)
	}
	if st := mapStatusJSON(t, root); st["farm:Stall skos:exactMatch zoo:Stall"] != "proposed" {
		t.Errorf("a written suggestion is not proposed: %v", st)
	}
	// Already proposed: suggest does not repeat it.
	if _, out := gloss(t, "map", "suggest", "--format", "json", "--root", root); strings.Contains(out, "zoo:Stall") {
		t.Errorf("suggest repeats a proposed pair:\n%s", out)
	}

	if code, out := gloss(t, "map", "export", "--root", root); code != 0 || !strings.Contains(out, `"subject_id": "farm:Stall"`) || !strings.Contains(out, "https://x/farm/") {
		t.Errorf("export json: exit %d\n%s", code, out)
	}
	if code, out := gloss(t, "map", "export", "--format", "rdf", "--root", root); code != 0 || !strings.Contains(out, "<https://x/farm/Stall>") {
		t.Errorf("export rdf: exit %d\n%s", code, out)
	}
}
