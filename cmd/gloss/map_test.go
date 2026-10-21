// @gloss verifies gmi:MappingCommands scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/mcp"
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

// mcpCall sends one tools/call to the MCP server over the repository and
// returns its structuredContent (or the error text).
func mcpCall(t *testing.T, root, tool string, args any) (json.RawMessage, string) {
	t.Helper()
	a, _ := json.Marshal(args)
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + string(a) + `}}` + "\n"
	var buf strings.Builder
	if err := mcp.New(&mcpBackend{root: root}, strings.NewReader(req), &buf, "test").Serve(); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Result struct {
			IsError           bool            `json:"isError"`
			StructuredContent json.RawMessage `json:"structuredContent"`
			Content           []struct{ Text string }
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(buf.String()), &resp); err != nil {
		t.Fatalf("%v: %s", err, buf.String())
	}
	if resp.Result.IsError {
		return nil, resp.Result.Content[0].Text
	}
	return resp.Result.StructuredContent, ""
}

// Agents propose mappings over MCP, search shows accepted counterparts, and
// proposing a concept that another namespace already names warns of the
// duplicate.
func TestMapMCP(t *testing.T) {
	t.Setenv("GLOSS_ACTOR", "agent:claude-code")
	root := zooFarm(t)
	gloss(t, "map", "init", "zoo-farm", "--owner", "farm-ops", "--root", root)
	out, errText := mcpCall(t, root, "propose_mapping", map[string]any{"subject": "zoo:Horse", "predicate": "exact", "object": "farm:Equine", "conf": 0.9, "comment": "same animal"})
	if errText != "" {
		t.Fatalf("propose_mapping: %s", errText)
	}
	var res MappingResult
	json.Unmarshal(out, &res)
	if res.Subject != "farm:Equine" || !res.Swapped || res.Gate != "HARD" || !strings.Contains(res.Next, "gloss map accept") {
		t.Errorf("propose_mapping = %+v", res)
	}
	if f := setFile(t, root); !strings.Contains(f, "semapv:LLMBasedMatching") || !strings.Contains(f, "agent:claude-code") {
		t.Errorf("an agent proposal is not attributed as LLM-based:\n%s", f)
	}
	if _, errText := mcpCall(t, root, "propose_mapping", map[string]any{"subject": "farm:Equine", "predicate": "exact", "object": "zoo:Pegasus"}); errText == "" {
		t.Errorf("propose_mapping to a missing concept succeeded")
	}

	t.Setenv("GLOSS_ACTOR", "human:bob")
	gloss(t, "map", "accept", "farm:Equine", "exact", "zoo:Horse", "--root", root)
	out, _ = mcpCall(t, root, "search_concepts", map[string]any{"query": "equine"})
	var hits []mcp.Hit
	json.Unmarshal(out, &hits)
	found := false
	for _, h := range hits {
		if h.Curie == "farm:Equine" {
			found = true
			if len(h.Mapped) != 1 || h.Mapped[0] != "skos:exactMatch zoo:Horse (accepted)" {
				t.Errorf("farm:Equine mapped = %v", h.Mapped)
			}
		}
	}
	if !found {
		t.Errorf("search_concepts equine: no farm:Equine in %s", out)
	}

	out, errText = mcpCall(t, root, "propose_concept", map[string]any{"curie": "farm:Horse", "kind": "entity", "doc": "A domesticated equine."})
	if errText != "" {
		t.Fatalf("propose_concept: %s", errText)
	}
	if !strings.Contains(string(out), "possible_duplicates") || !strings.Contains(string(out), "zoo:Horse") {
		t.Errorf("propose_concept farm:Horse does not warn of zoo:Horse: %s", out)
	}
}

// gloss check reports what is wrong with a mapping set's rows (ADR 0025 §7).
func TestMapCheckCodes(t *testing.T) {
	root := zooFarm(t)
	zoo := filepath.Join(filepath.Dir(root), "zoo", "ontology", "zoo.gloss.yaml")
	b, _ := os.ReadFile(zoo)
	os.WriteFile(zoo, append(b, []byte("  Mare: { kind: entity, deprecated: true, replaced-by: Horse }\n")...), 0o644)
	farm := filepath.Join(root, "ontology", "farm.gloss.yaml")
	b, _ = os.ReadFile(farm)
	os.WriteFile(farm, append(b, []byte("  Pony: { kind: entity, proposed: true, proposed-by: \"agent:x\" }\n")...), 0o644)
	gloss(t, "map", "init", "zoo-farm", "--root", root)
	set := filepath.Join(root, "mappings", "zoo-farm.sssom.tsv")
	b, _ = os.ReadFile(set)
	head, _, _ := strings.Cut(string(b), "subject_id")
	rows := []string{
		"farm:Equine\tskos:exactMatch\t\tzoo:Unicorn\tsemapv:ManualMappingCuration\t",     // GM701 missing
		"farm:Equine\tskos:relatedMatch\t\tnope:Thing\tsemapv:ManualMappingCuration\t",    // GM701 prefix
		"zoo:Horse\tskos:closeMatch\t\tfarm:Equine\tsemapv:ManualMappingCuration\t",       // GM702 orientation
		"farm:Stall\tskos:exactMatch\t\tfarm:Equine\tsemapv:ManualMappingCuration\t",      // GM705 same namespace
		"farm:FoalBirth\tskos:exactMatch\t\tzoo:FoalBorn\tsemapv:ManualMappingCuration\t", // fine
		"farm:FoalBirth\tskos:relatedMatch\t\tzoo:Stall\tsemapv:ManualMappingCuration\t",  // fine
		"farm:Colt\tskos:exactMatch\t\tzoo:Foal\tsemapv:ManualMappingCuration\t",          // GM707 with the next
		"farm:Colt\tskos:exactMatch\t\tzoo:Horse\tsemapv:ManualMappingCuration\t",
		"farm:Stall\tskos:exactMatch\t\tzoo:Horse\tsemapv:ManualMappingCuration\t", // GM706 kinds
		"farm:Colt\tskos:closeMatch\t\tzoo:Animal\tsemapv:ManualMappingCuration\t", // GM704 with the next
		"farm:Colt\tskos:closeMatch\tNot\tzoo:Animal\tsemapv:ManualMappingCuration\thuman:x",
		"farm:Colt\trdfs:seeAlso\t\tzoo:Foal\tsemapv:ManualMappingCuration\t",           // GM709
		"farm:Equine\tskos:closeMatch\t\tzoo:Mare\tsemapv:ManualMappingCuration\t",      // GM710
		"farm:Pony\tskos:closeMatch\t\tzoo:Foal\tsemapv:ManualMappingCuration\thuman:x", // GM711
	}
	os.WriteFile(set, []byte(head+"subject_id\tpredicate_id\tpredicate_modifier\tobject_id\tmapping_justification\treviewer_id\n"+strings.Join(rows, "\n")+"\n"), 0o644)
	out, _ := capture(t, "check", "--root", root)
	for _, want := range [][2]string{
		{"GM701", "zoo:Unicorn does not exist"}, {"GM701", "nope:Thing: unknown prefix"}, {"GM702", "zoo:Horse skos:closeMatch farm:Equine"},
		{"GM704", "farm:Colt skos:closeMatch zoo:Animal"}, {"GM705", "farm:Stall exactly matches farm:Equine"}, {"GM706", "farm:Stall (resource) exactly matches zoo:Horse"},
		{"GM707", "farm:Colt exactly matches 2 concepts of zoo"}, {"GM709", "rdfs:seeAlso"}, {"GM710", "zoo:Mare is deprecated"}, {"GM711", "farm:Pony is a proposed concept"},
	} {
		ok := false
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "["+want[0]+"]") && strings.Contains(l, want[1]) {
				ok = true
			}
		}
		if !ok {
			t.Errorf("check lacks %s %s:\n%s", want[0], want[1], out)
		}
	}
	// GM707 points at the row that adds the second counterpart.
	b, _ = os.ReadFile(set)
	second := 0
	for i, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "farm:Colt\tskos:exactMatch\t\tzoo:Horse") {
			second = i + 1
		}
	}
	if !strings.Contains(out, fmt.Sprintf("zoo-farm.sssom.tsv:%d: warning: farm:Colt exactly matches 2", second)) {
		t.Errorf("GM707 is not at the second row:\n%s", out)
	}
	if strings.Contains(out, "[GM700]") {
		t.Errorf("init did not declare the namespaces in curie_map:\n%s", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "zoo:FoalBorn") || strings.Contains(l, "zoo:Stall") && strings.Contains(l, "relatedMatch") {
			t.Errorf("a valid row is flagged: %s", l)
		}
	}
}
