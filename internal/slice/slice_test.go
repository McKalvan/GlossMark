// @gloss verifies gmi:ViewProjector scope=file

package slice_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/slice"
)

func build(t *testing.T, rel string) (*load.Workspace, *graph.Graph) {
	t.Helper()
	root, _ := filepath.Abs(filepath.Join("..", "..", "examples", rel))
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := extract.Build(ws)
	if err != nil {
		t.Fatal(err)
	}
	rollup.Derive(res.Graph, ws.Vocab)
	return ws, res.Graph
}

// payments returns the landscape of the two payments repositories, unioned
// by IRI as gloss landscape does.
func payments(t *testing.T) *graph.Graph {
	t.Helper()
	ws1, g1 := build(t, "payments/payments-api")
	ws2, g2 := build(t, "payments/settlement-worker")
	ws, err := load.Merge([]*load.Workspace{ws1, ws2})
	if err != nil {
		t.Fatal(err)
	}
	u := graph.New(ws.Prefixes)
	for _, g := range []*graph.Graph{g1, g2} {
		for _, n := range g.Nodes() {
			nn := *n
			if nn.NodeType == model.NodeConcept || nn.NodeType == model.NodeNamespace {
				nn.Imported = !ws.Owned[nn.NS]
			}
			if ex := u.Node(nn.ID); ex != nil && ex.Imported && !nn.Imported {
				*ex = nn
				continue
			}
			u.AddNode(&nn)
		}
		for _, e := range g.Edges() {
			if e.Source != graph.SourceDerived {
				ee := *e
				u.AddEdge(&ee)
			}
		}
	}
	rollup.Derive(u, ws.Vocab)
	return u
}

func of(t *testing.T, g *graph.Graph, opt slice.Options, seeds ...string) *slice.Slice {
	t.Helper()
	var ids []string
	for _, s := range seeds {
		got, err := slice.Resolve(g, "", s)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, got...)
	}
	return slice.Compute(g, ids, opt)
}

// expect checks roles: want maps node ID to role, "" meaning absent.
func expect(t *testing.T, s *slice.Slice, want map[string]string) {
	t.Helper()
	for id, role := range want {
		n := s.Node(id)
		switch {
		case role == "" && n != nil:
			t.Errorf("slice of %v: %s should be absent, has role %s via %v", s.Seeds, id, n.Role, n.Via)
		case role != "" && n == nil:
			t.Errorf("slice of %v: %s missing, want role %s", s.Seeds, id, role)
		case role != "" && n.Role != role:
			t.Errorf("slice of %v: %s has role %s via %v, want %s", s.Seeds, id, n.Role, n.Via, role)
		}
	}
}

var ctxOn = slice.Options{Context: true}

func TestPaymentsSlices(t *testing.T) {
	g := payments(t)
	const cap1, cap2 = "services/api/handlers/capture.py#capture", "services/api/handlers/capture.py#emit_capture_confirmed"
	const refund = "services/api/handlers/refund.py#refund"
	t.Run("core seed", func(t *testing.T) {
		s := of(t, g, ctxOn, "pay:Capture")
		expect(t, s, map[string]string{
			"pay:Capture": slice.RoleSeed, "pa:CaptureHandler": slice.RoleRealization, "pa:CaptureEmitter": slice.RolePart,
			cap1: slice.RoleArtifact, cap2: slice.RoleArtifact,
			"acme:payments-api": slice.RoleBoundary, "acme:payments-platform": slice.RoleBoundary,
			"pay:CaptureConfirmed": slice.RoleContext, "pay:Payment": slice.RoleContext,
			// The subscriber on the other side of the published seam.
			"sw:BatchSettler": slice.RoleContext, "acme:settlement-worker": slice.RoleContext,
			// Unrelated: refunding and its implementation, the settlement mechanism.
			"pay:Refunding": "", refund: "", "pay:Refund": "", "pa:AuthorizeHandler": "", "pay:Merchant": "", "pay:Settlement": "",
		})
		if v := s.Node(cap1).Via; strings.Join(v, " · ") != "pa:CaptureHandler realizes pay:Capture · "+cap1+" implements pa:CaptureHandler" {
			t.Errorf("via of %s = %v", cap1, v)
		}
		if v := s.Node("sw:BatchSettler").Via; len(v) == 0 || v[len(v)-1] != "sw:BatchSettler subscribes pay:CaptureConfirmed" {
			t.Errorf("via of sw:BatchSettler = %v", v)
		}
		// Every edge has both ends in the slice.
		for _, e := range s.Edges {
			if s.Node(e.S) == nil || s.Node(e.O) == nil {
				t.Errorf("edge %s %s %s leaves the slice", e.S, e.P, e.O)
			}
		}
		if s.Summary.Concepts != 3 || s.Summary.Artifacts != 2 {
			t.Errorf("summary = %+v", s.Summary)
		}
	})
	t.Run("context off", func(t *testing.T) {
		s := of(t, g, slice.Options{}, "pay:Capture")
		expect(t, s, map[string]string{"pa:CaptureHandler": slice.RoleRealization, "acme:payments-api": slice.RoleBoundary, "pay:CaptureConfirmed": "", "pay:Payment": ""})
	})
	t.Run("implementation seed", func(t *testing.T) {
		s := of(t, g, ctxOn, "pa:CaptureHandler")
		expect(t, s, map[string]string{
			"pa:CaptureHandler": slice.RoleSeed, "pa:CaptureEmitter": slice.RolePart, cap1: slice.RoleArtifact, cap2: slice.RoleArtifact,
			"acme:payments-api": slice.RoleBoundary,
			// What it stands for is context: realizes is never followed upward into the members.
			"pay:Capture": slice.RoleContext, "pay:CaptureConfirmed": slice.RoleContext,
			"pay:Refunding": "", refund: "",
		})
	})
	t.Run("boundary seed", func(t *testing.T) {
		s := of(t, g, ctxOn, "acme:settlement-worker")
		expect(t, s, map[string]string{
			"acme:settlement-worker": slice.RoleSeed, "pay:Settlement": slice.RoleHoused, "sw:BatchSettler": slice.RoleHoused,
			"sw:PartialSettlement": slice.RoleKind, "acme:payments-platform": slice.RoleBoundary,
			"pay:CaptureConfirmed": slice.RoleContext, "pa:CaptureEmitter": slice.RoleContext,
			"pa:CaptureHandler": "", "pay:Capture": "", cap1: "",
		})
	})
	t.Run("event seed", func(t *testing.T) {
		s := of(t, g, ctxOn, "pay:CaptureConfirmed")
		expect(t, s, map[string]string{
			"pay:CaptureConfirmed": slice.RoleSeed, "pa:CaptureEmitter": slice.RoleRealization, cap2: slice.RoleArtifact,
			"acme:payments-api": slice.RoleBoundary, "acme:settlement-worker": slice.RoleBoundary,
			"sw:BatchSettler": slice.RoleContext, "pa:CaptureHandler": slice.RoleContext,
			cap1: "", "pay:Capture": "",
		})
	})
	t.Run("published concept", func(t *testing.T) {
		s := of(t, g, ctxOn, "pay:SettlementCutoff")
		expect(t, s, map[string]string{
			"pay:SettlementCutoff": slice.RoleSeed, "sw:CutoffClock": slice.RolePublished,
			"acme:settlement-worker": slice.RoleBoundary, "pay:Settlement": slice.RoleContext,
			"sw:BatchSettler": "", "pay:Capture": "",
		})
		s = of(t, g, ctxOn, "sw:CutoffClock")
		expect(t, s, map[string]string{"sw:CutoffClock": slice.RoleSeed, "pay:SettlementCutoff": slice.RolePublished, "pay:Settlement": slice.RoleContext})
	})
	t.Run("path seed", func(t *testing.T) {
		s := of(t, g, ctxOn, "services/api/handlers/capture.py")
		if strings.Join(s.Seeds, ",") != "pa:CaptureEmitter,pa:CaptureHandler" {
			t.Errorf("path seeds = %v", s.Seeds)
		}
	})
}

func TestHorseFactorySlices(t *testing.T) {
	_, g := build(t, "horse-factory")
	t.Run("parts, realizations and tests", func(t *testing.T) {
		s := of(t, g, ctxOn, "zoo:Breeding")
		expect(t, s, map[string]string{
			"impl:HorseBuilder": slice.RoleRealization, "impl:HoofAssembler": slice.RolePart, "impl:HoofStep": slice.RolePart,
			"services/horse-factory/tests/test_engine.py#test_foal_male": slice.RoleTest,
			"services/horse-factory/breeding/engine.py#foal_male":        slice.RoleArtifact,
			"acme:horse-factory-svc":                                     slice.RoleBoundary, "zoo:Foal": slice.RoleContext, "zoo:Mare": slice.RoleContext,
			// Foaling is another feature: it produces Foal too, but is on the same side.
			"impl:FoalingPipeline": "", "zoo:Foaling": "",
		})
		if s.Summary.Tests != 2 {
			t.Errorf("tests = %d, want 2", s.Summary.Tests)
		}
	})
	t.Run("kinds without context", func(t *testing.T) {
		s := of(t, g, slice.Options{}, "zoo:Horse")
		expect(t, s, map[string]string{"zoo:Mare": slice.RoleKind, "zoo:Foal": slice.RoleKind, "zoo:Animal": "", "zoo:Breeding": ""})
		s = of(t, g, ctxOn, "zoo:Horse")
		expect(t, s, map[string]string{"zoo:Animal": slice.RoleContext, "zoo:Donkey": ""})
	})
	t.Run("follow", func(t *testing.T) {
		s := of(t, g, slice.Options{Follow: []string{"produces"}}, "zoo:Breeding")
		expect(t, s, map[string]string{"zoo:Foal": slice.RoleFlow, "zoo:Stallion": slice.RoleFlow, "db/migrations/0001_foals.sql#table.foals": slice.RoleArtifact, "zoo:Mare": ""})
	})
	t.Run("constraint", func(t *testing.T) {
		s := of(t, g, ctxOn, "zoo:ProductionCapacity")
		expect(t, s, map[string]string{"impl:FoalingPipelineThroughput": slice.RolePublished, "zoo:HorseOrder": slice.RoleContext})
	})
}

func TestParseFollowAndResolve(t *testing.T) {
	if _, err := slice.ParseFollow("produces,calls"); err == nil {
		t.Error("calls is not a flow relation")
	}
	if f, err := slice.ParseFollow(" produces, consumes "); err != nil || strings.Join(f, ",") != "produces,consumes" {
		t.Errorf("ParseFollow = %v, %v", f, err)
	}
	_, g := build(t, "horse-factory")
	if _, err := slice.Resolve(g, "", "zoo:Nope"); err == nil {
		t.Error("an unknown CURIE should not resolve")
	}
}
