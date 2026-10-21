// @gloss implements gmi:Gate scope=file
// @gloss implements gmi:MappingSets scope=file

package gate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/mapping"
	"github.com/mckalvan/glossmark/internal/model"
)

// ClassifyMapping gates one mapping edge of g against policy (ADR 0025 §5):
// exact, broad and narrow matches change what the landscape derives and are
// HARD; close and related matches are SOFT. A proposed or deprecated
// endpoint, an agent proposal under the confidence threshold, and the
// criticality of either endpoint raise it.
func ClassifyMapping(g *graph.Graph, ws *load.Workspace, e *graph.Edge, pol *model.Policy) (Class, []string) {
	gate := SOFT
	var reasons []string
	if mapping.SemanticRelation(e.P) {
		gate = HARD
		reasons = append(reasons, "changes what the landscape derives (exact, broad or narrow match)")
	}
	for _, id := range []string{e.S, e.O} {
		n := g.Node(id)
		if n == nil {
			continue
		}
		if n.Proposed {
			gate = HARD
			reasons = append(reasons, fmt.Sprintf("endpoint %s is a proposed concept awaiting acceptance", n.CURIE))
		}
		if n.Deprecated {
			gate = HARD
			reasons = append(reasons, fmt.Sprintf("endpoint %s is deprecated", n.CURIE))
		}
	}
	if p := e.Prov; p != nil && p.Proposed {
		if p.Conf != nil && *p.Conf < pol.SoftConfThreshold {
			gate = HARD
			reasons = append(reasons, fmt.Sprintf("conf %.2f below threshold %.2f", *p.Conf, pol.SoftConfThreshold))
		} else if p.Conf == nil && strings.HasPrefix(p.Actor, "agent:") {
			gate = HARD
			reasons = append(reasons, "agent proposal without confidence")
		}
	}
	if lvl, from := MappingCriticality(g, e); lvl != "" {
		if want := Class(pol.GateForCriticality(lvl)); want != "" {
			gate = Max(gate, want)
			reasons = append(reasons, fmt.Sprintf("criticality %s: %s", lvl, from))
		}
	}
	if gate == SOFT {
		reasons = append(reasons, "informational match (search, viewer, briefing); auto-accept eligible")
	}
	return gate, reasons
}

// MappingCriticality is the higher governance level of a mapping's endpoints.
func MappingCriticality(g *graph.Graph, e *graph.Edge) (lvl, from string) {
	for _, id := range []string{e.S, e.O} {
		if n := g.Node(id); n != nil && model.CriticalityRank(n.Criticality) > model.CriticalityRank(lvl) {
			lvl, from = n.Criticality, n.CURIE
		}
	}
	return lvl, from
}

func mappingEdges(g *graph.Graph) map[string]*graph.Edge {
	out := map[string]*graph.Edge{}
	for _, e := range g.Edges() {
		if e.Source == graph.SourceMapping && e.Prov != nil && e.Prov.ID != "" {
			out[e.Prov.ID] = e
		}
	}
	return out
}

func (c *classifier) mappingOwner(set string) string {
	for _, n := range c.head.NodesOf(model.NodeMappingSet) {
		if n.CURIE == set && n.Owner != "" {
			return n.Owner
		}
	}
	return ""
}

// mappings reports mapping changes (ADR 0025 §4-5, §9): proposals and
// additions, acceptances, rejections, removals, mappings whose endpoint's
// definition changes in this diff (they go stale), endpoints the diff removes,
// and the mapping-set version bump the decisions require.
func (c *classifier) mappings() {
	be, he := mappingEdges(c.base), mappingEdges(c.head)
	ids := make([]string, 0, len(he)+len(be))
	for id := range he {
		ids = append(ids, id)
	}
	for id := range be {
		if he[id] == nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	bumps := map[string]string{} // set → required bump
	need := func(set, bump string) {
		if rankBump(bump) > rankBump(bumps[set]) {
			bumps[set] = bump
		}
	}
	for _, id := range ids {
		b, h := be[id], he[id]
		switch {
		case h != nil && b == nil:
			gate, why := ClassifyMapping(c.head, c.ws, h, c.opt.Policy)
			kind, glyph := "mapping-added", "+"
			if h.Prov.Proposed {
				kind, glyph = "mapping-proposed", "?"
			} else {
				need(h.Prov.Set, "MINOR")
			}
			conf := 0.0
			if h.Prov.Conf != nil {
				conf = *h.Prov.Conf
			}
			lvl, from := MappingCriticality(c.head, h)
			c.add(Change{Gate: gate, Kind: kind, Glyph: glyph, Relation: h.Prov.Raw, Subject: c.head.Curie(h.S), Object: c.head.Curie(h.O), Path: h.Prov.Path, Line: h.Prov.Line,
				Proposed: h.Prov.Proposed, Conf: conf, Actor: h.Prov.Actor, Reasons: why, MarkerID: id, Criticality: lvl, CriticalityFrom: from,
				Msg: fmt.Sprintf("mapping %s %s %s (set %s)", c.head.Curie(h.S), h.Prov.Raw, c.head.Curie(h.O), h.Prov.Set)})
		case h != nil && b != nil:
			if b.Prov.Proposed && !h.Prov.Proposed {
				need(h.Prov.Set, "MINOR")
				c.add(Change{Gate: AUTO, Kind: "mapping-accepted", Glyph: "+", Relation: h.Prov.Raw, Subject: c.head.Curie(h.S), Object: c.head.Curie(h.O), Path: h.Prov.Path, Line: h.Prov.Line, MarkerID: id,
					Reasons: []string{"proposal reviewed (reviewer_id set)"}, Msg: fmt.Sprintf("mapping %s %s %s accepted", c.head.Curie(h.S), h.Prov.Raw, c.head.Curie(h.O))})
			}
			if !h.Prov.Proposed {
				c.mappingAffected(id, h)
			}
		case b != nil:
			rejected := false
			if le, ok := c.opt.HeadLedger[id]; ok && le.Status == model.StatusRejected {
				rejected = true
			}
			subject, object := c.base.Curie(b.S), c.base.Curie(b.O)
			switch {
			case rejected:
				if !b.Prov.Proposed {
					need(b.Prov.Set, "MAJOR")
				}
				c.add(Change{Gate: AUTO, Kind: "mapping-rejected", Glyph: "-", Relation: b.Prov.Raw, Subject: subject, Object: object, Path: b.Prov.Path, MarkerID: id,
					Reasons: []string{"reviewed negation (predicate_modifier: Not)"}, Msg: fmt.Sprintf("mapping %s %s %s rejected", subject, b.Prov.Raw, object)})
			default:
				gate, why := SOFT, []string{"mapping removed"}
				if !b.Prov.Proposed {
					need(b.Prov.Set, "MAJOR")
					if mapping.SemanticRelation(b.P) {
						gate, why = HARD, []string{"an accepted exact, broad or narrow match removed: the landscape changes"}
					}
				}
				c.add(Change{Gate: gate, Kind: "mapping-removed", Glyph: "-", Relation: b.Prov.Raw, Subject: subject, Object: object, Path: b.Prov.Path, MarkerID: id,
					Reasons: why, Msg: fmt.Sprintf("mapping %s %s %s removed", subject, b.Prov.Raw, object)})
			}
		}
	}
	c.mappingVersions(bumps)
}

// mappingAffected reports an accepted mapping whose endpoint this diff
// changes: its meaning moved, and the ledger marks it stale until reviewed.
func (c *classifier) mappingAffected(id string, h *graph.Edge) {
	for _, end := range []string{h.S, h.O} {
		bn, hn := c.base.Node(end), c.head.Node(end)
		owner := c.mappingOwner(h.Prov.Set)
		who := "set " + h.Prov.Set
		if owner != "" {
			who += ", owner " + owner
		}
		mapped := fmt.Sprintf("%s %s %s", c.head.Curie(h.S), h.Prov.Raw, c.head.Curie(h.O))
		switch {
		case bn != nil && hn == nil:
			c.add(Change{Gate: HARD, Kind: "mapping-dangling", Glyph: "!", Relation: h.Prov.Raw, Subject: c.base.Curie(end), Path: h.Prov.Path, Line: h.Prov.Line, MarkerID: id,
				Reasons: []string{"mapped concept removed"}, Msg: fmt.Sprintf("%s removed: mapping %s dangles (%s)", c.base.Curie(end), mapped, who)})
		case bn != nil && hn != nil && bn.NodeType == model.NodeConcept:
			facts := mapping.ChangedFacts(mapping.Facts(c.base, end), mapping.Facts(c.head, end))
			if len(facts) == 0 {
				continue
			}
			gate, why := SOFT, []string{"mapped concept changed; the mapping is stale until re-reviewed"}
			lvl, from := MappingCriticality(c.head, h)
			if mapping.SemanticRelation(h.P) && lvl != "" {
				if want := Class(c.opt.Policy.GateForCriticality(lvl)); want == HARD {
					gate = HARD
					why = append(why, fmt.Sprintf("criticality %s: %s", lvl, from))
				}
			}
			msg := fmt.Sprintf("%s %s changed: mapping %s goes stale until re-reviewed (%s)", hn.CURIE, strings.Join(facts, ", "), mapped, who)
			if hn.Deprecated && hn.ReplacedBy != "" {
				msg += fmt.Sprintf("; re-point it to %s with gloss map repoint", hn.ReplacedBy)
			}
			c.add(Change{Gate: gate, Kind: "mapping-affected", Glyph: "~", Relation: h.Prov.Raw, Subject: hn.CURIE, Object: c.head.Curie(otherEnd(h, end)), Path: h.Prov.Path, Line: h.Prov.Line,
				MarkerID: id, Reasons: why, Criticality: lvl, CriticalityFrom: from, Msg: msg})
		}
	}
}

func otherEnd(e *graph.Edge, end string) string {
	if end == e.S {
		return e.O
	}
	return e.S
}

// mappingVersions checks each changed set's mapping_set_version: an accepted
// mapping added is MINOR, one removed or rejected is MAJOR. Proposals need no
// bump: nothing relies on them yet.
func (c *classifier) mappingVersions(bumps map[string]string) {
	names := make([]string, 0, len(bumps))
	for n := range bumps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		var b, h *graph.Node
		for _, n := range c.head.NodesOf(model.NodeMappingSet) {
			if n.CURIE == name {
				h = n
			}
		}
		for _, n := range c.base.NodesOf(model.NodeMappingSet) {
			if n.CURIE == name {
				b = n
			}
		}
		if b == nil || h == nil {
			continue
		}
		required, actual := bumps[name], bumpOf(b.Version, h.Version)
		if rankBump(actual) >= rankBump(required) {
			continue
		}
		c.add(Change{Gate: HARD, Kind: "version-bump", Glyph: "!", Subject: "mapping-set:" + name, Path: h.File, Bump: required,
			Reasons: []string{"semver: bump mapping_set_version; importers of the set rely on it"},
			Msg:     fmt.Sprintf("mapping set %s: changes require a %s bump but mapping_set_version went %s → %s (%s)", name, required, orNone(b.Version), orNone(h.Version), actual)})
	}
}
