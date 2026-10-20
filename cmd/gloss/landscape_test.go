package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestLandscapePathsCarryRepo: boundary and ontology findings name their
// repository as marker findings do (#115).
func TestLandscapePathsCarryRepo(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{
		"core/glossmark.yaml":           "owns:\n  grid: { iri: https://x.test/grid/, tier: core }\n",
		"core/ontology/grid.gloss.yaml": "version: 1.0.0\nconcepts:\n  Meter: { kind: entity }\n  Tariff: { kind: entity }\n",
		"core/.gloss.yaml":              "boundary: grid:core\nlevel: system\n",
		"ana/glossmark.yaml":            "imports:\n  grid: { iri: https://x.test/grid/, version: \"^1.0\", source: ../core/ontology/grid.gloss.yaml }\n",
		"ana/.gloss.yaml":               "boundary: grid:analytics\nlevel: system\nholds: [grid:Tariff]\n",
	})
	out, _ := capture(t, "landscape", "--fail-on", "none", filepath.Join(tmp, "core"), filepath.Join(tmp, "ana"))
	for _, want := range []string{"[ana/.gloss.yaml:", "[core/ontology/grid.gloss.yaml:3]"} {
		if !strings.Contains(out, want) {
			t.Errorf("landscape output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[.gloss.yaml") || strings.Contains(out, "[ontology/") {
		t.Errorf("a location without its repository:\n%s", out)
	}
}

// TestLandscapeContextMap: the context map reads consumer → producer, as its
// header says, and draws no edge between a boundary and one that contains it,
// by directory nesting or part-of (issue #103).
func TestLandscapeContextMap(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{
		"hq/glossmark.yaml":           "owns:\n  hire: { iri: https://x.test/hire/, file: ontology/hire.gloss.yaml, tier: core }\nprefixes:\n  bw: https://x.test/b/\n",
		"hq/ontology/hire.gloss.yaml": "version: 1.0.0\nconcepts:\n  Hire: { kind: entity }\n",
		"hq/.gloss.yaml":              "boundary: bw:tool-hire\nlevel: system\n",
		"hq/mono/.gloss.yaml":         "boundary: bw:hirehq\nlevel: container\n",
		"hq/mono/hires.py":            "# @gloss produces hire:Hire\ndef create_hire(): pass\n",
		"hq/mono/inv/.gloss.yaml":     "boundary: bw:hirehq-invoicing\nlevel: component\n",
		"hq/mono/inv/invoice.py":      "# @gloss consumes hire:Hire\ndef invoice(): pass\n",
		"inv/glossmark.yaml":          "imports:\n  hire: { iri: https://x.test/hire/, source: ../hq/ontology/hire.gloss.yaml }\nprefixes:\n  bw: https://x.test/b/\n",
		"inv/.gloss.yaml":             "boundary: bw:invoicing-service\nlevel: container\npart-of: bw:tool-hire\n",
		"inv/run.go":                  "package inv\n\n// @gloss consumes hire:Hire\nfunc Run() {}\n",
	})
	out, _ := capture(t, "landscape", "--fail-on", "none", filepath.Join(tmp, "hq"), filepath.Join(tmp, "inv"))
	i := strings.Index(out, "Context map (consumer → producer")
	if i < 0 {
		t.Fatalf("no context map header:\n%s", out)
	}
	var edges []string
	for _, l := range strings.Split(out[i:], "\n")[1:] {
		if !strings.HasPrefix(l, "  ") {
			break
		}
		edges = append(edges, strings.TrimSpace(l))
	}
	if got, want := strings.Join(edges, "\n"), "bw:invoicing-service → bw:hirehq via hire:Hire"; got != want {
		t.Fatalf("context map:\n%s\nwant:\n%s", got, want)
	}
}
