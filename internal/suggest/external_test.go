// @gloss verifies gmi:SourcePlugin scope=file

package suggest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/infer"
	"github.com/mckalvan/glossmark/internal/infer/wire"
)

// The test binary doubles as a plugin when GLOSS_SUGGEST_SERVE names what
// it serves: a built-in source over the wire protocol (what gloss source
// serve does), a fake source, or a wrong contract version.
func TestMain(m *testing.M) {
	switch mode := os.Getenv("GLOSS_SUGGEST_SERVE"); mode {
	case "":
		os.Exit(m.Run())
	case "fake":
		if err := wire.Serve(&fakeSource{}, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "v99":
		var in wire.Message
		json.NewDecoder(os.Stdin).Decode(&in)
		fmt.Println(`{"type":"indexed","contract":99,"name":"fake"}`)
	default:
		src := infer.BuiltinNamed(mode)
		if src == nil {
			fmt.Fprintln(os.Stderr, "no built-in source", mode)
			os.Exit(2)
		}
		if err := wire.Serve(src, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	os.Exit(0)
}

// fakeSource is a plugin built outside the runner: it declares one item
// in the contract file and one hint for a declaration, with a prior
// above the auto-accept threshold.
type fakeSource struct{}

func (fakeSource) Name() string { return "fake" }
func (fakeSource) Priors() map[string]float64 {
	return map[string]float64{"spec": 0.99, "field/func": 0.95}
}
func (fakeSource) Index(infer.Files, infer.Vocabulary) error { return nil }
func (fakeSource) Items(p string) []infer.Item {
	if p != "contracts/asyncapi.yaml" {
		return nil
	}
	return []infer.Item{{Symbol: "info", Role: "info", Relation: "documents", Line: 2, End: 10, Target: infer.Target{Concept: "claim:ClaimFiled", Basis: "spec", Terms: "fake"}}}
}
func (fakeSource) Hints(d infer.Decl) []infer.Hint {
	if d.Symbol != "assess" {
		return nil
	}
	return []infer.Hint{{Target: infer.Target{Names: []string{"claim assessment"}, Kinds: []string{"process"}, Basis: "field", Terms: "assess"}, Relation: "implements", Line: d.Line}}
}

// withSources appends sources: entries that run this test binary as the
// plugin, in the stripped copy's glossmark.yaml.
func withSources(t *testing.T, root, mode string, names ...string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOSS_SUGGEST_SERVE", mode)
	cfg := filepath.Join(root, "glossmark.yaml")
	b, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b) + "sources:\n"
	for _, n := range names {
		s += fmt.Sprintf("  - { name: %s, command: [%q] }\n", n, exe)
	}
	if err := os.WriteFile(cfg, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestExternalAsyncAPIIsByteIdentical (#288): on claims-events stripped of
// its markers, gloss suggest's output is the same whether the built-in
// AsyncAPI source runs in the binary or as a plugin over the wire, which
// proves the protocol carries everything the interface does.
func TestExternalAsyncAPIIsByteIdentical(t *testing.T) {
	root, inProcess, _ := stripped(t, claimsEvents, nil, 0.01)
	withSources(t, root, "asyncapi", "asyncapi")
	ws, g := build(t, root)
	var warned []string
	external, _, err := Run(ws, g, nil, Options{MinConf: 0.01, Warn: func(m string) { warned = append(warned, m) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(warned) > 0 {
		t.Fatal(warned)
	}
	asJSON := func(ps []Proposal) string {
		b, _ := json.MarshalIndent(ps, "", " ")
		return string(b)
	}
	if a, b := asJSON(inProcess), asJSON(external); a != b {
		t.Errorf("proposals differ:\nin process:\n%s\nexternal:\n%s", a, b)
	}
	pa, err := Patch(root, inProcess)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := Patch(root, external)
	if err != nil {
		t.Fatal(err)
	}
	if pa != pb {
		t.Error("patches differ")
	}
	n := 0
	for _, p := range external {
		if p.Source == "asyncapi" {
			n++
		}
	}
	if n < 10 {
		t.Errorf("only %d asyncapi proposals through the plugin", n)
	}
}

// TestExternalSourceContributes: a plugin's items and hints become
// proposals under its own name, anchored and rendered by the runner; its
// prior is capped below the auto-accept threshold, so an unmeasured
// source never auto-accepts; a hint with no prior reads Unmeasured.
func TestExternalSourceContributes(t *testing.T) {
	root, _, _ := stripped(t, claimsEvents, nil, 0.01)
	withSources(t, root, "fake", "fake")
	ws, g := build(t, root)
	ps, _, err := Run(ws, g, nil, Options{MinConf: 0.01, AutoAcceptThreshold: 0.8})
	if err != nil {
		t.Fatal(err)
	}
	var item, hint *Proposal
	for i := range ps {
		p := &ps[i]
		if p.Source != "fake" {
			continue
		}
		switch {
		case p.Path == "contracts/asyncapi.yaml" && p.Symbol == "info":
			item = p
		case strings.HasSuffix(p.Path, "rules.py") && p.Symbol == "assess":
			hint = p
		}
	}
	if item == nil {
		t.Fatal("the plugin's item was not proposed")
	}
	if item.Relation != "documents" || item.Target != "claim:ClaimFiled" || item.Scope != "block" || item.Basis != "fake/spec/info" || item.Conf != 0.79 || !strings.Contains(item.Marker, "@gloss? documents claim:ClaimFiled") {
		t.Errorf("item: %+v", *item)
	}
	if item.AutoAccepts(0.8) {
		t.Error("an unmeasured source auto-accepted")
	}
	if hint == nil {
		t.Fatal("the plugin's hint was not proposed")
	}
	// The name ties between the core process and the mechanism that
	// realizes it; either way the basis is the plugin's and the conf its
	// capped prior.
	if hint.Relation != "implements" || !strings.Contains(hint.Target, "Assessment") || !strings.HasPrefix(hint.Basis, "fake/field/func") || hint.Conf != 0.79 {
		t.Errorf("hint: %+v", *hint)
	}
	// --source keeps the plugin's proposals alone.
	only, _, err := Run(ws, g, nil, Options{MinConf: 0.01, Sources: []string{"fake"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(only) != 2 {
		t.Errorf("--source fake kept %d proposals, want 2", len(only))
	}
	for _, p := range only {
		if p.Source != "fake" {
			t.Errorf("kept %s", p.Source)
		}
	}
}

// TestExternalContractMismatchFails: a plugin on another contract version
// fails the run, naming both versions and the plugin (#209).
func TestExternalContractMismatchFails(t *testing.T) {
	root := t.TempDir()
	copyStripped(t, claimsEvents, root)
	withSources(t, root, "v99", "fake")
	ws, g := build(t, root)
	_, _, err := Run(ws, g, nil, Options{MinConf: 0.01})
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"v99", fmt.Sprintf("v%d", wire.Contract), "fake"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
}

// TestExternalFailureIsWarned: a plugin that breaks is reported with
// GM700 and the other sources still run.
func TestExternalFailureIsWarned(t *testing.T) {
	root := t.TempDir()
	copyStripped(t, claimsEvents, root)
	withSources(t, root, "fake", "fake", "broken")
	cfg := filepath.Join(root, "glossmark.yaml")
	b, _ := os.ReadFile(cfg)
	s := strings.Replace(string(b), "name: broken, command: [", "name: broken, command: [\"/nonexistent/gloss-source-broken\", ", 1)
	os.WriteFile(cfg, []byte(s), 0o644)
	ws, g := build(t, root)
	var warned []string
	ps, _, err := Run(ws, g, nil, Options{MinConf: 0.01, Warn: func(m string) { warned = append(warned, m) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "source plugin broken") || !strings.Contains(warned[0], "[GM700]") {
		t.Errorf("warnings: %v", warned)
	}
	srcs := map[string]bool{}
	for _, p := range ps {
		srcs[p.Source] = true
	}
	if !srcs["fake"] || !srcs["asyncapi"] {
		t.Errorf("sources after a failure: %v", srcs)
	}
}
