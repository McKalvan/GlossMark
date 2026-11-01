// @gloss represents gmi:FileFormats scope=file
// @gloss represents gm:Namespace, gm:Boundary scope=file

package model

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// StringList accepts a scalar or a sequence in YAML.
type StringList []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *StringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		var v string
		if err := n.Decode(&v); err != nil {
			return err
		}
		if v == "" {
			*s = nil
			return nil
		}
		*s = StringList{v}
		return nil
	case yaml.SequenceNode:
		var v []string
		if err := n.Decode(&v); err != nil {
			return err
		}
		*s = StringList(v)
		return nil
	case 0:
		*s = nil
		return nil
	}
	return fmt.Errorf("line %d: expected a string or a list of strings", n.Line)
}

// ---------------------------------------------------------------------------
// glossmark.yaml

// Config is the repository-level file: namespaces owned and imported.
type Config struct {
	// Grammar pins the `@gloss` grammar version the repository is written in
	// (like Go's `go 1.24` directive). Default 1.
	Grammar  int                       `yaml:"grammar,omitempty" json:"grammar,omitempty"`
	Owns     map[string]*NamespaceDecl `yaml:"owns" json:"owns"`
	Imports  map[string]*ImportDecl    `yaml:"imports,omitempty" json:"imports,omitempty"`
	Prefixes map[string]string         `yaml:"prefixes,omitempty" json:"prefixes,omitempty"`
	// RepoIRI is the repository's own IRI. A boundary prefix not declared
	// under prefixes: expands to <repo-iri>boundaries/<prefix>/ instead of a
	// urn with a warning, so most repositories need no prefixes: at all.
	RepoIRI string  `yaml:"repo-iri,omitempty" json:"repo-iri,omitempty"`
	Views   string  `yaml:"views,omitempty" json:"views,omitempty"`
	Policy  *Policy `yaml:"policy,omitempty" json:"policy,omitempty"`
	// Adoption is the rung of the adoption ladder the repository has reached
	// (0 vocabulary, 1 manifests, 2 criticality, 3 full; docs/QUICKSTART.md).
	// It caps how hard check, drift and diff bite. Unset means 3.
	Adoption *int     `yaml:"adoption,omitempty" json:"adoption,omitempty"`
	Exclude  []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	// CommentStyles maps a file extension (".foo"), a file name
	// ("crontab") or a path glob ("**/cron.d/*") to its comment tokens,
	// extending the builtin table.
	CommentStyles map[string][]string `yaml:"comment-styles,omitempty" json:"comment-styles,omitempty"`
	// SourceURL links paths in the viewer to a code host:
	// "https://github.com/o/r/blob/main/{path}#L{line}".
	SourceURL string `yaml:"source-url,omitempty" json:"source-url,omitempty"`
	// TestPaths are globs whose markers default to verifies when relation
	// inference runs (pass "inferrers"). Default: common test layouts.
	TestPaths []string `yaml:"test-paths,omitempty" json:"test-paths,omitempty"`
	// Suggest tunes gloss suggest.
	Suggest *SuggestConfig `yaml:"suggest,omitempty" json:"suggest,omitempty"`
	// History tunes gloss history (ADR 0040).
	History *HistoryConfig `yaml:"history,omitempty" json:"history,omitempty"`
	// Relations is the relation profile: starter (what gloss init writes;
	// gloss check hints at any other relation, GM250, and the agent
	// surfaces list only StarterRelations) or full. Unset means full, so
	// the graph of an existing repository is unchanged (issue #261).
	Relations string `yaml:"relations,omitempty" json:"relations,omitempty"`
	// Links maps paths and symbols anywhere in the repository to concepts,
	// as a manifest's links: does for its directory (ADR 0045).
	Links []*Link `yaml:"links,omitempty" json:"links,omitempty"`
	// Sources are external link sources for gloss suggest, beside the
	// executables named gloss-source-<name> on PATH: each a command that
	// speaks the wire contract (docs/PLUGINS.md, ADR 0050).
	Sources []*SourceDecl `yaml:"sources,omitempty" json:"sources,omitempty"`

	// Dir is the directory holding the file (not serialized).
	Dir string `yaml:"-" json:"-"`
}

// SourceDecl is one entry of sources: in glossmark.yaml: an external link
// source gloss suggest runs. Name is the source id (Proposal.Source,
// --source, the namespace of its calibration bases) and must be what the
// plugin calls itself; Command is the executable and its arguments,
// written as a list or as one string split on spaces, run from the
// repository root.
type SourceDecl struct {
	Name    string  `yaml:"name" json:"name"`
	Command Command `yaml:"command" json:"command"`
}

// Command is an argument vector that YAML may write as a list or as one
// string split on spaces.
type Command []string

func (c *Command) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*c = strings.Fields(n.Value)
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := n.Decode(&list); err != nil {
			return err
		}
		*c = list
		return nil
	}
	return fmt.Errorf("line %d: command must be a string or a list of strings", n.Line)
}

// sourceName is what a source id may look like: the suffix of a
// gloss-source-<name> executable, a basis namespace, a --source value.
var sourceName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ValidSourceName reports whether name can be a link source's id.
func ValidSourceName(name string) bool { return sourceName.MatchString(name) }

// HistoryConfig is the history: block of glossmark.yaml.
type HistoryConfig struct {
	// Step is what one history step spans: commit (default), first-parent
	// or version. gloss history --step overrides it.
	Step string `yaml:"step,omitempty" json:"step,omitempty"`
}

// NamespaceDecl describes an owned namespace.
type NamespaceDecl struct {
	IRI     string `yaml:"iri" json:"iri"`
	File    string `yaml:"file,omitempty" json:"file,omitempty"`
	Tier    string `yaml:"tier" json:"tier"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
	Owner   string `yaml:"owner,omitempty" json:"owner,omitempty"`
}

// ImportDecl describes an imported namespace.
type ImportDecl struct {
	IRI     string `yaml:"iri" json:"iri"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
	// Source is a local path (directory or file), a git+ URL or an https URL.
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
	Tier   string `yaml:"tier,omitempty" json:"tier,omitempty"`
}

// Policy is the configurable gate policy. Defaults follow the design.
type Policy struct {
	// SoftConfThreshold: proposed links at or above this confidence are SOFT.
	// suggest.auto-accept-precision, when set, derives the threshold
	// instead; when neither is set the default precision derives it
	// (Config.AutoAccept, ADR 0024).
	SoftConfThreshold float64 `yaml:"soft-conf-threshold,omitempty" json:"soft-conf-threshold,omitempty"`
	// HardPaths are globs whose changes are always HARD (default: ontology dir, manifests).
	HardPaths []string `yaml:"hard-paths,omitempty" json:"hard-paths,omitempty"`
	// AutoAcceptSoft: when true, gloss accept --policy accepts SOFT proposals.
	AutoAcceptSoft bool `yaml:"auto-accept-soft,omitempty" json:"auto-accept-soft,omitempty"`
	// Waivers are time-boxed exceptions for known gaps.
	Waivers []Waiver `yaml:"waivers,omitempty" json:"waivers,omitempty"`
	// CoverageFailUnder is the default for gloss coverage --fail-under
	// (percent of in-scope concepts with an artifact and a verifier; an empty
	// scope is n/a and never fails),
	// or "current" for the base's coverage (a ratchet).
	CoverageFailUnder FailUnder `yaml:"coverage-fail-under,omitempty" json:"coverage-fail-under,omitempty"`
	// CriticalityGates raises the gate of any change that touches a concept or
	// boundary at or above a criticality level: level → AUTO | SOFT | HARD.
	// Default: critical → HARD. See docs/GOVERNANCE.md.
	CriticalityGates map[string]string `yaml:"criticality-gates,omitempty" json:"criticality-gates,omitempty"`
}

// FailUnder is a coverage threshold as written: a percentage, or "current".
// It is validated when glossmark.yaml is read.
type FailUnder string

// UnmarshalYAML accepts a number or the word current.
func (f *FailUnder) UnmarshalYAML(n *yaml.Node) error {
	v := strings.TrimSpace(n.Value)
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: coverage-fail-under: want a percentage or current", n.Line)
	}
	if strings.EqualFold(v, "current") {
		*f = "current"
		return nil
	}
	p, err := strconv.ParseFloat(v, 64)
	if err != nil || p < 0 || p > 100 {
		return fmt.Errorf("line %d: coverage-fail-under: %q is neither a percentage (0 to 100) nor current", n.Line, v)
	}
	*f = FailUnder(v)
	return nil
}

// Waiver suppresses one finding until it expires.
type Waiver struct {
	// Finding is the drift finding id or a concept CURIE.
	Finding string `yaml:"finding" json:"finding"`
	Owner   string `yaml:"owner" json:"owner"`
	Expires string `yaml:"expires" json:"expires"` // YYYY-MM-DD
	Reason  string `yaml:"reason,omitempty" json:"reason,omitempty"`
}

// DefaultPolicy returns the policy from the design when none is configured.
func DefaultPolicy() *Policy {
	return &Policy{
		// @gloss publishes-value gmi:DefaultSoftThreshold scope=line
		SoftConfThreshold: 0.7,
		HardPaths:         []string{"ontology/**", "**/.gloss.yaml", "glossmark.yaml"},
		CriticalityGates:  map[string]string{"critical": "HARD"},
	}
}

// GateForCriticality returns the minimum gate the policy demands for a change
// touching something at the given criticality level ("" when none applies).
// The strictest matching rule wins; a rule at a lower level covers every level above it.
func (p *Policy) GateForCriticality(level string) string {
	r := CriticalityRank(level)
	if p == nil || r < 0 {
		return ""
	}
	best, bestRank := "", -1
	for lvl, gate := range p.CriticalityGates {
		if CriticalityRank(lvl) < 0 || CriticalityRank(lvl) > r {
			continue
		}
		if gr := gateRank(gate); gr > bestRank {
			best, bestRank = gate, gr
		}
	}
	return best
}

func gateRank(g string) int {
	switch g {
	case "AUTO":
		return 0
	case "SOFT":
		return 1
	case "HARD":
		return 2
	}
	return -1
}

// RelationProfile returns the profile in force: relations: as written, or
// full when unset or when there is no config at all.
func (c *Config) RelationProfile() string {
	if c == nil || c.Relations == "" {
		return ProfileFull
	}
	return c.Relations
}

// EffectivePolicy merges the configured policy over the defaults. Its
// SoftConfThreshold is the auto-accept rule's threshold (Config.AutoAccept).
func (c *Config) EffectivePolicy() *Policy {
	d := DefaultPolicy()
	if c == nil || c.Policy == nil {
		d.SoftConfThreshold = c.AutoAccept().Threshold
		return d
	}
	p := *c.Policy
	p.SoftConfThreshold = c.AutoAccept().Threshold
	if len(p.HardPaths) == 0 {
		p.HardPaths = d.HardPaths
	}
	if p.CriticalityGates == nil {
		p.CriticalityGates = d.CriticalityGates
	}
	return &p
}

// maxFile caps what a loader reads: a path in glossmark.yaml can name
// /dev/zero or a FIFO, which would read forever.
const maxFile = 16 << 20

func readFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return nil, err
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("%s is not a regular file", path)
	case info.Size() > maxFile:
		return nil, fmt.Errorf("%s is larger than %d MiB", path, maxFile>>20)
	}
	return os.ReadFile(path)
}

// LoadConfig reads glossmark.yaml.
func LoadConfig(path string) (*Config, error) {
	b, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c.Dir = filepath.Dir(path)
	if c.Grammar == 0 {
		c.Grammar = 1
	}
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err == nil {
		linkLines(&root, c.Links)
	}
	if c.Grammar < 0 {
		return nil, fmt.Errorf("%s: grammar must be a positive integer", path)
	}
	if c.Relations != "" && c.Relations != ProfileStarter && c.Relations != ProfileFull {
		return nil, fmt.Errorf("%s: relations must be starter or full, got %q", path, c.Relations)
	}
	for prefix, ns := range c.Owns {
		if ns == nil {
			return nil, fmt.Errorf("%s: owns.%s is empty", path, prefix)
		}
		if ns.File == "" {
			ns.File = filepath.ToSlash(filepath.Join("ontology", prefix+".gloss.yaml"))
		}
		if ns.Tier == "" {
			return nil, fmt.Errorf("%s: owns.%s: tier is required (core | implementation)", path, prefix)
		}
		if ns.Tier != TierCore && ns.Tier != TierImpl {
			return nil, fmt.Errorf("%s: owns.%s: tier must be core or implementation, got %q", path, prefix, ns.Tier)
		}
		if ns.IRI == "" {
			return nil, fmt.Errorf("%s: owns.%s: iri is required", path, prefix)
		}
		if ns.Version != "" {
			if _, err := ParseSemver(ns.Version); err != nil {
				return nil, fmt.Errorf("%s: owns.%s: version %q is not semver", path, prefix, ns.Version)
			}
		}
	}
	for prefix, im := range c.Imports {
		if im == nil {
			return nil, fmt.Errorf("%s: imports.%s is empty", path, prefix)
		}
		if im.IRI == "" {
			return nil, fmt.Errorf("%s: imports.%s: iri is required", path, prefix)
		}
		if im.Tier != "" && im.Tier != TierCore && im.Tier != TierImpl {
			return nil, fmt.Errorf("%s: imports.%s: tier must be core or implementation, got %q", path, prefix, im.Tier)
		}
		if im.Version != "" {
			if _, err := ParseConstraint(im.Version); err != nil {
				return nil, fmt.Errorf("%s: imports.%s: version range %q: %v", path, prefix, im.Version, err)
			}
		}
	}
	if c.Adoption != nil && !ValidAdoption(*c.Adoption) {
		return nil, fmt.Errorf("%s: adoption must be a level from %d to %d (docs/QUICKSTART.md), got %d", path, AdoptionVocabulary, AdoptionFull, *c.Adoption)
	}
	if c.Policy != nil {
		for lvl, gate := range c.Policy.CriticalityGates {
			if CriticalityRank(lvl) < 0 {
				return nil, fmt.Errorf("%s: policy.criticality-gates: unknown level %q (one of %s)", path, lvl, strings.Join(CriticalityLevels, ", "))
			}
			if gateRank(gate) < 0 {
				return nil, fmt.Errorf("%s: policy.criticality-gates.%s: gate must be AUTO, SOFT or HARD, got %q", path, lvl, gate)
			}
		}
	}
	if err := c.loadSuggest(path); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i, s := range c.Sources {
		switch {
		case s == nil || s.Name == "":
			return nil, fmt.Errorf("%s: sources[%d]: name is required (the id the plugin answers with)", path, i)
		case !ValidSourceName(s.Name):
			return nil, fmt.Errorf("%s: sources[%d]: name %q must be lower-case letters, digits and hyphens", path, i, s.Name)
		case len(s.Command) == 0:
			return nil, fmt.Errorf("%s: sources[%d] (%s): command is required", path, i, s.Name)
		case seen[s.Name]:
			return nil, fmt.Errorf("%s: sources[%d]: %s is listed twice", path, i, s.Name)
		}
		seen[s.Name] = true
	}
	if c.Views == "" {
		c.Views = "ontology/views.yaml"
	}
	return &c, nil
}

// ---------------------------------------------------------------------------
// *.gloss.yaml ontology files

// Ontology is one namespace's concepts.
type Ontology struct {
	Namespace string                  `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	IRI       string                  `yaml:"iri,omitempty" json:"iri,omitempty"`
	Version   string                  `yaml:"version,omitempty" json:"version,omitempty"`
	Tier      string                  `yaml:"tier,omitempty" json:"tier,omitempty"`
	Doc       string                  `yaml:"doc,omitempty" json:"doc,omitempty"`
	Concepts  map[string]*Concept     `yaml:"concepts" json:"concepts"`
	Relations map[string]RelationDecl `yaml:"relations,omitempty" json:"relations,omitempty"`

	// Path is the file the ontology was read from (not serialized).
	Path string `yaml:"-" json:"-"`
	// Lines maps concept name → line number for diagnostics.
	Lines map[string]int `yaml:"-" json:"-"`
}

// Published marks an implementation concept as crossing into the business view.
type Published struct {
	As string `yaml:"as" json:"as"`
}

// Concept is one ontology entry. Known structural keys are typed; every other
// key is a relation (produces, consumes, constrains, ...) resolved against the
// vocabulary, or a documented attribute (value, label, doc).
type Concept struct {
	Kind       string     `yaml:"kind,omitempty" json:"kind,omitempty"`
	Label      string     `yaml:"label,omitempty" json:"label,omitempty"`
	Doc        string     `yaml:"doc,omitempty" json:"doc,omitempty"`
	IsA        StringList `yaml:"is-a,omitempty" json:"is-a,omitempty"`
	PartOf     StringList `yaml:"part-of,omitempty" json:"part-of,omitempty"`
	HasPart    StringList `yaml:"has-part,omitempty" json:"has-part,omitempty"`
	Realizes   StringList `yaml:"realizes,omitempty" json:"realizes,omitempty"`
	Published  *Published `yaml:"published,omitempty" json:"published,omitempty"`
	Internal   bool       `yaml:"internal,omitempty" json:"internal,omitempty"`
	Deprecated bool       `yaml:"deprecated,omitempty" json:"deprecated,omitempty"`
	ReplacedBy string     `yaml:"replaced-by,omitempty" json:"replaced-by,omitempty"`
	Value      any        `yaml:"value,omitempty" json:"value,omitempty"`
	// ValueFrom binds value to the lines of code it comes from (one
	// location, or a list when the value is duplicated, say in the API and
	// in a page), so a change to any of them without a change to value is
	// "published value drifted".
	ValueFrom ValueFromList `yaml:"value-from,omitempty" json:"value-from,omitempty"`
	// Proposed marks an entry an agent (or anyone) added with the same semantics
	// as `@gloss?`: it exists so links resolve, but a human accepts it
	// (gloss accept --concept) before it counts as part of the ontology.
	Proposed   bool   `yaml:"proposed,omitempty" json:"proposed,omitempty"`
	ProposedBy string `yaml:"proposed-by,omitempty" json:"proposed-by,omitempty"`
	// Planned marks a concept the specification describes and nothing
	// implements yet: `planned: true`, or `planned: { by: YYYY-MM-DD,
	// ticket: <uri> }`. It is out of coverage and orphans and a plan item in
	// drift until code implements it (ADR 0025 6.1, ADR 0034).
	Planned *Plan `yaml:"planned,omitempty" json:"planned,omitempty"`
	// Aliases are other names people and agents use for the concept; search matches them.
	Aliases StringList `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	// Criticality classifies how much a change here matters (low | medium |
	// high | critical). Implementation concepts inherit it from what they
	// realize when unset. See docs/GOVERNANCE.md.
	Criticality string `yaml:"criticality,omitempty" json:"criticality,omitempty"`
	// Shapes is reserved for SHACL-style instance validation (Phase 3).
	Shapes any `yaml:"shapes,omitempty" json:"shapes,omitempty"`
	// Relations holds every other key: relation name → targets.
	Relations map[string]StringList `yaml:",inline" json:"relations,omitempty"`

	Line int `yaml:"-" json:"-"`
	// NullKeys lists the entry's keys whose value is null in the file, in file
	// order; Flow reports that the entry is written as a { } flow mapping. An
	// unquoted doc with a comma inside { } ends at the comma and the rest
	// becomes null keys, so both feed the GM304/GM305 lint. Not serialized.
	NullKeys []string `yaml:"-" json:"-"`
	Flow     bool     `yaml:"-" json:"-"`
}

// Plan is a planned: value. Both fields are optional: `planned: true` is a
// plan with neither. By is YYYY-MM-DD, inclusive; Ticket is provenance the
// tool never reads.
type Plan struct {
	By     string `yaml:"by,omitempty" json:"by,omitempty"`
	Ticket string `yaml:"ticket,omitempty" json:"ticket,omitempty"`
	// none records planned: false, which the loaders turn into no plan.
	none bool
}

// PlanDate is the layout of a plan's by: date (and a waiver's expires:).
const PlanDate = "2006-01-02"

// UnmarshalYAML accepts true, false (no plan) or a { by, ticket } mapping.
func (p *Plan) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		var b bool
		if err := n.Decode(&b); err != nil {
			return fmt.Errorf("line %d: planned: want true, false or { by: YYYY-MM-DD, ticket: <uri> }", n.Line)
		}
		*p = Plan{none: !b}
		return nil
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if v.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: planned.%s: want a scalar", v.Line, k.Value)
			}
			switch k.Value {
			case "by":
				p.By = v.Value
			case "ticket":
				p.Ticket = v.Value
			default:
				return fmt.Errorf("line %d: planned: unknown key %q; the keys are by and ticket", k.Line, k.Value)
			}
		}
		return nil
	}
	return fmt.Errorf("line %d: planned: want true or { by: YYYY-MM-DD, ticket: <uri> }", n.Line)
}

// normalize returns nil for planned: false.
func (p *Plan) normalize() *Plan {
	if p == nil || p.none {
		return nil
	}
	return p
}

// String renders the plan the way drift messages name it: "by 2026-11-30,
// https://...", either part alone, or "" for a plan with neither.
func (p *Plan) String() string {
	if p == nil {
		return ""
	}
	var parts []string
	if p.By != "" {
		parts = append(parts, "by "+p.By)
	}
	if p.Ticket != "" {
		parts = append(parts, p.Ticket)
	}
	return strings.Join(parts, ", ")
}

// Validate reports a by: that is not a YYYY-MM-DD date.
func (p *Plan) Validate() error {
	if p == nil || p.By == "" {
		return nil
	}
	if _, err := time.Parse(PlanDate, p.By); err != nil {
		return fmt.Errorf("planned.by must be a date (YYYY-MM-DD), got %q", p.By)
	}
	return nil
}

// ValueFrom names the code a concept's value comes from: a file and either
// a symbol (the first line that assigns or defaults it: "swr: float = 0.04")
// or a 1-based line.
type ValueFrom struct {
	Path   string `yaml:"path" json:"path"`
	Symbol string `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Line   int    `yaml:"line,omitempty" json:"line,omitempty"`
}

// ValueFromList is value-from: one location (a mapping) or several (a
// sequence of mappings).
type ValueFromList []ValueFrom

// UnmarshalYAML accepts a mapping or a sequence of mappings.
func (l *ValueFromList) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.SequenceNode {
		var vs []ValueFrom
		if err := n.Decode(&vs); err != nil {
			return err
		}
		*l = vs
		return nil
	}
	var v ValueFrom
	if err := n.Decode(&v); err != nil {
		return err
	}
	*l = ValueFromList{v}
	return nil
}

// numLiteral matches a number as code or prose writes it: a sign, digit
// group separators (1_000, or 1,000 in thousands), a fraction, an exponent.
var numLiteral = regexp.MustCompile(`[-+]?(?:\d{1,3}(?:,\d{3})+(?:\.\d+)?|\d[\d_]*(?:\.\d+)?|\.\d+)(?:[eE][-+]?\d+)?`)

// ValueInLine reports whether a published value appears in the bound code:
// some number in the value equals a number in the line (a percentage also
// as a fraction, so 4% matches 0.04; a suffix such as 60L or 30d is not part
// of the number), or, for a value without numbers, the value's text appears
// in the line, unquoted and case-insensitive. A value with nothing to
// compare (none, or a list or map) is not judged.
func ValueInLine(value any, line string) bool {
	text := strings.TrimSpace(fmt.Sprint(value))
	if value == nil || text == "" {
		return true
	}
	switch value.(type) {
	case []any, map[string]any:
		return true
	}
	nums := func(s string) []float64 {
		var out []float64
		for _, m := range numLiteral.FindAllStringIndex(s, -1) {
			if m[0] > 0 && isIdentByte(s[m[0]-1]) {
				continue // the digits of a name: MAX_2, v1
			}
			f, err := strconv.ParseFloat(strings.NewReplacer("_", "", ",", "").Replace(s[m[0]:m[1]]), 64)
			if err != nil {
				continue
			}
			out = append(out, f)
			if m[1] < len(s) && s[m[1]] == '%' {
				out = append(out, f/100)
			}
		}
		return out
	}
	want := nums(text)
	if len(want) == 0 {
		return strings.Contains(strings.ToLower(line), strings.ToLower(strings.Trim(text, `"'`)))
	}
	have := nums(line)
	for _, w := range want {
		for _, h := range have {
			if w == h || math.Abs(w-h) <= 1e-9*math.Max(math.Abs(w), math.Abs(h)) {
				return true
			}
		}
	}
	return false
}

func isIdentByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// RelationNames returns the concept's generic relation keys, sorted.
func (c *Concept) RelationNames() []string {
	out := make([]string, 0, len(c.Relations))
	for k := range c.Relations {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// LoadOntology reads a *.gloss.yaml file. prefix is the namespace prefix the
// config assigns it; the file may repeat it under namespace:.
func LoadOntology(path string) (*Ontology, error) {
	b, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var o Ontology
	if err := yaml.Unmarshal(b, &o); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	o.Path = path
	if o.Concepts == nil {
		o.Concepts = map[string]*Concept{}
	}
	// Recover line numbers for diagnostics.
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err == nil && len(root.Content) > 0 {
		o.Lines = map[string]int{}
		m := root.Content[0]
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == "concepts" && m.Content[i+1].Kind == yaml.MappingNode {
				cm := m.Content[i+1]
				for j := 0; j+1 < len(cm.Content); j += 2 {
					name := cm.Content[j].Value
					o.Lines[name] = cm.Content[j].Line
					c := o.Concepts[name]
					if c == nil {
						c = &Concept{}
						o.Concepts[name] = c
					}
					if entry := cm.Content[j+1]; entry.Kind == yaml.MappingNode {
						c.Flow = entry.Style&yaml.FlowStyle != 0
						for k := 0; k+1 < len(entry.Content); k += 2 {
							if v := entry.Content[k+1]; v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
								c.NullKeys = append(c.NullKeys, entry.Content[k].Value)
							}
						}
					}
				}
			}
		}
		for name, c := range o.Concepts {
			if c == nil {
				o.Concepts[name] = &Concept{}
				c = o.Concepts[name]
			}
			c.Line = o.Lines[name]
		}
	}
	for _, c := range o.Concepts {
		c.Planned = c.Planned.normalize()
	}
	return &o, nil
}

// ---------------------------------------------------------------------------
// .gloss.yaml boundary manifests

// Manifest is a per-directory boundary declaration.
type Manifest struct {
	Boundary   string     `yaml:"boundary" json:"boundary"`
	Level      string     `yaml:"level,omitempty" json:"level,omitempty"`
	Label      string     `yaml:"label,omitempty" json:"label,omitempty"`
	Doc        string     `yaml:"doc,omitempty" json:"doc,omitempty"`
	PartOf     StringList `yaml:"part-of,omitempty" json:"part-of,omitempty"`
	Realizes   StringList `yaml:"realizes,omitempty" json:"realizes,omitempty"`
	Produces   StringList `yaml:"produces,omitempty" json:"produces,omitempty"`
	Consumes   StringList `yaml:"consumes,omitempty" json:"consumes,omitempty"`
	Publishes  StringList `yaml:"publishes,omitempty" json:"publishes,omitempty"`
	Subscribes StringList `yaml:"subscribes,omitempty" json:"subscribes,omitempty"`
	// Holds names the entities whose data lives in this boundary (tables,
	// files, packages); the code's represents links on entities derive it.
	Holds StringList `yaml:"holds,omitempty" json:"holds,omitempty"`
	// Presents and Hosts name the concepts this boundary is a UI surface or a
	// shell for; they are compared like realizes but never imply it.
	Presents StringList `yaml:"presents,omitempty" json:"presents,omitempty"`
	Hosts    StringList `yaml:"hosts,omitempty" json:"hosts,omitempty"`
	// Configures names the concepts whose value this boundary edits at
	// runtime (an admin page for a limit); compared like presents, never
	// implies realizes (ADR 0012).
	Configures StringList `yaml:"configures,omitempty" json:"configures,omitempty"`
	// GuardedBy names the mechanisms every artifact under the directory sits
	// behind (authentication, a rate limit): guarded code without a guards:
	// list in the ontology (ADR 0015).
	GuardedBy        StringList `yaml:"guarded-by,omitempty" json:"guarded-by,omitempty"`
	DefaultNamespace string     `yaml:"default-namespace,omitempty" json:"default-namespace,omitempty"`
	Allowed          StringList `yaml:"allowed,omitempty" json:"allowed,omitempty"`
	Generated        StringList `yaml:"generated,omitempty" json:"generated,omitempty"`
	Owner            string     `yaml:"owner,omitempty" json:"owner,omitempty"`
	// Criticality of the boundary (low | medium | high | critical). When unset
	// it is the highest criticality of the concepts the boundary realizes,
	// represents or configures, else its parent's unless the boundary only
	// presents or hosts. See docs/GOVERNANCE.md.
	Criticality string `yaml:"criticality,omitempty" json:"criticality,omitempty"`
	// CriticalityFloor makes the boundary's level a floor for every artifact
	// beneath it, including nested boundaries. Without it an artifact takes
	// only the level of the concepts it links to. See docs/GOVERNANCE.md.
	CriticalityFloor bool `yaml:"criticality-floor,omitempty" json:"criticality-floor,omitempty"`
	// Deprecated marks a boundary that is being retired; ReplacedBy names
	// the boundary that takes over (a CURIE, possibly in another repository)
	// and RetireBy the date it is due to go (YYYY-MM-DD). See ADR 0007.
	Deprecated bool   `yaml:"deprecated,omitempty" json:"deprecated,omitempty"`
	ReplacedBy string `yaml:"replaced-by,omitempty" json:"replaced-by,omitempty"`
	RetireBy   string `yaml:"retire-by,omitempty" json:"retire-by,omitempty"`
	// Planned declares capability the boundary will have: an ordinary
	// declaration for the rollup, inherit and the derived allowed set, which
	// drift reports as a plan item instead of a gap until code under the
	// boundary derives it (ADR 0034).
	Planned *PlannedDecl `yaml:"planned,omitempty" json:"planned,omitempty"`
	// Links maps paths (globs, relative to the manifest's directory) and
	// symbols under the directory to concepts, in place of inline markers
	// (ADR 0045).
	Links []*Link `yaml:"links,omitempty" json:"links,omitempty"`

	// Path is the manifest file; Dir its directory relative to the repo root (slash-separated, "" for root).
	Path string `yaml:"-" json:"-"`
	Dir  string `yaml:"-" json:"-"`
	// Empty records the declared relations written as an explicit empty
	// list (realizes: []): "this boundary does none of it", which gloss
	// drift compares, unlike a relation that is not written (ADR 0012).
	// part-of: [] is recorded too: the boundary is part of nothing, not of
	// the manifest in an enclosing directory (ADR 0026).
	Empty map[string]bool `yaml:"-" json:"-"`
}

// DeclaresNone reports whether the manifest wrote rel as an explicit empty
// list, such as realizes: [] on a boundary that is only a surface.
func (m *Manifest) DeclaresNone(rel string) bool {
	return m != nil && m.Empty[rel] && len(m.DeclaredSets()[rel]) == 0 && !m.Inherits(rel)
}

// PlannedDecl is a manifest's planned: block: the flows and realizations
// the boundary intends to have by a date (ADR 0034).
type PlannedDecl struct {
	Realizes   StringList `yaml:"realizes,omitempty" json:"realizes,omitempty"`
	Produces   StringList `yaml:"produces,omitempty" json:"produces,omitempty"`
	Consumes   StringList `yaml:"consumes,omitempty" json:"consumes,omitempty"`
	Publishes  StringList `yaml:"publishes,omitempty" json:"publishes,omitempty"`
	Subscribes StringList `yaml:"subscribes,omitempty" json:"subscribes,omitempty"`
	By         string     `yaml:"by,omitempty" json:"by,omitempty"`
}

// PlannedRelations are the manifest relations a planned: block may declare.
var PlannedRelations = []string{"realizes", "produces", "consumes", "publishes", "subscribes"}

// PlannedSets returns what the manifest plans, per relation (nil when it
// plans nothing).
func (m *Manifest) PlannedSets() map[string]StringList {
	if m == nil || m.Planned == nil {
		return nil
	}
	p := m.Planned
	out := map[string]StringList{}
	for rel, l := range map[string]StringList{"realizes": p.Realizes, "produces": p.Produces, "consumes": p.Consumes, "publishes": p.Publishes, "subscribes": p.Subscribes} {
		if len(l) > 0 {
			out[rel] = l
		}
	}
	return out
}

// Plans reports whether the manifest's planned: block names target under
// rel, as written (a CURIE or a bare name).
func (m *Manifest) Plans(rel, target string) bool {
	for _, t := range m.PlannedSets()[rel] {
		if t == target {
			return true
		}
	}
	return false
}

// InheritMarker in a manifest's realizes:, holds:, produces:, consumes:,
// publishes: or subscribes: list means "what my nested boundaries declare
// under it"; the manifest may list additions beside it (ADR 0026).
const InheritMarker = "inherit"

// InheritableRelations are the declared relations that take the inherit
// marker. presents, hosts and configures describe a surface of the
// boundary itself, so they do not.
var InheritableRelations = []string{"realizes", "holds", "produces", "consumes", "publishes", "subscribes"}

// ImplicitPartOf reports whether the manifest takes the boundary of the
// nearest manifest in an enclosing directory as its part-of: it names no
// part-of and does not write the part-of: [] that opts out (ADR 0026).
func (m *Manifest) ImplicitPartOf() bool {
	return m != nil && len(m.PartOf) == 0 && !m.Empty["part-of"]
}

// DerivedMarker in a manifest's allowed: list means "the derived set";
// the other entries are added to it instead of replacing it.
const DerivedMarker = "derived"

// ExtendsDerivedAllowed reports whether allowed: starts from the derived set.
func (m *Manifest) ExtendsDerivedAllowed() bool {
	for _, a := range m.Allowed {
		if a == DerivedMarker {
			return true
		}
	}
	return false
}

// InheritsRealizes reports whether the manifest wrote realizes: inherit.
func (m *Manifest) InheritsRealizes() bool { return m.Inherits("realizes") }

// Inherits reports whether the manifest wrote inherit in rel's list. Only
// the InheritableRelations take it.
func (m *Manifest) Inherits(rel string) bool {
	if m == nil || !inheritable(rel) {
		return false
	}
	for _, r := range m.lists()[rel] {
		if r == InheritMarker {
			return true
		}
	}
	return false
}

func inheritable(rel string) bool {
	for _, r := range InheritableRelations {
		if r == rel {
			return true
		}
	}
	return false
}

// DeclaredSets returns the manifest's declared intent relations (without
// the inherit marker on the relations that take it), the planned: entries
// included: a planned declaration is a declaration for every purpose but
// the drift finding it produces (ADR 0034), and Plans tells them apart.
func (m *Manifest) DeclaredSets() map[string]StringList {
	out := m.lists()
	for _, rel := range InheritableRelations {
		var kept StringList
		for _, r := range out[rel] {
			if r != InheritMarker {
				kept = append(kept, r)
			}
		}
		out[rel] = kept
	}
	for rel, planned := range m.PlannedSets() {
		for _, t := range planned {
			dup := false
			for _, r := range out[rel] {
				dup = dup || r == t
			}
			if !dup {
				out[rel] = append(out[rel], t)
			}
		}
	}
	return out
}

// lists returns the declared relation lists as written.
func (m *Manifest) lists() map[string]StringList {
	return map[string]StringList{
		"realizes":   m.Realizes,
		"produces":   m.Produces,
		"consumes":   m.Consumes,
		"publishes":  m.Publishes,
		"subscribes": m.Subscribes,
		"holds":      m.Holds,
		"presents":   m.Presents,
		"hosts":      m.Hosts,
		"configures": m.Configures,
	}
}

// Link is one links: entry of a manifest or of glossmark.yaml: a path glob
// or a declaration symbol mapped to concepts under one or more relations,
// written where an inline marker would otherwise go (ADR 0045). Every key
// but path, symbol, proposed, conf and note is a relation, as on a concept.
type Link struct {
	// Path is a glob over the files beneath the directory of the file that
	// holds the entry (gitignore-style: * within a segment, ** across; a
	// pattern without / matches a basename at any depth). Alone it links
	// every matching file at file scope; with Symbol it narrows the files
	// the symbol is looked for in.
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
	// Symbol is a declaration name as the anchorer records it (catalog_book,
	// Service.Cancel, resource.aws_dynamodb_table.loans); the link anchors
	// to that declaration in every file beneath the directory that holds
	// it, as a marker above it would.
	Symbol   string   `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Proposed bool     `yaml:"proposed,omitempty" json:"proposed,omitempty"`
	Conf     *float64 `yaml:"conf,omitempty" json:"conf,omitempty"`
	Note     string   `yaml:"note,omitempty" json:"note,omitempty"`
	// Relations holds every other key: relation name → targets.
	Relations map[string]StringList `yaml:",inline" json:"relations,omitempty"`

	// Line and EndLine span the entry in its file (not serialized).
	Line    int `yaml:"-" json:"-"`
	EndLine int `yaml:"-" json:"-"`
}

// RelationNames returns the entry's relation keys, sorted.
func (l *Link) RelationNames() []string {
	out := make([]string, 0, len(l.Relations))
	for k := range l.Relations {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Where names the entry the way diagnostics do: its path glob, its symbol,
// or both.
func (l *Link) Where() string {
	switch {
	case l.Path != "" && l.Symbol != "":
		return fmt.Sprintf("symbol %q in %q", l.Symbol, l.Path)
	case l.Symbol != "":
		return fmt.Sprintf("symbol %q", l.Symbol)
	}
	return fmt.Sprintf("path %q", l.Path)
}

// linkLines records, on each decoded links: entry, the lines its mapping
// spans in the file, so a diagnostic and a review command find the entry.
func linkLines(doc *yaml.Node, links []*Link) {
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return
	}
	mn := doc.Content[0]
	for i := 0; i+1 < len(mn.Content); i += 2 {
		if mn.Content[i].Value != "links" || mn.Content[i+1].Kind != yaml.SequenceNode {
			continue
		}
		for j, item := range mn.Content[i+1].Content {
			if j < len(links) && links[j] != nil {
				links[j].Line, links[j].EndLine = item.Line, lastLine(item)
			}
		}
	}
}

// lastLine is the last line a YAML node's content reaches.
func lastLine(n *yaml.Node) int {
	end := n.Line
	if n.Kind == yaml.ScalarNode {
		end += strings.Count(strings.TrimRight(n.Value, "\n"), "\n")
	}
	for _, c := range n.Content {
		end = max(end, lastLine(c))
	}
	return end
}

// UnknownKey is a top-level manifest key the format does not define.
type UnknownKey struct {
	Key  string
	Line int
}

// LoadManifest reads a .gloss.yaml manifest. Unknown top-level keys do not
// fail the load: they are returned, with their lines, for the caller to
// report, and the known keys still make the boundary, so one misspelt key
// does not hand the directory's markers to the parent boundary.
func LoadManifest(path string) (*Manifest, []UnknownKey, error) {
	b, err := readFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	var unknown []UnknownKey
	if len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		known := yamlKeys(Manifest{})
		plannedKeys := yamlKeys(PlannedDecl{})
		mn := doc.Content[0]
		var kept []*yaml.Node
		for i := 0; i+1 < len(mn.Content); i += 2 {
			if k := mn.Content[i]; !known[k.Value] {
				unknown = append(unknown, UnknownKey{Key: k.Value, Line: k.Line})
				continue
			}
			// A misspelt key inside planned: is reported at its line too.
			if k, v := mn.Content[i], mn.Content[i+1]; k.Value == "planned" && v.Kind == yaml.MappingNode {
				var pk []*yaml.Node
				for j := 0; j+1 < len(v.Content); j += 2 {
					if pkn := v.Content[j]; !plannedKeys[pkn.Value] {
						unknown = append(unknown, UnknownKey{Key: "planned." + pkn.Value, Line: pkn.Line})
						continue
					}
					pk = append(pk, v.Content[j], v.Content[j+1])
				}
				v.Content = pk
			}
			kept = append(kept, mn.Content[i], mn.Content[i+1])
		}
		mn.Content = kept
	}
	var m Manifest
	if len(doc.Content) > 0 {
		if err := doc.Decode(&m); err != nil {
			return nil, unknown, fmt.Errorf("%s: %w", path, err)
		}
	}
	if len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		declared := m.DeclaredSets()
		mn := doc.Content[0]
		for i := 0; i+1 < len(mn.Content); i += 2 {
			k, v := mn.Content[i].Value, mn.Content[i+1]
			if _, ok := declared[k]; (ok || k == "part-of") && v.Kind == yaml.SequenceNode && len(v.Content) == 0 {
				if m.Empty == nil {
					m.Empty = map[string]bool{}
				}
				m.Empty[k] = true
			}
		}
	}
	m.Path = path
	linkLines(&doc, m.Links)
	if m.Boundary == "" {
		return nil, unknown, fmt.Errorf("%s: boundary is required", path)
	}
	if m.Level == "" {
		m.Level = "component"
	}
	ok := false
	for _, l := range BoundaryLevels {
		if l == m.Level {
			ok = true
		}
	}
	if !ok {
		return nil, unknown, fmt.Errorf("%s: level must be one of %s, got %q", path, strings.Join(BoundaryLevels, ", "), m.Level)
	}
	if !ValidCriticality(m.Criticality) {
		return nil, unknown, fmt.Errorf("%s: criticality must be one of %s, got %q", path, strings.Join(CriticalityLevels, ", "), m.Criticality)
	}
	if m.RetireBy != "" {
		if _, err := time.Parse("2006-01-02", m.RetireBy); err != nil {
			return nil, unknown, fmt.Errorf("%s: retire-by must be a date (YYYY-MM-DD), got %q", path, m.RetireBy)
		}
	}
	if m.Planned != nil && m.Planned.By != "" {
		if _, err := time.Parse(PlanDate, m.Planned.By); err != nil {
			return nil, unknown, fmt.Errorf("%s: planned.by must be a date (YYYY-MM-DD), got %q", path, m.Planned.By)
		}
	}
	return &m, unknown, nil
}

// yamlKeys lists the YAML keys a struct decodes.
func yamlKeys(v any) map[string]bool {
	out := map[string]bool{}
	t := reflect.TypeOf(v)
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			out[name] = true
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// views.yaml

// Views is the declared set of projections.
type Views struct {
	Views map[string]*View `yaml:"views" json:"views"`
}

// View is one projection over tier × zoom.
type View struct {
	Doc           string            `yaml:"doc,omitempty" json:"doc,omitempty"`
	Include       StringList        `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude       StringList        `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	Collapse      map[string]string `yaml:"collapse,omitempty" json:"collapse,omitempty"`
	Expand        map[string]int    `yaml:"expand,omitempty" json:"expand,omitempty"`
	ShowPublished *bool             `yaml:"show-published,omitempty" json:"show-published,omitempty"`
	Relations     StringList        `yaml:"relations,omitempty" json:"relations,omitempty"`
	Lift          string            `yaml:"lift,omitempty" json:"lift,omitempty"`
	Zoom          string            `yaml:"zoom,omitempty" json:"zoom,omitempty"`
	// Boundaries: when true (default) boundaries at or above zoom are shown.
	Boundaries *bool `yaml:"boundaries,omitempty" json:"boundaries,omitempty"`
	// Slice makes a saved slice: the view shows the feature slice of these
	// seeds (docs/adr/0023-feature-slices.md) instead of an include list.
	Slice StringList `yaml:"slice,omitempty" json:"slice,omitempty"`
}

// DefaultViews returns the two presets from the design, for prefixes grouped by tier.
func DefaultViews(coreNS, implNS []string) *Views {
	inc := StringList{}
	for _, p := range coreNS {
		inc = append(inc, p+":*")
	}
	collapse := map[string]string{}
	all := append(StringList{}, inc...)
	for _, p := range implNS {
		collapse[p+":*"] = "via realizes"
		all = append(all, p+":*")
	}
	t := true
	return &Views{Views: map[string]*View{
		"business": {
			Doc:           "Core tier only; implementation folds into what it realizes; published concepts shown under their core name.",
			Include:       inc,
			Collapse:      collapse,
			ShowPublished: &t,
			Relations:     StringList{"produces", "consumes", "constrains", "publishes", "subscribes", "reverses", "is-a", "realizes"},
			Lift:          LiftRangeLCS,
			Zoom:          "system",
		},
		"engineering": {
			Doc:     "Both tiers, part-of expanded two levels, container zoom.",
			Include: all,
			Expand:  map[string]int{"part-of": 2},
			Zoom:    "container",
		},
	}}
}

// LoadViews reads views.yaml. Unknown keys, at the top or inside a view, are
// returned with their lines (Key is "views.<name>.<key>" inside a view) for
// the caller to report; the rest of the file still applies.
func LoadViews(path string) (*Views, []UnknownKey, error) {
	b, err := readFile(path)
	if err != nil {
		return nil, nil, err
	}
	return ParseViews(path, b)
}

// ParseViews parses views.yaml content; path is used in messages only.
func ParseViews(path string, b []byte) (*Views, []UnknownKey, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	var unknown []UnknownKey
	strip := func(mn *yaml.Node, known map[string]bool, prefix string) {
		var kept []*yaml.Node
		for i := 0; i+1 < len(mn.Content); i += 2 {
			if k := mn.Content[i]; !known[k.Value] {
				unknown = append(unknown, UnknownKey{Key: prefix + k.Value, Line: k.Line})
				continue
			}
			kept = append(kept, mn.Content[i], mn.Content[i+1])
		}
		mn.Content = kept
	}
	if len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		top := doc.Content[0]
		strip(top, map[string]bool{"views": true}, "")
		viewKeys := yamlKeys(View{})
		for i := 0; i+1 < len(top.Content); i += 2 {
			if vs := top.Content[i+1]; vs.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(vs.Content); j += 2 {
					if vn := vs.Content[j+1]; vn.Kind == yaml.MappingNode {
						strip(vn, viewKeys, "views."+vs.Content[j].Value+".")
					}
				}
			}
		}
	}
	var v Views
	if len(doc.Content) > 0 {
		if err := doc.Decode(&v); err != nil {
			return nil, unknown, fmt.Errorf("%s: %w", path, err)
		}
	}
	for name, view := range v.Views {
		if view == nil {
			return nil, unknown, fmt.Errorf("%s: view %s is empty", path, name)
		}
		if len(view.Slice) > 0 && len(view.Include) > 0 {
			return nil, unknown, fmt.Errorf("%s: view %s: slice and include are two ways to choose nodes; use one", path, name)
		}
		if view.Zoom != "" {
			ok := false
			for _, z := range ZoomLevels {
				if z == view.Zoom {
					ok = true
				}
			}
			if !ok {
				return nil, unknown, fmt.Errorf("%s: view %s: zoom must be one of %s", path, name, strings.Join(ZoomLevels, ", "))
			}
		}
	}
	return &v, unknown, nil
}

// ---------------------------------------------------------------------------
// Ledger

// Review states.
const (
	StatusProposed = "proposed"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
	StatusStale    = "stale"
	StatusRemoved  = "removed" // marker no longer present in the tree, without a review decision
	// StatusDrifted is a value:<curie> baseline whose bound code changed while
	// the published value did not. It is not stale: clearing stale links must
	// never clear a value drift.
	StatusDrifted = "drifted"
)

// LedgerEntry records provenance and review state for one marker.
type LedgerEntry struct {
	MarkerID          string   `json:"marker_id"`
	Path              string   `json:"path"`
	Relation          string   `json:"relation"`
	Targets           []string `json:"targets"`
	Actor             string   `json:"actor,omitempty"`
	Model             string   `json:"model,omitempty"`
	Conf              float64  `json:"conf,omitempty"`
	Created           string   `json:"created"`
	Status            string   `json:"status"`
	ReviewedBy        string   `json:"reviewed_by,omitempty"`
	ReviewedAt        string   `json:"reviewed_at,omitempty"`
	AnchorFingerprint string   `json:"anchor_fingerprint,omitempty"`
	Rationale         string   `json:"rationale,omitempty"`
	ReviewReason      string   `json:"review_reason,omitempty"` // gloss reject --reason; Rationale is the author's note=
	Gate              string   `json:"gate,omitempty"`
	LastSeen          string   `json:"last_seen,omitempty"`
	// Source is "manifest" for a link a links: entry maps (ADR 0045); empty
	// for an inline marker.
	Source string `json:"source,omitempty"`
	// Value and Live record, for a value binding (marker id value:<curie>),
	// the ontology value and the bound code line when last accepted.
	Value string `json:"value,omitempty"`
	Live  string `json:"live,omitempty"`
	// PreviousIDs are the ids this marker had before a review command
	// rewrote its target (propose --from-new, reject --concept --remove)
	// and so changed its derived id; the entry carries its history across
	// the rewrite instead of leaving a removed row behind (ADR 0027).
	PreviousIDs []string `json:"previous_ids,omitempty"`
}
