// @gloss verifies gmi:RollupEngine, gmi:DriftComparator, gmi:ViewProjector scope=file

package e2e

import (
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/views"
)

// claimsRepo writes a small event-driven workspace: claim filing publishes
// ClaimFiled (MotorClaimFiled is one), payout notification subscribes to
// two payout events, and claims come in two subtypes beside assessments
// and payouts that share nothing with them.
func claimsRepo(t *testing.T) (string, func(rel, content string)) {
	t.Helper()
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  cl: { iri: https://x/cl/, tier: core }\n")
	write("ontology/cl.gloss.yaml", "concepts:\n"+
		"  ClaimFiling: { kind: process, publishes: [ClaimFiled] }\n"+
		"  ClaimFiled: { kind: event }\n"+
		"  MotorClaimFiled: { kind: event, is-a: ClaimFiled }\n"+
		"  PayoutNotification: { kind: process, subscribes: [PayoutAuthorized, PayoutFailed] }\n"+
		"  PayoutAuthorized: { kind: event }\n"+
		"  PayoutFailed: { kind: event }\n"+
		"  Claim: { kind: entity }\n"+
		"  MotorClaim: { kind: entity, is-a: Claim }\n"+
		"  FloodClaim: { kind: entity, is-a: Claim }\n"+
		"  Assessment: { kind: entity }\n"+
		"  Payout: { kind: entity }\n"+
		"  Unrelated: { kind: event }\n"+
		"relations:\n  produces: { lift: range-lcs }\n")
	return root, write
}

func compare(t *testing.T, root string) (*rollup.Report, *graph.Graph) {
	t.Helper()
	ws, g, _, lg := build(t, root)
	return rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()}), g
}

func about(rep *rollup.Report, boundary, rel string) []rollup.Finding {
	var out []rollup.Finding
	for _, f := range rep.Findings {
		if f.Boundary == boundary && f.Relation == rel {
			out = append(out, f)
		}
	}
	return out
}

// TestIntendedFlows (E16, ADR 0026): a concept's own flows lift to the
// boundary as intent, apart from the code's. A container may declare only
// the subtype its code publishes, and intent never closes a gap.
func TestIntendedFlows(t *testing.T) {
	root, write := claimsRepo(t)
	write("intake/.gloss.yaml", "boundary: o:intake\nlevel: container\nrealizes: [cl:ClaimFiling]\npublishes: [cl:MotorClaimFiled]\n")
	write("intake/file.py", "# @gloss implements cl:ClaimFiling\n# @gloss publishes cl:MotorClaimFiled\ndef file_claim(): pass\n")
	write("payout/.gloss.yaml", "boundary: o:payout\nlevel: container\nrealizes: [cl:PayoutNotification]\nsubscribes: [cl:PayoutAuthorized, cl:PayoutFailed]\n")
	write("payout/notify.py", "# @gloss implements cl:PayoutNotification\n# @gloss subscribes cl:PayoutAuthorized\ndef notify(): pass\n")
	rep, g := compare(t, root)
	// (a) The intent publishes cl:ClaimFiled; the manifest declares a subtype of it.
	fs := about(rep, "o:intake", "publishes")
	if len(fs) != 1 || fs[0].Kind != rollup.KindConsistent || strings.Join(fs[0].Intended, ",") != "cl:ClaimFiled" || strings.Join(fs[0].Derived, ",") != "cl:MotorClaimFiled" {
		t.Errorf("intake publishes: %+v, want consistent with intended {cl:ClaimFiled} apart from derived {cl:MotorClaimFiled}", fs)
	}
	// (b) The concept intends cl:PayoutFailed and the manifest declares it,
	// but no code subscribes to it: a gap that names the intent.
	fs = about(rep, "o:payout", "subscribes")
	if len(fs) != 1 || fs[0].Kind != rollup.KindGap || fs[0].Concept != "cl:PayoutFailed" ||
		!strings.Contains(fs[0].Msg, "o:payout declares subscribes cl:PayoutFailed and cl:PayoutNotification intends it, but no code under it subscribes to it") {
		t.Errorf("payout subscribes: %+v, want one gap on cl:PayoutFailed", fs)
	}
	// The derived edge says where it came from.
	n := 0
	for _, e := range g.Out(mustIRI(t, g, "o:payout"), "subscribes") {
		if e.Source == graph.SourceDerived && g.Curie(e.O) == "cl:PayoutFailed" {
			n++
			if e.Prov == nil || e.Prov.Rule != rollup.RuleIntended {
				t.Errorf("intended edge rule = %+v, want %s", e.Prov, rollup.RuleIntended)
			}
		}
	}
	if n != 1 {
		t.Errorf("o:payout subscribes cl:PayoutFailed edges = %d, want 1", n)
	}
	// (c) Intent the manifest neither covers nor refines is a violation that says so.
	write("intake/.gloss.yaml", "boundary: o:intake\nlevel: container\nrealizes: [cl:ClaimFiling]\npublishes: [cl:MotorClaimFiled, cl:Unrelated]\n")
	write("intake/file.py", "# @gloss implements cl:ClaimFiling\ndef file_claim(): pass\n")
	write("intake/other.py", "# @gloss publishes cl:MotorClaimFiled, cl:Unrelated\ndef other(): pass\n")
	write("payout/.gloss.yaml", "boundary: o:payout\nlevel: container\nrealizes: [cl:PayoutNotification]\nsubscribes: [cl:PayoutAuthorized]\n")
	rep, _ = compare(t, root)
	if fs := about(rep, "o:intake", "publishes"); len(fs) != 1 || fs[0].Kind != rollup.KindConsistent {
		t.Errorf("a refinement of the intent beside other events: %+v", fs)
	}
	fs = about(rep, "o:payout", "subscribes")
	if len(fs) != 1 || fs[0].Kind != rollup.KindViolation || fs[0].Concept != "cl:PayoutFailed" ||
		!strings.Contains(fs[0].Msg, "o:payout subscribes cl:PayoutFailed by intent (cl:PayoutNotification intends it), but declares subscribes {cl:PayoutAuthorized}") {
		t.Errorf("payout subscribes: %+v, want one intent violation on cl:PayoutFailed", fs)
	}
}

// TestPartialRangeLCS (E17, ADR 0026): lift: range-lcs folds each maximal
// subset that shares a subsumer, in drift and in views, instead of all or
// nothing per boundary and relation.
func TestPartialRangeLCS(t *testing.T) {
	root, write := claimsRepo(t)
	write("ontology/views.yaml", "views:\n  flow:\n    include: [cl:*]\n    relations: [produces]\n    lift: range-lcs\n    zoom: container\n")
	write("svc/.gloss.yaml", "boundary: o:svc\nlevel: container\nproduces: [cl:Claim, cl:Assessment, cl:Payout]\n")
	write("svc/run.py", "# @gloss produces cl:Claim, cl:MotorClaim, cl:FloodClaim, cl:Assessment, cl:Payout\ndef run(): pass\n")
	rep, _ := compare(t, root)
	fs := about(rep, "o:svc", "produces")
	if len(fs) != 1 || !strings.Contains(fs[0].Msg, "o:svc produces {cl:Assessment, cl:Claim, cl:Payout} ⊑ declared") {
		t.Errorf("consistent: %+v, want the claims folded into cl:Claim", fs)
	}
	// A violation names what each lifted concept summarizes.
	write("svc/.gloss.yaml", "boundary: o:svc\nlevel: container\nproduces: [cl:Claim]\n")
	rep, _ = compare(t, root)
	var v []rollup.Finding
	for _, f := range about(rep, "o:svc", "produces") {
		if f.Kind == rollup.KindViolation {
			v = append(v, f)
		}
	}
	if len(v) != 2 || !strings.Contains(v[0].Msg, "rollup: produces {cl:Assessment, cl:Claim, cl:Payout} (cl:Claim from cl:Claim, cl:FloodClaim, cl:MotorClaim) ⊄ declared") {
		t.Errorf("violations: %+v", v)
	}
	// The view draws one lifted edge to cl:Claim beside the two others.
	ws, g, _, _ := build(t, root)
	flow, _ := view(t, views.ProjectAll(g, ws.Views), "flow")
	var got []string
	for _, e := range flow.Edges {
		if e.S == "o:svc" && e.P == "produces" {
			s := e.O
			if e.Lifted {
				s += "(lifted " + strings.Join(e.Via, " ") + ")"
			}
			got = append(got, s)
		}
	}
	if strings.Join(got, ", ") != "cl:Assessment, cl:Claim(lifted cl:Claim cl:FloodClaim cl:MotorClaim), cl:Payout" {
		t.Errorf("flow: o:svc produces %v", got)
	}
}

// TestNestingIsContainment (FF11, ADR 0026): a manifest that names no
// part-of is part of the nearest manifest in an enclosing directory, so the
// parent's rollup counts its code. part-of: [] opts out.
func TestNestingIsContainment(t *testing.T) {
	root, write := claimsRepo(t)
	write(".gloss.yaml", "boundary: o:platform\nlevel: system\npublishes: [cl:ClaimFiled]\n")
	write("services/intake/.gloss.yaml", "boundary: o:intake\nlevel: container\n")
	write("services/intake/file.py", "# @gloss publishes cl:ClaimFiled\ndef file_claim(): pass\n")
	rep, g := compare(t, root)
	if fs := about(rep, "o:platform", "publishes"); len(fs) != 1 || fs[0].Kind != rollup.KindConsistent {
		t.Errorf("platform publishes: %+v, want consistent through the nested manifest", fs)
	}
	var partOf []string
	for _, e := range g.Out(mustIRI(t, g, "o:intake"), "part-of") {
		partOf = append(partOf, g.Curie(e.O)+" "+e.Prov.Rule)
	}
	if strings.Join(partOf, ",") != "o:platform nearest-manifest" {
		t.Errorf("o:intake part-of = %v, want o:platform from the nearest manifest", partOf)
	}
	// An explicit part-of wins, and part-of: [] is part of nothing.
	write("services/intake/.gloss.yaml", "boundary: o:intake\nlevel: container\npart-of: []\n")
	rep, g = compare(t, root)
	if fs := about(rep, "o:platform", "publishes"); len(fs) != 1 || fs[0].Kind != rollup.KindGap {
		t.Errorf("opted out: platform publishes %+v, want a gap", fs)
	}
	if ps := g.Out(mustIRI(t, g, "o:intake"), "part-of"); len(ps) != 0 {
		t.Errorf("opted out: o:intake part-of %v", ps)
	}
}

// TestInheritOnFlowsAndHolds (L19, ADR 0026): inherit works on holds and
// the four flows as on realizes. Inherited entries only cover; a nested
// boundary that declares nothing is reported once, on its own manifest.
func TestInheritOnFlowsAndHolds(t *testing.T) {
	root, write := claimsRepo(t)
	write(".gloss.yaml", "boundary: o:platform\nlevel: system\nholds: [inherit, cl:Payout]\nproduces: [inherit]\n")
	write("config/payouts.sql", "-- @gloss represents cl:Payout\nCREATE TABLE payouts (id int);\n")
	write("services/claims/.gloss.yaml", "boundary: o:claims\nlevel: container\nholds: [cl:Claim]\nproduces: [cl:Claim]\n")
	write("services/claims/store.py", "# @gloss represents cl:MotorClaim\n# @gloss produces cl:MotorClaim\nclass MotorClaimRow: pass\n")
	ws, g, _, lg := build(t, root)
	for _, d := range check.Run(ws, g) {
		if d.Code == "GM211" || d.Severity == "error" {
			t.Errorf("gloss check: %s", d)
		}
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	for _, rel := range []string{"holds", "produces"} {
		if fs := about(rep, "o:platform", rel); len(fs) != 1 || fs[0].Kind != rollup.KindConsistent {
			t.Errorf("platform %s: %+v, want consistent", rel, fs)
		}
	}
	// A nested boundary that holds data but declares no holds.
	write("services/assess/.gloss.yaml", "boundary: o:assess\nlevel: container\n")
	write("services/assess/rows.py", "# @gloss represents cl:Assessment\nclass AssessmentRow: pass\n")
	rep, _ = compare(t, root)
	var v []rollup.Finding
	for _, f := range rep.Findings {
		if f.Kind == rollup.KindViolation {
			v = append(v, f)
		}
	}
	if len(v) != 1 || v[0].Boundary != "o:assess" || v[0].Relation != "holds" || v[0].Path != "services/assess/.gloss.yaml" ||
		!strings.Contains(v[0].Msg, "o:assess holds 1 concept but declares no holds; o:platform inherits from it: declare it under holds in services/assess/.gloss.yaml: {cl:Assessment}") {
		t.Errorf("violations = %+v", v)
	}
	// presents takes no inherit: the word stays an unresolved target.
	write(".gloss.yaml", "boundary: o:platform\nlevel: system\npresents: [inherit]\n")
	ws, g, _, _ = build(t, root)
	n := 0
	for _, d := range check.Run(ws, g) {
		if d.Code == "GM211" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("presents: [inherit]: %d GM211, want 1", n)
	}
}
