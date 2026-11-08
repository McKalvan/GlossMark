// @gloss implements gmi:SourcePlugin scope=file

// Package wire is the infer.Source interface over a pipe: the contract an
// executable outside the binary speaks to be a link source for gloss
// suggest (#209, step B). One process runs per source and per suggest
// run; the core writes one JSON object per line on its stdin and reads
// one per line from its stdout. The core opens with "index" (the
// workspace's files and the vocabulary); the plugin may ask back for
// file content ("read") or the vocabulary's lookups ("named",
// "ancestor") and answers "indexed" (its name, the items of every spec
// file it read, the priors it declares). Then, for each code
// declaration, the core sends "hints" and the plugin answers "hints".
// Closing stdin ends the run. Every shape is the infer package's, so what
// a plugin returns is what a built-in source returns, and the runner
// does the rest: matching, conf, dedupe, anchoring, direction and the
// ledger. A plugin's output is proposals only, never accepted links.
package wire

import (
	"github.com/mckalvan/glossmark/internal/infer"
)

// Contract is the protocol version this binary speaks. A plugin that
// answers with another fails the command naming both (#209).
const Contract = 1

// Message types, in the Type field. The core sends TypeIndex, TypeFiles,
// TypeConcepts, TypeAncestor and TypeHints; the plugin sends TypeRead,
// TypeNamed, TypeAncestor, TypeIndexed, TypeHints and TypeError.
const (
	TypeIndex    = "index"    // core → plugin: contract, root, files, vocabulary
	TypeRead     = "read"     // plugin → core: paths, globs (workspace files whose content it wants)
	TypeFiles    = "files"    // core → plugin: the contents asked for
	TypeNamed    = "named"    // plugin → core: name, kinds (Vocabulary.Named)
	TypeConcepts = "concepts" // core → plugin: the concepts found
	TypeAncestor = "ancestor" // plugin → core: curies (Vocabulary.CommonAncestor); core → plugin: curie
	TypeIndexed  = "indexed"  // plugin → core: contract, name, items, priors
	TypeHints    = "hints"    // core → plugin: decl; plugin → core: hints
	TypeError    = "error"    // plugin → core: message, in place of indexed or hints
)

// Message is the one envelope both sides write, one per line. Which
// fields are set depends on Type; the rest are left out.
type Message struct {
	Type string `json:"type"`
	// Contract is set on index and indexed: the version each side speaks.
	Contract int `json:"contract,omitempty"`

	// index: the workspace. Root is absolute; Files are the scanned
	// files in workspace order, each with its boundary directory and
	// whether it is a test path. Vocabulary is every concept the plugin
	// may match against.
	Root       string          `json:"root,omitempty"`
	Files      []infer.File    `json:"files,omitempty"`
	Vocabulary []infer.Concept `json:"vocabulary,omitempty"`

	// read: workspace files the plugin wants the content of, by path or
	// by glob over the repo-relative path (** crosses directories). Only
	// files in the index's list are answered. files: their contents.
	Paths    []string `json:"paths,omitempty"`
	Globs    []string `json:"globs,omitempty"`
	Contents []File   `json:"contents,omitempty"`

	// named: an exact name or alias and the concept kinds allowed (none:
	// any). concepts: what Vocabulary.Named returns, core tier first.
	// ancestor: the CURIEs asked about, and in the answer the nearest
	// common is-a ancestor, or "".
	Name     string          `json:"name,omitempty"`
	Kinds    []string        `json:"kinds,omitempty"`
	Concepts []infer.Concept `json:"concepts,omitempty"`
	Curies   []string        `json:"curies,omitempty"`
	Curie    string          `json:"curie,omitempty"`

	// indexed: Name is the source's id (Proposal.Source, --source, the
	// namespace of its calibration bases); it must be the name the core
	// discovered the plugin under. Items are the anchorable items per
	// spec file, keyed by path. Priors are the precision the plugin
	// expects of its bases ("field/func", or "field" for every role),
	// as a hint to the reviewer: the core uses one only until the
	// repository measures the basis, and caps it below the auto-accept
	// threshold in force (ADR 0049, 0050).
	Items  map[string][]infer.Item `json:"items,omitempty"`
	Priors map[string]float64      `json:"priors,omitempty"`

	// hints: the declaration asked about (core → plugin) and what it
	// names through the spec (plugin → core).
	Decl  *infer.Decl  `json:"decl,omitempty"`
	Hints []infer.Hint `json:"hints,omitempty"`

	// error: what went wrong, in the plugin's words.
	Message string `json:"message,omitempty"`
}

// File is one workspace file's content, answered to a read.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Core sources a plugin may not take the name of: the runner's own
// evidence (ADR 0049).
var coreSources = map[string]bool{"code": true, "route": true, "topic": true, "schema": true}
