// @gloss implements gmi:Linter scope=file
// @gloss implements gmi:MappingSets scope=file

package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/mapping"
	"github.com/mckalvan/glossmark/internal/model"
)

// mappings checks the SSSOM mapping sets (ADR 0025 §7). GM700 (parse) and
// GM709 (uninterpreted predicate) come from the loader and the extractor.
//
//	GM701 an endpoint does not exist, is not a concept, or its prefix is unknown
//	GM702 a repository set row with no owned endpoint, or the owned endpoint as object
//	GM703 an endpoint is an unpublished concept of an imported implementation namespace
//	GM704 a pair asserted both as a mapping and as its negation, or twice
//	GM705 an exact match within one namespace: a duplicate, use replaced-by
//	GM706 an exact match between concepts of different kinds
//	GM707 a concept exactly matched to two concepts of one namespace
//	GM708 a stale mapping: an endpoint changed since it was reviewed
//	GM710 an endpoint is deprecated: re-point the mapping to its successor
//	GM711 an accepted mapping to a proposed concept
func (c *checker) mappings() {
	type key struct{ s, p, o string }
	type at struct {
		path string
		line int
	}
	pos := map[key]at{} // first positive row per pair (across sets)
	neg := map[key]bool{}
	exact := map[string]map[string][]string{} // concept → other namespace → counterparts
	exactAt := map[[2]string]at{}             // concept, counterpart → row
	for _, s := range c.ws.MappingSets {
		inSet := map[key][]int{} // lines of positive rows per pair in this set
		for _, r := range s.Rows {
			res := s.Resolve(r, c.ws.Prefixes)
			for _, u := range res.Unresolved {
				c.add(s.Path, r.Line, "error", "GM701", fmt.Sprintf("%s: unknown prefix; declare it in the set's curie_map or under prefixes: in glossmark.yaml", u))
			}
			if len(res.Unresolved) > 0 {
				continue
			}
			sc, oc := c.g.Curie(res.Subject), c.g.Curie(res.Object)
			sn, on := c.g.Node(res.Subject), c.g.Node(res.Object)
			c.endpoint(s, r, res.Subject, sn)
			c.endpoint(s, r, res.Object, on)
			sNS, oNS := c.prefixOf(sc), c.prefixOf(oc)
			if s.Scope != model.MappingScopeLandscape {
				switch {
				case !c.ws.Owned[sNS] && !c.ws.Owned[oNS]:
					c.add(s.Path, r.Line, "error", "GM702", fmt.Sprintf("%s %s %s: neither concept is in a namespace this repository owns; keep the mapping in a set with scope: landscape (a registry repository), or in a repository that owns one side", sc, res.Predicate.CURIE, oc))
				case !c.ws.Owned[sNS]:
					c.add(s.Path, r.Line, "warning", "GM702", fmt.Sprintf("%s %s %s: write the concept this repository owns (%s) as subject_id (swap the columns; broadMatch and narrowMatch swap too)", sc, res.Predicate.CURIE, oc, oc))
				}
			}
			if !res.Supported {
				continue
			}
			k := key{sc, res.Predicate.CURIE, oc}
			if r.Negative() {
				neg[k] = true
				continue
			}
			if _, ok := pos[k]; !ok {
				pos[k] = at{s.Path, r.Line}
			}
			inSet[k] = append(inSet[k], r.Line)
			if r.Reviewed() {
				for _, n := range []*graph.Node{sn, on} {
					if n != nil && n.Proposed {
						c.add(s.Path, r.Line, "error", "GM711", fmt.Sprintf("%s is a proposed concept: a mapping to it can only be proposed (no reviewer_id) until the concept is accepted", n.CURIE))
					}
				}
			}
			if res.Predicate.Relation == mapping.RelExact {
				if sNS == oNS {
					c.add(s.Path, r.Line, "warning", "GM705", fmt.Sprintf("%s exactly matches %s in the same namespace: a duplicate concept; deprecate one with replaced-by instead of mapping them", sc, oc))
				}
				if sn != nil && on != nil && sn.Kind != "" && on.Kind != "" && sn.Kind != on.Kind {
					c.add(s.Path, r.Line, "warning", "GM706", fmt.Sprintf("%s (%s) exactly matches %s (%s): an exact match between different kinds is usually a closeMatch or relatedMatch", sc, sn.Kind, oc, on.Kind))
				}
				add := func(a, b string) {
					if exact[a] == nil {
						exact[a] = map[string][]string{}
					}
					ns := c.prefixOf(b)
					exact[a][ns] = append(exact[a][ns], b)
					if _, ok := exactAt[[2]string{a, b}]; !ok {
						exactAt[[2]string{a, b}] = at{s.Path, r.Line}
					}
				}
				add(sc, oc)
				add(oc, sc)
			}
		}
		for k, lines := range inSet {
			if len(lines) > 1 {
				c.add(s.Path, lines[1], "warning", "GM704", fmt.Sprintf("%s %s %s is asserted %d times; keep one row", k.s, k.p, k.o, len(lines)))
			}
		}
	}
	for k, first := range pos {
		if neg[k] {
			c.add(first.path, first.line, "error", "GM704", fmt.Sprintf("%s %s %s is asserted and negated (predicate_modifier: Not); the negation stands, delete one row", k.s, k.p, k.o))
		}
	}
	concepts := make([]string, 0, len(exact))
	for k := range exact {
		concepts = append(concepts, k)
	}
	sort.Strings(concepts)
	for _, a := range concepts {
		for ns, bs := range exact[a] {
			bs = uniqStrings(bs)
			if len(bs) > 1 {
				second := exactAt[[2]string{a, bs[1]}]
				c.add(second.path, second.line, "warning", "GM707", fmt.Sprintf("%s exactly matches %d concepts of %s: (%s); an exact match is one to one, use closeMatch or broadMatch for the others", a, len(bs), ns, strings.Join(bs, ", ")))
			}
		}
	}
	for _, e := range c.g.Edges() {
		if e.Source != graph.SourceMapping || e.Prov == nil || e.Prov.Review != model.StatusStale {
			continue
		}
		c.add(e.Prov.Path, e.Prov.Line, "warning", "GM708", fmt.Sprintf("%s %s %s is stale: an endpoint's definition changed since the mapping was reviewed; re-review it with gloss map accept, or gloss map reject", c.g.Curie(e.S), e.Prov.Raw, c.g.Curie(e.O)))
	}
}

// endpoint checks one side of a row.
func (c *checker) endpoint(s *mapping.Set, r *mapping.Row, iri string, n *graph.Node) {
	curie := c.g.Curie(iri)
	ns := c.prefixOf(curie)
	if n == nil {
		if _, loaded := c.ws.Ontologies[ns]; loaded {
			c.add(s.Path, r.Line, "error", "GM701", fmt.Sprintf("%s does not exist in namespace %s:", curie, ns))
		}
		// A name under prefixes: only is not loaded here; gloss landscape judges it.
		return
	}
	if n.NodeType != model.NodeConcept {
		c.add(s.Path, r.Line, "error", "GM701", fmt.Sprintf("%s is a %s, not a concept; mappings relate concepts", curie, n.NodeType))
		return
	}
	if n.Imported && n.Tier == model.TierImpl && n.Published == "" {
		c.add(s.Path, r.Line, "error", "GM703", fmt.Sprintf("%s is an unpublished concept of imported namespace %s:; only published concepts cross repositories", curie, ns))
	}
	if n.Deprecated && !r.Negative() {
		succ := "its successor"
		if n.ReplacedBy != "" {
			succ = n.ReplacedBy
		}
		c.add(s.Path, r.Line, "warning", "GM710", fmt.Sprintf("%s is deprecated; re-point the mapping to %s (gloss map repoint %s %s %s)", curie, succ, r.Get("subject_id"), r.Get("predicate_id"), r.Get("object_id")))
	}
}

func uniqStrings(xs []string) []string {
	m := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !m[x] {
			m[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
