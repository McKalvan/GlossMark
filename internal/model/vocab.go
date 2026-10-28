// @gloss represents gm:Relation scope=file

// Package model holds the GlossMark file formats (glossmark.yaml, *.gloss.yaml,
// .gloss.yaml manifests, views.yaml, the ledger) and the relation vocabulary.
package model

import "sort"

// NodeType names the five node types of the metamodel.
type NodeType string

const (
	NodeConcept   NodeType = "concept"
	NodeBoundary  NodeType = "boundary"
	NodeArtifact  NodeType = "artifact"
	NodeNamespace NodeType = "namespace"
	NodeActor     NodeType = "actor"
)

// Tiers.
const (
	TierCore = "core"
	TierImpl = "implementation"
)

// ConceptKinds is the closed set of concept kinds.
var ConceptKinds = []string{"entity", "process", "capability", "event", "measure", "constraint", "resource", "system-type"}

// BoundaryLevels in zoom order, coarsest first.
var BoundaryLevels = []string{"domain", "system", "container", "component"}

// ZoomLevels in order, coarsest first. "landscape" is above every boundary.
var ZoomLevels = []string{"landscape", "system", "container", "component", "code"}

// CriticalityLevels in increasing order. Empty means unclassified. See docs/GOVERNANCE.md.
var CriticalityLevels = []string{"low", "medium", "high", "critical"}

// CriticalityRank orders levels; unclassified is -1.
func CriticalityRank(level string) int {
	for i, l := range CriticalityLevels {
		if l == level {
			return i
		}
	}
	return -1
}

// ValidCriticality reports whether level is empty or one of CriticalityLevels.
func ValidCriticality(level string) bool { return level == "" || CriticalityRank(level) >= 0 }

// MaxCriticality returns the higher of two levels.
func MaxCriticality(a, b string) string {
	if CriticalityRank(b) > CriticalityRank(a) {
		return b
	}
	return a
}

// Scopes allowed on an inline marker.
var Scopes = []string{"file", "symbol", "block", "line", "range"}

// MarkerKeys allowed inline. Everything else lives in the ledger.
var MarkerKeys = []string{"scope", "conf", "id", "note"}

// Lift strategies: how gloss drift and views summarize a derived set.
// range-lcs, the default for produces, consumes, publishes and subscribes,
// shows a set by its least common subsumer; none shows the members. Neither
// changes coverage, which always subsumes (ADR 0011).
const (
	LiftRangeLCS = "range-lcs"
	LiftNone     = "none"
)

// Relation declares one relation type: who may be subject and object, and how
// it behaves. Every edge in the graph is checked against its Relation.
type Relation struct {
	Name       string     `json:"name" yaml:"name"`
	Domain     []NodeType `json:"domain" yaml:"domain"`
	Range      []NodeType `json:"range" yaml:"range"`
	Transitive bool       `json:"transitive,omitempty" yaml:"transitive,omitempty"`
	Inverse    string     `json:"inverse,omitempty" yaml:"inverse,omitempty"`
	Lift       string     `json:"lift,omitempty" yaml:"lift,omitempty"`
	// Inline reports whether the relation may appear in an inline marker.
	Inline bool `json:"inline,omitempty" yaml:"inline,omitempty"`
	// Ontology reports whether the relation may appear in an ontology file.
	Ontology bool `json:"ontology,omitempty" yaml:"ontology,omitempty"`
	// Manifest reports whether the relation may appear in a boundary manifest.
	Manifest bool `json:"manifest,omitempty" yaml:"manifest,omitempty"`
	// Derived reports whether the tooling also derives this relation.
	Derived bool `json:"derived,omitempty" yaml:"derived,omitempty"`
	// RangeKinds, when set, restricts the concept kinds a target may have
	// (a soft rule: violations are warnings).
	RangeKinds []string `json:"range-kinds,omitempty" yaml:"range-kinds,omitempty"`
	Doc        string   `json:"doc,omitempty" yaml:"doc,omitempty"`
	// Surfaces opts a custom concept-to-concept relation into the treatment
	// guards and constrains get: impact (gloss impact lists the code it
	// applies to) and context (gloss context --dir names it on an Applies
	// here: line). See docs/adr/0015-guards.md.
	Surfaces []string `json:"surfaces,omitempty" yaml:"surfaces,omitempty"`
}

// RelationSurfaces are the values surfaces: takes.
var RelationSurfaces = []string{"impact", "context"}

var (
	c    = []NodeType{NodeConcept}
	cb   = []NodeType{NodeConcept, NodeBoundary}
	a    = []NodeType{NodeArtifact}
	ab   = []NodeType{NodeArtifact, NodeBoundary}
	ac   = []NodeType{NodeArtifact, NodeConcept}
	acb  = []NodeType{NodeArtifact, NodeConcept, NodeBoundary}
	any_ = []NodeType{NodeConcept, NodeBoundary, NodeArtifact, NodeNamespace, NodeActor}
)

// BuiltinRelations is the relation vocabulary from the metamodel section of the
// design, plus the two relations the worked examples use (reverses, attaches-to).
var BuiltinRelations = []Relation{
	{Name: "is-a", Domain: c, Range: c, Ontology: true, Doc: "Taxonomic generalization. Stored as direct edges; the closure is computed."},
	{Name: "part-of", Domain: acb, Range: cb, Inverse: "has-part", Ontology: true, Manifest: true, Derived: true, Doc: "Partonomy. Concept→Concept, Boundary→Boundary, and Artifact→Boundary by containment."},
	{Name: "has-part", Domain: cb, Range: cb, Inverse: "part-of", Ontology: true, Doc: "Inverse of part-of."},
	{Name: "realizes", Domain: cb, Range: c, Ontology: true, Manifest: true, Derived: true, Doc: "impl Concept → core Concept; Boundary → Concept. Also derived by rollup."},
	{Name: "published-as", Domain: c, Range: c, Ontology: true, Doc: "impl Concept → core Concept under which it surfaces in the business view."},
	{Name: "replaced-by", Domain: cb, Range: cb, Ontology: true, Manifest: true, Doc: "A deprecated concept or boundary → its successor; written as the replaced-by: key, drawn in views. The successor may live in a namespace or repository this workspace does not import (ADR 0020)."},
	{Name: "implements", Domain: a, Range: c, Inline: true, RangeKinds: []string{"process", "capability", "system-type", "event", "constraint", "measure"}, Doc: "Artifact implements the behaviour of a concept."},
	{Name: "presents", Domain: acb, Range: c, Inline: true, Ontology: true, Manifest: true, Derived: true, Doc: "A UI surface for a concept (a page, a screen, a view): lifts to the boundary as presents, never as realizes, and carries no criticality. See docs/adr/0001-presents-and-hosts.md."},
	{Name: "hosts", Domain: acb, Range: c, Inline: true, Ontology: true, Manifest: true, Derived: true, Doc: "A shell or container for a concept (the app frame, a router, a plugin host): lifts as hosts, never as realizes, and carries no criticality."},
	{Name: "publishes-value", Domain: a, Range: c, Inline: true, RangeKinds: []string{"measure", "constraint"}, Doc: "The line below holds the concept's published value (scope=line): a change to it without a change to value: is a HARD published-value drift."},
	{Name: "represents", Domain: a, Range: c, Inline: true, RangeKinds: []string{"entity", "event", "measure", "resource", "constraint"}, Doc: "Artifact is the data shape of a concept."},
	{Name: "tests", Domain: a, Range: c, Inline: true, Derived: true, Doc: "Accepted alias of verifies (GM240 asks for verifies)."},
	{Name: "verifies", Domain: a, Range: c, Inline: true, Derived: true, Doc: "Test artifact verifies a concept: the one test relation."},
	{Name: "provisions", Domain: a, Range: cb, Inline: true, Doc: "IaC artifact provisions a resource or boundary. On a resource or system-type concept it lifts to the boundary as realizes."},
	{Name: "configures", Domain: ab, Range: cb, Inline: true, Manifest: true, Derived: true, Doc: "A config artifact, or a page that edits a value at runtime, configures a concept or boundary: lifts to the boundary as configures (and, on a resource or system-type concept, as realizes); it carries the concept's criticality and never places it. See docs/adr/0012-surfaces.md."},
	{Name: "deploys", Domain: a, Range: cb, Inline: true, Doc: "CI artifact deploys a system-type or boundary. On a resource or system-type concept it lifts to the boundary as realizes."},
	{Name: "holds", Domain: []NodeType{NodeBoundary}, Range: c, Manifest: true, Doc: "Boundary owns the data of a concept (an entity's records, a measure's stored values): declared in a manifest only; gloss drift checks that code under it represents it."},
	{Name: "documents", Domain: a, Range: cb, Inline: true, Doc: "Markdown documents a concept or boundary."},
	{Name: "instructs", Domain: a, Range: cb, Inline: true, Doc: "An agent file's instructions concern a concept."},
	{Name: "may-modify", Domain: a, Range: cb, Inline: true, Doc: "An agent's change remit. Targets may be globs (pa:*)."},
	{Name: "produces", Domain: acb, Range: c, Inline: true, Ontology: true, Manifest: true, Derived: true, Lift: LiftRangeLCS, Doc: "Intended (concept) or actual (artifact) production; lifted to boundaries."},
	{Name: "consumes", Domain: acb, Range: c, Inline: true, Ontology: true, Manifest: true, Derived: true, Lift: LiftRangeLCS},
	{Name: "publishes", Domain: acb, Range: c, Inline: true, Ontology: true, Manifest: true, Derived: true, Lift: LiftRangeLCS, RangeKinds: []string{"event"}},
	{Name: "subscribes", Domain: acb, Range: c, Inline: true, Ontology: true, Manifest: true, Derived: true, Lift: LiftRangeLCS, RangeKinds: []string{"event"}},
	{Name: "constrains", Domain: c, Range: c, Ontology: true, Doc: "A rule that applies to a concept. gloss impact and gloss context show it on both ends; it carries no criticality."},
	{Name: "guards", Domain: c, Range: c, Ontology: true, Doc: "A cross-cutting mechanism (authentication, a rate limit, a flag) applies to a concept and the code that implements it. gloss impact and gloss context show it on both ends; gloss diff reviews a change to guarded code at the guard's level without the code inheriting it. See docs/adr/0015-guards.md."},
	{Name: "guarded-by", Domain: []NodeType{NodeBoundary}, Range: c, Manifest: true, Doc: "Boundary → the mechanism every artifact under its directory sits behind: declared in a manifest as guarded-by:. gloss impact and gloss context treat the code under it as guarded by the concept, gloss diff reviews it at the guard's level, and gloss drift reports a handler there that lacks the guard's decorator when its siblings carry it. See docs/adr/0015-guards.md."},
	{Name: "applies-to", Domain: c, Range: c, Ontology: true, Doc: "Accepted alias of guards in ontology files, stored as guards."},
	{Name: "reverses", Domain: c, Range: c, Ontology: true},
	{Name: "attaches-to", Domain: c, Range: c, Ontology: true},
	{Name: "depends-on", Domain: ab, Range: ab, Derived: true, Doc: "From imports and SCIP; lifted to boundaries by the implied-relationship rule."},
	{Name: "calls", Domain: ab, Range: ab, Derived: true},
	{Name: "owns", Domain: []NodeType{NodeActor}, Range: []NodeType{NodeBoundary}, Manifest: true, Doc: "Imported from CODEOWNERS or Backstage spec.owner."},
	{Name: "derived-from", Domain: any_, Range: any_},
	{Name: "same-as", Domain: any_, Range: any_, Doc: "SSSOM equivalence."},
}

// Relation profiles: which part of the vocabulary a repository shows and
// lints by (glossmark.yaml relations:, issue #261). The starter profile is
// the handful a repository needs on day one; the full profile is every
// relation. An unset relations: means full, so a repository written before
// the switch existed reads as before; gloss init writes starter.
const (
	ProfileStarter = "starter"
	ProfileFull    = "full"
)

// RelationProfiles are the values relations: takes.
var RelationProfiles = []string{ProfileStarter, ProfileFull}

// StarterRelations are the relations the starter profile shows and writes
// without a hint: the five inline ones a repository writes first, part-of
// in manifests and ontology files, and is-a in ontology files.
var StarterRelations = []string{"implements", "represents", "verifies", "produces", "consumes", "part-of", "is-a"}

// StructuralRelations belong to no profile: the tier seam itself asks for
// the first three (GM320 wants realizes on an implementation concept, GM340
// wants replaced-by on a deprecated one), and the rest are written by the
// tool from CODEOWNERS, Backstage or an import, never chosen by hand, so no
// profile hints against them.
var StructuralRelations = []string{"realizes", "replaced-by", "published-as", "owns", "same-as", "derived-from"}

// InStarter reports whether a relation is written without a hint under the
// starter profile. tests is the accepted alias of verifies, so it is.
func InStarter(name string) bool {
	if name == "tests" {
		return true
	}
	for _, r := range StarterRelations {
		if r == name {
			return true
		}
	}
	for _, r := range StructuralRelations {
		if r == name {
			return true
		}
	}
	return false
}

// Vocabulary is the effective relation table: builtins plus ontology overrides.
type Vocabulary struct {
	rel map[string]*Relation
}

// NewVocabulary returns the builtin vocabulary.
func NewVocabulary() *Vocabulary {
	v := &Vocabulary{rel: map[string]*Relation{}}
	for i := range BuiltinRelations {
		r := BuiltinRelations[i]
		v.rel[r.Name] = &r
	}
	return v
}

// Override merges a relation declaration from an ontology file. Only the
// fields set in decl are applied; unknown names create a new concept→concept
// relation usable in ontology files.
func (v *Vocabulary) Override(name string, decl RelationDecl) {
	r, ok := v.rel[name]
	if !ok {
		r = &Relation{Name: name, Domain: c, Range: c, Ontology: true}
		v.rel[name] = r
	}
	if decl.Lift != "" {
		r.Lift = decl.Lift
	}
	if decl.Transitive != nil {
		r.Transitive = *decl.Transitive
	}
	if decl.Inverse != "" {
		r.Inverse = decl.Inverse
	}
	if len(decl.Domain) > 0 {
		r.Domain = decl.Domain
	}
	if len(decl.Range) > 0 {
		r.Range = decl.Range
	}
	if decl.Doc != "" {
		r.Doc = decl.Doc
	}
	if len(decl.Surfaces) > 0 {
		r.Surfaces = decl.Surfaces
	}
}

// Get returns the relation or nil.
func (v *Vocabulary) Get(name string) *Relation { return v.rel[name] }

// Names returns the relation names sorted.
func (v *Vocabulary) Names() []string {
	out := make([]string, 0, len(v.rel))
	for k := range v.rel {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// InlineNames returns the relations an inline marker may carry under a
// profile, sorted, without the tests alias (GM240 asks for verifies): the
// list the AGENTS.md snippet and the MCP server show.
func (v *Vocabulary) InlineNames(profile string) []string {
	var out []string
	for _, r := range v.Names() {
		rel := v.rel[r]
		if !rel.Inline || r == "tests" {
			continue
		}
		if profile == ProfileStarter && !InStarter(r) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Allows reports whether subject and object node types fit the relation.
func (r *Relation) Allows(s, o NodeType) bool {
	return contains(r.Domain, s) && contains(r.Range, o)
}

func contains(xs []NodeType, x NodeType) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// RelationDecl is how an ontology file declares or tunes a relation.
type RelationDecl struct {
	Lift       string     `yaml:"lift,omitempty" json:"lift,omitempty"`
	Transitive *bool      `yaml:"transitive,omitempty" json:"transitive,omitempty"`
	Inverse    string     `yaml:"inverse,omitempty" json:"inverse,omitempty"`
	Domain     []NodeType `yaml:"domain,omitempty" json:"domain,omitempty"`
	Range      []NodeType `yaml:"range,omitempty" json:"range,omitempty"`
	Doc        string     `yaml:"doc,omitempty" json:"doc,omitempty"`
	Surfaces   []string   `yaml:"surfaces,omitempty" json:"surfaces,omitempty"`
}
