// @gloss verifies gmi:Gate, gmi:DriftComparator scope=file

package e2e

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// TestGuards: guards (and its alias applies-to) says a cross-cutting
// mechanism applies to concepts (ADR 0015). gloss impact shows it on both
// ends with the guarded files, gloss context names it for the directory,
// and gloss diff reviews a change to guarded code at the guard's level
// while the code keeps its own level in the graph (#73).
func TestGuards(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Approving: { kind: process }\n  Listing: { kind: process }\n  Access: { kind: constraint, constrains: [Approving] }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Approver: { kind: process, realizes: z:Approving }\n  Lister: { kind: process, realizes: z:Listing }\n"+
		"  Auth: { kind: process, realizes: z:Access, criticality: high, guards: [Approver, Lister] }\n  Limit: { kind: process, internal: true, applies-to: [Lister] }\n")
	write(".gloss.yaml", "boundary: o:svc\nlevel: container\ndefault-namespace: i\n")
	const handlers = "# @gloss implements .Approver\ndef approve(): pass\n\n\n# @gloss implements .Lister\n@requires_login\ndef listing(): pass\n"
	write("svc/handlers/claims.py", handlers)
	write("svc/platform/auth.py", "# @gloss implements .Auth scope=file\n")
	write("svc/tests/test_claims.py", "# @gloss verifies .Lister scope=file\n")
	ws, base, _, _ := build(t, root)
	for _, d := range check.Run(ws, base) {
		if d.Severity == "error" {
			t.Errorf("check: %s", d)
		}
	}
	var alias []string
	for _, e := range base.Edges() {
		if e.P == "guards" && e.Prov != nil && e.Prov.Alias == "applies-to" {
			alias = append(alias, base.Curie(e.S)+" "+base.Curie(e.O))
		}
	}
	if !reflect.DeepEqual(alias, []string{"i:Limit i:Lister"}) {
		t.Errorf("applies-to edges = %v, want i:Limit i:Lister stored as guards", alias)
	}
	// The handler keeps its own level: a guard is not inherited.
	for _, a := range base.NodesOf(model.NodeArtifact) {
		if a.Path == "svc/handlers/claims.py" && a.Criticality != "" {
			t.Errorf("%s inherited %s from %s", a.ID, a.Criticality, a.CriticalityFrom)
		}
	}
	touch := map[string]gate.ConceptTouch{}
	fts := gate.Touched(base, ws, []string{"svc/handlers/claims.py", "svc/platform/auth.py"})
	for _, ft := range fts {
		for _, c := range ft.Concepts {
			touch[ft.Path+" "+c.Curie] = c
		}
	}
	if got := touch["svc/handlers/claims.py i:Lister"].GuardedBy; !reflect.DeepEqual(got, []string{"i:Auth (high)", "i:Limit"}) {
		t.Errorf("i:Lister guarded by %v", got)
	}
	if ft := fts[0]; ft.Criticality != "high" || ft.CriticalityFrom != "i:Auth (guard)" {
		t.Errorf("claims.py reviewed at %q from %q, want high from the guard", ft.Criticality, ft.CriticalityFrom)
	}
	if c := touch["svc/platform/auth.py i:Auth"]; !reflect.DeepEqual(c.Guards, []string{"i:Approver", "i:Lister"}) || !reflect.DeepEqual(c.GuardedCode, []string{"svc/handlers/claims.py"}) {
		t.Errorf("i:Auth guards %v, guarded code %v", c.Guards, c.GuardedCode)
	}
	if c := touch["svc/platform/auth.py z:Access"]; !reflect.DeepEqual(c.ConstrainedCode, []string{"svc/handlers/claims.py"}) {
		t.Errorf("z:Access constrained code %v", c.ConstrainedCode)
	}
	if out := gate.RenderTouched(fts); !strings.Contains(out, "guarded by i:Auth (high), i:Limit") || !strings.Contains(out, "guarded code: svc/handlers/claims.py") {
		t.Errorf("impact text:\n%s", out)
	}
	rep := rollup.Compare(base, ws, rollup.Options{Policy: ws.Config.EffectivePolicy()})
	brief := agents.Context(ws, base, ws.ManifestFor("svc/handlers/claims.py"), rep, agents.ContextOptions{Dir: "svc/handlers"})
	if want := "Applies here: i:Auth (high) guards i:Approver, i:Lister; i:Limit guards i:Lister; z:Access constrains z:Approving"; !strings.Contains(brief, want) {
		t.Errorf("briefing lacks %q:\n%s", want, brief)
	}
	if brief := agents.Context(ws, base, nil, rep, agents.ContextOptions{Dir: "svc/tests"}); strings.Contains(brief, "Applies here") {
		t.Errorf("a test directory is not guarded:\n%s", brief)
	}
	// Deleting the login decorator on the unclassified Lister is SOFT and
	// reviewed at the guard's level.
	write("svc/handlers/claims.py", strings.Replace(handlers, "@requires_login\n", "", 1))
	_, head, _, _ := build(t, root)
	im := gate.Classify(base, head, ws, gate.Options{Base: "base", Policy: ws.Config.EffectivePolicy()})
	var got []string
	for _, ch := range im.Changes {
		got = append(got, string(ch.Gate)+" "+ch.Kind+" "+ch.Object+" "+ch.Criticality+" "+ch.CriticalityFrom)
	}
	if want := []string{"SOFT decorators-changed i:Lister high i:Auth"}; !reflect.DeepEqual(got, want) {
		t.Errorf("changes = %q, want %q", got, want)
	}
}

// TestDecoratorFingerprintMigrates: a link accepted before the decorator
// stack joined the fingerprint (ADR 0014) stays accepted after the upgrade,
// and is stale once a decorator is then deleted.
func TestDecoratorFingerprintMigrates(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Approving: { kind: process }\n")
	const src = "# @gloss implements z:Approving\n@requires_role(\"approver\")\ndef approve(): pass\n"
	write("h.py", src)
	_, g, res, _ := build(t, root)
	lp := filepath.Join(t.TempDir(), "ledger.jsonl")
	lg, _ := ledger.Open(lp)
	lg.Sync(g, res.Markers, "human:test", time.Now())
	if len(res.Markers) != 1 {
		t.Fatalf("%d markers", len(res.Markers))
	}
	id := res.Markers[0].ID
	// What the previous release recorded: the signature line alone.
	lg.Entries[id].AnchorFingerprint = res.Markers[0].Anchor.Legacy
	if lg.Entries[id].AnchorFingerprint == "" || lg.Entries[id].AnchorFingerprint == res.Markers[0].Anchor.Fingerprint {
		t.Fatalf("no legacy fingerprint for a decorated anchor: %+v", res.Markers[0].Anchor)
	}
	lg.Sync(g, res.Markers, "human:test", time.Now())
	if e := lg.Entries[id]; e.Status != model.StatusAccepted || e.AnchorFingerprint != res.Markers[0].Anchor.Fingerprint {
		t.Errorf("after upgrade: %s %s, want accepted with the new fingerprint", e.Status, e.AnchorFingerprint)
	}
	write("h.py", "# @gloss implements z:Approving\ndef approve(): pass\n")
	_, g, res, _ = build(t, root)
	lg.Sync(g, res.Markers, "human:test", time.Now())
	if e := lg.Entries[id]; e.Status != model.StatusStale {
		t.Errorf("guard deleted: %s, want stale", e.Status)
	}
}

// TestRepeatedMarkerRemedy: GM131 suggests what spans the run. Above a run
// of top-level functions that is scope=file, since scope=block covers one
// function; above methods it is the enclosing class. Applying the advice
// clears the warning and leaves one edge per concept (#76).
func TestRepeatedMarkerRemedy(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Log: { kind: process, internal: true }\n  Queries: { kind: capability, internal: true }\n")
	fns := "def zero():\n    return 0\n"
	methods := "class Handlers:\n"
	for _, f := range []string{"one", "two", "three"} {
		fns += "\n\n# @gloss implements i:Log\n@route(\"/" + f + "\")\ndef " + f + "():\n    return 1\n"
		methods += "    # @gloss implements i:Queries\n    def " + f + "(self):\n        return 1\n\n"
	}
	write("probe.py", fns)
	write("methods.py", methods)
	gm131 := func() map[string]string {
		ws, g, _, _ := build(t, root)
		out := map[string]string{}
		for _, d := range check.Run(ws, g) {
			if d.Code == "GM131" {
				out[d.Path] = d.Msg
			}
		}
		return out
	}
	got := gm131()
	if m := got["probe.py"]; !strings.Contains(m, "one marker with scope=file at the top of the file") || !strings.Contains(m, "guards:") {
		t.Errorf("top-level run: %q", m)
	}
	if m := got["methods.py"]; !strings.Contains(m, "one marker on the enclosing class or block") {
		t.Errorf("method run: %q", m)
	}
	// Follow the advice.
	write("probe.py", "# @gloss implements i:Log scope=file\n"+strings.ReplaceAll(fns, "# @gloss implements i:Log\n", ""))
	write("methods.py", "# @gloss implements i:Queries scope=block\n"+strings.ReplaceAll(methods, "    # @gloss implements i:Queries\n", ""))
	if got := gm131(); len(got) != 0 {
		t.Errorf("after the remedy: %v", got)
	}
	_, g, _, _ := build(t, root)
	edges := map[string]int{}
	for _, e := range g.Edges() {
		if e.P == "implements" && e.Source == "inline" {
			edges[g.Curie(e.O)]++
		}
	}
	if !reflect.DeepEqual(edges, map[string]int{"i:Log": 1, "i:Queries": 1}) {
		t.Errorf("implements edges after the remedy = %v, want one per concept", edges)
	}
}

// TestValueFromConfigKey: value-from symbol: in a YAML file is a dotted key
// path (or a key that ends only one path); it binds, and changing the
// value in the config drifts (#77). A key that ends several paths is an
// error naming them.
func TestValueFromConfigKey(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  i: { iri: https://x/i/, tier: implementation }\n")
	ont := "concepts:\n  WriteLimit:\n    kind: constraint\n    internal: true\n    value: \"30\"\n    value-from: { path: config/ratelimit.yaml, symbol: policies.claims-write.per_minute }\n"
	write("ontology/i.gloss.yaml", ont)
	cfg := "policies:\n  claims-write:\n    per_minute: 30\n  export:\n    per_minute: 2\n"
	write("config/ratelimit.yaml", cfg)
	lgPath := filepath.Join(root, ".glossmark", "ledger.jsonl")
	run := func() []string {
		t.Helper()
		ws, g, res, _ := build(t, root)
		lg, _ := ledger.Open(lgPath)
		lg.Sync(g, res.Markers, "human:test", time.Now())
		lg.Save()
		if n := g.NodeByCurie("i:WriteLimit"); n.ValueFrom != "config/ratelimit.yaml:3" {
			t.Errorf("bound to %q, want config/ratelimit.yaml:3", n.ValueFrom)
		}
		return concepts(findings(rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries}), rollup.KindValueDrift))
	}
	if d := run(); len(d) != 0 {
		t.Errorf("baseline drift = %v", d)
	}
	write("config/ratelimit.yaml", strings.Replace(cfg, "per_minute: 30", "per_minute: 60", 1))
	if d := run(); !reflect.DeepEqual(d, []string{"i:WriteLimit"}) {
		t.Errorf("after the config changed: drift = %v, want [i:WriteLimit]", d)
	}
	// A tail that names one key binds too; a bare key that names two is an error.
	write("ontology/i.gloss.yaml", strings.Replace(ont, "symbol: policies.claims-write.per_minute", "symbol: claims-write.per_minute", 1))
	run()
	write("ontology/i.gloss.yaml", strings.Replace(ont, "symbol: policies.claims-write.per_minute", "symbol: per_minute", 1))
	ws, _ := load.Load(root)
	res, _ := extract.Build(ws)
	var msgs []string
	for _, d := range res.Diags {
		if d.Code == "GM360" {
			msgs = append(msgs, d.Msg)
		}
	}
	if len(msgs) != 1 || !strings.Contains(msgs[0], "policies.claims-write.per_minute, policies.export.per_minute") {
		t.Errorf("GM360 = %v, want one naming both candidates", msgs)
	}
}

// TestImpactTestsVia: gloss impact counts a verifier of a concept that
// realizes a core concept, as gloss drift does, and names it; it warns
// exactly where drift reports untested (#79).
func TestImpactTestsVia(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Approving: { kind: process }\n  History: { kind: capability }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Decider: { kind: process, realizes: z:Approving }\n  Queries: { kind: capability, realizes: z:History }\n")
	write("claims.py", "# @gloss implements i:Decider\ndef approve(): pass\n\n# @gloss implements i:Queries\ndef history(): pass\n")
	write("tests/test_claims.py", "# @gloss verifies i:Decider\ndef test_approve(): pass\n")
	ws, g, _, lg := build(t, root)
	untested := concepts(findings(rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries}), rollup.KindUntested))
	if !reflect.DeepEqual(untested, []string{"z:History"}) {
		t.Fatalf("drift untested = %v", untested)
	}
	fts := gate.Touched(g, ws, []string{"claims.py"})
	var warned []string
	for _, c := range fts[0].Concepts {
		for _, w := range c.Warnings {
			if strings.Contains(w, "no tests") {
				warned = append(warned, c.Curie)
			}
		}
		if c.Curie == "z:Approving" && !reflect.DeepEqual(c.TestsVia, []gate.ViaTests{{Via: "i:Decider", Tests: []string{"tests/test_claims.py"}}}) {
			t.Errorf("z:Approving tests via %+v", c.TestsVia)
		}
	}
	if !reflect.DeepEqual(warned, []string{"i:Queries", "z:History"}) {
		t.Errorf("impact warns on %v, want i:Queries and z:History (what drift calls untested, and its realizer)", warned)
	}
	if out := gate.RenderTouched(fts); !strings.Contains(out, "tests (via i:Decider): tests/test_claims.py") {
		t.Errorf("impact text:\n%s", out)
	}
}

// TestInternalCriticalUntested: an internal concept is left out of untested
// and concept coverage unless its effective criticality is high or critical;
// then losing its only test shows (#74).
func TestInternalCriticalUntested(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Auth: { kind: process, internal: true, criticality: high }\n  Logging: { kind: process, internal: true, criticality: low }\n  Flags: { kind: capability, internal: true }\n")
	write("auth.py", "# @gloss implements i:Auth scope=file\n")
	write("log.py", "# @gloss implements i:Logging scope=file\n")
	write("flags.py", "# @gloss implements i:Flags scope=file\n")
	write("tests/test_auth.py", "# @gloss verifies i:Auth scope=file\n")
	report := func() *rollup.Report {
		ws, g, _, lg := build(t, root)
		return rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	}
	rep := report()
	if u := concepts(findings(rep, rollup.KindUntested)); len(u) != 0 {
		t.Errorf("untested = %v, want none (the high one is tested, the others are below high)", u)
	}
	if m := rep.Metrics; m.ConceptsInScope != 1 || m.ConceptsCovered != 1 {
		t.Errorf("coverage %d of %d, want 1 of 1 (only the high internal concept counts)", m.ConceptsCovered, m.ConceptsInScope)
	}
	write("tests/test_auth.py", "def test_auth(): pass\n")
	rep = report()
	if u := concepts(findings(rep, rollup.KindUntested)); !reflect.DeepEqual(u, []string{"i:Auth"}) {
		t.Errorf("without its test: untested = %v, want [i:Auth]", u)
	}
	if m := rep.Metrics; m.ConceptsInScope != 1 || m.ConceptsCovered != 0 {
		t.Errorf("without its test: coverage %d of %d, want 0 of 1", m.ConceptsCovered, m.ConceptsInScope)
	}
}
