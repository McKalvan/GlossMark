// @gloss implements gmi:AdoptionLadder scope=file

package rollup

import (
	"fmt"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

// FailingAtLevel keeps, of the findings --fail-on selected, those the
// adoption level lets fail (docs/QUICKSTART.md, issue #258): none below
// level 2, at level 2 only findings on a concept or boundary classified high
// or critical, everything at level 3. The rest are advisory and stay in the
// report. A finding with no concept or boundary carries no criticality and
// is advisory at level 2.
func FailingAtLevel(failing []Finding, g *graph.Graph, level int) []Finding {
	if !model.AdoptionDriftFails(level) {
		return nil
	}
	min := model.AdoptionMinCriticality(level)
	if min == "" {
		return failing
	}
	var out []Finding
	for _, f := range failing {
		if model.CriticalityRank(FindingCriticality(g, f)) >= model.CriticalityRank(min) {
			out = append(out, f)
		}
	}
	return out
}

// FindingCriticality is the highest effective criticality among a finding's
// concepts, or of its boundary when it names no concept (an overdue
// boundary), "" when none is classified. The concept decides, not the
// boundary around it: a gap on an unclassified concept inside a critical
// boundary is still a gap on an unclassified concept.
func FindingCriticality(g *graph.Graph, f Finding) string {
	lvl := ""
	subjects := strings.Split(f.Concept, ", ")
	if strings.TrimSpace(f.Concept) == "" {
		subjects = []string{f.Boundary}
	}
	for _, cu := range subjects {
		cu = strings.TrimSpace(cu)
		if cu == "" {
			continue
		}
		iri, _ := g.Expand(cu)
		if n := g.Node(iri); n != nil {
			lvl = model.MaxCriticality(lvl, n.Criticality)
		}
	}
	return lvl
}

// Observed is what the workspace has written, read against the ladder.
type Observed struct {
	// Level is the highest rung every lower rung of which is present.
	Level int
	// Has lists, per rung, the evidence found for it ("" when nothing was).
	Has [model.AdoptionFull + 1]string
	// Missing lists, per rung, what the rung asks for and the tree lacks.
	Missing [model.AdoptionFull + 1]string
}

// ObservedAdoption reads the rung a workspace has reached from what it has
// written: level 0 needs a concept of its own; level 1 a manifest and a link
// from code; level 2 a declared criticality; level 3 a
// symbol-level marker, a value binding or an import.
func ObservedAdoption(ws *load.Workspace, g *graph.Graph) Observed {
	var o Observed
	concepts := 0
	declared := 0
	bound := 0
	for _, n := range g.NodesOf(model.NodeConcept) {
		// An imported concept's declared criticality governs here too.
		if n.CriticalityFrom == "declared" {
			declared++
		}
		if n.Imported {
			continue
		}
		concepts++
		if n.ValueFrom != "" {
			bound++
		}
	}
	for _, n := range g.NodesOf(model.NodeBoundary) {
		if n.CriticalityFrom == "declared" {
			declared++
		}
	}
	manifests := 0
	for _, m := range ws.Manifests {
		if m.Boundary != "" {
			manifests++
		}
	}
	fileLinks, symbolLinks := 0, 0
	for _, e := range g.Edges() {
		// Inline links only: a marker in code, not an ontology or manifest
		// edge (which carry their file too) and not a derived one.
		if e.Prov == nil || e.Prov.Rule != "" || e.Prov.Scope == "" {
			continue
		}
		if sn := g.Node(e.S); sn == nil || sn.NodeType != model.NodeArtifact {
			continue
		}
		if e.Prov.Scope == "file" {
			fileLinks++
		} else {
			symbolLinks++
		}
	}
	imports := 0
	if ws.Config != nil {
		imports = len(ws.Config.Imports)
	}

	if concepts > 0 {
		o.Has[0] = fmt.Sprintf("%d concept(s)", concepts)
	} else {
		o.Missing[0] = "no concept of its own yet"
	}
	switch {
	case manifests > 0 && fileLinks+symbolLinks > 0:
		o.Has[1] = fmt.Sprintf("%d manifest(s), %d link(s) from code", manifests, fileLinks+symbolLinks)
	case manifests > 0:
		o.Missing[1] = "no @gloss link from code yet"
	case fileLinks+symbolLinks > 0:
		o.Missing[1] = "no .gloss.yaml manifest"
	default:
		o.Missing[1] = "no .gloss.yaml manifest and no @gloss link from code"
	}
	if declared > 0 {
		o.Has[2] = fmt.Sprintf("criticality declared on %d concept(s) or boundary(ies)", declared)
	} else {
		o.Missing[2] = "no criticality: declared"
	}
	var full []string
	if symbolLinks > 0 {
		full = append(full, fmt.Sprintf("%d marker(s) on declarations", symbolLinks))
	}
	if bound > 0 {
		full = append(full, fmt.Sprintf("%d value binding(s)", bound))
	}
	if imports > 0 {
		full = append(full, fmt.Sprintf("%d import(s)", imports))
	}
	if len(full) > 0 {
		o.Has[3] = strings.Join(full, ", ")
	} else {
		o.Missing[3] = "no marker on a declaration, no value-from binding, no imports"
	}
	o.Level = -1
	for i := range o.Has {
		if o.Has[i] == "" {
			break
		}
		o.Level = i
	}
	if o.Level < 0 {
		o.Level = 0
	}
	return o
}
