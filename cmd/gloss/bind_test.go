// @gloss verifies gmi:BindCommand scope=file

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bindWorkspace is a two-tier workspace with one core rule to publish and
// an implementation namespace to bind into; each test adds the code file.
func bindWorkspace(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	base := map[string]string{
		"glossmark.yaml":            "owns:\n  claim: { iri: https://x/claim/, tier: core, owner: claims-product }\n  tide: { iri: https://x/tide/, tier: implementation, owner: claims-eng }\nprefixes: { b: https://x/b/ }\n",
		"ontology/claim.gloss.yaml": "namespace: claim\nconcepts:\n  AutoApprovalLimit: { kind: constraint, criticality: high, doc: \"The largest loss approved without an adjuster.\" }\n  Threshold: { kind: constraint }\n",
		"ontology/tide.gloss.yaml":  "namespace: tide\nconcepts:\n  Existing: { kind: constraint, realizes: claim:Threshold }   # keep my style\n",
		".gloss.yaml":               "boundary: b:root\nlevel: system\nrealizes: inherit\ndefault-namespace: tide\n",
		"svc/.gloss.yaml":           "boundary: b:svc\nlevel: container\nrealizes: [claim:AutoApprovalLimit, claim:Threshold]\ndefault-namespace: tide\nowner: claims-decisioning\n",
	}
	for k, v := range files {
		base[k] = v
	}
	writeTree(t, root, base)
	return root
}

// TestBindLanguages: gloss bind reads the constant, writes the ontology
// entry and the marker in the file's comment style, and the result passes
// gloss check with the value bound (#272).
func TestBindLanguages(t *testing.T) {
	cases := []struct {
		name, path, symbol, code, value, marker string
		line                                    int // the marker's line after the write
	}{
		{"python", "svc/settings.py", "AUTO_APPROVE_LIMIT_EUR",
			"\"\"\"Settings.\"\"\"\n\n# Approved without an adjuster at or below this.\nAUTO_APPROVE_LIMIT_EUR = 5_000\n",
			"5000", "# @gloss publishes-value .AutoApprovalRule scope=line", 4},
		{"go", "svc/policy.go", "AutoApproveLimitEUR",
			"package svc\n\n// AutoApproveLimitEUR is the auto-approval limit.\nconst AutoApproveLimitEUR = 5000 // EUR\n",
			"5000", "// @gloss publishes-value .AutoApprovalRule scope=line", 4},
		{"kotlin", "svc/src/main/kotlin/Thresholds.kt", "AUTO_APPROVE_LIMIT_EUR",
			"package tidewater\n\nobject Thresholds {\n    const val AUTO_APPROVE_LIMIT_EUR = 5_000L\n}\n",
			"5000", "    // @gloss publishes-value .AutoApprovalRule scope=line", 4},
		{"java", "svc/src/main/java/Thresholds.java", "AUTO_APPROVE_LIMIT_EUR",
			"package tidewater;\n\nimport java.math.BigDecimal;\n\npublic final class Thresholds {\n    public static final BigDecimal AUTO_APPROVE_LIMIT_EUR = new BigDecimal(\"5000\");\n}\n",
			"5000", "    // @gloss publishes-value .AutoApprovalRule scope=line", 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := bindWorkspace(t, map[string]string{c.path: c.code})
			code, out := gloss(t, "bind", c.path+":"+c.symbol, "--as", "tide:AutoApprovalRule", "--publish-as", "claim:AutoApprovalLimit", "--root", root)
			if code != 0 {
				t.Fatalf("bind: exit %d\n%s", code, out)
			}
			if !strings.Contains(out, "tide:AutoApprovalRule = \""+c.value+"\"") {
				t.Errorf("bind should report the value read from the code:\n%s", out)
			}
			ont, _ := os.ReadFile(filepath.Join(root, "ontology", "tide.gloss.yaml"))
			for _, want := range []string{
				"  Existing: { kind: constraint, realizes: claim:Threshold }   # keep my style\n",
				"  AutoApprovalRule:\n    kind: constraint\n    realizes: claim:AutoApprovalLimit\n    published: { as: claim:AutoApprovalLimit }\n    value: \"" + c.value + "\"\n    value-from: { path: " + c.path + ", symbol: " + c.symbol + " }\n    doc: \"" + c.symbol + " in " + c.path + ".\"\n",
			} {
				if !strings.Contains(string(ont), want) {
					t.Errorf("ontology lacks %q:\n%s", want, ont)
				}
			}
			src, _ := os.ReadFile(filepath.Join(root, c.path))
			lines := strings.Split(string(src), "\n")
			if c.line > len(lines) || lines[c.line-1] != c.marker {
				t.Errorf("marker at line %d = %q, want %q\n%s", c.line, lines[c.line-1], c.marker, src)
			}
			// The result passes check, is bound in the graph and is drift-free.
			if out, code := capture(t, "check", "--strict", "--root", root); code != 0 {
				t.Errorf("check after bind: exit %d\n%s", code, out)
			}
			if out, code := capture(t, "drift", "--fail-on", "value-drift", "--root", root); code != 0 {
				t.Errorf("drift after bind: exit %d\n%s", code, out)
			}
			// Changing the constant alone is drift.
			src, _ = os.ReadFile(filepath.Join(root, c.path))
			writeTree(t, root, map[string]string{c.path: strings.NewReplacer("5_000", "7_500", "5000", "7500").Replace(string(src))})
			out, code = capture(t, "drift", "--fail-on", "value-drift", "--root", root)
			if code == 0 || !strings.Contains(out, "tide:AutoApprovalRule publishes \""+c.value+"\"") {
				t.Errorf("drift after the change: exit %d\n%s", code, out)
			}
		})
	}
}

// TestBindExistingAndConfig: binding an existing concept adds the fields to
// its entry and keeps its style; a key path in a YAML file binds without a
// marker when asked; a dry run writes nothing; the errors name the fix.
func TestBindExistingAndConfig(t *testing.T) {
	root := bindWorkspace(t, map[string]string{
		"svc/limits.py":      "LIMIT = 3\n",
		"svc/ratelimit.yaml": "policies:\n  claims-write:\n    per_minute: 30\n",
		"svc/cfg.json":       "{\n  \"ceiling\": 12\n}\n",
	})
	// An existing flow entry: fields are added, the comment stays.
	if code, out := gloss(t, "bind", "svc/limits.py:LIMIT", "--as", "tide:Existing", "--root", root); code != 0 {
		t.Fatalf("bind existing: exit %d\n%s", code, out)
	}
	ont, _ := os.ReadFile(filepath.Join(root, "ontology", "tide.gloss.yaml"))
	if want := "  Existing: { kind: constraint, realizes: claim:Threshold, value: \"3\", value-from: { path: svc/limits.py, symbol: LIMIT } }   # keep my style\n"; !strings.Contains(string(ont), want) {
		t.Errorf("existing entry:\n%s\nwant %q", ont, want)
	}
	src, _ := os.ReadFile(filepath.Join(root, "svc", "limits.py"))
	if string(src) != "# @gloss publishes-value .Existing scope=line\nLIMIT = 3\n" {
		t.Errorf("limits.py = %q", src)
	}
	// Binding it again (the value changed) rewrites value: in place.
	writeTree(t, root, map[string]string{"svc/limits.py": "# @gloss publishes-value .Existing scope=line\nLIMIT = 4\n"})
	if code, out := gloss(t, "bind", "svc/limits.py:LIMIT", "--as", "tide:Existing", "--root", root); code != 0 || !strings.Contains(out, "already bound to svc/limits.py:2") {
		t.Fatalf("re-bind: exit %d\n%s", code, out)
	}
	ont, _ = os.ReadFile(filepath.Join(root, "ontology", "tide.gloss.yaml"))
	if !strings.Contains(string(ont), "value: \"4\", value-from:") || strings.Count(string(ont), "value-from") != 1 {
		t.Errorf("re-bind should replace value: in place:\n%s", ont)
	}
	src, _ = os.ReadFile(filepath.Join(root, "svc", "limits.py"))
	if strings.Count(string(src), "@gloss") != 1 {
		t.Errorf("re-bind duplicated the marker:\n%s", src)
	}
	// A dotted key path in YAML, engineering-private, no marker.
	code, out := gloss(t, "bind", "svc/ratelimit.yaml:policies.claims-write.per_minute", "--as", "tide:WriteLimit", "--internal", "--no-marker", "--root", root)
	if code != 0 || !strings.Contains(out, "tide:WriteLimit = \"30\"") {
		t.Fatalf("bind key path: exit %d\n%s", code, out)
	}
	yml, _ := os.ReadFile(filepath.Join(root, "svc", "ratelimit.yaml"))
	if strings.Contains(string(yml), "@gloss") {
		t.Errorf("--no-marker wrote a marker:\n%s", yml)
	}
	// JSON has no comments: the marker is skipped and said so.
	if code, out := gloss(t, "bind", "svc/cfg.json:ceiling", "--as", "tide:Ceiling", "--internal", "--root", root); code != 0 || !strings.Contains(out, "no marker") || !strings.Contains(out, "= \"12\"") {
		t.Fatalf("bind json: exit %d\n%s", code, out)
	}
	if out, code := capture(t, "check", "--strict", "--root", root); code != 0 {
		t.Errorf("check: exit %d\n%s", code, out)
	}
	// Dry run: nothing written.
	before, _ := os.ReadFile(filepath.Join(root, "ontology", "tide.gloss.yaml"))
	if code, out := gloss(t, "bind", "-n", "svc/limits.py:LIMIT", "--as", "tide:Dry", "--publish-as", "claim:Threshold", "--root", root); code != 0 || !strings.Contains(out, "would bind") {
		t.Fatalf("dry run: exit %d\n%s", code, out)
	}
	if after, _ := os.ReadFile(filepath.Join(root, "ontology", "tide.gloss.yaml")); string(after) != string(before) {
		t.Errorf("dry run wrote the ontology")
	}
	// Errors say what to pass.
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"svc/limits.py:NOPE", "--as", "tide:X", "--internal"}, "is not assigned anywhere"},
		{[]string{"svc/limits.py:LIMIT", "--as", "tide:New"}, "--publish-as <core:Concept>"},
		{[]string{"svc/limits.py:LIMIT", "--as", "claim:Rule", "--publish-as", "claim:Threshold"}, "core concept and realizes nothing"},
		{[]string{"svc/limits.py:LIMIT", "--as", "tide:New", "--publish-as", "claim:Missing"}, "no such concept"},
		{[]string{"svc/limits.py:LIMIT", "--as", "tide:New", "--publish-as", "tide:Existing"}, "must name a core concept"},
		{[]string{"svc/limits.py", "--as", "tide:New"}, "<path>:<symbol>"},
	} {
		out, code := capture(t, append(append([]string{"bind"}, c.args...), "--root", root)...)
		if code == 0 || !strings.Contains(out, c.want) {
			t.Errorf("bind %v: exit %d, want %q in\n%s", c.args, code, c.want, out)
		}
	}
}

// TestBindExample: the shipped value-binding example binds as docs/DEMO.md
// shows, and the gate goes HARD with the boundary's owner named.
func TestBindExample(t *testing.T) {
	root := workspace(t, "value-binding")
	code, out := gloss(t, "bind", "services/assessment/assessment/settings.py:AUTO_APPROVE_LIMIT_EUR", "--as", "tide:AutoApprovalRule", "--publish-as", "claim:AutoApprovalLimit", "--value", "5000 EUR", "--root", root)
	if code != 0 || !strings.Contains(out, "tide:AutoApprovalRule = \"5000 EUR\"") {
		t.Fatalf("bind: exit %d\n%s", code, out)
	}
	if out, code := capture(t, "check", "--strict", "--root", root); code != 0 {
		t.Errorf("check: exit %d\n%s", code, out)
	}
	if out, code := capture(t, "drift", "--root", root); code != 0 {
		t.Errorf("drift after bind: exit %d\n%s", code, out)
	}
	writeTree(t, root, map[string]string{".git/HEAD": "ref: refs/heads/main\n"})
	p := filepath.Join(root, "services", "assessment", "assessment", "settings.py")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(b), "5_000", "7_500", 1)), 0o644)
	out, code = capture(t, "drift", "--root", root)
	if code == 0 || !strings.Contains(out, "Published values drifted") {
		t.Errorf("drift: exit %d\n%s", code, out)
	}
}
