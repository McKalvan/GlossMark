// @gloss verifies gmi:MarkerSuggester scope=file

package suggest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/rollup"
)

var (
	warehouse = filepath.Join("testdata", "calibration", "warehouse-legacy")
	clinic    = filepath.Join("testdata", "calibration", "clinic-monorepo")
)

// stripped copies a fixture without its markers, adds files, and returns
// the proposals at the given floor, keyed path#symbol.
func stripped(t *testing.T, src string, extra map[string]string, minConf float64) (string, []Proposal, map[string]Proposal) {
	t.Helper()
	root := t.TempDir()
	copyStripped(t, src, root)
	for p, c := range extra {
		full := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ws, g := build(t, root)
	ps, _, err := Run(ws, g, nil, Options{MinConf: minConf})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Proposal{}
	for _, p := range ps {
		got[p.Path+"#"+p.Symbol] = p
	}
	return root, ps, got
}

func want(t *testing.T, got map[string]Proposal, key, rel, target string) {
	t.Helper()
	p, ok := got[key]
	if !ok {
		t.Errorf("%s: no proposal, want %s %s", key, rel, target)
		return
	}
	if p.Relation != rel || p.Target != target {
		t.Errorf("%s: got %s %s [%s], want %s %s", key, p.Relation, p.Target, p.Matched, rel, target)
	}
}

// TestConceptPoorEvidence (#58): in code whose names carry no domain
// words, docstrings, comments, the tables a class touches and the calls a
// test makes find the concept.
func TestConceptPoorEvidence(t *testing.T) {
	_, _, got := stripped(t, warehouse, map[string]string{
		"zz.py": "# Top up the pick faces (replenishment).\ndef zz1():\n    pass\n\n\ndef zz2():\n    \"\"\"Replenishment: top up the pick faces.\"\"\"\n    pass\n",
	}, DefaultMinConf)
	want(t, got, "zz.py#zz1", "implements", "wh:Replenishment")
	want(t, got, "zz.py#zz2", "implements", "wh:Replenishment")
	want(t, got, "helpers.py#process_data", "implements", "wh:CycleCount")
	want(t, got, "helpers.py#handle", "implements", "wh:StockAdjustment")
	want(t, got, "web/functions.php#handle_post", "implements", "wh:StockAdjustment")
	want(t, got, "core/Manager.py#doIt", "implements", ".ReplenishmentRun")
	want(t, got, "core/Manager.py#run_batch", "implements", ".WaveBatch")
	want(t, got, "tests/test_manager.py#test_doIt", "verifies", ".ReplenishmentRun")
	want(t, got, "tests/test_manager.py#test_process_data", "verifies", "wh:CycleCount")
	p := got["core/Processor.py#Processor"]
	if p.Target != "wh:ReturnsProcessing" || !strings.Contains(p.Context, "wh:ReturnAuthorization (tbl_rma)") {
		t.Errorf("Processor touches tbl_rma, which represents wh:ReturnAuthorization, which ReturnsProcessing consumes: %+v", p)
	}
}

// TestGenericWordsDoNotMatch (#59): programming vocabulary and fragments of
// concept-name words match nothing, at any floor; an alias bypasses the
// stoplist and glossmark.yaml extends it.
func TestGenericWordsDoNotMatch(t *testing.T) {
	probes := "def ent():\n    pass\n\n\ndef plen():\n    pass\n\n\ndef nish():\n    pass\n\n\ndef process():\n    pass\n\n\ndef handler():\n    pass\n\n\n" +
		"class Handler:\n    pass\n\n\nclass Manager:\n    pass\n\n\ndef h():\n    pass\n\n\ndef q():\n    pass\n\n\ndef replenishment_report():\n    pass\n"
	_, ps, got := stripped(t, warehouse, map[string]string{"probes.py": probes}, 0.01)
	for _, sym := range []string{"ent", "plen", "nish", "process", "handler", "Handler", "Manager", "h", "q"} {
		if p, ok := got["probes.py#"+sym]; ok {
			t.Errorf("%s matched %s [%s]", sym, p.Target, p.Matched)
		}
	}
	for _, p := range ps {
		if p.Symbol == "app" || p.Symbol == "Manager" || p.Symbol == "Handler" || p.Path == "web/functions.php" && (p.Symbol == "h" || p.Symbol == "q") {
			t.Errorf("generic declaration proposed: %+v", p)
		}
	}
	// The one mechanism that realizes the concept the name matched.
	want(t, got, "probes.py#replenishment_report", "implements", ".ReplenishmentRun")

	// An alias written on purpose matches; a configured stopword does not.
	root := t.TempDir()
	copyStripped(t, warehouse, root)
	onto := filepath.Join(root, "ontology", "wh.gloss.yaml")
	b, _ := os.ReadFile(onto)
	os.WriteFile(onto, []byte(strings.Replace(string(b), "aliases: [move stock]", "aliases: [move stock, handler]", 1)), 0o644)
	cfg := filepath.Join(root, "glossmark.yaml")
	b, _ = os.ReadFile(cfg)
	os.WriteFile(cfg, append(b, []byte("suggest:\n  stopwords: [replenishment]\n")...), 0o644)
	os.WriteFile(filepath.Join(root, "probes.py"), []byte(probes), 0o644)
	ws, g := build(t, root)
	ps, _, err := Run(ws, g, nil, Options{MinConf: 0.01})
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]Proposal{}
	for _, p := range ps {
		got[p.Path+"#"+p.Symbol] = p
	}
	want(t, got, "probes.py#Handler", "implements", "wh:StockAdjustment")
	if p, ok := got["probes.py#replenishment_report"]; ok {
		t.Errorf("a configured stopword still matched: %+v", p)
	}
}

// TestPrecisionRisesWithTheFloor (#59, #69): raising --min-conf never
// lowers precision on the concept-poor fixture.
func TestPrecisionRisesWithTheFloor(t *testing.T) {
	_, g0 := build(t, warehouse)
	labels := LabelsFromGraph(g0)
	root := t.TempDir()
	copyStripped(t, warehouse, root)
	ws, g := build(t, root)
	last := -1.0
	for _, min := range []float64{0.1, 0.3, 0.5, 0.7} {
		ps, _, _ := Run(ws, g, nil, Options{MinConf: min})
		if len(ps) == 0 {
			break
		}
		rep := Evaluate("", Score(ws, ps, labels), labels, 0.7)
		if p := rep.All.Precision(); p < last {
			t.Errorf("min-conf %.1f: precision %.2f fell from %.2f", min, p, last)
		} else {
			last = p
		}
	}
}

// TestMethodsAreProposed (#60): methods of a class are scanned like
// top-level declarations; a member that repeats its class's link is not.
func TestMethodsAreProposed(t *testing.T) {
	_, _, got := stripped(t, warehouse, map[string]string{
		"core/Counter.py": "class Counter:\n    \"\"\"Recount one zone.\"\"\"\n\n    def again(self):\n        \"\"\"Recount it again.\"\"\"\n        pass\n",
	}, 0.01)
	want(t, got, "core/Manager.py#run_batch", "implements", ".WaveBatch")
	want(t, got, "common/src/main/java/com/acme/ops/common/StringUtils.java#check", "implements", ".SsccCheckDigit")
	want(t, got, "common/src/main/java/com/acme/ops/common/Handler.java#doIt", "implements", "wh:Replenishment")
	want(t, got, "core/Counter.py#Counter", "implements", "wh:CycleCount")
	if p, ok := got["core/Counter.py#again"]; ok {
		t.Errorf("a member repeating its class's link: %+v", p)
	}
}

// TestThinOntology (#64): an empty tier gets a hint instead of silence, and
// --concepts clusters the returns code around its tables.
func TestThinOntology(t *testing.T) {
	root := t.TempDir()
	copyStripped(t, warehouse, root)
	os.WriteFile(filepath.Join(root, "ontology", "wh.gloss.yaml"), []byte("namespace: wh\nversion: 0.1.0\nconcepts: {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "ontology", "ops.gloss.yaml"), []byte("namespace: ops\nversion: 0.1.0\nconcepts: {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".gloss.yaml"), []byte("boundary: whb:opstool\nlevel: container\n"), 0o644)
	ws, g := build(t, root)
	hints := strings.Join(Hints(ws, g), "\n")
	if !strings.Contains(hints, "wh: has 0 concepts; write the core tier first (adopt-glossmark step 3)") || !strings.Contains(hints, "ops: has 0 concepts") {
		t.Errorf("hints: %q", hints)
	}
	ps, _, err := Run(ws, g, nil, Options{})
	if err != nil || ps == nil || len(ps) != 0 {
		t.Errorf("an empty ontology proposes an empty list, not null: %v %v", ps, err)
	}
	var returns *ConceptProposal
	cs := Concepts(ws, g, nil)
	for i, c := range cs {
		m := strings.Join(c.Members, " ")
		if strings.Contains(m, "core/Processor.py#Processor") && strings.Contains(m, "table.tbl_rma") && strings.Contains(m, "table.tbl_rma_vendor") {
			returns = &cs[i]
		}
	}
	if returns == nil || !strings.HasPrefix(returns.Target, "wh:NEW:") {
		t.Fatalf("no returns cluster: %+v", cs)
	}
	if strings.Contains(returns.Target, "Process") || strings.Contains(returns.Target, "Handl") {
		t.Errorf("a generic word named the cluster: %s", returns.Target)
	}
}

// TestSuggestOutputPassesCheck (#66): what suggest --write writes trips
// neither the allowed set gloss check derives (GM502, GM503) nor the
// repeated-marker lint (GM131), and a run of identical proposals is one
// marker.
func TestSuggestOutputPassesCheck(t *testing.T) {
	extra := map[string]string{
		"services/booking-api/internal/booking/patient.go": "package booking\n\ntype Patient struct{}\n",
		"replenish.py": "def replenishment_start():\n    pass\n\n\ndef replenishment_stop():\n    pass\n\n\ndef replenishment_retry():\n    pass\n\n\ndef cycle_count_now():\n    pass\n",
	}
	for _, src := range []string{clinic, warehouse} {
		root, ps, got := stripped(t, src, extra, 0.01)
		if err := Write(root, ps); err != nil {
			t.Fatal(err)
		}
		ws, g := build(t, root)
		for _, d := range check.Run(ws, g) {
			if d.Severity == "error" || d.Code == "GM131" || d.Code == "GM502" || d.Code == "GM503" || d.Code == "GM221" {
				t.Errorf("%s: after write: %s", src, d)
			}
		}
		if src == clinic {
			if p, ok := got["services/booking-api/internal/booking/patient.go#Patient"]; ok {
				t.Errorf("outside the booking API's derived allowed set: %+v", p)
			}
			p := got["services/reminder-worker/tests/test_scheduler.py#"]
			if p.Scope != "file" || p.Target != ".ReminderScheduler" || !strings.Contains(p.Marker, "scope=file") {
				t.Errorf("three tests of one concept are one scope=file marker: %+v", p)
			}
		} else {
			p := got["replenish.py#replenishment_start"]
			if p.Scope != "block" || p.Line != 1 {
				t.Errorf("a run of three is one scope=block marker above the first: %+v", p)
			}
			if _, ok := got["replenish.py#replenishment_stop"]; ok {
				t.Error("the rest of the run is not proposed again")
			}
			want(t, got, "replenish.py#cycle_count_now", "implements", "wh:CycleCount")
		}
	}
}

// TestDerivedAllowedExpanded (#66): allowed: [derived, X] proposes exactly
// what the expanded list gloss check --explain prints proposes.
func TestDerivedAllowedExpanded(t *testing.T) {
	run := func(allowed string) []string {
		root := t.TempDir()
		copyStripped(t, clinic, root)
		if allowed != "" {
			m := filepath.Join(root, "services", "reminder-worker", ".gloss.yaml")
			b, _ := os.ReadFile(m)
			os.WriteFile(m, []byte(strings.Replace(string(b), "allowed: [derived, clinic:Appointment]", "allowed: ["+allowed+"]", 1)), 0o644)
		}
		ws, g := build(t, root)
		ps, _, _ := Run(ws, g, nil, Options{MinConf: 0.01})
		var out []string
		for _, p := range ps {
			out = append(out, p.Path+"#"+p.Symbol+" "+p.Relation+" "+p.Target)
		}
		if allowed == "" {
			for _, m := range ws.Manifests {
				if strings.HasPrefix(m.Path, "services/reminder-worker") {
					expanded = strings.Join(rollup.AllowedFor(g, ws, m).Patterns, ", ")
				}
			}
		}
		return out
	}
	withDerived := run("")
	if expanded == "" || strings.Contains(expanded, "derived") {
		t.Fatalf("expansion: %q", expanded)
	}
	if a, b := strings.Join(withDerived, "\n"), strings.Join(run(expanded), "\n"); a != b {
		t.Errorf("derived and its expansion disagree:\n%s\n---\n%s", a, b)
	}
	found := false
	for _, p := range withDerived {
		found = found || strings.Contains(p, "compute_noshow_report implements .NoShowReport")
	}
	if !found {
		t.Errorf("an in-scope proposal was dropped by the derived keyword: %v", withDerived)
	}
}

var expanded string

// TestSurfacesOnly (#68): package clauses, namespace modules, fakes and
// fixtures in test files, SQL indexes and types, Rust impl blocks,
// module-level variables and private declarations are never proposed.
func TestSurfacesOnly(t *testing.T) {
	_, ps, _ := stripped(t, clinic, map[string]string{
		"services/booking-api/src/lib.rs": "pub struct BookingWindow {}\n\nimpl BookingWindow {\n    pub fn open(&self) {}\n}\n\nfn booking_helper() {}\n",
	}, 0.01)
	for _, p := range ps {
		bad := false
		switch {
		case p.Symbol == "booking" || p.Symbol == "httpapi" || p.Symbol == "store" || p.Symbol == "Portal":
			bad = true
		case strings.HasSuffix(p.Path, "fake_test.go"):
			bad = true
		case strings.HasPrefix(p.Symbol, "index.") || strings.HasPrefix(p.Symbol, "type."):
			bad = true
		case p.Symbol == "overlaps" || p.Symbol == "router" || p.Symbol == "appointment" || p.Symbol == "booking_helper" || p.Symbol == "CancellationWindow":
			bad = true
		case strings.HasSuffix(p.Path, "lib.rs") && p.Line == 3:
			bad = true
		}
		if bad {
			t.Errorf("not a public surface: %+v", p)
		}
	}
}

// TestRelationsBeyondStarterFive (#67): pages present, layouts host,
// Kubernetes workloads deploy, Markdown headings document, listeners
// subscribe, and YAML and Markdown files are read.
func TestRelationsBeyondStarterFive(t *testing.T) {
	_, ps, got := stripped(t, clinic, map[string]string{
		"apps/web/app/bookings/layout.tsx": "export default function BookingsLayout({ children }) {\n  return <section>{children}</section>;\n}\n",
	}, 0.01)
	rels, exts := map[string]bool{}, map[string]bool{}
	for _, p := range ps {
		rels[p.Relation] = true
		exts[filepath.Ext(p.Path)] = true
	}
	for _, r := range []string{"presents", "hosts", "deploys", "documents", "subscribes", "provisions", "represents", "verifies", "implements"} {
		if !rels[r] {
			t.Errorf("no %s proposal", r)
		}
	}
	if !exts[".yaml"] || !exts[".md"] {
		t.Errorf("file types: %v", exts)
	}
	want(t, got, "apps/web/app/bookings/page.tsx#BookingsPage", "presents", "clinic:Booking")
	want(t, got, "apps/web/app/bookings/layout.tsx#BookingsLayout", "hosts", "clinic:Booking")
	want(t, got, "deploy/k8s/booking-api.yaml#Deployment/booking-api", "deploys", ".BookingApi")
	want(t, got, "docs/booking.md#Booking", "documents", "clinic:Booking")
	want(t, got, "services/reminder-worker/worker/scheduler.py#on_booking_confirmed", "subscribes", "clinic:BookingConfirmed")
	for _, r := range []string{"presents", "hosts", "deploys", "documents", "subscribes", ".yaml", ".md"} {
		if !strings.Contains(Coverage, r) {
			t.Errorf("gloss suggest -h does not list %s", r)
		}
	}
}

// TestConfSpreads (#69): conf is no longer ten rescaled match scores: it
// takes a dozen or more values, and a threshold sweep from 0.3 to 0.9
// changes the selected set at most 0.1 steps. (Where it does not, no kind
// of evidence measured a precision in that band; the calibration report
// shows which.)
func TestConfSpreads(t *testing.T) {
	all, _ := pool(runCalibration(t), "")
	values := map[float64]bool{}
	for _, s := range all {
		values[s.Conf] = true
	}
	if len(values) < 12 {
		t.Errorf("conf takes %d values", len(values))
	}
	prev, same := -1, 0
	for _, th := range []float64{0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9} {
		n := 0
		for _, s := range all {
			if s.Conf >= th {
				n++
			}
		}
		if prev >= 0 && n == prev {
			same++
		}
		prev = n
	}
	if same > 2 {
		t.Errorf("%d of 6 threshold steps select the same proposals as the step below", same)
	}
}
