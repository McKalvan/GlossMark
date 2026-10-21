// @gloss implements gmi:MappingSets scope=file

package mapping

// Predicate describes a supported mapping predicate.
type Predicate struct {
	IRI      string
	CURIE    string // canonical CURIE
	Relation string // the graph relation it is stored as
	// Semantic predicates change what the landscape derives (ADR 0025 §5):
	// their proposals are HARD; the others are SOFT.
	Semantic bool
}

// Predicates are the mapping predicates GlossMark understands, keyed by IRI.
var Predicates = map[string]Predicate{
	Builtin["skos"] + "exactMatch":     {Builtin["skos"] + "exactMatch", "skos:exactMatch", RelExact, true},
	Builtin["owl"] + "equivalentClass": {Builtin["owl"] + "equivalentClass", "owl:equivalentClass", RelExact, true},
	Builtin["owl"] + "sameAs":          {Builtin["owl"] + "sameAs", "owl:sameAs", RelExact, true},
	Builtin["skos"] + "broadMatch":     {Builtin["skos"] + "broadMatch", "skos:broadMatch", RelBroad, true},
	Builtin["skos"] + "narrowMatch":    {Builtin["skos"] + "narrowMatch", "skos:narrowMatch", RelNarrow, true},
	Builtin["skos"] + "closeMatch":     {Builtin["skos"] + "closeMatch", "skos:closeMatch", RelClose, false},
	Builtin["skos"] + "relatedMatch":   {Builtin["skos"] + "relatedMatch", "skos:relatedMatch", RelRelated, false},
}

// Graph relations mapping edges are stored as.
const (
	RelExact   = "same-as"
	RelBroad   = "broad-match"
	RelNarrow  = "narrow-match"
	RelClose   = "close-match"
	RelRelated = "related-match"
)

// IsMappingRelation reports whether a graph relation comes from a mapping.
func IsMappingRelation(rel string) bool {
	switch rel {
	case RelExact, RelBroad, RelNarrow, RelClose, RelRelated:
		return true
	}
	return false
}

// SemanticRelation reports whether a mapping relation changes landscape derivation.
func SemanticRelation(rel string) bool {
	switch rel {
	case RelExact, RelBroad, RelNarrow:
		return true
	}
	return false
}

// ByCURIE returns the predicate for a canonical CURIE such as skos:exactMatch.
func ByCURIE(c string) (Predicate, bool) {
	for _, p := range Predicates {
		if p.CURIE == c {
			return p, true
		}
	}
	return Predicate{}, false
}

// ShortNames lets the CLI accept exact, close, broad, narrow, related.
var ShortNames = map[string]string{
	"exact": "skos:exactMatch", "exactMatch": "skos:exactMatch", "equivalent": "owl:equivalentClass", "equivalentClass": "owl:equivalentClass",
	"close": "skos:closeMatch", "closeMatch": "skos:closeMatch", "broad": "skos:broadMatch", "broadMatch": "skos:broadMatch",
	"narrow": "skos:narrowMatch", "narrowMatch": "skos:narrowMatch", "related": "skos:relatedMatch", "relatedMatch": "skos:relatedMatch",
}

// Justifications maps CLI shorthands to semapv terms.
var Justifications = map[string]string{
	"manual": "semapv:ManualMappingCuration", "lexical": "semapv:LexicalMatching", "llm": "semapv:LLMBasedMatching",
	"agent": "semapv:AgentBasedMatching", "review": "semapv:MappingReview", "unspecified": "semapv:UnspecifiedMatching",
	"logical": "semapv:LogicalReasoning", "structural": "semapv:StructuralMatching",
}
