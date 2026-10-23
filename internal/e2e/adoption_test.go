// @gloss verifies gmi:Linter, gmi:WorkspaceLoader, gmi:DriftComparator, gmi:CriticalityStamper scope=file

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// codesOf runs gloss check (load and lint) on a root and groups messages by code.
func codesOf(t *testing.T, root string) map[string][]string {
	t.Helper()
	ws, g, res, _ := build(t, root)
	codes := map[string][]string{}
	for _, d := range append(append(ws.Diags, res.Diags...), check.Run(ws, g)...) {
		codes[d.Code] = append(codes[d.Code], d.Path+": "+d.Msg)
	}
	return codes
}

// TestImplementsOnEntityWarns: implements on an entity, its kind inherited
// through is-a and the link only proposed, warns GM221 and names represents
// (issue #160, E6).
func TestImplementsOnEntityWarns(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  claim: { iri: https://x/claim/, tier: core }\n")
	write("ontology/claim.gloss.yaml", "concepts:\n  Claim: { kind: entity }\n  MotorClaim: { is-a: Claim }\n  Filing: { kind: process }\n")
	write("app/.gloss.yaml", "boundary: o:app\nlevel: container\n")
	write("app/Claim.kt", "// @gloss? implements claim:MotorClaim conf=0.95\ndata class MotorClaim(val id: String)\n// @gloss implements claim:Filing\nfun file() {}\n")
	gm221 := codesOf(t, root)["GM221"]
	if len(gm221) != 1 || !strings.Contains(gm221[0], "claim:MotorClaim is an entity") || !strings.Contains(gm221[0], "represents it") {
		t.Errorf("GM221 = %v, want one for claim:MotorClaim naming represents", gm221)
	}
}

// TestHasPartCompletesTheParts: has-part on a whole that realizes something
// completes its parts for GM320, as part-of on each part does (issue #160,
// FF9). GM400 on has-part of an imported concept suggests what is allowed,
// not the inverse edge GM401 forbids (FF10).
func TestHasPartCompletesTheParts(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  dsp: { iri: https://x/dsp/, tier: implementation }\nimports:\n  frt: { iri: https://x/frt/, source: ./upstream, tier: core }\n")
	write("upstream/frt.gloss.yaml", "namespace: frt\nconcepts:\n  Dispatching: { kind: process }\n  Parcel: { kind: entity }\n")
	write("ontology/dsp.gloss.yaml", "concepts:\n"+
		"  PickupPlanner: { kind: process, realizes: frt:Dispatching, has-part: [DriverMatcher, RouteOptimizer] }\n"+
		"  DriverMatcher: { kind: process }\n"+
		"  RouteOptimizer: { kind: process }\n"+
		"  Loose: { kind: process }\n"+
		"  PickupManifest: { kind: entity, internal: true, has-part: [frt:Parcel] }\n")
	codes := codesOf(t, root)
	if gm320 := codes["GM320"]; len(gm320) != 1 || !strings.Contains(gm320[0], "dsp:Loose") {
		t.Errorf("GM320 = %v, want only dsp:Loose", gm320)
	}
	gm400 := codes["GM400"]
	if len(gm400) != 1 || !strings.Contains(gm400[0], "Declare a dsp: concept that is-a frt:Parcel and part-of dsp:PickupManifest") || strings.Contains(gm400[0], "instead") {
		t.Errorf("GM400 = %v, want the allowed fixes", gm400)
	}
}

// TestForeignKeyIsOwnSubject: a frt: key in dsp's file is the own-subject
// rule's (GM401), not a casing warning on dsp:frt:... (issue #175).
func TestForeignKeyIsOwnSubject(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  dsp: { iri: https://x/dsp/, tier: implementation }\nimports:\n  frt: { iri: https://x/frt/, source: ./upstream, tier: core }\n")
	write("upstream/frt.gloss.yaml", "namespace: frt\nconcepts:\n  Pickup: { kind: process }\n")
	write("ontology/dsp.gloss.yaml", "concepts:\n  frt:ExpressPickup: { is-a: frt:Pickup }\n")
	codes := codesOf(t, root)
	if gm := codes["GM302"]; len(gm) != 0 {
		t.Errorf("GM302 = %v, want none", gm)
	}
	if gm := codes["GM401"]; len(gm) != 1 || !strings.Contains(gm[0], ": frt:ExpressPickup: concept defined in namespace dsp:") {
		t.Errorf("GM401 = %v, want frt:ExpressPickup in dsp:", gm)
	}
}

// TestGroupedViolationPointsAtSets: the second violation of one boundary and
// relation points at the line that prints the sets, as a reference rather
// than part of the claim (issue #175).
func TestGroupedViolationPointsAtSets(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  frt: { iri: https://x/frt/, tier: core }\n")
	write("ontology/frt.gloss.yaml", "concepts:\n  ShipmentBooked: { kind: entity }\n  ShipmentPickedUp: { kind: entity }\n  Route: { kind: entity }\n")
	write(".gloss.yaml", "boundary: o:planner\nlevel: container\nholds: [frt:Route]\n")
	write("a.py", "# @gloss represents frt:ShipmentBooked\nclass A: pass\n# @gloss represents frt:ShipmentPickedUp\nclass B: pass\n")
	ws, g, _, lg := build(t, root)
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	vs := findings(rep, rollup.KindViolation)
	if len(vs) != 2 || vs[0].Concept != "frt:ShipmentBooked" {
		t.Fatalf("violations = %+v", vs)
	}
	if want := "o:planner holds frt:ShipmentPickedUp, which its declared holds does not cover (declared and derived sets: see the frt:ShipmentBooked line)"; vs[1].Msg != want {
		t.Errorf("second violation = %q, want %q", vs[1].Msg, want)
	}
}

// TestUnloadedOntologyFileWarns: an ontology file under ontology/ that no
// owns: or imports: entry loads warns GM402 with the namespace it declares;
// loaded and vendored files do not (issue #160, FF19).
func TestUnloadedOntologyFileWarns(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  dsp: { iri: https://x/dsp/, file: ontology/dsp.gloss.yaml, tier: implementation }\nimports:\n  frt: { iri: https://x/frt/, tier: core }\n")
	write("ontology/frt.gloss.yaml", "namespace: frt\nconcepts:\n  Parcel: { kind: entity }\n") // the vendored import
	write("ontology/dsp.gloss.yaml", "concepts:\n  Planner: { kind: process, internal: true }\n")
	write("ontology/frt-ext.gloss.yaml", "namespace: frt\nconcepts:\n  Pallet: { kind: entity }\n")
	write("ontology/more/dsp-more.gloss.yaml", "namespace: dsp\nconcepts:\n  Extra: { kind: process }\n")
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range ws.Diags {
		if d.Code == "GM402" {
			got = append(got, d.Path+": "+d.Msg)
		}
	}
	if len(got) != 2 {
		t.Fatalf("GM402 = %v, want frt-ext and dsp-more", got)
	}
	if !strings.HasPrefix(got[0], "ontology/frt-ext.gloss.yaml: ") || !strings.Contains(got[0], "namespace frt:, which this repo does not own") {
		t.Errorf("GM402 for the extension = %q", got[0])
	}
	if !strings.HasPrefix(got[1], "ontology/more/dsp-more.gloss.yaml: ") || !strings.Contains(got[1], "owns.dsp.file is ontology/dsp.gloss.yaml") {
		t.Errorf("GM402 for the second dsp file = %q", got[1])
	}
}

// TestOrphanIgnoresIntent: a concept the ontology says a reached concept
// subscribes to or consumes is still an orphan, as the manifest's gap on it
// says no code subscribes to it; a marker on the code reaches it and closes
// the gap, and a documents link alone never does (issues #160 FF12, #173).
func TestOrphanIgnoresIntent(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  frt: { iri: https://x/frt/, tier: core }\n  sc: { iri: https://x/sc/, tier: implementation }\n")
	write("ontology/frt.gloss.yaml", "concepts:\n  ShipmentPickedUp: { kind: event }\n  ShipmentDelivered: { kind: event }\n  Manifest: { kind: entity }\n  Tracking: { kind: process }\n")
	write("ontology/sc.gloss.yaml", "concepts:\n  StatusProjector: { kind: process, realizes: frt:Tracking, subscribes: [frt:ShipmentPickedUp], consumes: [frt:Manifest] }\n")
	write(".gloss.yaml", "boundary: o:sc\nlevel: container\nsubscribes: [frt:ShipmentPickedUp]\n")
	write("listener.py", "# @gloss implements sc:StatusProjector\ndef on_event(): pass\n")
	write("docs/events.md", "<!-- @gloss documents frt:ShipmentDelivered scope=file -->\n# Events\n")
	report := func() *rollup.Report {
		ws, g, _, lg := build(t, root)
		return rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	}
	rep := report()
	if got := strings.Join(concepts(findings(rep, rollup.KindOrphan)), ","); got != "frt:Manifest,frt:ShipmentDelivered,frt:ShipmentPickedUp" {
		t.Errorf("intent only: orphans = %s, want frt:Manifest, frt:ShipmentDelivered and frt:ShipmentPickedUp", got)
	}
	if gaps := findings(rep, rollup.KindGap); len(gaps) != 1 || gaps[0].Concept != "frt:ShipmentPickedUp" || !strings.Contains(gaps[0].Msg, "intends it, but no code under it subscribes to it") {
		t.Errorf("intent only: gaps = %+v, want the intent gap on frt:ShipmentPickedUp", gaps)
	}
	write("listener.py", "# @gloss implements sc:StatusProjector\n# @gloss subscribes frt:ShipmentPickedUp\ndef on_event(): pass\n\n\n# @gloss consumes frt:Manifest\ndef load(): pass\n")
	rep = report()
	orphans := findings(rep, rollup.KindOrphan)
	if got := strings.Join(concepts(orphans), ","); got != "frt:ShipmentDelivered" {
		t.Errorf("orphans = %s, want only frt:ShipmentDelivered", got)
	}
	if len(orphans) == 1 && !strings.Contains(orphans[0].Msg, "documents or instructs link does not count") {
		t.Errorf("orphan message = %q", orphans[0].Msg)
	}
	if gaps := findings(rep, rollup.KindGap); len(gaps) != 0 {
		t.Errorf("gaps = %+v, want none once code subscribes", gaps)
	}
}

// TestDriftReportsDeprecatedUse: drift lists code on a deprecated concept as
// deprecated-use, which does not fail by default and a waiver silences, and
// the status export marks the concept deprecated (issue #160, L27).
func TestDriftReportsDeprecatedUse(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  lib: { iri: https://x/lib/, tier: core }\n")
	write("ontology/lib.gloss.yaml", "concepts:\n  Patron: { kind: entity }\n  Borrower: { kind: entity, deprecated: true, replaced-by: Patron }\n")
	write(".gloss.yaml", "boundary: o:lib\nlevel: container\nholds: [lib:Borrower]\n")
	write("borrower.py", "# @gloss represents lib:Borrower\nclass Borrower: pass\n")
	ws, g, _, lg := build(t, root)
	pol := ws.Config.EffectivePolicy()
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: pol})
	rollup.AddDeprecatedUse(rep, g, pol, time.Now())
	uses := findings(rep, rollup.KindDeprecatedUse)
	if len(uses) != 2 {
		t.Fatalf("deprecated-use = %+v, want the marker and the manifest", uses)
	}
	kinds, _ := rollup.ParseFailOn("violation,value-drift,gap,dangling,overdue")
	for _, f := range rep.Failing(kinds) {
		if f.Kind == rollup.KindDeprecatedUse {
			t.Errorf("deprecated-use fails by default: %+v", f)
		}
	}
	if !strings.Contains(rollup.RenderText(rep, false), "Deprecated in use") {
		t.Error("drift text has no deprecated-use section")
	}
	var row rollup.ConceptStatusRow
	for _, r := range rollup.ConceptStatus(g, rep) {
		if r.Concept == "lib:Borrower" {
			row = r
		}
	}
	if !row.Deprecated || row.ReplacedBy != "lib:Patron" || !strings.Contains(strings.Join(row.Findings, ","), rollup.KindDeprecatedUse) {
		t.Errorf("status row = %+v, want deprecated, replaced-by lib:Patron and the deprecated-use finding", row)
	}
}

// TestSecurityTestWords: the security-test rule of criticality suggest
// matches whole words of a test's name, so a payment state such as
// authorized does not make a test a security test (issue #160, E20).
func TestSecurityTestWords(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  pay: { iri: https://x/pay/, tier: core }\n")
	write("ontology/pay.gloss.yaml", "concepts:\n  PayoutNotification: { kind: process }\n  Login: { kind: process }\n  Session: { kind: process }\n")
	write(".gloss.yaml", "boundary: o:pay\nlevel: container\n")
	write("notify.go", "package pay\n\n// @gloss implements pay:PayoutNotification, pay:Login, pay:Session\nfunc Notify() {}\n")
	write("notify_test.go", "package pay\n\nimport \"testing\"\n\n// @gloss verifies pay:PayoutNotification\nfunc TestPayoutAuthorizedNotifiesThePolicyholder(t *testing.T) {}\n\n"+
		"// @gloss verifies pay:Login\nfunc TestUnauthorizedLoginIsRejected(t *testing.T) {}\n\n// @gloss verifies pay:Session\nfunc TestCSRFTokenRotates(t *testing.T) {}\n")
	_, g, _, _ := build(t, root)
	got := map[string]bool{}
	for _, s := range rollup.SuggestCriticality(g) {
		got[s.Concept] = true
	}
	if got["pay:PayoutNotification"] {
		t.Error("a test named for an authorized payout was read as a security test")
	}
	if !got["pay:Login"] || !got["pay:Session"] {
		t.Errorf("unauthorized and CSRFToken tests are security tests; suggestions: %v", got)
	}
}
