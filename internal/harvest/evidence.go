// @gloss implements gmi:EvidenceHarvester scope=file

// Package harvest collects evidence about a repository's domain vocabulary
// before it has an ontology (ADR 0025): every observation of a term in code,
// schemas, tests, routes, error messages, UI copy, docs and git history,
// with where it was seen. Harvesting is deterministic; deciding which
// observations make a concept is left to the derive-ontology skill.
package harvest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Families group sources by how they distort the domain (ADR 0025, section 2).
const (
	FamilyCode         = "code"          // types, functions, modules
	FamilyData         = "data"          // schemas, migrations, tables
	FamilyBehaviour    = "behaviour"     // tests, validators, guards
	FamilySurface      = "surface"       // UI copy, routes: what users see
	FamilyDoc          = "doc"           // docs in the repository
	FamilyHistory      = "history"       // git renames
	FamilyVocabulary   = "vocabulary"    // a glossary or enum file the owner keeps
	FamilyReference    = "reference"     // a reference ontology the owner supplies
	FamilyDocConnector = "doc-connector" // docs outside the repository, read by the skill
	FamilyTracker      = "tracker"       // issue trackers, read by the skill
)

// signalFamily is the closed list of signals and the family each belongs to.
var signalFamily = map[string]string{
	"type-decl": FamilyCode, "field": FamilyCode, "function-decl": FamilyCode, "inherits": FamilyCode, "enum-type": FamilyCode, "enum-value": FamilyCode, "module": FamilyCode, "transition": FamilyCode, "value-set": FamilyCode, "topic": FamilyCode,
	"table": FamilyData, "column": FamilyData, "foreign-key": FamilyData, "schema-type": FamilyData,
	"test-name": FamilyBehaviour, "guard-message": FamilyBehaviour,
	"route": FamilySurface, "ui-string": FamilySurface,
	"doc-heading": FamilyDoc, "doc-definition": FamilyDoc, "doc-phrase": FamilyDoc, "rule-sentence": FamilyDoc,
	"rename":         FamilyHistory,
	"glossary-entry": FamilyVocabulary,
	"reference-term": FamilyReference,
	"issue-title":    FamilyTracker, "acceptance-criterion": FamilyTracker,
}

// Signals lists the closed signal vocabulary.
func Signals() []string {
	out := make([]string, 0, len(signalFamily))
	for s := range signalFamily {
		out = append(out, s)
	}
	return out
}

// FamilyOf returns the family a signal belongs to, or "".
func FamilyOf(signal string) string { return signalFamily[signal] }

// Evidence is one observation of a term. Quotes are never recorded: a record
// keeps a locator, and for sources outside the repository a hash.
type Evidence struct {
	ID      string   `json:"id"`
	Unit    string   `json:"unit,omitempty"`
	Signal  string   `json:"signal"`
	Surface string   `json:"surface"`
	Source  Source   `json:"source"`
	Context []string `json:"context,omitempty"`
	// FirstSeen and LastSeen date the surface from git history
	// (YYYY-MM-DD): when it first appeared, and when it was last seen (the
	// HEAD commit for the current tree, the last commit naming it for a
	// retired term). Empty outside git or with --no-git.
	FirstSeen string `json:"first_seen,omitempty"`
	LastSeen  string `json:"last_seen,omitempty"`
}

// Source says where an observation was made: a path and line in the
// repository, or a URI and locator outside it.
type Source struct {
	Family   string `json:"family"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Rev      string `json:"rev,omitempty"`
	URI      string `json:"uri,omitempty"`
	Locator  string `json:"locator,omitempty"`
	Modified string `json:"modified,omitempty"`
	Hash     string `json:"hash,omitempty"`
}

// newEvidence fills the family from the signal and derives a stable id from
// what was seen and where.
func newEvidence(unit, signal, surface, path string, line int, context ...string) Evidence {
	e := Evidence{Unit: unit, Signal: signal, Surface: surface, Source: Source{Family: signalFamily[signal], Path: path, Line: line}, Context: context}
	e.ID = idFor(e)
	return e
}

func idFor(e Evidence) string {
	loc := e.Source.Path
	if loc == "" {
		loc = e.Source.URI + "#" + e.Source.Locator
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%s", e.Signal, e.Surface, loc, e.Source.Line, e.Source.Rev)))
	return "ev:" + hex.EncodeToString(h[:])[:12]
}

// Validate checks a record produced outside the harvester (by a connector
// or tracker reader) before it is merged: a known signal whose family
// matches, a surface, and a location. A missing id is derived.
func (e *Evidence) Validate() error {
	fam, ok := signalFamily[e.Signal]
	if !ok {
		return fmt.Errorf("unknown signal %q", e.Signal)
	}
	if e.Source.Family == "" {
		e.Source.Family = fam
	}
	if e.Source.Family != fam {
		// A heading read from a connector is a doc-heading in the
		// doc-connector family; only the in-repository families are fixed.
		if !(e.Source.Family == FamilyDocConnector && fam == FamilyDoc) {
			return fmt.Errorf("signal %q belongs to family %s, not %s", e.Signal, fam, e.Source.Family)
		}
	}
	if strings.TrimSpace(e.Surface) == "" {
		return fmt.Errorf("signal %q: empty surface", e.Signal)
	}
	if e.Source.Path == "" && e.Source.URI == "" {
		return fmt.Errorf("signal %q %q: no path or uri", e.Signal, e.Surface)
	}
	if e.ID == "" {
		e.ID = idFor(*e)
	}
	return nil
}
