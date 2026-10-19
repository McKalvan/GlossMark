package rollup

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

// Finding kinds.
const (
	KindConsistent = "consistent"
	KindViolation  = "violation" // boundary exceeds its declared capability
	KindGap        = "gap"       // declared capability not implemented
	KindOrphan     = "orphan"    // concept with no artifacts
	KindRogue      = "rogue"     // link to a concept that does not exist (NEW: proposal)
	KindUntested   = "untested"  // implemented but no tests/verifies
	KindStale      = "stale"     // anchor fingerprint changed
	KindDangling   = "dangling"  // unresolved reference
	KindProposed   = "proposed"  // proposed concept awaiting acceptance
	// KindPresentedOnly: something presents the concept but nothing implements it.
	KindPresentedOnly = "presented-only"
	// KindValueDrift: the code a published value is bound to changed and the value did not.
	KindValueDrift = "value-drift"
)

// ComparedRelations are the manifest relations compared against derivation.
var ComparedRelations = []string{"realizes", "holds", "presents", "hosts", "produces", "consumes", "publishes", "subscribes"}

// Finding is one drift result.
type Finding struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Severity string   `json:"severity"`
	Boundary string   `json:"boundary,omitempty"`
	Relation string   `json:"relation,omitempty"`
	Concept  string   `json:"concept,omitempty"`
	Declared []string `json:"declared,omitempty"`
	Derived  []string `json:"derived,omitempty"`
	Via      []string `json:"via,omitempty"`
	Proposed bool     `json:"proposed,omitempty"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	Msg      string   `json:"msg"`
	Waived   bool     `json:"waived,omitempty"`
	Waiver   string   `json:"waiver,omitempty"`
}

// Decision is a request for a human decision about the ontology.
type Decision struct {
	Kind     string   `json:"kind"` // new-concept | extension-upstream
	Concept  string   `json:"concept"`
	Relation string   `json:"relation,omitempty"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	Note     string   `json:"note,omitempty"`
	Actor    string   `json:"actor,omitempty"`
	Owners   []string `json:"owners,omitempty"`
}

// Metrics are the CI numbers from the design. The headline is concept
// coverage: implementation concepts that have at least one artifact and one
// verifier. File coverage (files carrying an accepted link) is secondary,
// because one marker per helper raises it without adding knowledge.
type Metrics struct {
	ConceptsInScope    int     `json:"concepts_in_scope"`
	ConceptsCovered    int     `json:"concepts_covered"`
	ConceptCoveragePct float64 `json:"concept_coverage_pct"`
	InScopeFiles       int     `json:"in_scope_files"`
	CoveredFiles       int     `json:"covered_files"`
	CoveragePct        float64 `json:"coverage_pct"`
	// Markers counts marker lines; Targets the links they state (a marker
	// may name several targets). The review-state counts are per marker.
	Markers          int `json:"markers"`
	Targets          int `json:"targets"`
	Accepted         int `json:"accepted"`
	Proposed         int `json:"proposed"`
	Stale            int `json:"stale"`
	Rejected         int `json:"rejected"`
	Orphans          int `json:"orphans"`
	Rogues           int `json:"rogues"`
	Violations       int `json:"violations"`
	Gaps             int `json:"gaps"`
	Untested         int `json:"untested"`
	ProposedConcepts int `json:"proposed_concepts"`
	Dangling         int `json:"dangling"`
	BacklogOldestDay int `json:"review_backlog_oldest_days"`
	Concepts         int `json:"concepts"`
	Boundaries       int `json:"boundaries"`
	Artifacts        int `json:"artifacts"`
}

// Report is the drift output.
type Report struct {
	Findings  []Finding  `json:"findings"`
	Decisions []Decision `json:"decisions"`
	Metrics   Metrics    `json:"metrics"`
}

// Options tunes Compare.
type Options struct {
	Ledger map[string]*model.LedgerEntry
	Policy *model.Policy
	Now    time.Time
	// Files are the in-scope files (for coverage); nil → every artifact path.
	Files []string
	// Manifests resolve which files are in scope.
	ManifestFor func(string) *model.Manifest
}

// Compare classifies every declared-vs-derived pair and computes metrics.
func Compare(g *graph.Graph, ws *load.Workspace, opt Options) *Report {
	r := &Report{}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	c := &comparer{g: g, ws: ws, opt: opt, r: r}
	c.boundaries()
	c.orphans()
	c.rogues()
	c.untested()
	c.presentedOnly()
	c.stale()
	c.metrics()
	c.waive()
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if rank(a.Kind) != rank(b.Kind) {
			return rank(a.Kind) < rank(b.Kind)
		}
		return a.ID < b.ID
	})
	return r
}

func rank(kind string) int {
	switch kind {
	case KindViolation:
		return 0
	case KindGap:
		return 1
	case KindRogue:
		return 2
	case KindDangling:
		return 3
	case KindStale:
		return 4
	case KindOrphan:
		return 5
	case KindUntested:
		return 6
	case KindPresentedOnly:
		return 7
	case KindValueDrift:
		return 1
	}
	return 9
}

type comparer struct {
	g   *graph.Graph
	ws  *load.Workspace
	opt Options
	r   *Report
}

func (c *comparer) add(f Finding) {
	if f.ID == "" {
		f.ID = f.Kind + ":" + strings.Join(filterEmpty([]string{f.Boundary, f.Relation, f.Concept}), ":")
	}
	c.r.Findings = append(c.r.Findings, f)
}

func (c *comparer) curies(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, c.g.Curie(id))
	}
	sort.Strings(out)
	return out
}

// boundaries compares each boundary's declared sets with its derived sets.
func (c *comparer) boundaries() {
	for _, b := range c.g.NodesOf(model.NodeBoundary) {
		for _, rel := range ComparedRelations {
			var declared []string
			for _, e := range c.g.Out(b.ID, rel) {
				if e.Source == graph.SourceDeclared {
					declared = append(declared, e.O)
				}
			}
			// realizes: inherit means "what my nested boundaries declare";
			// their gaps are theirs to report, so inherited entries only cover.
			inherited := map[string]bool{}
			if rel == "realizes" {
				if m := c.manifestOf(b.CURIE); m != nil && m.InheritsRealizes() {
					for _, iri := range c.inheritedRealizes(m, map[*model.Manifest]bool{}) {
						inherited[iri] = true
						declared = append(declared, iri)
					}
				}
			}
			if len(declared) == 0 {
				continue // no claim, nothing to compare
			}
			derivedEdges := map[string][]*graph.Edge{}
			for _, e := range c.g.Out(b.ID, rel) {
				if e.Source == graph.SourceDerived {
					derivedEdges[e.O] = append(derivedEdges[e.O], e)
				}
			}
			derived := sortedKeys(derivedEdges)
			declared = uniq(declared)
			consistent := true
			// Violations: derived not covered by any declared.
			for _, d := range derived {
				dn := c.g.Node(d)
				if dn != nil && dn.Internal {
					continue // engineering-internal mechanisms realize nothing the manifest must claim
				}
				if rel == "realizes" && c.eventOnly(d) {
					continue // events are compared through publishes/subscribes, not realizes
				}
				covered := false
				for _, s := range declared {
					if Covers(c.g, s, d) {
						covered = true
						break
					}
				}
				if covered {
					continue
				}
				consistent = false
				es := derivedEdges[d]
				proposed := true
				var via []string
				path, line := "", 0
				for _, e := range es {
					if e.Prov != nil {
						if !e.Prov.Proposed {
							proposed = false
						}
						via = append(via, e.Prov.Via...)
						if path == "" {
							path, line = e.Prov.Path, e.Prov.Line
						}
					}
				}
				via = uniq(via)
				lifted := c.lifted(rel, derived)
				msg := fmt.Sprintf("%s %s %s but declares %s {%s}", b.CURIE, rel, c.g.Curie(d), rel, strings.Join(c.curies(declared), ", "))
				if len(lifted) > 0 {
					msg += fmt.Sprintf("; rollup: %s {%s} ⊄ declared", rel, strings.Join(lifted, ", "))
				}
				if proposed {
					msg += " [proposed link]"
				}
				c.add(Finding{Kind: KindViolation, Severity: "error", Boundary: b.CURIE, Relation: rel, Concept: c.g.Curie(d), Declared: c.curies(declared), Derived: c.curies(derived), Via: via, Proposed: proposed, Path: path, Line: line, Msg: msg})
			}
			// Gaps: declared not reached by any derived.
			for _, s := range declared {
				if inherited[s] {
					continue
				}
				reached := false
				for _, d := range derived {
					if Covers(c.g, s, d) || c.g.Subsumes(s, d) {
						reached = true
						break
					}
				}
				if reached {
					continue
				}
				consistent = false
				c.add(Finding{Kind: KindGap, Severity: "warning", Boundary: b.CURIE, Relation: rel, Concept: c.g.Curie(s), Declared: c.curies(declared), Derived: c.curies(derived), Path: b.File, Line: b.Line,
					Msg: fmt.Sprintf("%s declares %s %s but nothing under it %s it: intended capability not implemented, or the declaration is wrong", b.CURIE, rel, c.g.Curie(s), verb(rel))})
			}
			if consistent {
				lifted := c.lifted(rel, derived)
				c.add(Finding{Kind: KindConsistent, Severity: "info", Boundary: b.CURIE, Relation: rel, Declared: c.curies(declared), Derived: c.curies(derived),
					Msg: fmt.Sprintf("%s %s {%s} ⊑ declared {%s}", b.CURIE, rel, strings.Join(lifted, ", "), strings.Join(c.curies(declared), ", "))})
			}
		}
	}
}

// lifted summarizes a derived set with the least common subsumer when the relation lifts.
// manifestOf finds the manifest declaring a boundary CURIE.
func (c *comparer) manifestOf(curie string) *model.Manifest {
	for _, m := range c.ws.Manifests {
		if m.Boundary == curie {
			return m
		}
	}
	return nil
}

// inheritedRealizes is the union of what m's nested boundaries declare under
// realizes (following their own inherit markers), as IRIs.
func (c *comparer) inheritedRealizes(m *model.Manifest, seen map[*model.Manifest]bool) []string {
	if seen[m] {
		return nil
	}
	seen[m] = true
	set := map[string]bool{}
	for _, child := range Children(c.ws, m) {
		for _, t := range child.DeclaredSets()["realizes"] {
			iri, _ := c.g.Expand(t)
			set[iri] = true
		}
		if child.InheritsRealizes() {
			for _, iri := range c.inheritedRealizes(child, seen) {
				set[iri] = true
			}
		}
	}
	return sortedKeys(set)
}

func (c *comparer) lifted(rel string, derived []string) []string {
	r := c.ws.Vocab.Get(rel)
	var concepts []string
	for _, d := range derived {
		if n := c.g.Node(d); n != nil && n.NodeType == model.NodeConcept && !n.Internal {
			concepts = append(concepts, d)
		}
	}
	if r != nil && r.Lift == model.LiftRangeLCS && len(concepts) > 1 {
		lcs := c.g.LCS(concepts)
		if len(lcs) < len(concepts) {
			return c.curies(lcs)
		}
	}
	return c.curies(concepts)
}

func verb(rel string) string {
	switch rel {
	case "realizes":
		return "implements"
	case "produces":
		return "produces"
	case "consumes":
		return "consumes"
	case "publishes":
		return "publishes"
	case "subscribes":
		return "subscribes to"
	case "holds":
		return "represents"
	case "presents":
		return "presents"
	case "hosts":
		return "hosts"
	}
	return rel
}

// hasArtifacts reports whether any artifact links to c, or to a concept that
// realizes* c or is-a* c.
func (c *comparer) reachedByArtifact(id string) (bool, string) {
	return c.reached(id, map[string]bool{})
}

func (c *comparer) reached(id string, seen map[string]bool) (bool, string) {
	if seen[id] {
		return false, ""
	}
	seen[id] = true
	for _, e := range c.g.In(id, "") {
		if n := c.g.Node(e.S); n != nil && n.NodeType == model.NodeArtifact && e.P != "may-modify" && e.P != "instructs" && e.P != "documents" {
			return true, e.P
		}
	}
	// Through realizes (impl concepts that realize this core concept) and published-as.
	for _, p := range []string{"realizes", "published-as"} {
		for _, e := range c.g.In(id, p) {
			if e.Source == graph.SourceDerived {
				continue
			}
			if n := c.g.Node(e.S); n != nil && n.NodeType == model.NodeConcept {
				if ok, rel := c.reached(e.S, seen); ok {
					return true, rel
				}
			}
		}
	}
	// Through intended relations: a concept produced or published by a reached concept is reached.
	for _, p := range []string{"produces", "publishes"} {
		for _, e := range c.g.In(id, p) {
			if e.Source != graph.SourceDeclared {
				continue
			}
			if n := c.g.Node(e.S); n != nil && n.NodeType == model.NodeConcept {
				if ok, rel := c.reached(e.S, seen); ok {
					return true, rel
				}
			}
		}
	}
	// Through subclasses.
	for _, d := range c.g.Descendants(id) {
		for _, e := range c.g.In(d, "") {
			if n := c.g.Node(e.S); n != nil && n.NodeType == model.NodeArtifact && e.P != "may-modify" && e.P != "instructs" && e.P != "documents" {
				return true, e.P
			}
		}
	}
	// Through parts (has-part / part-of chains): a mechanism with implemented parts is implemented.
	for _, e := range c.g.In(id, "part-of") {
		if e.Source == graph.SourceDerived {
			continue
		}
		if n := c.g.Node(e.S); n != nil && n.NodeType == model.NodeConcept {
			if ok, rel := c.reached(e.S, seen); ok {
				return true, rel
			}
		}
	}
	return false, ""
}

// eventOnly reports whether a concept is an event or an implementation concept
// that realizes nothing but events (an emitter).
func (c *comparer) eventOnly(id string) bool {
	n := c.g.Node(id)
	if n == nil {
		return false
	}
	if n.Kind == "event" {
		return true
	}
	cores := RealizesStar(c.g, id)
	if len(cores) == 0 {
		return false
	}
	for _, k := range cores {
		if kn := c.g.Node(k); kn == nil || kn.Kind != "event" {
			return false
		}
	}
	return true
}

func (c *comparer) orphans() {
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if n.Imported || n.Deprecated {
			continue
		}
		if ok, _ := c.reachedByArtifact(n.ID); ok {
			continue
		}
		what := "no artifact links to it"
		if n.Tier == model.TierImpl {
			what = "implementation concept has no artifacts"
			for _, r := range c.g.Objects(n.ID, "realizes") {
				what += fmt.Sprintf("; it claims to realize %s", c.g.Curie(r))
			}
		}
		c.add(Finding{Kind: KindOrphan, Severity: "warning", Concept: n.CURIE, Path: n.File, Line: n.Line, Msg: fmt.Sprintf("%s: orphan concept, %s", n.CURIE, what)})
	}
}

func (c *comparer) rogues() {
	seen := map[string]bool{}
	for _, e := range c.g.Edges() {
		if e.Source != graph.SourceInline || e.Prov == nil {
			continue
		}
		switch {
		case strings.HasPrefix(e.O, "urn:gloss:new:"):
			cur := c.g.Curie(e.O)
			key := cur + "|" + e.Prov.Path + "|" + fmt.Sprint(e.Prov.Line)
			if seen[key] {
				continue
			}
			seen[key] = true
			c.add(Finding{ID: KindRogue + ":" + cur + ":" + e.Prov.Path + ":" + fmt.Sprint(e.Prov.Line), Kind: KindRogue, Severity: "warning", Concept: cur, Relation: e.P, Path: e.Prov.Path, Line: e.Prov.Line, Proposed: e.Prov.Proposed,
				Msg: fmt.Sprintf("%s:%d %s %s: rogue concept; becomes a decision request", e.Prov.Path, e.Prov.Line, e.P, cur)})
			ns := strings.SplitN(StripNewCurie(cur), ":", 2)[0]
			var owners []string
			if d, ok := c.ws.Config.Owns[ns]; ok && d.Owner != "" {
				owners = []string{d.Owner}
			}
			c.r.Decisions = append(c.r.Decisions, Decision{Kind: "new-concept", Concept: StripNewCurie(cur), Relation: e.P, Path: e.Prov.Path, Line: e.Prov.Line, Note: e.Prov.Note, Actor: e.Prov.Actor, Owners: owners})
		case strings.HasPrefix(e.O, "urn:gloss:unresolved:"):
			cur := c.g.Curie(e.O)
			c.add(Finding{ID: KindDangling + ":" + cur + ":" + e.Prov.Path + ":" + fmt.Sprint(e.Prov.Line), Kind: KindDangling, Severity: "error", Concept: cur, Relation: e.P, Path: e.Prov.Path, Line: e.Prov.Line,
				Msg: fmt.Sprintf("%s:%d %s %s: unresolved reference", e.Prov.Path, e.Prov.Line, e.P, cur)})
		default:
			if c.g.Node(e.O) == nil && !strings.HasPrefix(e.O, "urn:gloss:glob:") {
				cur := c.g.Curie(e.O)
				c.add(Finding{ID: KindDangling + ":" + cur + ":" + e.Prov.Path + ":" + fmt.Sprint(e.Prov.Line), Kind: KindDangling, Severity: "error", Concept: cur, Relation: e.P, Path: e.Prov.Path, Line: e.Prov.Line,
					Msg: fmt.Sprintf("%s:%d %s %s: concept does not exist (write %s:NEW:%s with @gloss? to propose it)", e.Prov.Path, e.Prov.Line, e.P, cur, nsOf(cur), nameOf(cur))})
			}
		}
	}
	// Proposed concepts: in the ontology with proposed: true, awaiting a human.
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if !n.Proposed || n.Imported {
			continue
		}
		by := ""
		if n.ProposedBy != "" {
			by = " by " + n.ProposedBy
		}
		c.add(Finding{Kind: KindProposed, Severity: "info", Concept: n.CURIE, Path: n.File, Line: n.Line, Proposed: true,
			Msg: fmt.Sprintf("%s is a proposed %s%s; accept with gloss accept --concept %s", n.CURIE, n.Kind, by, n.CURIE)})
		var owners []string
		if d, ok := c.ws.Config.Owns[n.NS]; ok && d.Owner != "" {
			owners = []string{d.Owner}
		}
		c.r.Decisions = append(c.r.Decisions, Decision{Kind: "proposed-concept", Concept: n.CURIE, Path: n.File, Line: n.Line, Note: n.Doc, Actor: n.ProposedBy, Owners: owners})
	}
	// Extension proposals: owned concepts that is-a an imported core concept are candidates for upstreaming.
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if n.Imported || n.Deprecated {
			continue
		}
		for _, p := range c.g.Objects(n.ID, "is-a") {
			if pn := c.g.Node(p); pn != nil && pn.Imported {
				c.r.Decisions = append(c.r.Decisions, Decision{Kind: "extension-upstream", Concept: n.CURIE, Relation: "is-a " + pn.CURIE, Path: n.File, Line: n.Line,
					Note: fmt.Sprintf("%s extends imported %s; if the core should have it, propose %s:%s upstream and deprecate %s with replaced-by", n.CURIE, pn.CURIE, pn.NS, nameOf(n.CURIE), n.CURIE)})
			}
		}
	}
}

// StripNewCurie turns impl:NEW:X into impl:X.
func StripNewCurie(cur string) string { return strings.Replace(cur, ":NEW:", ":", 1) }

func nsOf(cur string) string   { return strings.SplitN(cur, ":", 2)[0] }
func nameOf(cur string) string { p := strings.SplitN(cur, ":", 2); return p[len(p)-1] }

// presentedOnly reports concepts a UI surface presents while nothing
// implements them: a page with no engine behind it.
func (c *comparer) presentedOnly() {
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if n.Imported || n.Internal || n.Deprecated || n.Kind == "event" {
			continue
		}
		var by []string
		// hosts is excluded: a shell hosting a whole system-type is expected to
		// have no single implementation.
		for _, rel := range []string{"presents"} {
			for _, e := range c.g.In(n.ID, rel) {
				if e.Source == graph.SourceDerived {
					continue
				}
				if s := c.g.Node(e.S); s != nil && s.NodeType != model.NodeBoundary {
					by = append(by, c.g.Curie(e.S))
				}
			}
		}
		if len(by) == 0 || c.implemented(n.ID, 0) {
			continue
		}
		by = uniq(by)
		c.add(Finding{Kind: KindPresentedOnly, Severity: "warning", Concept: n.CURIE, Path: n.File, Line: n.Line, Via: by,
			Msg: fmt.Sprintf("%s is presented (%s) but nothing implements it: a surface with no mechanism behind it", n.CURIE, strings.Join(by, ", "))})
	}
}

func (c *comparer) untested() {
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if n.Imported || n.Internal {
			continue
		}
		if !c.implemented(n.ID, 0) || n.Kind == "event" {
			continue
		}
		if c.tested(n.ID, 0) {
			continue
		}
		msg := fmt.Sprintf("%s is implemented but nothing tests or verifies it", n.CURIE)
		if n.Tier == model.TierImpl {
			// Report at the core level: skip the impl concept when a core concept it
			// realizes is itself reported as untested.
			cores := RealizesStar(c.g, n.ID)
			skip := false
			for _, k := range cores {
				if kn := c.g.Node(k); kn != nil && kn.Kind != "event" && !kn.Imported && c.implemented(k, 0) && !c.tested(k, 0) {
					skip = true
				}
			}
			if skip {
				continue
			}
			if len(cores) > 0 {
				msg += " (nor " + c.g.Curie(cores[0]) + ", which it realizes)"
			}
		}
		c.add(Finding{Kind: KindUntested, Severity: "info", Concept: n.CURIE, Path: n.File, Line: n.Line, Msg: msg})
	}
}

// implemented reports whether an artifact implements the concept or any concept realizing it.
func (c *comparer) implemented(id string, depth int) bool {
	if depth > 8 {
		return false
	}
	for _, e := range c.g.In(id, "implements") {
		if a := c.g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact {
			return true
		}
	}
	for _, e := range c.g.In(id, "realizes") {
		if e.Source != graph.SourceDerived {
			if n := c.g.Node(e.S); n != nil && n.NodeType == model.NodeConcept && c.implemented(e.S, depth+1) {
				return true
			}
		}
	}
	return false
}

func (c *comparer) tested(id string, depth int) bool {
	if depth > 8 {
		return false
	}
	for _, p := range []string{"tests", "verifies"} {
		for _, e := range c.g.In(id, p) {
			if a := c.g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact {
				return true
			}
		}
	}
	// An implementation concept is tested when a core concept it realizes is tested directly.
	for _, core := range RealizesStar(c.g, id) {
		for _, p := range []string{"tests", "verifies"} {
			for _, e := range c.g.In(core, p) {
				if a := c.g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact {
					return true
				}
			}
		}
	}
	// A core concept is tested when something that realizes it is tested; an impl concept when a core it realizes is tested directly.
	for _, e := range c.g.In(id, "realizes") {
		if e.Source != graph.SourceDerived {
			if n := c.g.Node(e.S); n != nil && n.NodeType == model.NodeConcept && c.tested(e.S, depth+1) {
				return true
			}
		}
	}
	for _, d := range c.g.Descendants(id) {
		if c.tested(d, depth+1) {
			return true
		}
	}
	return false
}

func (c *comparer) stale() {
	for id, e := range c.opt.Ledger {
		if e.Status != model.StatusStale {
			continue
		}
		if strings.HasPrefix(id, "value:") {
			cur := strings.TrimPrefix(id, "value:")
			live := ""
			if n := c.g.NodeByCurie(cur); n != nil {
				live = n.LiveValue
			}
			c.add(Finding{ID: KindValueDrift + ":" + cur, Kind: KindValueDrift, Severity: "error", Path: e.Path, Concept: cur,
				Msg: fmt.Sprintf("published value drifted: %s publishes %q, bound to %s, which changed from %q to %q; update value: in the ontology, or confirm with gloss accept --value %s", cur, e.Value, e.Path, e.Live, live, cur)})
			continue
		}
		c.add(Finding{ID: KindStale + ":" + id, Kind: KindStale, Severity: "warning", Path: e.Path, Relation: e.Relation, Concept: strings.Join(e.Targets, ", "),
			Msg: fmt.Sprintf("%s: %s %s is stale, the anchored signature changed since it was accepted; re-confirm with gloss accept %s", e.Path, e.Relation, strings.Join(e.Targets, ", "), id)})
	}
}

func (c *comparer) metrics() {
	m := &c.r.Metrics
	m.Concepts = len(c.g.NodesOf(model.NodeConcept))
	m.Boundaries = len(c.g.NodesOf(model.NodeBoundary))
	m.Artifacts = len(c.g.NodesOf(model.NodeArtifact))
	for _, f := range c.r.Findings {
		switch f.Kind {
		case KindViolation:
			m.Violations++
		case KindGap:
			m.Gaps++
		case KindOrphan:
			m.Orphans++
		case KindRogue:
			m.Rogues++
		case KindUntested:
			m.Untested++
		case KindDangling:
			m.Dangling++
		case KindProposed:
			m.ProposedConcepts++
		}
	}
	// Review state per marker (not per target), and the files an accepted link covers.
	accepted := map[string]bool{}
	counted := map[string]bool{}
	for _, e := range c.g.Edges() {
		if e.Source != graph.SourceInline || e.Prov == nil {
			continue
		}
		m.Targets++
		if counted[e.Prov.ID] {
			continue
		}
		counted[e.Prov.ID] = true
		m.Markers++
		status := model.StatusAccepted
		if e.Prov.Proposed {
			status = model.StatusProposed
		}
		if le, ok := c.opt.Ledger[e.Prov.ID]; ok && le.Status != "" && le.Status != model.StatusRemoved {
			status = le.Status
		}
		switch status {
		case model.StatusAccepted:
			m.Accepted++
			accepted[e.Prov.Path] = true
		case model.StatusProposed:
			m.Proposed++
			if le, ok := c.opt.Ledger[e.Prov.ID]; ok {
				if t, err := time.Parse(time.RFC3339, le.Created); err == nil {
					days := int(c.opt.Now.Sub(t).Hours() / 24)
					if days > m.BacklogOldestDay {
						m.BacklogOldestDay = days
					}
				}
			}
		case model.StatusRejected:
			m.Rejected++
		case model.StatusStale:
			m.Stale++
		}
	}
	// Concept coverage: owned implementation concepts (not internal,
	// deprecated or still proposed) with an artifact and a verifier.
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if n.Imported || n.Tier != model.TierImpl || n.Internal || n.Deprecated || n.Proposed {
			continue
		}
		m.ConceptsInScope++
		if ok, _ := c.reachedByArtifact(n.ID); ok && c.tested(n.ID, 0) {
			m.ConceptsCovered++
		}
	}
	if m.ConceptsInScope > 0 {
		m.ConceptCoveragePct = 100 * float64(m.ConceptsCovered) / float64(m.ConceptsInScope)
	}
	files := c.opt.Files
	if files == nil {
		files = c.ws.Files
	}
	hasManifests := len(c.ws.Manifests) > 0
	for _, f := range files {
		if hasManifests && c.ws.ManifestFor(f) == nil {
			continue
		}
		m.InScopeFiles++
		if accepted[f] {
			m.CoveredFiles++
		}
	}
	if m.InScopeFiles > 0 {
		m.CoveragePct = 100 * float64(m.CoveredFiles) / float64(m.InScopeFiles)
	}
}

func (c *comparer) waive() {
	if c.opt.Policy == nil {
		return
	}
	for i := range c.r.Findings {
		f := &c.r.Findings[i]
		for _, w := range c.opt.Policy.Waivers {
			if w.Finding != f.ID && w.Finding != f.Concept {
				continue
			}
			if exp, err := time.Parse("2006-01-02", w.Expires); err == nil && c.opt.Now.After(exp.Add(24*time.Hour)) {
				f.Msg += fmt.Sprintf(" [waiver by %s expired %s]", w.Owner, w.Expires)
				continue
			}
			f.Waived = true
			f.Waiver = fmt.Sprintf("%s until %s", w.Owner, w.Expires)
		}
	}
}

// Failing reports the findings that fail the build for the given kinds.
func (r *Report) Failing(kinds []string) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Waived {
			continue
		}
		for _, k := range kinds {
			if f.Kind == k {
				out = append(out, f)
			}
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uniq(xs []string) []string {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return sortedKeys(m)
}

func filterEmpty(xs []string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}
