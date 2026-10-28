// @gloss verifies gmi:DriftComparator scope=file

package main

import (
	"strings"
	"testing"
)

// TestCoverageCountsDirectCoreImplementations: a core concept that code
// implements directly is in coverage scope, and an empty scope is n/a and
// never fails --fail-under (#53, ADR 0013).
func TestCoverageCountsDirectCoreImplementations(t *testing.T) {
	base := map[string]string{
		"glossmark.yaml":         "owns:\n  hp: { iri: https://x/hp/, tier: core }\n",
		"ontology/hp.gloss.yaml": "namespace: hp\nconcepts:\n  Pricing: { kind: process }\n",
		".gloss.yaml":            "boundary: hpb:storefront\nlevel: container\n",
		"tests.py":               "# @gloss verifies hp:Pricing\ndef test_quote(): pass\n",
	}
	for _, tc := range []struct {
		name, view string
		code       int
		want       string
	}{
		{"implements a core concept", "# @gloss implements hp:Pricing\ndef quote_view(): pass\n", 0, "concepts 100% (1 of 1 "},
		{"presents only", "# @gloss presents hp:Pricing\ndef quote_view(): pass\n", 0, "concepts n/a (0 of 0 "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"views.py": tc.view}
			for k, v := range base {
				files[k] = v
			}
			root := tiny(t, files)
			code, out := gloss(t, "coverage", "--fail-under", "50", "--root", root)
			if code != tc.code || !strings.HasPrefix(out, tc.want) {
				t.Errorf("gloss coverage exited %d with %q, want %d and %q", code, firstLine(out), tc.code, tc.want)
			}
		})
	}
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}
