// @gloss represents gmi:FileFormats scope=file
// @gloss represents gm:Namespace, gm:Boundary scope=file

package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	RepoIRI string   `yaml:"repo-iri,omitempty" json:"repo-iri,omitempty"`
	Views   string   `yaml:"views,omitempty" json:"views,omitempty"`
	Policy  *Policy  `yaml:"policy,omitempty" json:"policy,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	// CommentStyles maps a file extension (".foo") to its comment tokens,
	// extending the builtin table.
	CommentStyles map[string][]string `yaml:"comment-styles,omitempty" json:"comment-styles,omitempty"`
	// SourceURL links paths in the viewer to a code host:
	// "https://github.com/o/r/blob/main/{path}#L{line}".
	SourceURL string `yaml:"source-url,omitempty" json:"source-url,omitempty"`
	// TestPaths are globs whose markers default to verifies when relation
	// inference runs (pass "inferrers"). Default: common test layouts.
	TestPaths []string `yaml:"test-paths,omitempty" json:"test-paths,omitempty"`

	// Dir is the directory holding the file (not serialized).
	Dir string `yaml:"-" json:"-"`
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
	SoftConfThreshold float64 `yaml:"soft-conf-threshold,omitempty" json:"soft-conf-threshold,omitempty"`
	// HardPaths are globs whose changes are always HARD (default: ontology dir, manifests).
	HardPaths []string `yaml:"hard-paths,omitempty" json:"hard-paths,omitempty"`
	// AutoAcceptSoft: when true, gloss accept --policy accepts SOFT proposals.
	AutoAcceptSoft bool `yaml:"auto-accept-soft,omitempty" json:"auto-accept-soft,omitempty"`
	// Waivers are time-boxed exceptions for known gaps.
	Waivers []Waiver `yaml:"waivers,omitempty" json:"waivers,omitempty"`
	// CoverageFailUnder is the default for gloss coverage --fail-under
	// (percent of implementation concepts with an artifact and a verifier).
	CoverageFailUnder float64 `yaml:"coverage-fail-under,omitempty" json:"coverage-fail-under,omitempty"`
	// CriticalityGates raises the gate of any change that touches a concept or
	// boundary at or above a criticality level: level → AUTO | SOFT | HARD.
	// Default: critical → HARD. See docs/GOVERNANCE.md.
	CriticalityGates map[string]string `yaml:"criticality-gates,omitempty" json:"criticality-gates,omitempty"`
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
		CoverageFailUnder: 0,
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

// EffectivePolicy merges the configured policy over the defaults.
func (c *Config) EffectivePolicy() *Policy {
	d := DefaultPolicy()
	if c == nil || c.Policy == nil {
		return d
	}
	p := *c.Policy
	if p.SoftConfThreshold == 0 {
		p.SoftConfThreshold = d.SoftConfThreshold
	}
	if len(p.HardPaths) == 0 {
		p.HardPaths = d.HardPaths
	}
	if p.CriticalityGates == nil {
		p.CriticalityGates = d.CriticalityGates
	}
	return &p
}

// LoadConfig reads glossmark.yaml.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
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
	if c.Grammar < 0 {
		return nil, fmt.Errorf("%s: grammar must be a positive integer", path)
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
	// ValueFrom binds value to the line of code it comes from, so a change to
	// that line without a change to value is "published value drifted".
	ValueFrom *ValueFrom `yaml:"value-from,omitempty" json:"value-from,omitempty"`
	// Proposed marks an entry an agent (or anyone) added with the same semantics
	// as `@gloss?`: it exists so links resolve, but a human accepts it
	// (gloss accept --concept) before it counts as part of the ontology.
	Proposed   bool   `yaml:"proposed,omitempty" json:"proposed,omitempty"`
	ProposedBy string `yaml:"proposed-by,omitempty" json:"proposed-by,omitempty"`
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

// ValueFrom names the code a concept's value comes from: a file and either
// a symbol (the first line that assigns or defaults it: "swr: float = 0.04")
// or a 1-based line.
type ValueFrom struct {
	Path   string `yaml:"path" json:"path"`
	Symbol string `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Line   int    `yaml:"line,omitempty" json:"line,omitempty"`
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
	b, err := os.ReadFile(path)
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
	Presents         StringList `yaml:"presents,omitempty" json:"presents,omitempty"`
	Hosts            StringList `yaml:"hosts,omitempty" json:"hosts,omitempty"`
	DefaultNamespace string     `yaml:"default-namespace,omitempty" json:"default-namespace,omitempty"`
	Allowed          StringList `yaml:"allowed,omitempty" json:"allowed,omitempty"`
	Generated        StringList `yaml:"generated,omitempty" json:"generated,omitempty"`
	Owner            string     `yaml:"owner,omitempty" json:"owner,omitempty"`
	// Criticality of the boundary (low | medium | high | critical). When unset
	// it is the highest criticality of the concepts the boundary realizes,
	// else its parent's. See docs/GOVERNANCE.md.
	Criticality string `yaml:"criticality,omitempty" json:"criticality,omitempty"`
	// CriticalityFloor makes the boundary's level a floor for every artifact
	// beneath it, including nested boundaries. Without it an artifact takes
	// only the level of the concepts it links to. See docs/GOVERNANCE.md.
	CriticalityFloor bool `yaml:"criticality-floor,omitempty" json:"criticality-floor,omitempty"`

	// Path is the manifest file; Dir its directory relative to the repo root (slash-separated, "" for root).
	Path string `yaml:"-" json:"-"`
	Dir  string `yaml:"-" json:"-"`
}

// InheritMarker in a manifest's realizes: list means "what my nested
// boundaries declare"; the manifest may list additions beside it.
const InheritMarker = "inherit"

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
func (m *Manifest) InheritsRealizes() bool {
	for _, r := range m.Realizes {
		if r == InheritMarker {
			return true
		}
	}
	return false
}

// DeclaredSets returns the manifest's declared intent relations (without
// the inherit marker).
func (m *Manifest) DeclaredSets() map[string]StringList {
	var realizes StringList
	for _, r := range m.Realizes {
		if r != InheritMarker {
			realizes = append(realizes, r)
		}
	}
	return map[string]StringList{
		"realizes":   realizes,
		"produces":   m.Produces,
		"consumes":   m.Consumes,
		"publishes":  m.Publishes,
		"subscribes": m.Subscribes,
		"holds":      m.Holds,
		"presents":   m.Presents,
		"hosts":      m.Hosts,
	}
}

// LoadManifest reads a .gloss.yaml manifest.
func LoadManifest(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	m.Path = path
	if m.Boundary == "" {
		return nil, fmt.Errorf("%s: boundary is required", path)
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
		return nil, fmt.Errorf("%s: level must be one of %s, got %q", path, strings.Join(BoundaryLevels, ", "), m.Level)
	}
	if !ValidCriticality(m.Criticality) {
		return nil, fmt.Errorf("%s: criticality must be one of %s, got %q", path, strings.Join(CriticalityLevels, ", "), m.Criticality)
	}
	return &m, nil
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

// LoadViews reads views.yaml.
func LoadViews(path string) (*Views, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v Views
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for name, view := range v.Views {
		if view == nil {
			return nil, fmt.Errorf("%s: view %s is empty", path, name)
		}
		if view.Zoom != "" {
			ok := false
			for _, z := range ZoomLevels {
				if z == view.Zoom {
					ok = true
				}
			}
			if !ok {
				return nil, fmt.Errorf("%s: view %s: zoom must be one of %s", path, name, strings.Join(ZoomLevels, ", "))
			}
		}
	}
	return &v, nil
}

// ---------------------------------------------------------------------------
// Ledger

// Review states.
const (
	StatusProposed = "proposed"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
	StatusStale    = "stale"
	StatusRemoved  = "removed" // marker no longer present in the tree
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
	// Value and Live record, for a value binding (marker id value:<curie>),
	// the ontology value and the bound code line when last accepted.
	Value string `json:"value,omitempty"`
	Live  string `json:"live,omitempty"`
}
