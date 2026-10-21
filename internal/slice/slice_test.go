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

// library returns the landscape of the two library repositories, unioned
// by IRI as gloss landscape does.
func library(t *testing.T) *graph.Graph {
	t.Helper()
	ws1, g1 := build(t, "library/circulation")
	ws2, g2 := build(t, "library/holds-worker")
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

func TestLibrarySlices(t *testing.T) {
	g := library(t)
	const checkIn, announce = "services/circulation/loans/returns.py#check_in", "services/circulation/loans/returns.py#announce_return"
	const renew = "services/circulation/loans/renew.py#renew"
	t.Run("core seed", func(t *testing.T) {
		s := of(t, g, ctxOn, "lib:Return")
		expect(t, s, map[string]string{
			"lib:Return": slice.RoleSeed, "circ:ReturnPipeline": slice.RoleRealization, "circ:ReturnNotifier": slice.RolePart,
			checkIn: slice.RoleArtifact, announce: slice.RoleArtifact,
			"acme:circulation-svc": slice.RoleBoundary, "acme:library-platform": slice.RoleBoundary,
			"lib:ItemReturned": slice.RoleContext, "lib:Item": slice.RoleContext,
			// The subscriber on the other side of the published seam.
			"hw:HoldQueue": slice.RoleContext, "acme:holds-worker": slice.RoleContext,
			// Unrelated: renewal and its implementation, checkout, the hold-fulfillment mechanism.
			"lib:Renewal": "", renew: "", "lib:Loan": "", "circ:CheckoutHandler": "", "lib:InterlibraryLoan": "", "lib:HoldFulfillment": "",
		})
		if v := s.Node(checkIn).Via; strings.Join(v, " · ") != "circ:ReturnPipeline realizes lib:Return · "+checkIn+" implements circ:ReturnPipeline" {
			t.Errorf("via of %s = %v", checkIn, v)
		}
		if v := s.Node("hw:HoldQueue").Via; len(v) == 0 || v[len(v)-1] != "hw:HoldQueue subscribes lib:ItemReturned" {
			t.Errorf("via of hw:HoldQueue = %v", v)
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
		s := of(t, g, slice.Options{}, "lib:Return")
		expect(t, s, map[string]string{"circ:ReturnPipeline": slice.RoleRealization, "acme:circulation-svc": slice.RoleBoundary, "lib:ItemReturned": "", "lib:Item": ""})
	})
	t.Run("implementation seed", func(t *testing.T) {
		s := of(t, g, ctxOn, "circ:ReturnPipeline")
		expect(t, s, map[string]string{
			"circ:ReturnPipeline": slice.RoleSeed, "circ:ReturnNotifier": slice.RolePart, checkIn: slice.RoleArtifact, announce: slice.RoleArtifact,
			"acme:circulation-svc": slice.RoleBoundary,
			// What it stands for is context: realizes is never followed upward into the members.
			"lib:Return": slice.RoleContext, "lib:ItemReturned": slice.RoleContext,
			"lib:Renewal": "", renew: "",
		})
	})
	t.Run("boundary seed", func(t *testing.T) {
		s := of(t, g, ctxOn, "acme:holds-worker")
		expect(t, s, map[string]string{
			"acme:holds-worker": slice.RoleSeed, "lib:HoldFulfillment": slice.RoleHoused, "hw:HoldQueue": slice.RoleHoused,
			"hw:CurbsidePickup": slice.RoleKind, "acme:library-platform": slice.RoleBoundary,
			"lib:ItemReturned": slice.RoleContext, "circ:ReturnNotifier": slice.RoleContext,
			// lib:Return publishes lib:ItemReturned itself, so it is context too;
			// its implementation is not.
			"lib:Return": slice.RoleContext, "circ:ReturnPipeline": "", checkIn: "",
		})
	})
	t.Run("event seed", func(t *testing.T) {
		s := of(t, g, ctxOn, "lib:ItemReturned")
		expect(t, s, map[string]string{
			"lib:ItemReturned": slice.RoleSeed, "circ:ReturnNotifier": slice.RoleRealization, announce: slice.RoleArtifact,
			"acme:circulation-svc": slice.RoleBoundary, "acme:holds-worker": slice.RoleBoundary,
			"hw:HoldQueue": slice.RoleContext, "circ:ReturnPipeline": slice.RoleContext, "lib:Return": slice.RoleContext,
			checkIn: "",
		})
	})
	t.Run("published concept", func(t *testing.T) {
		s := of(t, g, ctxOn, "lib:PickupWindow")
		expect(t, s, map[string]string{
			"lib:PickupWindow": slice.RoleSeed, "hw:PickupClock": slice.RolePublished,
			"acme:holds-worker": slice.RoleBoundary, "lib:HoldFulfillment": slice.RoleContext,
			"hw:HoldQueue": "", "lib:Return": "",
		})
		s = of(t, g, ctxOn, "hw:PickupClock")
		expect(t, s, map[string]string{"hw:PickupClock": slice.RoleSeed, "lib:PickupWindow": slice.RolePublished, "lib:HoldFulfillment": slice.RoleContext})
	})
	t.Run("path seed", func(t *testing.T) {
		s := of(t, g, ctxOn, "services/circulation/loans/returns.py")
		if strings.Join(s.Seeds, ",") != "circ:ReturnNotifier,circ:ReturnPipeline,lib:ItemReturned" {
			t.Errorf("path seeds = %v", s.Seeds)
		}
	})
}

func TestCirculationSlices(t *testing.T) {
	_, g := build(t, "library/circulation")
	t.Run("parts, realizations and tests", func(t *testing.T) {
		s := of(t, g, ctxOn, "lib:Cataloging")
		expect(t, s, map[string]string{
			"circ:CatalogIntake": slice.RoleRealization, "circ:MarcParser": slice.RolePart, "circ:MarcFieldMapper": slice.RolePart,
			"services/circulation/tests/test_intake.py#test_catalog_book": slice.RoleTest,
			"services/circulation/cataloging/intake.py#catalog_book":      slice.RoleArtifact,
			"acme:circulation-svc": slice.RoleBoundary, "lib:Book": slice.RoleContext, "lib:Media": slice.RoleContext,
			// Returning is another feature: it is realized in the same
			// container, but nothing ties it to cataloging.
			"circ:ReturnPipeline": "", "lib:Return": "",
		})
		if s.Summary.Tests != 1 {
			t.Errorf("tests = %d, want 1", s.Summary.Tests)
		}
	})
	t.Run("kinds without context", func(t *testing.T) {
		s := of(t, g, slice.Options{}, "lib:Media")
		expect(t, s, map[string]string{"lib:Book": slice.RoleKind, "lib:DVD": slice.RoleKind, "lib:Item": "", "lib:Cataloging": ""})
		s = of(t, g, ctxOn, "lib:Media")
		expect(t, s, map[string]string{"lib:Item": slice.RoleContext, "lib:Laptop": ""})
	})
	t.Run("follow", func(t *testing.T) {
		s := of(t, g, slice.Options{Follow: []string{"produces"}}, "lib:Cataloging")
		expect(t, s, map[string]string{"lib:Media": slice.RoleFlow, "lib:Book": slice.RoleFlow, "services/circulation/models/book.ts#Book": slice.RoleArtifact, "lib:Item": ""})
	})
	t.Run("constraint", func(t *testing.T) {
		s := of(t, g, ctxOn, "lib:LoanPeriod")
		expect(t, s, map[string]string{"circ:DefaultLoanPeriod": slice.RolePublished, "lib:Loan": slice.RoleContext})
	})
}

func TestParseFollowAndResolve(t *testing.T) {
	if _, err := slice.ParseFollow("produces,calls"); err == nil {
		t.Error("calls is not a flow relation")
	}
	if f, err := slice.ParseFollow(" produces, consumes "); err != nil || strings.Join(f, ",") != "produces,consumes" {
		t.Errorf("ParseFollow = %v, %v", f, err)
	}
	_, g := build(t, "library/circulation")
	if _, err := slice.Resolve(g, "", "lib:Nope"); err == nil {
		t.Error("an unknown CURIE should not resolve")
	}
}
