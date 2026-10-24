package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Changes a PR author could use to get past the gate, each of which must
// come out HARD (from the chaos sweep).
func TestGateBypasses(t *testing.T) {
	const onto = "concepts:\n  Secret: { kind: entity, criticality: critical }\n  Thing: { kind: entity }\n  Fine: { kind: entity, proposed: true }\n"
	const views = "views:\n  business:\n    include: [z:*]\n  policy:\n    include: [z:Secret]\n"
	const code = "# @gloss consumes z:Secret\n@requires_role(\"librarian\")\ndef read(card: str):\n    pass\n"
	for _, c := range []struct {
		name   string
		attack func(root string, write func(rel, content string), git func(...string))
		kind   string // a HARD change of this kind must be reported
	}{
		{"forged ledger row hides a policy edit", func(root string, write func(string, string), _ func(...string)) {
			f, _ := os.OpenFile(filepath.Join(root, ".glossmark", "ledger.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
			f.WriteString(`{"marker_id":"concept:z:Thing","path":"glossmark.yaml","relation":"concept","targets":["z:Thing"],"status":"stale"}` + "\n")
			f.Close()
			write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\npolicy:\n  criticality-gates: {}\n")
		}, "hard-path"},
		{"accepting a concept hides a kind change", func(_ string, write func(string, string), _ func(...string)) {
			write("ontology/z.gloss.yaml", strings.Replace(onto, "Fine: { kind: entity, proposed: true }", "Fine: { kind: measure }", 1))
		}, "concept-changed"},
		{"accepting a concept hides a relations edit", func(_ string, write func(string, string), _ func(...string)) {
			// Dropping proposed is AUTO and explains the file to the
			// hard-path rule (ADR 0034); the relations: block is no node
			// or edge, so the namespace fingerprint has to catch it.
			write("ontology/z.gloss.yaml", strings.Replace(onto, "Fine: { kind: entity, proposed: true }", "Fine: { kind: entity }", 1)+"relations:\n  produces: { lift: none }\n")
		}, "namespace-changed"},
		{"a signature edit hides a dropped decorator", func(_ string, write func(string, string), _ func(...string)) {
			write("svc.py", "# @gloss consumes z:Secret\ndef read(card: str, _=None):\n    pass\n")
		}, "decorators-changed"},
		{"a reused id= hides a new critical link", func(root string, write func(string, string), _ func(...string)) {
			id := markerID(t, root, "svc.py")
			write("other.py", "# @gloss consumes z:Secret id="+id+"\ndef other():\n    pass\n")
		}, "link-added"},
		{"a renamed views file drops a view", func(_ string, _ func(string, string), git func(...string)) {
			git("mv", "ontology/views.yaml", "docs-views.yaml")
		}, "view-removed"},
		{"a moved views path redefines a view", func(root string, write func(string, string), git func(...string)) {
			write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\nviews: arch/views.yaml\n")
			write("arch/views.yaml", "views:\n  business:\n    include: [z:Thing]\n")
		}, "view-changed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root, write, git := gitRepo(t)
			write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
			write("ontology/z.gloss.yaml", onto)
			write("ontology/views.yaml", views)
			write("svc.py", code)
			gloss(t, "build", "-q", "--root", root)
			git("add", "-A")
			git("commit", "-qm", "base")
			git("checkout", "-qb", "evil")
			c.attack(root, write, git)
			gloss(t, "build", "-q", "--root", root)
			git("add", "-A")
			git("commit", "-qm", "evil")
			out := filepath.Join(t.TempDir(), "impact.json")
			gloss(t, "diff", "--root", root, "--base", "master", "--format", "json", "-o", out)
			b, _ := os.ReadFile(out)
			var im struct {
				Gate    string `json:"gate"`
				Changes []struct{ Gate, Kind, Msg string }
			}
			if err := json.Unmarshal(b, &im); err != nil {
				t.Fatalf("%v\n%s", err, b)
			}
			for _, ch := range im.Changes {
				if ch.Kind == c.kind && ch.Gate == "HARD" {
					return
				}
			}
			t.Errorf("no HARD %s change; gate %s: %+v", c.kind, im.Gate, im.Changes)
		})
	}
}

func markerID(t *testing.T, root, path string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(root, ".glossmark", "ledger.jsonl"))
	for _, l := range strings.Split(string(b), "\n") {
		var e struct {
			ID   string `json:"marker_id"`
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(l), &e) == nil && e.Path == path && strings.HasPrefix(e.ID, "m-") {
			return e.ID
		}
	}
	t.Fatalf("no marker in %s", path)
	return ""
}
