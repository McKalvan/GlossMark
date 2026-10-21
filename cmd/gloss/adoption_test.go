// @gloss verifies gmi:ReviewCommands, gmi:DriftComparator, gmi:Gate scope=file

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr returns what f writes to stderr.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	done := make(chan []byte)
	go func() { b, _ := io.ReadAll(r); done <- b }()
	f()
	w.Close()
	os.Stderr = old
	return string(<-done)
}

// TestValueInLine: a published value matches its bound line by number (a
// percentage also as a fraction, separators and suffixes ignored, never the
// digits of a name) or, without numbers, by text (issue #160, E14).
func TestValueInLine(t *testing.T) {
	cases := []struct {
		value any
		line  string
		want  bool
	}{
		{"0.80", "const val REFERRAL_SCORE_THRESHOLD = 0.75", false},
		{"0.80", "const val REFERRAL_SCORE_THRESHOLD = 0.8", true},
		{0.75, "const val REFERRAL_SCORE_THRESHOLD = 0.75", true},
		{"4% (25x annual expenses)", "def plan(years: int, swr: float = 0.04):", true},
		{"4% (25x annual expenses)", "def plan(years: int, swr: float = 0.035):", false},
		{"60 days", "const val FILING_WINDOW_DAYS = 60L", true},
		{"10,000", "CEILING = 10_000", true},
		{"2", "MAX_2 = 10", false},
		{"strict", `mode = "STRICT"`, true},
		{"strict", `mode = "lenient"`, false},
		{nil, "anything", true},
		{[]any{"a", "b"}, "anything", true},
	}
	for _, c := range cases {
		if got := valueInLine(c.value, c.line); got != c.want {
			t.Errorf("valueInLine(%v, %q) = %v, want %v", c.value, c.line, got, c.want)
		}
	}
}

// TestAcceptValueRefusesAMismatch: accept --value does not confirm a line
// that no longer holds the published value unless --force says so, and
// prints both (issue #160, E14).
func TestAcceptValueRefusesAMismatch(t *testing.T) {
	root, write, _ := gitRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Threshold: { kind: constraint, value: \"0.80\", value-from: { path: fraud.kt, symbol: REFERRAL_SCORE_THRESHOLD } }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\n")
	write("fraud.kt", "const val REFERRAL_SCORE_THRESHOLD = 0.80\n")
	if code := cmdBuild([]string{"--root", root, "-q"}); code != 0 {
		t.Fatalf("build exited %d", code)
	}
	write("fraud.kt", "const val REFERRAL_SCORE_THRESHOLD = 0.75\n")
	var code int
	errOut := captureStderr(t, func() { code, _ = gloss(t, "accept", "--value", "z:Threshold", "--root", root) })
	if code == 0 {
		t.Fatal("accept --value confirmed a line that does not hold the value")
	}
	for _, want := range []string{"0.75", `"0.80"`, "--force"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("refusal %q does not mention %s", errOut, want)
		}
	}
	if code, _ := gloss(t, "drift", "--root", root); code == 0 {
		t.Error("the refused accept cleared the value drift")
	}
	errOut = captureStderr(t, func() { code, _ = gloss(t, "accept", "--value", "z:Threshold", "--force", "--root", root) })
	if code != 0 || !strings.Contains(errOut, "confirming anyway") {
		t.Errorf("accept --value --force: exit %d, stderr %q", code, errOut)
	}
	if code, out := gloss(t, "drift", "--root", root); code != 0 {
		t.Errorf("drift after a forced accept exited %d:\n%s", code, out)
	}
	// A line that still holds the value is confirmed without --force.
	write("fraud.kt", "const val REFERRAL_SCORE_THRESHOLD: Double = 0.80\n")
	if code, _ := gloss(t, "accept", "--value", "z:Threshold", "--root", root); code != 0 {
		t.Errorf("accept --value on a matching line exited %d", code)
	}
}

// TestDriftIDs: drift --ids ends each finding's line with the id a waiver
// names; without it the text has no ids (issue #160, L24). The trailer
// leaves file coverage out (E24).
func TestDriftIDs(t *testing.T) {
	root := workspace(t, "library/circulation")
	_, plain := gloss(t, "drift", "--root", root)
	_, ids := gloss(t, "drift", "--ids", "--root", root)
	if strings.Contains(plain, "(id ") {
		t.Errorf("drift prints ids without --ids:\n%s", plain)
	}
	if !strings.Contains(ids, "(id gap:") || !strings.Contains(ids, "(id orphan:") {
		t.Errorf("drift --ids lacks finding ids:\n%s", ids)
	}
	if strings.Contains(plain, "Files ") {
		t.Errorf("the drift trailer carries file coverage:\n%s", plain)
	}
}

// TestCoverageRatchetSaysWhy: the ratchet against a base without the
// workspace says it is skipped, and a missing base names --base (issue #160,
// L29).
func TestCoverageRatchetSaysWhy(t *testing.T) {
	root, write, git := gitRepo(t)
	write("README.md", "before GlossMark\n")
	git("add", ".")
	git("commit", "-q", "-m", "base")
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Doing: { kind: process }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\nrealizes: [z:Doing]\n")
	write("a.py", "# @gloss implements z:Doing\ndef a(): pass\n")
	var code int
	errOut := captureStderr(t, func() { code = cmdCoverage([]string{"--root", root, "--base", "HEAD", "--fail-under", "current"}) })
	if code != 0 || !strings.Contains(errOut, "base HEAD has no workspace at .") || !strings.Contains(errOut, "ratchet skipped") {
		t.Errorf("adopting commit: exit %d, stderr %q", code, errOut)
	}
	errOut = captureStderr(t, func() {
		code = cmdCoverage([]string{"--root", root, "--base", "origin/main", "--fail-under", "current"})
	})
	if code == 0 || !strings.Contains(errOut, "--base") || strings.Contains(errOut, "exit status") {
		t.Errorf("missing base: exit %d, stderr %q", code, errOut)
	}
}

// TestProposeFromNewCoreTarget: a core proposal is complete as proposed, so
// propose --from-new asks for realizes: only when it proposed an
// implementation concept (issue #160, L30).
func TestProposeFromNewCoreTarget(t *testing.T) {
	for _, c := range []struct {
		target string
		want   bool
	}{{"lib:NEW:FinePayment", false}, {"circ:NEW:DamageCheck", true}} {
		root := workspace(t, "library/circulation")
		f := filepath.Join(root, "services/circulation/loans/returns.py")
		b, _ := os.ReadFile(f)
		os.WriteFile(f, []byte(strings.Replace(string(b), "circ:NEW:DamageCheck", c.target, 1)), 0o644)
		code, out := gloss(t, "propose", "--from-new", "--root", root)
		if code != 0 {
			t.Fatalf("%s: propose --from-new exited %d", c.target, code)
		}
		if got := strings.Contains(out, "1 implementation concept still needs realizes:"); got != c.want {
			t.Errorf("%s: asks for realizes: %v, want %v:\n%s", c.target, got, c.want, out)
		}
	}
}

// TestImpactSkipsToolFiles: the tool's state, excluded and generated files
// are not listed as touched, so impact does not ask for a link that cannot
// be written (issue #160, K21).
func TestImpactSkipsToolFiles(t *testing.T) {
	root, write, git := gitRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\nexclude: [vendor/**]\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Doing: { kind: process }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\ngenerated: [gen/**]\n")
	write("a.py", "# @gloss implements z:Doing\ndef a(): pass\n")
	if code := cmdBuild([]string{"--root", root, "-q"}); code != 0 {
		t.Fatalf("build exited %d", code)
	}
	git("add", ".")
	git("commit", "-q", "-m", "base")
	write("a.py", "# @gloss implements z:Doing\ndef a(): return 1\n")
	write("vendor/lib.py", "def v(): pass\n")
	write("gen/out.py", "def g(): pass\n")
	if code := cmdBuild([]string{"--root", root, "-q"}); code != 0 {
		t.Fatalf("build exited %d", code)
	}
	_, out := gloss(t, "impact", "--root", root)
	if !strings.Contains(out, "a.py") {
		t.Errorf("impact lost the changed source file:\n%s", out)
	}
	for _, skip := range []string{".glossmark/", "vendor/lib.py", "gen/out.py"} {
		if strings.Contains(out, skip) {
			t.Errorf("impact lists %s:\n%s", skip, out)
		}
	}
	p, err := buildGraph(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if b := briefing(p, root, 0, 0, true); strings.Contains(b, ".glossmark/") {
		t.Errorf("context --changed lists the tool's files:\n%s", b)
	}
}

// TestRelOrAbs: an output path outside the workspace is printed absolute,
// never as a climb out of it (issue #160, C19).
func TestRelOrAbs(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "x", "viewer.html")
	if got := relOrAbs(root, out); got != out {
		t.Errorf("relOrAbs outside = %q, want %q", got, out)
	}
	if got := relOrAbs(root, filepath.Join(root, ".glossmark", "viewer.html")); got != filepath.Join(".glossmark", "viewer.html") {
		t.Errorf("relOrAbs inside = %q", got)
	}
}
