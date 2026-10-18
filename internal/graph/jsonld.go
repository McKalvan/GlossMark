// @gloss implements gmi:Publisher scope=file

package graph

import (
	"encoding/json"
	"sort"

	"github.com/mckalvan/glossmark/internal/model"
)

// VocabIRI is the base IRI of the GlossMark vocabulary (relations and node types).
const VocabIRI = "https://glossmark.dev/vocab#"

// JSONLDContext renders a JSON-LD context for a graph: every prefix plus every
// relation and record field, so graph.jsonl loads as RDF with a JSON-LD processor
// (each node line is a resource, each edge line a reified statement).
func JSONLDContext(g *Graph) string {
	ctx := map[string]any{
		"gm":   VocabIRI,
		"id":   "@id",
		"s":    map[string]any{"@id": "gm:subject", "@type": "@id"},
		"p":    map[string]any{"@id": "gm:predicate", "@type": "@vocab"},
		"o":    map[string]any{"@id": "gm:object", "@type": "@id"},
		"node": map[string]any{"@id": "gm:nodeType", "@type": "@vocab"},
		"kind": "gm:kind", "tier": "gm:tier", "ns": "gm:namespace", "curie": "gm:curie", "label": "http://www.w3.org/2000/01/rdf-schema#label",
		"doc": "http://www.w3.org/2000/01/rdf-schema#comment", "source": "gm:source", "prov": "gm:provenance", "path": "gm:path", "line": "gm:line",
		"published-as": map[string]any{"@id": "gm:publishedAs", "@type": "@id"},
		"replaced-by":  map[string]any{"@id": "http://purl.org/dc/terms/isReplacedBy", "@type": "@id"},
		"deprecated":   "http://www.w3.org/2002/07/owl#deprecated",
		"version":      "http://www.w3.org/2002/07/owl#versionInfo",
		"same-as":      map[string]any{"@id": "http://www.w3.org/2002/07/owl#sameAs", "@type": "@id"},
		"is-a":         map[string]any{"@id": "http://www.w3.org/2000/01/rdf-schema#subClassOf", "@type": "@id"},
	}
	prefixes := make([]string, 0, len(g.Prefixes))
	for p := range g.Prefixes {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	for _, p := range prefixes {
		ctx[p] = g.Prefixes[p]
	}
	for _, r := range model.BuiltinRelations {
		if _, ok := ctx[r.Name]; !ok {
			ctx[r.Name] = map[string]any{"@id": "gm:" + r.Name, "@type": "@id"}
		}
	}
	b, _ := json.MarshalIndent(map[string]any{"@context": ctx}, "", "  ")
	return string(b) + "\n"
}
