// @gloss verifies gmi:BackstageExport scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestExportBackstage (issue #266): gloss export --backstage writes one
// document per boundary of the claims-events showcase plus landscape.json,
// every file validates against schemas/backstage-export.schema.json, and the
// assessment service's document carries its concepts with criticality, the
// code under it, its violation and its coverage.
func TestExportBackstage(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "claims-events")
	src := filepath.Join("..", "..", "internal", "suggest", "testdata", "calibration", "glossmark-examples", "claims-events")
	if err := os.CopyFS(root, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "out")
	msg, code := capture(t, "export", "--root", root, "--backstage", "-o", out, "--source-url", "https://host.test/r/blob/main/{path}#L{line}")
	if code != 0 || !strings.Contains(msg, "8 boundaries + landscape.json") {
		t.Fatalf("export: %d\n%s", code, msg)
	}
	schema := loadSchema(t)
	files, _ := filepath.Glob(filepath.Join(out, "*.json"))
	if len(files) != 9 {
		t.Fatalf("files: %v", files)
	}
	for _, f := range files {
		var v any
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if errs := schema.validate(v, schema.root, "$"); len(errs) > 0 {
			t.Errorf("%s does not match the schema:\n  %s", filepath.Base(f), strings.Join(errs, "\n  "))
		}
	}
	var doc struct {
		Format   int    `json:"format"`
		Kind     string `json:"kind"`
		Repo     string `json:"repo"`
		Boundary struct {
			CURIE, Slug, Path, Owner, Criticality string
			PartOf                                []string `json:"partOf"`
		} `json:"boundary"`
		Concepts []struct {
			CURIE       string   `json:"curie"`
			Relations   []string `json:"relations"`
			Declared    bool     `json:"declared"`
			Derived     bool     `json:"derived"`
			Criticality string   `json:"criticality"`
			Status      string   `json:"status"`
			Artifacts   []struct{ Path, URL, Via string }
			Verifiers   []struct{ Path string }
			Findings    []string `json:"findings"`
		} `json:"concepts"`
		Coverage struct {
			ConceptsInScope int `json:"conceptsInScope"`
			InScopeFiles    int `json:"inScopeFiles"`
			CoveredFiles    int `json:"coveredFiles"`
		} `json:"coverage"`
		Findings []struct{ ID, Kind, Concept, URL string }
	}
	b, err := os.ReadFile(filepath.Join(out, "claimb-assessment.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Format != 1 || doc.Kind != "boundary" || doc.Repo != "claims-events" || doc.Boundary.CURIE != "claimb:assessment" || doc.Boundary.Path != "services/assessment" || doc.Boundary.Owner != "claims-decisioning" || doc.Boundary.Criticality != "high" || strings.Join(doc.Boundary.PartOf, ",") != "claimb:claims-events" {
		t.Errorf("boundary: %+v (format %d, kind %s, repo %s)", doc.Boundary, doc.Format, doc.Kind, doc.Repo)
	}
	var limit, referred bool
	for _, c := range doc.Concepts {
		switch c.CURIE {
		case "claim:AutoApprovalLimit":
			limit = true
			if strings.Join(c.Relations, ",") != "realizes" || !c.Declared || !c.Derived || c.Criticality != "high" || c.Status != "implemented" {
				t.Errorf("AutoApprovalLimit: %+v", c)
			}
			if len(c.Artifacts) != 1 || c.Artifacts[0].Path != "services/assessment/assessment/settings.py" || c.Artifacts[0].Via != "tide:AutoApprovalRule" || c.Artifacts[0].URL != "https://host.test/r/blob/main/services/assessment/assessment/settings.py#L8" {
				t.Errorf("AutoApprovalLimit artifacts: %+v", c.Artifacts)
			}
			if len(c.Verifiers) != 1 || !strings.HasPrefix(c.Verifiers[0].Path, "services/assessment/tests/") {
				t.Errorf("AutoApprovalLimit verifiers: %+v", c.Verifiers)
			}
		case "claim:ClaimReferred":
			referred = true
			// Published by the code, not declared: the violation's concept.
			if c.Declared || !c.Derived || strings.Join(c.Relations, ",") != "publishes" || strings.Join(c.Findings, ",") != "violation:claimb:assessment:publishes:claim:ClaimReferred" {
				t.Errorf("ClaimReferred: %+v", c)
			}
		}
		for _, a := range c.Artifacts {
			if !strings.HasPrefix(a.Path, "services/assessment/") {
				t.Errorf("%s: artifact outside the boundary: %s", c.CURIE, a.Path)
			}
		}
	}
	if !limit || !referred {
		t.Fatalf("concepts lack AutoApprovalLimit or ClaimReferred: %d concepts", len(doc.Concepts))
	}
	var kinds []string
	for _, f := range doc.Findings {
		kinds = append(kinds, f.Kind)
		if f.Kind == "violation" && f.URL != "https://host.test/r/blob/main/services/assessment/assessment/referral.py#L33" {
			t.Errorf("violation url: %q", f.URL)
		}
	}
	if !strings.Contains(strings.Join(kinds, ","), "violation") {
		t.Errorf("findings: %v", kinds)
	}
	if doc.Coverage.ConceptsInScope == 0 || doc.Coverage.InScopeFiles == 0 || doc.Coverage.CoveredFiles == 0 || doc.Coverage.CoveredFiles > doc.Coverage.InScopeFiles {
		t.Errorf("coverage: %+v", doc.Coverage)
	}
	var l struct {
		Kind       string `json:"kind"`
		Repos      []struct{ Name string }
		Boundaries []struct {
			CURIE    string         `json:"curie"`
			Document string         `json:"document"`
			Repo     string         `json:"repo"`
			Findings map[string]int `json:"findings"`
			Severity string         `json:"severity"`
		} `json:"boundaries"`
	}
	b, _ = os.ReadFile(filepath.Join(out, "landscape.json"))
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatal(err)
	}
	if l.Kind != "landscape" || len(l.Repos) != 1 || l.Repos[0].Name != "claims-events" || len(l.Boundaries) != 8 {
		t.Errorf("landscape: %+v", l)
	}
	for _, s := range l.Boundaries {
		if _, err := os.Stat(filepath.Join(out, s.Document)); err != nil {
			t.Errorf("%s: document %s missing", s.CURIE, s.Document)
		}
		if s.CURIE == "claimb:assessment" && (s.Findings["violation"] != 1 || s.Severity != "error") {
			t.Errorf("assessment summary: %+v", s)
		}
	}
}

// TestLandscapeBackstage: gloss landscape --backstage writes every
// repository's boundaries with repo-prefixed paths, each linked through its
// own repository's source-url, and one landscape.json naming both.
func TestLandscapeBackstage(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{
		"hq/glossmark.yaml":           "owns:\n  hire: { iri: https://x.test/hire/, tier: core }\nprefixes:\n  bw: https://x.test/b/\nsource-url: https://host.test/hq/blob/main/{path}#L{line}\n",
		"hq/ontology/hire.gloss.yaml": "version: 1.0.0\nconcepts:\n  Invoicing: { kind: process, criticality: high }\n  Invoice: { kind: entity }\n",
		"hq/.gloss.yaml":              "boundary: bw:hirehq\nlevel: container\nrealizes: [hire:Invoicing]\n",
		"hq/gen.py":                   "# @gloss implements hire:Invoicing\ndef gen(): pass\n",
		"inv/glossmark.yaml":          "imports:\n  hire: { iri: https://x.test/hire/, version: \"^1.0\", source: ../hq/ontology/hire.gloss.yaml }\nprefixes:\n  bw: https://x.test/b/\n",
		"inv/.gloss.yaml":             "boundary: bw:invoicing-service\nlevel: container\nholds: [hire:Invoice]\n",
		"inv/run.go":                  "package inv\n\n// @gloss represents hire:Invoice\ntype Invoice struct{}\n",
	})
	out := filepath.Join(tmp, "portal")
	msg, code := capture(t, "landscape", "--fail-on", "none", "--backstage", out, filepath.Join(tmp, "hq"), filepath.Join(tmp, "inv"))
	if code != 0 || !strings.Contains(msg, "2 boundaries + landscape.json") {
		t.Fatalf("landscape: %d\n%s", code, msg)
	}
	schema := loadSchema(t)
	read := func(name string) map[string]any {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		if errs := schema.validate(v, schema.root, "$"); len(errs) > 0 {
			t.Errorf("%s: %s", name, strings.Join(errs, "; "))
		}
		return v
	}
	hq := read("bw-hirehq.json")
	if hq["repo"] != "hq" || hq["boundary"].(map[string]any)["path"] != "hq" {
		t.Errorf("hq document: repo %v path %v", hq["repo"], hq["boundary"].(map[string]any)["path"])
	}
	cs := hq["concepts"].([]any)
	if len(cs) != 1 {
		t.Fatalf("hq concepts: %v", cs)
	}
	c := cs[0].(map[string]any)
	art := c["artifacts"].([]any)[0].(map[string]any)
	if c["criticality"] != "high" || art["path"] != "hq/gen.py" || art["url"] != "https://host.test/hq/blob/main/gen.py#L1" {
		t.Errorf("hq concept: %v", c)
	}
	inv := read("bw-invoicing-service.json")
	ia := inv["concepts"].([]any)[0].(map[string]any)["artifacts"].([]any)[0].(map[string]any)
	if inv["repo"] != "inv" || ia["path"] != "inv/run.go" || ia["url"] != nil {
		t.Errorf("inv document: repo %v artifact %v", inv["repo"], ia)
	}
	l := read("landscape.json")
	if n := len(l["repos"].([]any)); n != 2 {
		t.Errorf("landscape repos: %v", l["repos"])
	}
	if n := len(l["boundaries"].([]any)); n != 2 {
		t.Errorf("landscape boundaries: %v", l["boundaries"])
	}
}

// jsonSchema is enough of JSON Schema 2020-12 to check the export against
// schemas/backstage-export.schema.json: type, const, enum, pattern,
// properties, required, additionalProperties, items, oneOf and local $ref.
type jsonSchema struct {
	root map[string]any
	defs map[string]any
}

func loadSchema(t *testing.T) *jsonSchema {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "schemas", "backstage-export.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	defs, _ := root["$defs"].(map[string]any)
	return &jsonSchema{root: root, defs: defs}
}

func (s *jsonSchema) validate(v any, schema map[string]any, at string) []string {
	if ref, ok := schema["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		d, ok := s.defs[name].(map[string]any)
		if !ok {
			return []string{at + ": unknown $ref " + ref}
		}
		return s.validate(v, d, at)
	}
	var errs []string
	if alts, ok := schema["oneOf"].([]any); ok {
		matched := 0
		var last []string
		for _, a := range alts {
			if e := s.validate(v, a.(map[string]any), at); len(e) == 0 {
				matched++
			} else {
				last = e
			}
		}
		if matched != 1 {
			errs = append(errs, fmt.Sprintf("%s: matches %d of oneOf; last: %s", at, matched, strings.Join(last, "; ")))
		}
	}
	if c, ok := schema["const"]; ok && fmt.Sprint(c) != fmt.Sprint(v) {
		errs = append(errs, fmt.Sprintf("%s: %v is not %v", at, v, c))
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if e == v {
				found = true
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("%s: %v not in %v", at, v, enum))
		}
	}
	if typ, ok := schema["type"].(string); ok {
		okType := false
		switch typ {
		case "object":
			_, okType = v.(map[string]any)
		case "array":
			_, okType = v.([]any)
		case "string":
			_, okType = v.(string)
		case "boolean":
			_, okType = v.(bool)
		case "number":
			_, okType = v.(float64)
		case "integer":
			f, isNum := v.(float64)
			okType = isNum && f == float64(int64(f))
		}
		if !okType {
			return append(errs, fmt.Sprintf("%s: %v is not a %s", at, v, typ))
		}
	}
	if pat, ok := schema["pattern"].(string); ok {
		if str, isStr := v.(string); isStr && !regexp.MustCompile(pat).MatchString(str) {
			errs = append(errs, fmt.Sprintf("%s: %q does not match %s", at, str, pat))
		}
	}
	if obj, ok := v.(map[string]any); ok {
		props, _ := schema["properties"].(map[string]any)
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				if _, ok := obj[r.(string)]; !ok {
					errs = append(errs, fmt.Sprintf("%s: missing %s", at, r))
				}
			}
		}
		for k, val := range obj {
			if p, ok := props[k].(map[string]any); ok {
				errs = append(errs, s.validate(val, p, at+"."+k)...)
				continue
			}
			switch ap := schema["additionalProperties"].(type) {
			case bool:
				if !ap {
					errs = append(errs, fmt.Sprintf("%s: unexpected key %s", at, k))
				}
			case map[string]any:
				errs = append(errs, s.validate(val, ap, at+"."+k)...)
			}
		}
	}
	if arr, ok := v.([]any); ok {
		if items, ok := schema["items"].(map[string]any); ok {
			for i, it := range arr {
				errs = append(errs, s.validate(it, items, fmt.Sprintf("%s[%d]", at, i))...)
			}
		}
	}
	return errs
}
