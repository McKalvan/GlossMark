// @gloss implements gmi:MappingSets scope=file

package extract

import (
	"fmt"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/mapping"
	"github.com/mckalvan/glossmark/internal/model"
)

// FoundMapping is one row of a mapping set, resolved against the workspace.
type FoundMapping struct {
	ID  string // mapping:<subject>|<predicate>|<object>, workspace CURIEs
	Set *mapping.Set
	Row *mapping.Row
	// Subject and Object are IRIs; the CURIEs are the workspace's names for
	// them. Unresolved holds the endpoint CURIEs no prefix resolves.
	Subject, Object           string
	SubjectCURIE, ObjectCURIE string
	Unresolved                []string
	Predicate                 mapping.Predicate
	Supported                 bool
	Negative                  bool
	Edge                      *graph.Edge // nil for negative, unsupported or unresolved rows
}

// SetIRI is the graph id of a mapping set.
func SetIRI(s *mapping.Set) string {
	if strings.Contains(s.ID, "://") || strings.HasPrefix(s.ID, "urn:") {
		return s.ID
	}
	return "urn:gloss:mapping-set:" + s.Name
}

func (ex *extractor) mappings() {
	for _, s := range ex.ws.MappingSets {
		ex.g.AddNode(&graph.Node{NodeType: model.NodeMappingSet, ID: SetIRI(s), CURIE: s.Name, Label: s.Title, Kind: s.Scope,
			Version: s.Version, Owner: s.Owner, File: s.Path, Line: 1})
		for _, r := range s.Rows {
			ex.mapping(s, r)
		}
	}
}

func (ex *extractor) mapping(s *mapping.Set, r *mapping.Row) {
	res := s.Resolve(r, ex.ws.Prefixes)
	fm := FoundMapping{Set: s, Row: r, Negative: r.Negative(), Predicate: res.Predicate, Supported: res.Supported, Unresolved: res.Unresolved,
		Subject: res.Subject, Object: res.Object}
	if !res.Supported {
		ex.diag(s.Path, r.Line, "warning", "GM709", fmt.Sprintf("predicate %s is not one GlossMark interprets (skos:exactMatch, closeMatch, broadMatch, narrowMatch, relatedMatch, owl:equivalentClass, owl:sameAs); the row is kept and ignored", r.Get("predicate_id")))
	}
	for _, c := range res.Undeclared {
		ex.diag(s.Path, r.Line, "warning", "GM700", fmt.Sprintf("prefix of %s is not declared in the set's curie_map; GlossMark resolves it from glossmark.yaml, other SSSOM tools will reject the file (gloss map propose adds it)", c))
	}
	curie := func(iri, raw string) string {
		if iri == "" {
			return raw
		}
		return ex.g.Curie(iri)
	}
	fm.SubjectCURIE = curie(res.Subject, r.Get("subject_id"))
	fm.ObjectCURIE = curie(res.Object, r.Get("object_id"))
	fm.ID = mapping.ID(fm.SubjectCURIE, fm.Predicate.CURIE, fm.ObjectCURIE)
	if fm.Supported && !fm.Negative && len(fm.Unresolved) == 0 {
		prov := &graph.Prov{Path: s.Path, Line: r.Line, ID: fm.ID, Proposed: !r.Reviewed(), Note: r.Get("comment"), Raw: r.Get("predicate_id"), Set: s.Name}
		if a := r.Multi("author_id"); len(a) > 0 {
			prov.Actor = a[0]
		}
		if c := r.Confidence(); c >= 0 {
			prov.Conf = &c
		}
		e := &graph.Edge{S: fm.Subject, P: fm.Predicate.Relation, O: fm.Object, Source: graph.SourceMapping, Prov: prov}
		ex.g.AddEdge(e)
		fm.Edge = e
	}
	ex.res.Mappings = append(ex.res.Mappings, fm)
}
