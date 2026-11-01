// @gloss verifies gmi:SuggestCalibration scope=file

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/calib"
)

// proposalsRepo is a tiny workspace with three agent proposals at conf
// 0.4, 0.75 and 0.97, and the given suggest: and policy: blocks.
func proposalsRepo(t *testing.T, extraConfig string, files map[string]string) string {
	t.Helper()
	all := map[string]string{
		"glossmark.yaml":        "owns:\n  z: { iri: https://x/z/, file: ontology/z.gloss.yaml, tier: core }\n  i: { iri: https://x/i/, file: ontology/i.gloss.yaml, tier: implementation }\n" + extraConfig,
		"ontology/i.gloss.yaml": "namespace: i\nconcepts:\n  Doer: { kind: process, realizes: z:Doing }\n  Maker: { kind: process, realizes: z:Doing }\n  Taker: { kind: process, realizes: z:Doing }\n",
		"low.py":                "# @gloss? implements .Doer conf=0.4\ndef low(): pass\n",
		"mid.py":                "# @gloss? implements .Maker conf=0.75\ndef mid(): pass\n",
		"high.py":               "# @gloss? implements .Taker conf=0.97\ndef high(): pass\n",
	}
	for k, v := range files {
		all[k] = v
	}
	return tiny(t, all)
}

func acceptedByPolicy(t *testing.T, root string) []string {
	t.Helper()
	if code, out := gloss(t, "accept", "--policy", "--root", root); code != 0 {
		t.Fatalf("accept --policy: %d %s", code, out)
	}
	var got []string
	for _, f := range []string{"low.py", "mid.py", "high.py"} {
		b, _ := os.ReadFile(filepath.Join(root, f))
		if !strings.Contains(string(b), "@gloss?") {
			got = append(got, f)
		}
	}
	return got
}

// TestAcceptPolicyHonoursPrecisionTarget (ADR 0024): accept --policy
// accepts what the derived threshold makes SOFT. The default target (0.9
// on the shipped calibration) derives a threshold above 0.75; a lower
// target (0.8) derives one below 0.75 and above 0.4; policy.soft-conf-threshold
// alone still fixes it; and when no basis meets the target nothing is
// accepted.
func TestAcceptPolicyHonoursPrecisionTarget(t *testing.T) {
	def := calib.Derive(calib.Shipped.Confs(), calib.Shipped, calib.DefaultPrecision, calib.DefaultMinConf)
	if !def.Met || def.Threshold <= 0.75 || def.Threshold > 0.97 {
		t.Fatalf("the shipped default threshold %.2f no longer separates the fixture's 0.75 and 0.97", def.Threshold)
	}
	if low := calib.Derive(calib.Shipped.Confs(), calib.Shipped, 0.8, calib.DefaultMinConf); !low.Met || low.Threshold <= 0.4 || low.Threshold > 0.75 {
		t.Fatalf("the shipped threshold for the target 0.8, %.2f, no longer separates the fixture's 0.4 and 0.75", low.Threshold)
	}
	cases := []struct {
		name, config string
		files        map[string]string
		want         string
	}{
		{"default target", "", nil, "high.py"},
		{"lower target", "suggest:\n  auto-accept-precision: 0.8\n", nil, "mid.py high.py"},
		{"legacy fixed threshold", "policy:\n  soft-conf-threshold: 0.7\n", nil, "mid.py high.py"},
		{"target beats fixed threshold", "suggest:\n  auto-accept-precision: 0.9\npolicy:\n  soft-conf-threshold: 0.7\n", nil, "high.py"},
		{"no basis meets the target", "suggest:\n  calibration: cal.tsv\n", map[string]string{"cal.tsv": "name-exact/type\t20\t2\ndoc/func\t10\t1\n"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := proposalsRepo(t, c.config, c.files)
			if got := strings.Join(acceptedByPolicy(t, root), " "); got != c.want {
				t.Errorf("accepted %q, want %q", got, c.want)
			}
		})
	}
}

// TestSuggestReportsAutoAccept (ADR 0024): suggest's JSON carries the rule
// and marks each proposal; text tags each line; flags override the
// config, which overrides the default.
func TestSuggestReportsAutoAccept(t *testing.T) {
	type report struct {
		AutoAccept calib.AutoAccept `json:"auto_accept"`
		Proposals  []struct {
			Symbol     string  `json:"symbol"`
			Conf       float64 `json:"conf"`
			AutoAccept bool    `json:"auto_accept"`
		} `json:"proposals"`
	}
	files := map[string]string{"doer.py": "def doer(): pass\n"}
	run := func(config string, args ...string) report {
		t.Helper()
		root := tiny(t, map[string]string{"doer.py": files["doer.py"], "glossmark.yaml": "owns:\n  z: { iri: https://x/z/, file: ontology/z.gloss.yaml, tier: core }\n  i: { iri: https://x/i/, file: ontology/i.gloss.yaml, tier: implementation }\n" + config})
		code, js := gloss(t, append([]string{"suggest", "--format", "json", "--root", root}, args...)...)
		var r report
		if err := json.Unmarshal([]byte(js), &r); code != 0 || err != nil {
			t.Fatalf("suggest: %d %v %s", code, err, js)
		}
		return r
	}
	r := run("")
	if r.AutoAccept.Target != calib.DefaultPrecision || r.AutoAccept.MinConf != calib.DefaultMinConf || r.AutoAccept.Source != "shipped" || !r.AutoAccept.Met {
		t.Errorf("default rule: %+v", r.AutoAccept)
	}
	if len(r.Proposals) != 1 || r.Proposals[0].AutoAccept != (r.Proposals[0].Conf >= r.AutoAccept.Threshold) {
		t.Errorf("proposals: %+v (threshold %.2f)", r.Proposals, r.AutoAccept.Threshold)
	}
	r = run("suggest:\n  auto-accept-precision: 0.5\n  min-conf: 0.2\n")
	if r.AutoAccept.Target != 0.5 || r.AutoAccept.MinConf != 0.2 {
		t.Errorf("config rule: %+v", r.AutoAccept)
	}
	if len(r.Proposals) != 1 || !r.Proposals[0].AutoAccept {
		t.Errorf("at target 0.5 the name-exact proposal is auto-accepted: %+v", r.Proposals)
	}
	r = run("suggest:\n  auto-accept-precision: 0.5\n  min-conf: 0.2\n", "--auto-accept-precision", "0.95", "--min-conf", "0.4")
	if r.AutoAccept.Target != 0.95 || r.AutoAccept.MinConf != 0.4 {
		t.Errorf("flags override config: %+v", r.AutoAccept)
	}
	r = run("suggest:\n  min-conf: 0.99\n")
	if len(r.Proposals) != 0 {
		t.Errorf("suggest.min-conf 0.99 is the floor: %+v", r.Proposals)
	}
	root := tiny(t, files)
	_, out := gloss(t, "suggest", "--format", "text", "--root", root)
	if !strings.Contains(out, "conf=") || !(strings.Contains(out, " auto  [") || strings.Contains(out, " review  [")) {
		t.Errorf("text lines carry auto or review: %q", out)
	}
}

// TestSuggestConfigValidated: out-of-range suggest: values are refused
// when glossmark.yaml is read.
func TestSuggestConfigValidated(t *testing.T) {
	for _, cfg := range []string{"suggest:\n  auto-accept-precision: 1.5\n", "suggest:\n  min-conf: -0.1\n", "suggest:\n  calibration: cal.tsv\n"} {
		root := tiny(t, map[string]string{
			"glossmark.yaml": "owns:\n  z: { iri: https://x/z/, file: ontology/z.gloss.yaml, tier: core }\n" + cfg,
			"cal.tsv":        "name-exact/type\t3\t5\n",
		})
		if code, _ := gloss(t, "suggest", "--root", root); code == 0 {
			t.Errorf("%q: accepted", cfg)
		}
	}
}

// TestSuggestCalibrateWrite (ADR 0024): --calibrate measures the workspace
// against its own links on a stripped copy; --write saves the counts to
// suggest.calibration, and the threshold is then derived from them.
func TestSuggestCalibrateWrite(t *testing.T) {
	root := tiny(t, map[string]string{
		"glossmark.yaml": "owns:\n  z: { iri: https://x/z/, file: ontology/z.gloss.yaml, tier: core }\n  i: { iri: https://x/i/, file: ontology/i.gloss.yaml, tier: implementation }\nsuggest:\n  auto-accept-precision: 0.9\n  calibration: glossmark.calibration.tsv\n",
		"doer.py":        "# @gloss implements .Doer\ndef doer(): pass\n",
	})
	code, out := gloss(t, "suggest", "--calibrate", "--root", root)
	if code != 0 || !strings.Contains(out, "calibration on this workspace's own links: 1 proposals, 1 labels") || !strings.Contains(out, "recall: 1 of 1 labels") {
		t.Fatalf("--calibrate: %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, "glossmark.calibration.tsv")); err == nil {
		t.Fatal("--calibrate without --write wrote the file")
	}
	if code, _ := gloss(t, "suggest", "--calibrate", "--write", "--root", root); code != 0 {
		t.Fatalf("--calibrate --write: %d", code)
	}
	b, err := os.ReadFile(filepath.Join(root, "glossmark.calibration.tsv"))
	if err != nil || !strings.Contains(string(b), "name-exact/func\t1\t1\t0\t0\n") {
		t.Fatalf("calibration file: %v %q", err, b)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "doer.py")); !strings.Contains(string(b), "@gloss implements .Doer") {
		t.Errorf("--calibrate touched the workspace: %q", b)
	}
	code, js := gloss(t, "suggest", "--format", "json", "--root", root)
	var r struct {
		AutoAccept calib.AutoAccept `json:"auto_accept"`
	}
	if err := json.Unmarshal([]byte(js), &r); code != 0 || err != nil || r.AutoAccept.Source != "glossmark.calibration.tsv" || !r.AutoAccept.Met || r.AutoAccept.N != 1 {
		t.Errorf("rule from the repository's calibration: %+v", r.AutoAccept)
	}
	// Without a file to write to, --write says what to add.
	root2 := tiny(t, map[string]string{"doer.py": "# @gloss implements .Doer\ndef doer(): pass\n"})
	if code, _ := gloss(t, "suggest", "--calibrate", "--write", "--root", root2); code != 2 {
		t.Errorf("--calibrate --write without suggest.calibration: %d, want 2", code)
	}
}
