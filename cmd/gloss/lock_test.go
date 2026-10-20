package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLockRefusesOutOfRangeAndPins: gloss lock will not record a version
// the range forbids, and a lock pins the vendored copy instead of following
// the upstream on disk (issue #107).
func TestLockRefusesOutOfRangeAndPins(t *testing.T) {
	tmp := t.TempDir()
	grid := func(v string) string { return "version: " + v + "\nconcepts:\n  Meter: { kind: entity }\n" }
	writeTree(t, tmp, map[string]string{
		"billing/glossmark.yaml":          "owns:\n  bil: { iri: https://x.test/bil/, file: ontology/bil.gloss.yaml, tier: core }\nimports:\n  grid: { iri: https://x.test/grid/, version: \"~1.0.0\", source: ../meter-core/grid.gloss.yaml }\n",
		"billing/ontology/bil.gloss.yaml": "version: 0.1.0\nconcepts:\n  Invoice: { kind: entity }\n",
		"meter-core/grid.gloss.yaml":      grid("1.0.1"),
	})
	root := filepath.Join(tmp, "billing")
	upstream := filepath.Join(tmp, "meter-core", "grid.gloss.yaml")
	lockfile := filepath.Join(root, "glossmark.lock")
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }

	if out, code := capture(t, "lock", "--root", root); code != 0 {
		t.Fatalf("lock at 1.0.1: exit %d\n%s", code, out)
	}
	locked := read(lockfile)
	if read(filepath.Join(root, ".glossmark", "imports", "grid@1.0.1.gloss.yaml")) != grid("1.0.1") {
		t.Fatal("gloss lock did not vendor grid@1.0.1")
	}

	// 1.1.0 is outside ~1.0.0: --update refuses and leaves the lock alone.
	os.WriteFile(upstream, []byte(grid("1.1.0")), 0o644)
	out, code := capture(t, "lock", "--root", root, "--update")
	if code != 1 || !strings.Contains(out, "requires ~1.0.0 but the resolved ontology is 1.1.0") || read(lockfile) != locked {
		t.Fatalf("lock --update at 1.1.0: exit %d, lock changed: %v\n%s", code, read(lockfile) != locked, out)
	}
	// The import stays on the pinned copy, and check says what is available.
	os.WriteFile(upstream, []byte(grid("2.0.0")), 0o644)
	out, _ = capture(t, "check", "--root", root)
	if !strings.Contains(out, "2.0.0 available, outside ~1.0.0") || strings.Contains(out, "GM106") || strings.Contains(out, "GM107") {
		t.Fatalf("check at upstream 2.0.0:\n%s", out)
	}
	// Plain gloss lock keeps the pin; --allow-out-of-range is the override.
	if out, code := capture(t, "lock", "--root", root); code != 0 || read(lockfile) != locked {
		t.Fatalf("plain lock should keep the pin: exit %d\n%s", code, out)
	}
	if out, code := capture(t, "lock", "--root", root, "--update", "--allow-out-of-range"); code != 0 || !strings.Contains(read(lockfile), "version: 2.0.0") {
		t.Fatalf("--allow-out-of-range: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, ".glossmark", "imports", "grid@1.0.1.gloss.yaml")); err == nil {
		t.Fatal("the superseded copy should be removed")
	}

	// An upstream with no version under a range is refused too.
	os.Remove(lockfile)
	os.WriteFile(upstream, []byte("concepts:\n  Meter: { kind: entity }\n"), 0o644)
	if out, code := capture(t, "lock", "--root", root); code != 1 || !strings.Contains(out, "declares no version") {
		t.Fatalf("unversioned upstream: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(lockfile); err == nil {
		t.Fatal("a refused lock must not be written")
	}
}

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
