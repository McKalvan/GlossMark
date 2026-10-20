package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestLockOutputOrder: gloss lock lists imports sorted by prefix, as the
// lockfile is, whatever the map order (issue #114).
func TestLockOutputOrder(t *testing.T) {
	tmp := t.TempDir()
	cfg := "owns:\n  app: { iri: https://x.test/app/, tier: core, file: ontology/app.gloss.yaml }\nimports:\n"
	files := map[string]string{"app/ontology/app.gloss.yaml": "version: 1.0.0\nconcepts:\n  A: { kind: entity }\n"}
	for _, p := range []string{"zz", "mm", "aa", "kk", "bb"} {
		cfg += "  " + p + ": { iri: https://x.test/" + p + "/, source: ../up/" + p + ".gloss.yaml }\n"
		files["up/"+p+".gloss.yaml"] = "version: 1.0.0\nconcepts:\n  X: { kind: entity }\n"
	}
	files["app/glossmark.yaml"] = cfg
	writeTree(t, tmp, files)
	root := filepath.Join(tmp, "app")
	var first string
	for i := 0; i < 20; i++ {
		out, code := capture(t, "lock", "--root", root)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, out)
		}
		var prefixes []string
		for _, l := range strings.Split(out, "\n") {
			if f := strings.Fields(l); len(f) > 2 && f[2] != "written" {
				prefixes = append(prefixes, strings.TrimSuffix(f[2], ":"))
			}
		}
		if got := strings.Join(prefixes, " "); got != "aa bb kk mm zz" {
			t.Fatalf("run %d: prefixes in order %q", i, got)
		}
		if i == 0 {
			first = out
		} else if out != first {
			t.Fatalf("output changed between runs:\n%s\n%s", first, out)
		}
	}
}
