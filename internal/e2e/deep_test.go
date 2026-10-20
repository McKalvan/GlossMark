// @gloss verifies gmi:RollupEngine, gmi:DriftComparator, gmi:Linter, gmi:ViewProjector scope=file

package e2e

import (
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/views"
)

// catalogue writes the deep-hierarchies lab workspace from the
// glossmark-examples repository (lab/deep-hierarchies/catalogue): a product
// taxonomy seven is-a levels deep, an importer taxonomy five deep whose root
// alone realizes a core process, and two part-of chains five deep, under
// nine nested manifests. Tests rewrite single files with write.
func catalogue(t *testing.T) (string, func(rel, content string)) {
	t.Helper()
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  cyc: { iri: https://x/cyc/, tier: core }\n  spk: { iri: https://x/spk/, tier: implementation }\n")
	write("ontology/cyc.gloss.yaml", "namespace: cyc\nconcepts:\n"+
		"  Product: { kind: entity }\n"+
		"  Component: { is-a: Product }\n"+
		"  DrivetrainComponent: { is-a: Component }\n"+
		"  ShiftingComponent: { is-a: DrivetrainComponent }\n"+
		"  Cassette: { is-a: DrivetrainComponent }\n"+
		"  Derailleur: { is-a: ShiftingComponent }\n"+
		"  RearDerailleur: { is-a: Derailleur }\n"+
		"  FrontDerailleur: { is-a: Derailleur }\n"+
		"  ElectronicRearDerailleur: { is-a: [RearDerailleur, PoweredDevice] }\n"+
		"  ElectronicFrontDerailleur: { is-a: [FrontDerailleur, PoweredDevice] }\n"+
		"  BrakingComponent: { is-a: Component, criticality: critical }\n"+
		"  Brake: { is-a: BrakingComponent }\n"+
		"  DiscBrake: { is-a: Brake }\n"+
		"  HydraulicDiscBrake: { is-a: DiscBrake }\n"+
		"  Accessory: { is-a: Product }\n"+
		"  Lighting: { is-a: Accessory }\n"+
		"  FrontLight: { is-a: [Lighting, PoweredDevice] }\n"+
		"  PoweredDevice: { is-a: Product }\n"+
		"  CatalogueIngestion: { kind: process, criticality: low }\n"+
		"  ProductListing: { kind: capability }\n"+
		"  OrderFulfilment: { kind: process, criticality: high }\n"+
		"  Picking: { kind: process, part-of: OrderFulfilment }\n"+
		"  PickWave: { kind: process, part-of: Picking }\n"+
		"  BinPick: { kind: process, part-of: PickWave }\n"+
		"  ScanConfirm: { kind: process, part-of: BinPick }\n"+
		"relations:\n  produces: { lift: range-lcs }\n")
	write("ontology/spk.gloss.yaml", "namespace: spk\nconcepts:\n"+
		"  Importer: { kind: process, realizes: cyc:CatalogueIngestion, criticality: high }\n"+
		"  SupplierImporter: { is-a: Importer }\n"+
		"  FeedImporter: { is-a: SupplierImporter }\n"+
		"  XmlFeedImporter: { is-a: FeedImporter }\n"+
		"  ShimanoXmlImporter: { is-a: XmlFeedImporter }\n"+
		"  CsvFeedImporter: { is-a: FeedImporter }\n"+
		"  CsvRowParser: { kind: process, part-of: CsvFeedImporter }\n"+
		"  IngestScheduler: { kind: process, realizes: cyc:CatalogueIngestion }\n"+
		"  DerailleurListing: { kind: capability, realizes: [cyc:ProductListing, cyc:ElectronicRearDerailleur] }\n"+
		"  BrakeListing: { kind: capability, realizes: [cyc:ProductListing, cyc:HydraulicDiscBrake] }\n"+
		"  LightListing: { kind: capability, realizes: [cyc:ProductListing, cyc:FrontLight] }\n"+
		"  FulfilmentEngine: { kind: system-type, realizes: cyc:OrderFulfilment }\n"+
		"  PickPlanner: { kind: process, part-of: FulfilmentEngine }\n"+
		"  WaveBuilder: { kind: process, part-of: PickPlanner }\n"+
		"  BinRouter: { kind: process, part-of: WaveBuilder }\n"+
		"  ScanVerifier: { kind: process, part-of: BinRouter }\n"+
		"  BinPickHandler: { kind: process, realizes: cyc:BinPick }\n")
	write("ontology/views.yaml", "views:\n"+
		"  business:\n    include: [cyc:*]\n    collapse: { \"spk:*\": via realizes }\n    relations: [produces, is-a, part-of]\n    lift: range-lcs\n    zoom: system\n"+
		"  components:\n    include: [cyc:*, spk:*]\n    expand: { part-of: 5 }\n    zoom: component\n"+
		"  drivetrain:\n    include: [cyc:ElectronicRearDerailleur, cyc:ElectronicFrontDerailleur, cyc:Cassette]\n    lift: range-lcs\n    zoom: component\n")
	write(".gloss.yaml", "boundary: spw:spokewise\nlevel: system\nrealizes: inherit\n")
	write("services/catalogue/.gloss.yaml", "boundary: spw:catalogue\nlevel: container\npart-of: spw:spokewise\nrealizes: inherit\n")
	write("services/catalogue/db/.gloss.yaml", "boundary: spw:catalogue-db\nlevel: component\npart-of: spw:catalogue\nholds: [cyc:Product, cyc:HydraulicDiscBrake]\n")
	write("services/catalogue/db/0001.sql", "-- @gloss represents cyc:Product\nCREATE TABLE products (sku text);\n\n-- @gloss represents cyc:HydraulicDiscBrake\nCREATE TABLE brakes (sku text);\n")
	write("services/catalogue/ingest/.gloss.yaml", "boundary: spw:ingest\nlevel: component\npart-of: spw:catalogue\nrealizes: [inherit, cyc:CatalogueIngestion]\n")
	write("services/catalogue/ingest/scheduler.py", "# @gloss implements spk:IngestScheduler\ndef due_jobs(): pass\n")
	write("services/catalogue/ingest/feeds/.gloss.yaml", "boundary: spw:feeds\nlevel: component\npart-of: spw:ingest\nrealizes: [cyc:CatalogueIngestion]\nproduces: [cyc:Component]\n")
	write("services/catalogue/ingest/feeds/shimano_xml.py", "# @gloss implements spk:ShimanoXmlImporter\n# @gloss produces cyc:ElectronicRearDerailleur, cyc:ElectronicFrontDerailleur, cyc:Cassette\ndef parse_feed(): pass\n")
	write("services/catalogue/ingest/feeds/csv_feed.py", "# @gloss implements spk:CsvRowParser\ndef parse_row(): pass\n\n\n# @gloss implements spk:CsvFeedImporter\n# @gloss produces cyc:HydraulicDiscBrake\ndef import_csv(): pass\n")
	write("services/catalogue/listing/.gloss.yaml", "boundary: spw:listing\nlevel: component\npart-of: spw:catalogue\nrealizes: [cyc:ProductListing, cyc:Product]\n")
	write("services/catalogue/listing/brakes.py", "# @gloss implements spk:BrakeListing\ndef rotor_options(): pass\n")
	write("services/catalogue/listing/derailleurs.py", "# @gloss implements spk:DerailleurListing\ndef note(): pass\n")
	write("services/catalogue/listing/lighting.py", "# @gloss implements spk:LightListing\ndef shipping_note(): pass\n")
	write("services/fulfilment/.gloss.yaml", "boundary: spw:fulfilment\nlevel: container\npart-of: spw:spokewise\nrealizes: [inherit, spk:FulfilmentEngine]\n")
	write("services/fulfilment/engine.py", "# @gloss implements spk:FulfilmentEngine\ndef release(): pass\n")
	write("services/fulfilment/picking/.gloss.yaml", "boundary: spw:picking\nlevel: component\npart-of: spw:fulfilment\nrealizes: [inherit, cyc:Picking]\n")
	write("services/fulfilment/picking/bin_pick.py", "# @gloss implements spk:BinPickHandler\ndef record_pick(): pass\n")
	write("services/fulfilment/picking/scanning/.gloss.yaml", "boundary: spw:scanning\nlevel: component\npart-of: spw:picking\nrealizes: [cyc:OrderFulfilment]\n")
	write("services/fulfilment/picking/scanning/verifier.py", "# @gloss implements spk:ScanVerifier\ndef verify_scan(): pass\n")
	write("services/fulfilment/picking/scanning/test_verifier.py", "# @gloss verifies spk:ScanVerifier\ndef test_wrong_part(): pass\n")
	return root, write
}

// TestAllowedSetFollowsCoverage (#87): the derived allowed set holds what
// drift's coverage accepts, so check and drift agree. A boundary declaring
// a core concept admits the implementation concepts that realize it,
// through any mix of is-a and part-of; declaring an implementation root
// admits a part of one of its is-a descendants.
func TestAllowedSetFollowsCoverage(t *testing.T) {
	root, write := catalogue(t)
	gm503 := func() []string {
		t.Helper()
		ws, g, _, _ := build(t, root)
		var out []string
		for _, d := range check.Run(ws, g) {
			if d.Code == "GM503" {
				out = append(out, d.Path+": "+d.Msg)
			}
		}
		return out
	}
	if got := gm503(); len(got) != 0 {
		t.Errorf("committed catalogue: GM503 = %v, want none", got)
	}
	// Mixed chain: spk:CsvRowParser is part-of spk:CsvFeedImporter, an is-a descendant of the declared spk:Importer.
	write("services/catalogue/ingest/feeds/.gloss.yaml", "boundary: spw:feeds\nlevel: component\npart-of: spw:ingest\nrealizes: [spk:Importer]\nproduces: [cyc:Component]\n")
	if got := gm503(); len(got) != 0 {
		t.Errorf("declares spk:Importer: GM503 = %v, want none", got)
	}
	ws, g, _, _ := build(t, root)
	a := rollup.AllowedFor(g, ws, ws.ManifestFor("services/catalogue/ingest/feeds/csv_feed.py"))
	reasons := map[string]rollup.AllowedEntry{}
	for _, e := range a.Entries {
		reasons[e.Pattern] = e
	}
	if e := reasons["spk:CsvRowParser"]; e.Reason != rollup.AllowedCovered || e.From != "spk:Importer" {
		t.Errorf("spk:CsvRowParser entry = %+v, want covered from spk:Importer", e)
	}
	if e := reasons["cyc:CatalogueIngestion"]; e.Reason != rollup.AllowedRealized {
		t.Errorf("cyc:CatalogueIngestion entry = %+v, want realized", e)
	}
	// Still one way: declaring a part admits neither its whole nor the whole's other parts.
	write("services/catalogue/ingest/feeds/.gloss.yaml", "boundary: spw:feeds\nlevel: component\npart-of: spw:ingest\nrealizes: [spk:CsvRowParser]\nproduces: [cyc:Component]\n")
	if got := gm503(); len(got) != 2 || !strings.Contains(strings.Join(got, "\n"), "spk:CsvFeedImporter") || !strings.Contains(strings.Join(got, "\n"), "spk:ShimanoXmlImporter") {
		t.Errorf("declares the part spk:CsvRowParser: GM503 = %v, want spk:CsvFeedImporter and spk:ShimanoXmlImporter", got)
	}
}

// TestSupertypeReachedByRealization (#90): an abstract supertype is not an
// orphan when anything below it is built, whether code links the subtype
// directly or implements a concept that realizes it.
func TestSupertypeReachedByRealization(t *testing.T) {
	root, write := catalogue(t)
	orphans := func() []string {
		t.Helper()
		ws, g, _, lg := build(t, root)
		return concepts(findings(rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries}), rollup.KindOrphan))
	}
	// cyc:FrontLight is realized by spk:LightListing, which has code; its supertypes are reached.
	if got := orphans(); len(got) != 1 || got[0] != "cyc:ScanConfirm" {
		t.Errorf("orphans = %v, want only cyc:ScanConfirm", got)
	}
	// Without the produces markers only what nothing below is built becomes an orphan:
	// cyc:Cassette, cyc:ElectronicFrontDerailleur and cyc:FrontDerailleur (whose only
	// subtype is cyc:ElectronicFrontDerailleur). The Derailleur line is still
	// reached through spk:DerailleurListing.
	write("services/catalogue/ingest/feeds/shimano_xml.py", "# @gloss implements spk:ShimanoXmlImporter\ndef parse_feed(): pass\n")
	write("services/catalogue/ingest/feeds/csv_feed.py", "# @gloss implements spk:CsvRowParser\ndef parse_row(): pass\n\n\n# @gloss implements spk:CsvFeedImporter\ndef import_csv(): pass\n")
	want := []string{"cyc:Cassette", "cyc:ElectronicFrontDerailleur", "cyc:FrontDerailleur", "cyc:ScanConfirm"}
	if got := orphans(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("orphans without produces = %v, want %v", got, want)
	}
}

// view returns the projection of the named view, and its node by CURIE.
func view(t *testing.T, ps []*views.Projection, name string) (*views.Projection, map[string]views.PNode) {
	t.Helper()
	for _, p := range ps {
		if p.Name == name {
			nodes := map[string]views.PNode{}
			for _, n := range p.Nodes {
				nodes[n.ID] = n
			}
			return p, nodes
		}
	}
	t.Fatalf("no view %q", name)
	return nil, nil
}

// TestDeepProvenance (#89): a derived fact names its whole chain. The via of
// a lifted realizes lists every concept passed through and steps spells out
// each hop; an inherited criticality names the concept that declared it;
// collapse: via realizes folds is-a descendants of a realizer too.
func TestDeepProvenance(t *testing.T) {
	root, _ := catalogue(t)
	ws, g, _, _ := build(t, root)
	feeds, ci := g.NodeByCurie("spw:feeds"), g.NodeByCurie("cyc:CatalogueIngestion")
	found := false
	for _, e := range g.Out(feeds.ID, "realizes") {
		if e.O != ci.ID || e.Prov == nil || len(e.Prov.Via) == 0 || !strings.HasSuffix(e.Prov.Via[0], "shimano_xml.py#parse_feed") {
			continue
		}
		found = true
		want := "spk:ShimanoXmlImporter,spk:XmlFeedImporter,spk:FeedImporter,spk:SupplierImporter,spk:Importer"
		if got := strings.Join(e.Prov.Via[1:], ","); got != want {
			t.Errorf("via = %v, want the artifact then %s", e.Prov.Via, want)
		}
		if got := strings.Join(e.Prov.Steps, ", "); got != "is-a spk:XmlFeedImporter, is-a spk:FeedImporter, is-a spk:SupplierImporter, is-a spk:Importer, realizes cyc:CatalogueIngestion" {
			t.Errorf("steps = %s", got)
		}
	}
	if !found {
		t.Fatal("no spw:feeds realizes cyc:CatalogueIngestion edge from shimano_xml.py")
	}
	if n := g.NodeByCurie("cyc:HydraulicDiscBrake"); n.CriticalityFrom != "cyc:DiscBrake" || n.CriticalitySource != "cyc:BrakingComponent" {
		t.Errorf("cyc:HydraulicDiscBrake criticality from %q source %q, want from cyc:DiscBrake, source cyc:BrakingComponent", n.CriticalityFrom, n.CriticalitySource)
	}
	if n := g.NodeByCurie("spk:ShimanoXmlImporter"); n.CriticalitySource != "spk:Importer" {
		t.Errorf("spk:ShimanoXmlImporter criticality source %q, want spk:Importer", n.CriticalitySource)
	}
	if n := g.NodeByCurie("cyc:BrakingComponent"); n.CriticalitySource != "" {
		t.Errorf("a declared level has no source, got %q", n.CriticalitySource)
	}
	_, biz := view(t, views.ProjectAll(g, ws.Views), "business")
	if f := biz["cyc:CatalogueIngestion"].Folded; !has(f, "spk:ShimanoXmlImporter") || !has(f, "spk:CsvRowParser") {
		t.Errorf("business fold of cyc:CatalogueIngestion = %v, want spk:ShimanoXmlImporter and spk:CsvRowParser", f)
	}
}

// TestWholePlacedWithItsMechanism (#92): code implementing a concept whose
// own realizes names a core concept places the core concept as firmly as a
// direct link, so a top-level whole sits in the container that runs it, not
// in the leaf component whose manifest declares it to cover a deep part.
func TestWholePlacedWithItsMechanism(t *testing.T) {
	root, _ := catalogue(t)
	ws, g, _, _ := build(t, root)
	_, comp := view(t, views.ProjectAll(g, ws.Views), "components")
	for curie, want := range map[string]string{
		"cyc:OrderFulfilment":    "spw:fulfilment",
		"spk:FulfilmentEngine":   "spw:fulfilment",
		"cyc:CatalogueIngestion": "spw:ingest", // spk:IngestScheduler, implemented in ingest, realizes it
		"spk:ScanVerifier":       "spw:scanning",
	} {
		if got := comp[curie].Boundary; got != want {
			t.Errorf("components: %s in %q, want %q", curie, got, want)
		}
	}
	// A whole is never drawn inside the box of one of its parts.
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Whole: { kind: process }\n  Part: { kind: process, part-of: Whole }\n")
	write("ontology/views.yaml", "views:\n  all:\n    include: [z:*]\n    zoom: component\n")
	write("svc/.gloss.yaml", "boundary: o:svc\nlevel: container\nrealizes: [z:Whole]\n")
	write("svc/part.py", "# @gloss implements z:Part\ndef part(): pass\n")
	write("svc/leaf/.gloss.yaml", "boundary: o:leaf\nlevel: component\npart-of: o:svc\nrealizes: [z:Whole]\n")
	write("svc/leaf/x.py", "def x(): pass\n")
	ws, g, _, _ = build(t, root)
	_, all := view(t, views.ProjectAll(g, ws.Views), "all")
	if all["z:Part"].Boundary != "o:svc" || all["z:Whole"].Boundary != "o:svc" {
		t.Errorf("Part in %q, Whole in %q; want both in o:svc", all["z:Part"].Boundary, all["z:Whole"].Boundary)
	}
}

// TestExplainBySource (#93): check --explain groups a derived set by source
// and collapses a declared concept with what it covers, so a manifest that
// declares two concepts reads in two lines however deep the taxonomy.
func TestExplainBySource(t *testing.T) {
	root, _ := catalogue(t)
	ws, g, _, _ := build(t, root)
	ex := check.ExplainAllowed(ws, g)
	var block []string
	for _, l := range strings.Split(ex, "\n") {
		if strings.HasPrefix(l, "spw:catalogue-db ") || len(block) > 0 && strings.HasPrefix(l, "  ") {
			block = append(block, l)
		} else if len(block) > 0 {
			break
		}
	}
	if len(block) > 3 || len(block) < 2 || !strings.Contains(block[1], "cyc:Product (+") || !strings.Contains(block[1], "cyc:HydraulicDiscBrake") {
		t.Errorf("spw:catalogue-db explain = %q, want a header and a declared line naming cyc:Product (+N below)", block)
	}
	// Core concepts inherit criticality through is-a and part-of (GOVERNANCE.md).
	if n := g.NodeByCurie("cyc:HydraulicDiscBrake"); n.Criticality != "critical" {
		t.Errorf("cyc:HydraulicDiscBrake criticality = %q, want critical through is-a", n.Criticality)
	}
	if n := g.NodeByCurie("cyc:ScanConfirm"); n.Criticality != "high" {
		t.Errorf("cyc:ScanConfirm criticality = %q, want high through part-of", n.Criticality)
	}
}
