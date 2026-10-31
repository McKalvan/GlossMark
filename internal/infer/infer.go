// @gloss implements gmi:SpecInference scope=file

// Package infer is the edge between gloss suggest and the specs other
// stacks already write: AsyncAPI contracts, OpenAPI documents and, later,
// whatever an external tool can describe. A Source reads its files once
// per run and answers two questions in plain data: which items a spec
// file declares that a marker can anchor on (a channel, an operation, a
// schema), and which concepts a code declaration names through the spec
// (a topic, a route). It never decides a relation the code has to
// settle: which way a topic flows is code analysis, and the suggest
// runner does it for every source alike. Every shape here is a value
// with JSON tags, so a source outside the binary can return the same
// answers over a wire (#209, step B).
package infer

import "github.com/mckalvan/glossmark/internal/load"

// Concept is the read-only view of a vocabulary concept a source matches
// against.
type Concept struct {
	CURIE      string   `json:"curie"`
	Name       string   `json:"name"` // the local name, without the namespace
	Kind       string   `json:"kind,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	IsA        []string `json:"is-a,omitempty"`     // parent CURIEs
	Realizes   []string `json:"realizes,omitempty"` // core concepts an implementation concept realizes
	Impl       bool     `json:"impl,omitempty"`     // in an implementation tier
	Deprecated bool     `json:"deprecated,omitempty"`
}

// Vocabulary is what a source may read of the ontology: the concepts, an
// exact-name lookup (the suggest matcher's tokenization, so Appointments
// finds Appointment and an alias counts), and the nearest common is-a
// ancestor of a set, which is what a channel carrying several events
// stands for.
type Vocabulary interface {
	Concepts() []Concept
	// Named returns the concepts whose name or alias is exactly name, of
	// the given kinds (any kind when none is given), core tier first.
	Named(name string, kinds ...string) []Concept
	// CommonAncestor returns the CURIE every one of curies is-a (itself
	// included) that is nearest to them all, or "".
	CommonAncestor(curies []string) string
}

// Item is one anchorable thing a spec file declares: a marker on it means
// Relation to the concept the item stands for. The source names the
// concept when it can (Concept); else the runner matches Names and Doc
// against the vocabulary, restricted to Kinds.
type Item struct {
	Symbol   string `json:"symbol"`
	Role     string `json:"role"`     // channel | operation | schema: the calibration role
	Relation string `json:"relation"` // documents | represents
	Line     int    `json:"line"`     // 1-based line of the item's key
	End      int    `json:"end"`      // last line of its block
	Indent   string `json:"indent,omitempty"`
	Doc      string `json:"doc,omitempty"`
	Target
}

// Target is the concept an item or a hint points at: resolved, or left to
// the runner's matcher.
type Target struct {
	Concept string   `json:"concept,omitempty"` // CURIE, when the source resolved it
	Names   []string `json:"names,omitempty"`   // else names for the matcher (an operationId, a path's words)
	Kinds   []string `json:"kinds,omitempty"`   // concept kinds the target may have; an implementation concept that realizes one counts
	Basis   string   `json:"basis,omitempty"`   // the evidence, for the calibration bucket (messages, name, flow); "" keeps the matcher's kind
	Terms   string   `json:"terms,omitempty"`   // what matched, for the reviewer
}

// Decl is a code declaration a source is asked about.
type Decl struct {
	Path       string   `json:"path"`
	Dir        string   `json:"dir"` // its boundary's directory, "" at the root
	Symbol     string   `json:"symbol"`
	Role       string   `json:"role"` // func | member | type | ...
	Line       int      `json:"line"` // 1-based line of the declaration
	Decorators []string `json:"decorators,omitempty"`
	Body       string   `json:"body"` // the declaration's block, decorators included
}

// Hint is one thing a declaration names through a spec. A plain hint
// points at a target with a fixed or role-derived relation (a route the
// spec maps to a process: implements). A flow hint says the declaration
// names Channel, and Message on it when the code tells the channel's
// messages apart; the runner decides publishes or subscribes from the
// code, so a flow hint carries no relation.
type Hint struct {
	Target
	Relation string `json:"relation,omitempty"` // fixed relation; "" derives it from the role
	Channel  string `json:"channel,omitempty"`  // flow: the channel's address
	Message  string `json:"message,omitempty"`  // flow: the message named, if any
	Via      string `json:"via,omitempty"`      // the identifier or literal that named it
	Line     int    `json:"line,omitempty"`     // 1-based file line that named it, for the direction
}

// Source is one kind of spec.
type Source interface {
	// Name is the source id: Proposal.Source, gloss suggest --source, and
	// the namespace of its calibration bases (name/basis/role).
	Name() string
	// Index reads the source's files once per run.
	Index(ws *load.Workspace, v Vocabulary) error
	// Items lists what a spec file declares; nil for a file that is not
	// one of the source's specs.
	Items(path string) []Item
	// Hints lists what a code declaration names through the spec.
	Hints(d Decl) []Hint
}

// Builtin returns fresh instances of the sources shipped in the binary.
func Builtin() []Source {
	return []Source{&asyncAPI{}, &openAPI{}}
}
