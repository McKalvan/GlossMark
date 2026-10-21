// @gloss verifies gmi:Gate scope=file

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/gate"
)

// mapDiff runs gloss diff --format json against HEAD and returns each
// mapping or version-bump change as "kind gate subject".
func mapDiff(t *testing.T, root string) []string {
	t.Helper()
	_, out := gloss(t, "diff", "--base", "HEAD", "--format", "json", "--fail-on", "none", "--root", root)
	var im gate.Impact
	if err := json.Unmarshal([]byte(out), &im); err != nil {
		t.Fatalf("diff json: %v\n%s", err, out)
	}
	var got []string
	for _, c := range im.Changes {
		if strings.HasPrefix(c.Kind, "mapping-") || c.Kind == "version-bump" && strings.HasPrefix(c.Subject, "mapping-set:") {
			got = append(got, string(c.Kind)+" "+string(c.Gate)+" "+c.Subject)
		}
	}
	return got
}

func hasChange(got []string, want string) bool {
	for _, g := range got {
		if g == want {
			return true
		}
	}
	return false
}

// TestMapDiffGates: gloss diff gates mapping changes by predicate (exact
// HARD, close SOFT), passes acceptance and rejection as AUTO, reports an
// accepted mapping whose endpoint's definition changed, requires the
// mapping_set_version bump the decisions imply, and flags a mapped concept
// the diff removes.
func TestMapDiffGates(t *testing.T) {
	t.Setenv("GLOSS_ACTOR", "human:bob")
	root, write, git := gitRepo(t)
	write("glossmark.yaml", "owns:\n  farm: { iri: https://x/farm/, file: ontology/farm.gloss.yaml, tier: core, version: 0.1.0 }\nimports:\n  zoo: { iri: https://x/zoo/, source: vendor/zoo.gloss.yaml }\n")
	write("ontology/farm.gloss.yaml", "namespace: farm\nversion: 0.1.0\nconcepts:\n  Equine: { kind: entity, doc: \"Any horse.\" }\n  Stall: { kind: resource }\n  FoalBirth: { kind: event }\n")
	write("vendor/zoo.gloss.yaml", "namespace: zoo\nversion: 1.0.0\nconcepts:\n  Horse: { kind: entity }\n  Stall: { kind: resource }\n  FoalBorn: { kind: event }\n")
	write(".gloss.yaml", "boundary: o:farm\nlevel: container\n")
	gloss(t, "map", "init", "zoo-farm", "--owner", "farm-ops", "--root", root)
	gloss(t, "map", "propose", "farm:Equine", "exact", "zoo:Horse", "--root", root)
	gloss(t, "map", "accept", "farm:Equine", "exact", "zoo:Horse", "--root", root)
	git("add", ".")
	git("commit", "-q", "-m", "base")

	// Proposals gate by predicate; a definition change stales Equine = Horse.
	gloss(t, "map", "propose", "farm:Stall", "exact", "zoo:Stall", "--root", root)
	gloss(t, "map", "propose", "farm:FoalBirth", "close", "zoo:FoalBorn", "--root", root)
	b, _ := os.ReadFile(filepath.Join(root, "ontology/farm.gloss.yaml"))
	write("ontology/farm.gloss.yaml", strings.Replace(string(b), "Any horse.", "Any horse, donkey or mule.", 1))
	got := mapDiff(t, root)
	for _, want := range []string{"mapping-proposed HARD farm:Stall", "mapping-proposed SOFT farm:FoalBirth", "mapping-affected SOFT farm:Equine"} {
		if !hasChange(got, want) {
			t.Errorf("diff lacks %q: %v", want, got)
		}
	}
	if hasChange(got, "version-bump HARD mapping-set:zoo-farm") {
		t.Errorf("proposals alone required a version bump: %v", got)
	}
	if st := mapStatusJSON(t, root); st["farm:Equine skos:exactMatch zoo:Horse"] != "stale" {
		t.Errorf("Equine = Horse after the definition change: %v, want stale", st)
	}
	gloss(t, "map", "accept", "farm:Equine", "exact", "zoo:Horse", "--root", root)
	git("add", ".")
	git("commit", "-q", "-m", "proposals")

	// Accepting adds a mapping the landscape relies on: a MINOR bump.
	gloss(t, "map", "accept", "farm:Stall", "exact", "zoo:Stall", "--root", root)
	gloss(t, "map", "reject", "farm:FoalBirth", "close", "zoo:FoalBorn", "--reason", "a birth is not the announcement", "--root", root)
	got = mapDiff(t, root)
	for _, want := range []string{"mapping-accepted AUTO farm:Stall", "mapping-rejected AUTO farm:FoalBirth", "version-bump HARD mapping-set:zoo-farm"} {
		if !hasChange(got, want) {
			t.Errorf("diff lacks %q: %v", want, got)
		}
	}
	set := filepath.Join(root, "mappings", "zoo-farm.sssom.tsv")
	b, _ = os.ReadFile(set)
	os.WriteFile(set, []byte(strings.Replace(string(b), "mapping_set_version: 0.1.0", "mapping_set_version: 0.2.0", 1)), 0o644)
	if got := mapDiff(t, root); hasChange(got, "version-bump HARD mapping-set:zoo-farm") {
		t.Errorf("a MINOR bump did not satisfy the version check: %v", got)
	}
	git("add", ".")
	git("commit", "-q", "-m", "reviewed")

	// Removing a mapped concept leaves the mapping dangling.
	b, _ = os.ReadFile(filepath.Join(root, "ontology/farm.gloss.yaml"))
	write("ontology/farm.gloss.yaml", strings.Replace(string(b), "  Stall: { kind: resource }\n", "", 1))
	if got := mapDiff(t, root); !hasChange(got, "mapping-dangling HARD farm:Stall") {
		t.Errorf("diff lacks the dangling mapping: %v", got)
	}
}
