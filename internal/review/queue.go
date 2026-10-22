// @gloss implements gmi:ReviewEngine scope=file

package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// findingKinds are the drift findings the queue lists. Stale links and
// proposed concepts come from the ledger instead, overlap is information
// (ADR 0019) and consistent pairs need nothing.
var findingKinds = map[string]bool{
	rollup.KindViolation: true, rollup.KindGap: true, rollup.KindRogue: true, rollup.KindOrphan: true,
	rollup.KindUntested: true, rollup.KindPresentedOnly: true, rollup.KindDangling: true, rollup.KindOverdue: true,
	rollup.KindOutOfRange: true, rollup.KindDeprecatedUse: true, rollup.KindValueDrift: false,
}

func buildQueue(st *State) []Item {
	pol := st.WS.Config.EffectivePolicy()
	byID := map[string]extract.FoundMarker{}
	for _, fm := range st.Markers {
		if _, ok := byID[fm.ID]; !ok {
			byID[fm.ID] = fm
		}
	}
	var items []Item
	ids := make([]string, 0, len(st.Ledger.Entries))
	for id := range st.Ledger.Entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		le := st.Ledger.Entries[id]
		switch {
		case strings.HasPrefix(id, "concept:") && le.Status == model.StatusProposed:
			items = append(items, conceptItem(st, le))
		case strings.HasPrefix(id, "value:") && le.Status == model.StatusDrifted:
			items = append(items, valueItem(st, le))
		case le.Status == model.StatusProposed || le.Status == model.StatusStale:
			fm, ok := byID[id]
			if !ok {
				continue // the marker is gone; the next build records it removed
			}
			items = append(items, linkItem(st, pol, le, fm))
		}
	}
	for _, f := range st.Report.Findings {
		if findingKinds[f.Kind] && !f.Waived {
			items = append(items, findingItem(f))
		}
	}
	rank := map[string]int{}
	for i, k := range Kinds {
		rank[k] = i
	}
	sort.SliceStable(items, func(i, j int) bool { return rank[items[i].Kind] < rank[items[j].Kind] })
	return items
}

// linkGate classifies a marker's links the way gloss diff does: the
// strictest of its edges, never below SOFT, with the reasons for HARD.
func linkGate(st *State, pol *model.Policy, fm extract.FoundMarker) (gate.Class, []string) {
	cls := gate.SOFT
	var why []string
	seen := map[string]bool{}
	for _, ed := range fm.Edges {
		c, r := gate.ClassifyLink(st.Graph, st.WS, ed, pol)
		cls = gate.Max(cls, c)
		if c != gate.HARD {
			continue
		}
		for _, x := range r {
			if !seen[x] {
				seen[x] = true
				why = append(why, x)
			}
		}
	}
	return cls, why
}

func action(name string, cls gate.Class, reasons []string, params ...string) Action {
	return Action{Name: name, Gate: string(cls), Reasons: reasons, AgentMay: cls != gate.HARD, Params: params}
}

func linkItem(st *State, pol *model.Policy, le *model.LedgerEntry, fm extract.FoundMarker) Item {
	cls, why := linkGate(st, pol, fm)
	it := Item{
		ID: le.MarkerID, Path: fm.Path, Line: fm.Marker.Line, Relation: fm.Marker.Relation, Targets: fm.Marker.Targets,
		Actor: le.Actor, Conf: le.Conf, Since: firstNonEmpty(le.ReviewedAt, le.Created),
		Basis: basis(fm.Anchor.Fingerprint, fm.Marker.Raw),
	}
	if m := st.WS.ManifestFor(fm.Path); m != nil {
		it.Boundary = m.Boundary
	}
	what := fmt.Sprintf("%s:%d %s %s", fm.Path, fm.Marker.Line, fm.Marker.Relation, strings.Join(fm.Marker.Targets, ", "))
	if le.Status == model.StatusStale {
		it.Kind = KindStaleLink
		it.Summary = what + " (stale: the anchor changed since it was accepted)"
		it.Actions = []Action{action(ActReconfirm, gate.AUTO, nil), action(ActReject, cls, why, "reason", "remove")}
	} else {
		it.Kind = KindProposedLink
		it.Summary = what + " (proposed)"
		it.Actions = []Action{action(ActAccept, cls, why), action(ActReject, cls, why, "reason", "remove")}
	}
	it.Gate = it.Actions[0].Gate
	return it
}

func conceptItem(st *State, le *model.LedgerEntry) Item {
	curie := strings.TrimPrefix(le.MarkerID, "concept:")
	it := Item{ID: le.MarkerID, Kind: KindProposedConcept, Concept: curie, Path: le.Path, Actor: le.Actor, Since: le.Created}
	doc := le.Rationale
	if n := st.Graph.NodeByCurie(curie); n != nil {
		it.Path, it.Line, doc = n.File, n.Line, n.Doc
		it.Basis = basis(n.Kind, n.Doc)
	}
	it.Summary = fmt.Sprintf("%s proposed by %s: %s", curie, firstNonEmpty(le.Actor, "?"), firstNonEmpty(doc, "(no doc)"))
	why := []string{"a new concept is a human decision"}
	it.Actions = []Action{action(ActAccept, gate.HARD, why), action(ActReject, gate.HARD, why, "reason", "remove")}
	it.Gate = string(gate.HARD)
	return it
}

func valueItem(st *State, le *model.LedgerEntry) Item {
	curie := strings.TrimPrefix(le.MarkerID, "value:")
	it := Item{ID: le.MarkerID, Kind: KindValueDrift, Concept: curie, Since: firstNonEmpty(le.ReviewedAt, le.Created)}
	it.Summary = fmt.Sprintf("%s: the bound code changed and the published value did not", curie)
	if n := st.Graph.NodeByCurie(curie); n != nil {
		file, line, _ := strings.Cut(n.ValueFrom, ":")
		it.Path = file
		fmt.Sscanf(line, "%d", &it.Line)
		it.Summary = fmt.Sprintf("%s = %v, bound to %s, which now reads %q", curie, n.Value, n.ValueFrom, n.LiveValue)
		it.Basis = basis(n.ValueFP, fmt.Sprint(n.Value))
	}
	why := []string{"a published value is part of the seam"}
	it.Actions = []Action{action(ActConfirm, gate.HARD, why, "force")}
	it.Gate = string(gate.HARD)
	return it
}

func findingItem(f rollup.Finding) Item {
	it := Item{
		ID: "finding:" + f.ID, Kind: KindFinding, Finding: f.Kind, Severity: f.Severity, Path: f.Path, Line: f.Line,
		Relation: f.Relation, Concept: f.Concept, Boundary: f.Boundary, Summary: f.Kind + ": " + f.Msg, Basis: basis(f.Msg),
	}
	why := []string{"a waiver edits glossmark.yaml"}
	if f.Kind == rollup.KindRogue {
		// Proposing the concept answers the decision request with a
		// proposal, which a human then accepts or rejects.
		it.Actions = append(it.Actions, action(ActPropose, gate.SOFT, nil, "kind", "doc"))
	}
	it.Actions = append(it.Actions, action(ActWaive, gate.HARD, why, "owner", "expires", "reason"))
	it.Gate = it.Actions[0].Gate
	return it
}

func basis(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:8])
}

// editDocID is the item id of a recommended doc edit.
func editDocID(curie string) string { return "edit:doc:" + curie }

func docBasis(st *State, curie string) string {
	if n := st.Graph.NodeByCurie(curie); n != nil {
		return basis(n.Doc)
	}
	return ""
}

// recommendationItem is a recommendation without an item of its own: a
// recommended doc edit, which a person applies or dismisses, or one whose
// item was decided or went away, which only dismiss clears.
func recommendationItem(st *State, r Recommendation) Item {
	it := Item{ID: r.Item, Kind: KindRecommendation, Actor: r.By, Since: r.At}
	rc := r
	it.Recommendation = &rc
	d := r.Decision
	if d.Action == ActEditDoc && r.Item == editDocID(d.Curie) {
		it.Concept = d.Curie
		it.Basis = docBasis(st, d.Curie)
		rc.Outdated = rc.Basis != it.Basis
		if n := st.Graph.NodeByCurie(d.Curie); n != nil {
			it.Path, it.Line = n.File, n.Line
		}
		it.Summary = fmt.Sprintf("%s recommends a doc for %s: %q", r.By, d.Curie, d.Doc)
		it.Actions = []Action{action(ActApply, gate.HARD, []string{"a concept's definition is a human decision"}), action(ActDismiss, gate.AUTO, nil)}
	} else {
		it.Summary = fmt.Sprintf("%s recommended %s on %s, which no longer waits on review", r.By, d.Action, r.Item)
		it.Actions = []Action{action(ActDismiss, gate.AUTO, nil)}
	}
	it.Gate = it.Actions[0].Gate
	return it
}

// Filter keeps the items of the given kinds (an item kind or a finding kind)
// under the given files or directories; empty lists keep everything.
func Filter(items []Item, kinds, paths []string) []Item {
	var out []Item
	for _, it := range items {
		if len(kinds) > 0 && !contains(kinds, it.Kind) && !contains(kinds, it.Finding) {
			continue
		}
		if len(paths) > 0 && (it.Path == "" || !under(it.Path, paths)) {
			continue
		}
		out = append(out, it)
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if x != "" && y == x {
			return true
		}
	}
	return false
}

func under(path string, dirs []string) bool {
	for _, d := range dirs {
		d = strings.Trim(strings.TrimPrefix(d, "./"), "/")
		if d == "" || d == "." || path == d || strings.HasPrefix(path, d+"/") {
			return true
		}
	}
	return false
}
