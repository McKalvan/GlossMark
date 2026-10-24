// @gloss implements gmi:DriftComparator scope=file
// @gloss produces gm:Drift, gm:Coverage, gm:DecisionRequest, gm:PlanItem scope=file
// @gloss publishes gm:DriftDetected, gm:DecisionRequested scope=file

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
	// KindOverlap: two boundaries, neither nested in the other, realize or
	// hold the same concept, and no manifest declares it a migration.
	KindOverlap = "overlap"
	// KindOverdue: a deprecated boundary past its retire-by date still has code.
	KindOverdue = "overdue"
	// KindPlanned: a planned concept, or a manifest's planned declaration,
	// that nothing implements yet: an orphan or a gap with a deadline and a
	// kinder name (ADR 0034). Never in the default --fail-on set.
	KindPlanned = "planned"
	// KindPlannedClosed: code implements the planned thing; drop planned so
	// the flag does not go stale.
	KindPlannedClosed = "planned-closed"
	// KindPlanOverdue: a plan item past its inclusive by: date.
	KindPlanOverdue = "plan-overdue"
)

// ComparedRelations are the manifest relations compared against derivation.
var ComparedRelations = []string{"realizes", "holds", "presents", "hosts", "configures", "produces", "consumes", "publishes", "subscribes"}

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
	// Intended is the set the concepts a boundary's code implements intend
	// under Relation (their produces, consumes, publishes, subscribes),
	// kept apart from Derived, which is the code's own (ADR 0026).
	Intended []string `json:"intended,omitempty"`
	Via      []string `json:"via,omitempty"`
	// Lifted lists, for a violation, the uncovered concepts that are only
	// derived because Concept lifted them (the wholes above a part, the cores
	// it realizes); they are not reported again.
	Lifted   []string `json:"lifted,omitempty"`
	Proposed bool     `json:"proposed,omitempty"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	// By and Ticket are a plan item's deadline and tracker reference.
	By     string `json:"by,omitempty"`
	Ticket string `json:"ticket,omitempty"`
	Msg    string `json:"msg"`
	Waived bool   `json:"waived,omitempty"`
	Waiver string `json:"waiver,omitempty"`
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
// coverage: implementation concepts, and core concepts code implements
// directly, that have at least one artifact and one verifier (n/a when there
// are none). File coverage (files carrying an accepted link) is secondary,
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
	// PlanItems counts the open plan items (planned and plan-overdue).
	PlanItems        int `json:"plan_items"`
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
	c.flushOmissions()
	c.orphans()
	c.plans()
	c.rogues()
	c.untested()
	c.presentedOnly()
	c.overlaps()
	c.overdue()
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
	case KindValueDrift, KindOverdue, KindPlannedClosed, KindPlanOverdue:
		return 1
	case KindOverlap:
		return 8
	}
	return 9
}

// Migrating reports whether boundary a is declared as being replaced by b,
// or b by a: deprecated: true with replaced-by naming the other.
func Migrating(a, b *graph.Node) bool {
	return (a.Deprecated && a.ReplacedBy == b.CURIE) || (b.Deprecated && b.ReplacedBy == a.CURIE)
}

// overlaps reports each concept that two or more unrelated boundaries
// realize or declare they hold, unless every one of them is linked to another
// by a declared migration (deprecated + replaced-by), which says the overlap is
// intended and ends when the old side is deleted.
func (c *comparer) overlaps() {
	for _, rel := range []string{"realizes", "holds"} {
		sets := OwnRealizers(c.g, rel)
		if rel == "holds" {
			sets = Holders(c.g)
		}
		names := make([]string, 0, len(sets))
		for k := range sets {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, cu := range names {
			bs := sets[cu]
			if len(bs) < 2 {
				continue
			}
			nodes := make([]*graph.Node, len(bs))
			for i, b := range bs {
				nodes[i] = c.g.NodeByCurie(b)
			}
			// Union the boundaries joined by a declared migration.
			comp := make([]int, len(nodes))
			for i := range comp {
				comp[i] = i
			}
			var find func(int) int
			find = func(i int) int {
				for comp[i] != i {
					i = comp[i]
				}
				return i
			}
			for i := range nodes {
				for j := i + 1; j < len(nodes); j++ {
					if nodes[i] != nil && nodes[j] != nil && Migrating(nodes[i], nodes[j]) {
						comp[find(i)] = find(j)
					}
				}
			}
			one := true
			for i := range nodes {
				one = one && find(i) == find(0)
			}
			if one {
				continue
			}
			verb := "realized"
			if rel == "holds" {
				verb = "held"
			}
			path, line := "", 0
			if nodes[0] != nil {
				path, line = nodes[0].File, nodes[0].Line
			}
			c.add(Finding{Kind: KindOverlap, Severity: "info", Relation: rel, Concept: cu, Derived: bs, Path: path, Line: line,
				Msg: fmt.Sprintf("%s is %s by %s; if one replaces the other, mark the old manifest deprecated: true with replaced-by: and retire-by:", cu, verb, andList(bs))})
		}
	}
}

// overdue reports each deprecated boundary whose retire-by date has passed
// while code still sits under it.
func (c *comparer) overdue() {
	contained := map[string]int{}
	for _, a := range c.g.NodesOf(model.NodeArtifact) {
		if a.Boundary == "" {
			continue
		}
		contained[a.Boundary]++
		for _, b := range c.g.PartOfChain(a.Boundary) {
			contained[b]++
		}
	}
	for _, b := range c.g.NodesOf(model.NodeBoundary) {
		if !b.Deprecated || b.RetireBy == "" || contained[b.ID] == 0 {
			continue
		}
		due, err := time.Parse(WaiverDate, b.RetireBy)
		if err != nil || !WaiverExpired(due, c.opt.Now) {
			continue
		}
		var still []string
		for _, rel := range []string{"realizes", "holds"} {
			for _, e := range c.g.Out(b.ID, rel) {
				if e.Source == graph.SourceDerived {
					still = append(still, c.g.Curie(e.O))
				}
			}
		}
		sort.Strings(still)
		still = uniqStrings(still)
		msg := fmt.Sprintf("%s was due to retire by %s", b.CURIE, b.RetireBy)
		if b.ReplacedBy != "" {
			msg += " (replaced-by " + b.ReplacedBy + ")"
		}
		msg += fmt.Sprintf(" and still has %d artifact(s)", contained[b.ID])
		if len(still) > 0 {
			msg += "; it still realizes or holds " + strings.Join(still, ", ")
		}
		c.add(Finding{Kind: KindOverdue, Severity: "error", Boundary: b.CURIE, Derived: still, Path: b.File, Line: b.Line, Msg: msg})
	}
}

// plans reports each owned planned concept as a plan item: open (nothing
// implements it; a verifies link alone keeps it open, listed as verified,
// not implemented), overdue once its by: day has ended, or closed, once an
// implements or represents link reaches it directly, through a concept that
// realizes it, or through a subtype (ADR 0034 section 2).
func (c *comparer) plans() {
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if !n.Planned || n.Imported {
			continue
		}
		f := Finding{Concept: n.CURIE, Path: n.File, Line: n.Line, By: n.PlanBy, Ticket: n.PlanTicket}
		plan := planText(n.PlanBy, n.PlanTicket)
		switch {
		case c.planClosed(n.ID):
			f.Kind, f.Severity = KindPlannedClosed, "warning"
			f.Msg = fmt.Sprintf("%s is implemented: drop planned (gloss accept --planned %s)", n.CURIE, n.CURIE)
		case c.planOverdue(n.PlanBy):
			f.Kind, f.Severity = KindPlanOverdue, "error"
			f.Msg = fmt.Sprintf("planned concept %s was due by %s%s and nothing implements it yet", n.CURIE, n.PlanBy, ticketText(n.PlanTicket))
		default:
			f.Kind, f.Severity = KindPlanned, "info"
			f.Msg = fmt.Sprintf("planned concept %s%s: nothing implements it yet", n.CURIE, plan)
		}
		if f.Kind != KindPlannedClosed && c.tested(n.ID, 0) {
			f.Msg = strings.Replace(f.Msg, "nothing implements it yet", "verified, not implemented", 1)
		}
		c.add(f)
	}
}

// PlanClosed reports whether the implementation has reached a planned
// concept (see planClosed): gloss accept --planned warns when it has not.
func PlanClosed(g *graph.Graph, id string) bool { return (&comparer{g: g}).planClosed(id) }

// planClosed reports whether the implementation has reached a planned
// concept: an artifact implements or represents it, a concept realizing it,
// or something below it in is-a. verifies and the other links do not close.
func (c *comparer) planClosed(id string) bool {
	for _, x := range append([]string{id}, c.g.Descendants(id)...) {
		if c.implemented(x, 0) || c.represented(x) {
			return true
		}
	}
	return false
}

// planOverdue reports whether a plan's by: day has ended (inclusive, like a
// waiver's expires). A plan without a date is never overdue.
func (c *comparer) planOverdue(by string) bool {
	if by == "" {
		return false
	}
	due, err := time.Parse(model.PlanDate, by)
	return err == nil && WaiverExpired(due, c.opt.Now)
}

// planText renders a plan's deadline and ticket for a message: " (by
// 2026-11-30, https://...)" or "".
func planText(by, ticket string) string {
	p := (&model.Plan{By: by, Ticket: ticket}).String()
	if p == "" {
		return ""
	}
	return " (" + p + ")"
}

func ticketText(ticket string) string {
	if ticket == "" {
		return ""
	}
	return " (" + ticket + ")"
}

// plannedDeclaration reports one planned manifest declaration: open until
// code under the boundary derives it under the gap's subsumption rule, then
// closed, or overdue once the block's by: day has ended.
func (c *comparer) plannedDeclaration(b *graph.Node, rel, s string, reached bool, declared, derived []string, intended []string) {
	by := ""
	if m := c.manifestOf(b.CURIE); m != nil && m.Planned != nil {
		by = m.Planned.By
	}
	cur := c.g.Curie(s)
	f := Finding{Boundary: b.CURIE, Relation: rel, Concept: cur, Declared: c.curies(declared), Derived: c.curies(derived), Intended: intended, Path: b.File, Line: b.Line, By: by}
	switch {
	case reached:
		f.Kind, f.Severity = KindPlannedClosed, "warning"
		f.Msg = fmt.Sprintf("%s %s %s: move it from planned to %s in %s", b.CURIE, verb(rel), cur, rel, b.File)
	case c.planOverdue(by):
		f.Kind, f.Severity = KindPlanOverdue, "error"
		f.Msg = fmt.Sprintf("%s planned to %s %s by %s and no code under it %s it yet", b.CURIE, planVerb(rel), cur, by, verb(rel))
	default:
		f.Kind, f.Severity = KindPlanned, "info"
		f.Msg = fmt.Sprintf("%s plans to %s %s%s: no code under it %s it yet", b.CURIE, planVerb(rel), cur, planText(by, ""), verb(rel))
	}
	c.add(f)
}

// planVerb is verb(rel) in the infinitive: "plans to implement".
func planVerb(rel string) string {
	switch rel {
	case "realizes":
		return "implement"
	case "produces":
		return "produce"
	case "consumes":
		return "consume"
	case "publishes":
		return "publish"
	case "subscribes":
		return "subscribe to"
	}
	return rel
}

func andList(xs []string) string {
	if len(xs) < 2 {
		return strings.Join(xs, "")
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

type comparer struct {
	g   *graph.Graph
	ws  *load.Workspace
	opt Options
	r   *Report
	// realizers maps a core concept to the concepts that realize it (lazy).
	realizers map[string]map[string]bool
	// omissions: per boundary IRI and relation, what its code derives under
	// it while it declares nothing, found by its inheriting ancestors.
	omissions map[string]*omission
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
			planned := map[string]bool{}
			for _, e := range c.g.Out(b.ID, rel) {
				if e.Source == graph.SourceDeclared {
					declared = append(declared, e.O)
					if e.Prov != nil && e.Prov.Rule == graph.RulePlanned {
						planned[e.O] = true
					}
				}
			}
			// inherit means "what my nested boundaries declare" (realizes,
			// holds and the four flows, ADR 0026); their gaps are theirs to
			// report, so inherited entries only cover.
			inherited := map[string]bool{}
			if m := c.manifestOf(b.CURIE); m != nil && m.Inherits(rel) {
				for _, iri := range c.inheritedSet(m, rel, map[*model.Manifest]bool{}) {
					inherited[iri] = true
					declared = append(declared, iri)
				}
			}
			// An explicit empty list (realizes: []) is a claim of none: every
			// derived edge is a violation. A relation not written is no claim.
			none := c.manifestOf(b.CURIE).DeclaresNone(rel)
			if len(declared) == 0 && !none {
				continue // no claim, nothing to compare
			}
			// What the code derives, and apart from it what the concepts the
			// code implements intend: intent is compared with the manifest but
			// never covers a gap (ADR 0026).
			derivedEdges := map[string][]*graph.Edge{}
			intendedEdges := map[string][]*graph.Edge{}
			// holds is declared only; the code's side is what it represents
			// (ADR 0037).
			codeRel := rel
			if rel == "holds" {
				codeRel = "represents"
			}
			for _, e := range c.g.Out(b.ID, codeRel) {
				switch {
				case e.Source != graph.SourceDerived:
				case e.Prov != nil && e.Prov.Rule == RuleIntended:
					intendedEdges[e.O] = append(intendedEdges[e.O], e)
				default:
					derivedEdges[e.O] = append(derivedEdges[e.O], e)
				}
			}
			derived := sortedKeys(derivedEdges)
			intended := c.curies(sortedKeys(intendedEdges))
			declared = uniq(declared)
			consistent := true
			var violations []Finding
			// Violations: derived not covered by any declared.
			uncovered := map[string]bool{}
			for _, d := range derived {
				dn := c.g.Node(d)
				if dn != nil && dn.Internal {
					continue // engineering-internal mechanisms realize nothing the manifest must claim
				}
				if rel == "realizes" && c.eventOnly(d) {
					continue // events are compared through publishes/subscribes, not realizes
				}
				if rel == "holds" {
					continue // representing a concept is no claim to own it: a DTO or a cache is not a violation
				}
				covered := false
				for _, s := range declared {
					if Covers(c.g, s, d) {
						covered = true
						break
					}
				}
				if covered || c.wholeOfCovered(d, derivedEdges, declared) {
					continue
				}
				uncovered[d] = true
			}
			if len(uncovered) > 0 {
				consistent = false
			}
			// One finding per cause (ADR 0010): a concept that is only there
			// because an uncovered concept lifted it is listed under that
			// concept, not reported again.
			roots, liftedBy, own := c.roots(uncovered, derivedEdges, declared)
			for _, d := range roots {
				es := own[d]
				var lifted []string
				for _, l := range sortedKeys(liftedBy[d]) {
					lifted = append(lifted, c.g.Curie(l))
				}
				// With inherit, the omission belongs to the nested boundary
				// whose code derives the concept: report it there once.
				if causes := c.causes(b, rel, es); !causes[b.ID] {
					for x := range causes {
						c.omitted(x, rel, b, d, lifted, es)
					}
					continue
				}
				proposed := true
				var via []string
				path, line := "", 0
				for _, e := range es {
					if e.Prov != nil {
						if !e.Prov.Proposed {
							proposed = false
						}
						via = append(via, e.Prov.Via...)
						if path == "" || len(e.Prov.Via) == 1 {
							path, line = e.Prov.Path, e.Prov.Line
						}
					}
				}
				via = uniq(via)
				msg := fmt.Sprintf("%s %s %s%s but declares %s {%s}", b.CURIE, rel, c.g.Curie(d), liftsTo(lifted), rel, strings.Join(c.curies(declared), ", "))
				if len(declared) == 0 {
					msg = fmt.Sprintf("%s %s %s%s, declared %s []: the manifest says this boundary %s nothing", b.CURIE, rel, c.g.Curie(d), liftsTo(lifted), rel, verb(rel))
				} else if c.summarizes(rel) {
					msg += fmt.Sprintf("; rollup: %s %s ⊄ declared", rel, c.summary(rel, derived, true))
				}
				if proposed {
					msg += " [proposed link]"
				}
				violations = append(violations, Finding{Kind: KindViolation, Severity: "error", Boundary: b.CURIE, Relation: rel, Concept: c.g.Curie(d), Lifted: lifted, Declared: c.curies(declared), Derived: c.curies(derived), Intended: intended, Via: via, Proposed: proposed, Path: path, Line: line, Msg: msg})
			}
			// Intent the manifest does not account for: neither a declared
			// concept covers it nor does the manifest declare a refinement of
			// it (a subtype, a part, a concept that realizes it), so a
			// container may declare publishes: [MotorClaimFiled] where the
			// concept it implements publishes ClaimFiled (ADR 0026).
			for _, i := range sortedKeys(intendedEdges) {
				es := intendedEdges[i]
				if derivedEdges[i] != nil || c.intentFits(i, declared) {
					continue
				}
				// Under inherit, a nested boundary's intent is its own to
				// compare, or nobody's when it declares nothing.
				if causes := c.causes(b, rel, es); !causes[b.ID] {
					continue
				}
				consistent = false
				f := Finding{Kind: KindViolation, Severity: "error", Boundary: b.CURIE, Relation: rel, Concept: c.g.Curie(i), Declared: c.curies(declared), Derived: c.curies(derived), Intended: intended, Proposed: true}
				for _, e := range es {
					if e.Prov != nil {
						f.Via = append(f.Via, e.Prov.Via...)
						f.Proposed = f.Proposed && e.Prov.Proposed
						if f.Path == "" {
							f.Path, f.Line = e.Prov.Path, e.Prov.Line
						}
					}
				}
				f.Via = uniq(f.Via)
				f.Msg = fmt.Sprintf("%s %s %s by intent (%s), but declares %s {%s}: declare it or a refinement of it, or change the concept", b.CURIE, rel, f.Concept, c.intender(es), rel, strings.Join(f.Declared, ", "))
				if f.Proposed {
					f.Msg += " [proposed link]"
				}
				c.add(f) // printed whole: it does not share the code's sets line
			}
			// The sets are printed once per boundary and relation: on the
			// first violation in report order; the others name the concept.
			sort.SliceStable(violations, func(i, j int) bool { return violations[i].Concept < violations[j].Concept })
			for i, f := range violations {
				if i > 0 {
					f.Msg = fmt.Sprintf("%s %s %s%s, which its declared %s does not cover (declared and derived sets: see the %s line)", f.Boundary, rel, f.Concept, liftsTo(f.Lifted), rel, violations[0].Concept)
					if f.Proposed {
						f.Msg += " [proposed link]"
					}
				}
				c.add(f)
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
				if planned[s] {
					// A planned declaration is a plan item, not a gap, and
					// closes under the same rule (ADR 0034).
					c.plannedDeclaration(b, rel, s, reached, declared, derived, intended)
					continue
				}
				if reached {
					continue
				}
				consistent = false
				msg := fmt.Sprintf("%s declares %s %s but nothing under it %s it: intended capability not implemented, or the declaration is wrong", b.CURIE, rel, c.g.Curie(s), verb(rel))
				if es := c.intentFor(s, intendedEdges); es != nil {
					// Only a concept's intent reaches it: the code that does
					// is missing, or unmarked (ADR 0026).
					msg = fmt.Sprintf("%s declares %s %s and %s, but no code under it %s it: mark the code that does, or the declaration is wrong", b.CURIE, rel, c.g.Curie(s), c.intender(es), verb(rel))
				} else if other := c.reachedOtherwise(b.ID, rel, s); other != "" {
					msg = fmt.Sprintf("%s declares %s %s but nothing under it %s it; its code %s it instead: declare %s, or change the marker", b.CURIE, rel, c.g.Curie(s), verb(rel), verb(other), other)
				} else if links := c.unlifted(b.ID, s); len(links) > 0 {
					// The code links to it, but the links do not lift to rel:
					// usually the concept's kind (a retype upstream).
					msg = fmt.Sprintf("%s declares %s %s, but the links to it under it do not lift to %s: %s", b.CURIE, rel, c.g.Curie(s), rel, strings.Join(links, "; "))
					if why := KindMismatch(c.g, rel, s); why != "" {
						msg += " (" + why + ")"
					}
				}
				c.add(Finding{Kind: KindGap, Severity: "warning", Boundary: b.CURIE, Relation: rel, Concept: c.g.Curie(s), Declared: c.curies(declared), Derived: c.curies(derived), Intended: intended, Path: b.File, Line: b.Line, Msg: msg})
			}
			if consistent {
				c.add(Finding{Kind: KindConsistent, Severity: "info", Boundary: b.CURIE, Relation: rel, Declared: c.curies(declared), Derived: c.curies(derived), Intended: intended,
					Msg: fmt.Sprintf("%s %s %s ⊑ declared {%s}", b.CURIE, rel, c.summary(rel, derived, false), strings.Join(c.curies(declared), ", "))})
			}
		}
	}
}

// unlifted lists the inline links from artifacts under boundary b to concept
// s, as "path:line relation", for a gap the code links to but does not reach.
func (c *comparer) unlifted(b, s string) []string {
	var out []string
	for _, e := range c.g.Edges() {
		// A test of the concept is not an attempt to provide it.
		if e.O != s || e.Source != graph.SourceInline || e.Prov == nil || e.P == "verifies" || e.P == "tests" {
			continue
		}
		a := c.g.Node(e.S)
		if a == nil || a.NodeType != model.NodeArtifact || a.Boundary == "" {
			continue
		}
		under := a.Boundary == b
		for _, anc := range c.g.PartOfChain(a.Boundary) {
			under = under || anc == b
		}
		if under {
			out = append(out, fmt.Sprintf("%s:%d %s", e.Prov.Path, e.Prov.Line, e.P))
		}
	}
	sort.Strings(out)
	return uniqStrings(out)
}

// KindMismatch explains why a manifest relation cannot be satisfied for a
// concept of the wrong kind: publishes and subscribes take events (holds
// takes any kind, ADR 0013). Empty when the kind fits.
func KindMismatch(g *graph.Graph, rel, iri string) string {
	n := g.Node(iri)
	if n == nil || n.NodeType != model.NodeConcept || n.Kind == "" {
		return ""
	}
	want := map[string]string{"publishes": "event", "subscribes": "event"}[rel]
	if want == "" || n.Kind == want {
		return ""
	}
	plural := map[string]string{"event": "events"}[want]
	return fmt.Sprintf("%s is a %s; %s takes %s", n.CURIE, n.Kind, rel, plural)
}

func uniqStrings(xs []string) []string {
	var out []string
	for i, x := range xs {
		if i == 0 || x != xs[i-1] {
			out = append(out, x)
		}
	}
	return out
}

// reachedOtherwise returns the first other compared relation through which
// boundary b's code reaches declared concept s, or "": a manifest declaring
// presents hp:Pricing over code that implements it gets a gap that says so.
func (c *comparer) reachedOtherwise(b, rel, s string) string {
	for _, other := range ComparedRelations {
		if other == rel {
			continue
		}
		for _, e := range c.g.Out(b, other) {
			if e.Source == graph.SourceDerived && !intent(e) && (Covers(c.g, s, e.O) || c.g.Subsumes(s, e.O)) {
				return other
			}
		}
	}
	return ""
}

// intent reports whether a derived edge is a concept's intent, not the code's.
func intent(e *graph.Edge) bool { return e.Prov != nil && e.Prov.Rule == RuleIntended }

// intentFits reports whether the manifest accounts for intended concept i:
// a declared concept covers it, or the manifest declares a refinement of it
// (i covers the declared concept: a subtype, a part, a concept realizing
// it). A container implementing claim:ClaimFiling, which publishes
// claim:ClaimFiled, may declare only the claim:MotorClaimFiled it publishes.
func (c *comparer) intentFits(i string, declared []string) bool {
	for _, s := range declared {
		if Covers(c.g, s, i) || Covers(c.g, i, s) {
			return true
		}
	}
	return false
}

// intentFor returns the intended edges that reach declared concept s, or nil.
func (c *comparer) intentFor(s string, intended map[string][]*graph.Edge) []*graph.Edge {
	var out []*graph.Edge
	for _, i := range sortedKeys(intended) {
		if Covers(c.g, s, i) {
			out = append(out, intended[i]...)
		}
	}
	return out
}

// intender names the concepts whose own relation an intended edge came
// from: the last concept its derivation passed through.
func (c *comparer) intender(es []*graph.Edge) string {
	var by []string
	for _, e := range es {
		if e.Prov != nil && len(e.Prov.Via) >= 2 {
			by = append(by, e.Prov.Via[len(e.Prov.Via)-1])
		}
	}
	sort.Strings(by)
	by = uniqStrings(by)
	if len(by) == 1 {
		return by[0] + " intends it"
	}
	return andList(by) + " intend it"
}

// roots splits uncovered derived concepts into the ones reported on their
// own (some code links to them, or they lift from a concept that is neither
// uncovered nor a covered part of them) and the ones only there because an
// uncovered concept lifted them: liftedBy maps a root to those. One marker
// on a part five levels down is one finding that lists the four wholes
// above it, not five findings. own holds each root's edges that make it one.
func (c *comparer) roots(uncovered map[string]bool, derived map[string][]*graph.Edge, declared []string) ([]string, map[string]map[string]bool, map[string][]*graph.Edge) {
	liftedBy := map[string]map[string]bool{}
	own := map[string][]*graph.Edge{}
	var roots []string
	for _, d := range sortedKeys(uncovered) {
		from := map[string]bool{}
		for _, e := range derived[d] {
			if e.Prov != nil && len(e.Prov.Via) >= 2 {
				if o, _ := c.g.Expand(e.Prov.Via[1]); o != d && uncovered[o] {
					from[o] = true
					continue
				}
			}
			if c.edgeWholeOfCovered(d, e, derived, declared) {
				continue // lifted from a covered part: that edge is consistent
			}
			own[d] = append(own[d], e)
		}
		if len(own[d]) > 0 || len(from) == 0 {
			if len(own[d]) == 0 {
				own[d] = derived[d]
			}
			roots = append(roots, d)
			continue
		}
		for o := range from {
			if liftedBy[o] == nil {
				liftedBy[o] = map[string]bool{}
			}
			liftedBy[o][d] = true
		}
	}
	return roots, liftedBy, own
}

// causes returns, for an uncovered concept derived under rel at boundary b
// through edges es, the boundaries the finding belongs to. Walking down from
// b toward the code, every boundary that declares rel: inherit passes the
// question to the nested one; the first that does not is the cause. b itself
// when b does not inherit. A cause that declares rel reports the concept in
// its own comparison and is left out; one that declares nothing is where the
// declaration was omitted (ADR 0010, for every inheritable relation since
// ADR 0026).
func (c *comparer) causes(b *graph.Node, rel string, es []*graph.Edge) map[string]bool {
	out := map[string]bool{}
	for _, e := range es {
		x := b.ID
		if e.Prov != nil && e.Prov.Path != "" {
			if m := c.ws.ManifestFor(e.Prov.Path); m != nil {
				if a, _ := c.g.Expand(m.Boundary); a != "" {
					chain := append([]string{a}, c.g.PartOfChain(a)...)
					i := -1
					for j, id := range chain {
						if id == b.ID {
							i = j
							break
						}
					}
					for i > 0 {
						bm := c.manifestOf(c.g.Curie(chain[i]))
						if bm == nil || !bm.Inherits(rel) {
							break
						}
						i--
					}
					if i >= 0 {
						x = chain[i]
					}
				}
			}
		}
		if x != b.ID {
			if xm := c.manifestOf(c.g.Curie(x)); xm == nil || len(c.declaredRel(xm, rel)) > 0 {
				continue // x compares the concept itself
			}
		}
		out[x] = true
	}
	return out
}

// declaredRel is what m declares under rel, inherited entries included.
func (c *comparer) declaredRel(m *model.Manifest, rel string) []string {
	out := append([]string{}, m.DeclaredSets()[rel]...)
	if m.Inherits(rel) {
		out = append(out, c.inheritedSet(m, rel, map[*model.Manifest]bool{})...)
	}
	return out
}

// omission collects what one boundary's code derives under a relation
// while it declares nothing, as its inheriting ancestors find it.
type omission struct {
	boundary, rel     string
	roots, lifted, at map[string]bool
	via               []string
	proposed          bool
}

func (c *comparer) omitted(x, rel string, at *graph.Node, d string, lifted []string, es []*graph.Edge) {
	if c.omissions == nil {
		c.omissions = map[string]*omission{}
	}
	key := x + "|" + rel
	o := c.omissions[key]
	if o == nil {
		o = &omission{boundary: x, rel: rel, roots: map[string]bool{}, lifted: map[string]bool{}, at: map[string]bool{}, proposed: true}
		c.omissions[key] = o
	}
	o.roots[c.g.Curie(d)] = true
	for _, l := range lifted {
		o.lifted[l] = true
	}
	o.at[at.CURIE] = true
	for _, e := range es {
		if e.Prov != nil {
			o.via = append(o.via, e.Prov.Via...)
			if !e.Prov.Proposed {
				o.proposed = false
			}
		}
	}
}

// flushOmissions reports each omission once, at the boundary that omitted
// the declaration, naming its manifest and the ancestors that found it.
func (c *comparer) flushOmissions() {
	for _, k := range sortedKeys(c.omissions) {
		o := c.omissions[k]
		n, rel := c.g.Node(o.boundary), o.rel
		if n == nil {
			continue
		}
		roots := sortedKeys(o.roots)
		var lifted []string
		for _, l := range sortedKeys(o.lifted) {
			if !o.roots[l] {
				lifted = append(lifted, l)
			}
		}
		it := "it"
		if len(roots)+len(lifted) > 1 {
			it = "them"
		}
		declares := "declares no " + rel
		file := n.File
		if m := c.manifestOf(n.CURIE); m != nil {
			file = m.Path
			if m.Inherits(rel) {
				declares = "declares only " + rel + ": inherit, which covers none of " + it
			}
		}
		at := sortedKeys(o.at)
		verb := "inherits"
		if len(at) > 1 {
			verb = "inherit"
		}
		msg := fmt.Sprintf("%s %s %s but %s; %s %s from it: declare %s under %s in %s: {%s}%s", n.CURIE, rel, countNoun(len(roots)+len(lifted), "concept"), declares, strings.Join(at, ", "), verb, it, rel, file, strings.Join(roots, ", "), liftsTo(lifted))
		if o.proposed {
			msg += " [proposed link]"
		}
		c.add(Finding{Kind: KindViolation, Severity: "error", Boundary: n.CURIE, Relation: rel, Concept: strings.Join(roots, ", "), Lifted: lifted, Derived: append(append([]string{}, roots...), lifted...), Via: uniq(o.via), Proposed: o.proposed, Path: file, Line: 1, Msg: msg})
	}
}

// liftsTo describes the concepts a finding's concept lifted, naming at most three.
func liftsTo(lifted []string) string {
	switch {
	case len(lifted) == 0:
		return ""
	case len(lifted) <= 3:
		return " (lifting " + strings.Join(lifted, ", ") + ")"
	}
	return fmt.Sprintf(" (lifting %s and %d more)", strings.Join(lifted[:3], ", "), len(lifted)-3)
}

// wholeOfCovered reports whether derived concept d is only there because a
// covered part lifted it: every derived edge to d came through a derived
// concept o that d is a whole of (or what such a whole realizes), and o is
// covered. Code implementing a declared part makes the boundary realize the
// whole too; that lift is the part's, not a claim on the whole. Code that
// names the whole itself is not excused (ADR 0009).
func (c *comparer) wholeOfCovered(d string, derived map[string][]*graph.Edge, declared []string) bool {
	es := derived[d]
	if len(es) == 0 {
		return false
	}
	for _, e := range es {
		if !c.edgeWholeOfCovered(d, e, derived, declared) {
			return false
		}
	}
	return true
}

// edgeWholeOfCovered reports whether one derived edge to d came through a
// covered derived concept that d is a whole of: that edge is the part's.
func (c *comparer) edgeWholeOfCovered(d string, e *graph.Edge, derived map[string][]*graph.Edge, declared []string) bool {
	if e.Prov == nil || len(e.Prov.Via) < 2 {
		return false
	}
	o, _ := c.g.Expand(e.Prov.Via[1])
	if o == d || derived[o] == nil || !has(RealizesStar(c.g, o), d) || has(realizedBy(c.g, o), d) {
		return false
	}
	for _, s := range declared {
		if Covers(c.g, s, o) {
			return true
		}
	}
	return false
}

func has(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
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

// inheritedSet is the union of what m's nested boundaries declare under rel
// (following their own inherit markers), as IRIs.
func (c *comparer) inheritedSet(m *model.Manifest, rel string, seen map[*model.Manifest]bool) []string {
	if seen[m] {
		return nil
	}
	seen[m] = true
	set := map[string]bool{}
	for _, child := range Children(c.ws, m) {
		for _, t := range child.DeclaredSets()[rel] {
			iri, _ := c.g.Expand(t)
			set[iri] = true
		}
		if child.Inherits(rel) {
			for _, iri := range c.inheritedSet(child, rel, seen) {
				set[iri] = true
			}
		}
	}
	return sortedKeys(set)
}

// summarizes reports whether a relation's derived sets are summarized by
// their least common subsumer in drift output (lift: range-lcs, the default
// for produces, consumes, publishes and subscribes; lift: none turns it
// off). Coverage does not depend on it: a declared supertype always covers
// a derived subtype (ADR 0011).
func (c *comparer) summarizes(rel string) bool {
	r := c.ws.Vocab.Get(rel)
	return r != nil && r.Lift == model.LiftRangeLCS
}

// summary renders a derived set for a drift line: "{members}", or, when the
// relation summarizes, each maximal subset that shares a subsumer replaced
// by its least common subsumer (ADR 0026): "{LCS}" when the whole set has
// one, "{cl:Assessment, cl:Claim, cl:Payout}" when only the claim subtypes
// do. With from set, a violation names what was really derived: "(from
// members)" for a whole-set lift, "(cl:Claim from a, b; ...)" for a partial
// one. Members that share several incomparable minimal subsumers stay as
// they are and the line ends "(no unique subsumer: a, b)", so a lift that
// was tried and declined is visible.
func (c *comparer) summary(rel string, derived []string, from bool) string {
	var concepts []string
	for _, d := range derived {
		if n := c.g.Node(d); n != nil && n.NodeType == model.NodeConcept && !n.Internal {
			concepts = append(concepts, d)
		}
	}
	members := "{" + strings.Join(c.curies(concepts), ", ") + "}"
	if !c.summarizes(rel) || len(concepts) < 2 {
		return members
	}
	groups := c.g.LiftGroups(concepts)
	var parts, froms, candidates []string
	for _, gr := range groups {
		if gr.Subsumer == "" {
			parts = append(parts, gr.Members...)
			candidates = append(candidates, gr.Candidates...)
			continue
		}
		parts = append(parts, gr.Subsumer)
		froms = append(froms, c.g.Curie(gr.Subsumer)+" from "+strings.Join(c.curies(gr.Members), ", "))
	}
	out := "{" + strings.Join(c.curies(parts), ", ") + "}"
	switch {
	case len(groups) == 1 && len(froms) == 1 && from:
		out += " (from " + strings.Join(c.curies(concepts), ", ") + ")"
	case len(froms) > 0 && from:
		sort.Strings(froms)
		out += " (" + strings.Join(froms, "; ") + ")"
	}
	if candidates = uniq(c.curies(candidates)); len(candidates) > 1 {
		out += " (no unique subsumer: " + strings.Join(candidates, ", ") + ")"
	}
	return out
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
	case "configures":
		return "configures"
	}
	return rel
}

// reachedByArtifact reports whether code is built for a concept: an artifact
// links to it other than to document, instruct or remit it, or to a concept
// that realizes it, is published as it, is below it in is-a or part of it
// (the orphan rule, ADR 0028). A flow a reached concept declares in the
// ontology is intent and does not reach (ADR 0026).
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
	// Through subclasses, by any route: an abstract supertype is built when
	// something below it is, whether code links the subtype directly or
	// implements a concept that realizes it (ADR 0010).
	for _, d := range c.g.Descendants(id) {
		if ok, rel := c.reached(d, seen); ok {
			return true, rel
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
		if n.Imported || n.Deprecated || n.Planned {
			continue // a planned concept is a plan item, not an orphan (ADR 0025 6.1)
		}
		if ok, _ := c.reachedByArtifact(n.ID); ok {
			continue
		}
		if n.ValueFrom != "" {
			continue // value-from binds it to a line of code
		}
		what := "no code links to it (a documents or instructs link does not count)"
		if n.Tier == model.TierImpl {
			what = "implementation concept has no artifacts"
			for _, r := range c.g.Objects(n.ID, "realizes") {
				what += fmt.Sprintf("; it claims to realize %s", c.g.Curie(r))
			}
		}
		c.add(Finding{Kind: KindOrphan, Severity: "warning", Concept: n.CURIE, Path: n.File, Line: n.Line, Msg: fmt.Sprintf("%s: orphan concept, %s", n.CURIE, what)})
	}
}

// danglingDeclared reports unresolved concept references that markers do
// not carry: relation keys in manifests and in the workspace's own ontology
// files, and allowed: entries, at the file and line gloss check reports.
func (c *comparer) danglingDeclared() {
	for _, e := range c.g.Edges() {
		if e.Source != graph.SourceDeclared || e.Prov == nil || strings.HasPrefix(e.O, "urn:gloss:glob:") {
			continue
		}
		sn := c.g.Node(e.S)
		if sn == nil || (sn.NodeType == model.NodeConcept && sn.Imported) || (sn.NodeType != model.NodeConcept && sn.NodeType != model.NodeBoundary) {
			continue
		}
		if c.g.Node(e.O) != nil || (sn.NodeType == model.NodeBoundary && (e.P == "part-of" || e.P == "replaced-by")) {
			continue
		}
		// A successor in a namespace this workspace does not load is checked
		// where it is loaded: in its own repository, or in gloss landscape.
		if e.P == "replaced-by" && c.ws.Ontologies[nsOf(c.g.Curie(e.O))] == nil {
			continue
		}
		cur := c.g.Curie(e.O)
		if strings.HasPrefix(e.O, "urn:gloss:unresolved:") {
			cur = strings.TrimPrefix(e.O, "urn:gloss:unresolved:")
		}
		where := "ontology"
		if sn.NodeType == model.NodeBoundary {
			where = "manifest"
		}
		c.add(Finding{ID: KindDangling + ":" + cur + ":" + e.Prov.Path + ":" + fmt.Sprint(e.Prov.Line), Kind: KindDangling, Severity: "error", Boundary: boundaryCurie(sn), Concept: cur, Relation: e.P, Path: e.Prov.Path, Line: e.Prov.Line,
			Msg: fmt.Sprintf("%s:%d %s %s %s: concept does not exist (%s reference)", e.Prov.Path, e.Prov.Line, sn.CURIE, e.P, cur, where)})
	}
	for _, m := range c.ws.Manifests {
		for _, pat := range m.Allowed {
			if pat == model.DerivedMarker || strings.Contains(pat, "*") || c.g.NodeByCurie(pat) != nil {
				continue
			}
			c.add(Finding{ID: KindDangling + ":" + pat + ":" + m.Path + ":1", Kind: KindDangling, Severity: "error", Boundary: m.Boundary, Concept: pat, Relation: "allowed", Path: m.Path, Line: 1,
				Msg: fmt.Sprintf("%s:1 %s allowed %s: concept does not exist (manifest reference)", m.Path, m.Boundary, pat)})
		}
	}
}

func boundaryCurie(n *graph.Node) string {
	if n.NodeType == model.NodeBoundary {
		return n.CURIE
	}
	return ""
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
	c.danglingDeclared()
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
// implements (or, for data, represents) them: a page with no engine behind it.
func (c *comparer) presentedOnly() {
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if n.Imported || n.Internal || n.Deprecated || n.Planned || n.Kind == "event" {
			continue
		}
		var by []string
		rels := []string{"presents"}
		// A shell hosting a whole system-type, or a page concept, is expected
		// to have no single implementation; anything else hosted is a
		// surface like a presented one (ADR 0012).
		hosted := n.Kind != "system-type" && len(c.g.Objects(n.ID, "presents"))+len(c.g.Objects(n.ID, "hosts")) == 0
		if hosted {
			rels = append(rels, "hosts")
		}
		verbs := map[string]bool{}
		for _, rel := range rels {
			for _, e := range c.g.In(n.ID, rel) {
				if e.Source == graph.SourceDerived {
					continue
				}
				if s := c.g.Node(e.S); s != nil && s.NodeType != model.NodeBoundary {
					by = append(by, c.g.Curie(e.S))
					verbs[map[string]string{"presents": "presented", "hosts": "hosted"}[rel]] = true
				}
			}
		}
		if len(by) == 0 || c.implemented(n.ID, 0) || c.represented(n.ID) {
			continue
		}
		by = uniq(by)
		how := strings.Join(sortedKeys(verbs), " and ")
		c.add(Finding{Kind: KindPresentedOnly, Severity: "warning", Concept: n.CURIE, Path: n.File, Line: n.Line, Via: by,
			Msg: fmt.Sprintf("%s is %s (%s) but nothing implements it: a surface with no mechanism behind it", n.CURIE, how, strings.Join(by, ", "))})
	}
}

// represented reports whether an artifact represents the concept (or a
// concept that realizes it): an entity's mechanism is its data shape.
func (c *comparer) represented(id string) bool {
	for _, x := range append([]string{id}, c.g.Subjects(id, "realizes")...) {
		for _, e := range c.g.In(x, "represents") {
			if a := c.g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact {
				return true
			}
		}
	}
	return false
}

// InternalExempt reports whether an internal concept stays out of the
// untested finding and the concept-coverage denominator: an internal
// concept is an engineering detail unless its effective criticality is high
// or critical, when losing its only test matters as much as any other's.
func InternalExempt(n *graph.Node) bool {
	return n.Internal && model.CriticalityRank(n.Criticality) < model.CriticalityRank("high")
}

func (c *comparer) untested() {
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if n.Imported || InternalExempt(n) {
			continue
		}
		implemented := c.implemented(n.ID, 0) && n.Kind != "event"
		reached, _ := c.reachedByArtifact(n.ID)
		if !implemented && !(reached && c.inCoverage(n)) {
			continue
		}
		if c.tested(n.ID, 0) {
			continue
		}
		msg := fmt.Sprintf("%s is implemented but nothing tests or verifies it", n.CURIE)
		if !implemented {
			msg = fmt.Sprintf("%s has code but nothing tests or verifies it", n.CURIE)
		}
		if n.Tier == model.TierImpl {
			// Report at the core level: skip the impl concept when a core concept it
			// realizes is itself reported as untested. What its mechanism wholes
			// realize counts; the areas a core concept is part of do not, so an
			// untested domain area never hides an untested part (ADR 0025, 13).
			cores := realizedByMechanism(c.g, n.ID)
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
				// Not skipped: the core concept is an event (never reported
				// itself), or it counts as tested by tests of its own in
				// boundaries that hold none of this concept's code, or only
				// through another realizer's tests.
				msg += c.whyCoreTestMisses(n.ID, cores[0])
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
	for _, e := range append(c.g.In(id, "implements"), c.g.In(id, "publishes-value")...) {
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

// Tested reports whether gloss drift counts the concept as tested: verified
// directly, through a concept that realizes it, through a core concept it
// realizes (see tested), or through an is-a descendant.
func Tested(g *graph.Graph, id string) bool { return (&comparer{g: g}).tested(id, 0) }

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
	// An implementation concept is tested when a core concept it realizes is
	// tested directly, and it is that core concept's only realizer or the test
	// sits in a boundary holding one of its artifacts, directly or through a
	// nested boundary: the engine's test of a core concept does not cover a
	// renderer that realizes it too, but a system-level acceptance test does.
	for _, core := range RealizesStar(c.g, id) {
		sole := len(c.realizersOf(core)) == 1
		for _, p := range []string{"tests", "verifies"} {
			for _, e := range c.g.In(core, p) {
				if a := c.g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact && (sole || c.implementedIn(id, a.Boundary)) {
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

// realizersOf returns the concepts whose RealizesStar reaches core.
func (c *comparer) realizersOf(core string) map[string]bool {
	if c.realizers == nil {
		c.realizers = map[string]map[string]bool{}
		for _, n := range c.g.NodesOf(model.NodeConcept) {
			for _, k := range RealizesStar(c.g, n.ID) {
				if c.realizers[k] == nil {
					c.realizers[k] = map[string]bool{}
				}
				c.realizers[k][n.ID] = true
			}
		}
	}
	return c.realizers[core]
}

// whyCoreTestMisses is the untested line's suffix for impl: why the core
// concept it realizes does not lend it a test.
func (c *comparer) whyCoreTestMisses(impl, core string) string {
	if !c.tested(core, 0) {
		return " (nor " + c.g.Curie(core) + ", which it realizes)"
	}
	for _, p := range []string{"tests", "verifies"} {
		for _, e := range c.g.In(core, p) {
			if a := c.g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact {
				return "; the tests of " + c.g.Curie(core) + ", which it realizes, sit in boundaries that hold none of its code"
			}
		}
	}
	var via []string
	for r := range c.realizersOf(core) {
		if r != impl && c.tested(r, 0) {
			via = append(via, c.g.Curie(r))
		}
	}
	sort.Strings(via)
	if len(via) == 0 {
		return "; " + c.g.Curie(core) + ", which it realizes, counts as tested only through its subtypes"
	}
	verb := " is tested"
	if len(via) > 1 {
		verb = " are tested"
	}
	return "; " + c.g.Curie(core) + ", which it realizes, counts as tested only because " + strings.Join(via, ", ") + verb
}

// implementedIn reports whether an artifact in boundary b, or in a boundary
// nested in it, implements concept id, or provisions, configures or deploys
// it (a system-type's artifacts are often only those).
func (c *comparer) implementedIn(id, b string) bool {
	rels := append([]string{"implements", "publishes-value"}, InfraRelations...)
	for _, rel := range rels {
		for _, e := range c.g.In(id, rel) {
			if a := c.g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact && c.within(a.Boundary, b) {
				return true
			}
		}
	}
	return false
}

// within reports whether boundary x is b or sits under it by part-of
// (declared, or the nearest enclosing manifest's, ADR 0026).
func (c *comparer) within(x, b string) bool {
	seen := map[string]bool{}
	for next := []string{x}; len(next) > 0; {
		y := next[0]
		next = next[1:]
		if y == b {
			return true
		}
		if y == "" || seen[y] {
			continue
		}
		seen[y] = true
		next = append(next, c.g.Objects(y, "part-of")...)
	}
	return false
}

func (c *comparer) stale() {
	for id, e := range c.opt.Ledger {
		if strings.HasPrefix(id, "value:") {
			// stale is how older ledgers recorded a drifted value.
			if e.Status != model.StatusDrifted && e.Status != model.StatusStale {
				continue
			}
			cur := strings.TrimPrefix(id, "value:")
			live, bound := "", e.Path
			hint := "or confirm with gloss accept --value"
			if n := c.g.NodeByCurie(cur); n != nil {
				live = n.LiveValue
				if n.ValueFrom != "" {
					bound = n.ValueFrom
				}
				// The same check gloss accept --value refuses on.
				if !model.ValueInLine(n.Value, n.LiveValue) {
					hint = "or confirm the line as is with gloss accept --value --force"
				}
			}
			c.add(Finding{ID: KindValueDrift + ":" + cur, Kind: KindValueDrift, Severity: "error", Path: e.Path, Concept: cur,
				Msg: fmt.Sprintf("published value drifted: %s publishes %q, bound to %s, which changed from %q to %q; update value: in the ontology, %s %s", cur, e.Value, bound, e.Live, live, hint, cur)})
			continue
		}
		if e.Status != model.StatusStale {
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
		case KindPlanned, KindPlanOverdue:
			m.PlanItems++
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
		case model.StatusStale:
			m.Stale++
		}
	}
	// Rejected links have no edges (ledger.DropRejected); count the decisions.
	for id, le := range c.opt.Ledger {
		if le.Status == model.StatusRejected && !strings.HasPrefix(id, "concept:") && !strings.HasPrefix(id, "value:") {
			m.Rejected++
		}
	}
	// Concept coverage: owned implementation concepts (not deprecated, still
	// proposed, or internal below high criticality) with an artifact and a
	// verifier, and the owned core concepts code implements directly (a
	// server-rendered page, a CLI, a BFF marked implements hp:X), which are
	// mechanisms just the same (ADR 0013).
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if !c.inCoverage(n) {
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

// inCoverage reports whether a concept is in the concept-coverage
// denominator. untested reports every such concept code reaches that
// nothing verifies, so each uncovered one is an orphan, untested, or held
// back for the core concept it realizes, which is reported instead.
func (c *comparer) inCoverage(n *graph.Node) bool {
	if n.Imported || InternalExempt(n) || n.Deprecated || n.Proposed || n.Planned {
		return false
	}
	return n.Tier == model.TierImpl || c.implementedDirectly(n.ID)
}

// implementedDirectly reports whether an artifact's marker implements the
// concept itself (not a test-path inference).
func (c *comparer) implementedDirectly(id string) bool {
	for _, e := range c.g.In(id, "implements") {
		if a := c.g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact && e.Source == graph.SourceInline {
			return true
		}
	}
	return false
}

// ConceptCoverage formats concept coverage for a report: the percentage, or
// n/a when no concept is in scope, so an empty scope never reads as 0%.
func (m Metrics) ConceptCoverage(format string) string {
	if m.ConceptsInScope == 0 {
		return "n/a"
	}
	return fmt.Sprintf(format, m.ConceptCoveragePct)
}

// WaiverDate is the layout of a waiver's expires: date.
const WaiverDate = "2006-01-02"

// WaiverExpired reports whether a waiver expiring on exp (a date) no longer
// holds at now: expires is inclusive, so the waiver holds through that day.
func WaiverExpired(exp, now time.Time) bool {
	return now.After(exp.Add(24 * time.Hour))
}

// UnmatchedWaivers returns the policy's waivers that name no finding of the
// report, by finding id or concept: a mistyped id waives nothing. Waivers of
// stale findings are left out, since those need the ledger to be found.
func UnmatchedWaivers(r *Report, pol *model.Policy) []model.Waiver {
	if pol == nil {
		return nil
	}
	var out []model.Waiver
	for _, w := range pol.Waivers {
		if strings.HasPrefix(w.Finding, KindStale+":") {
			continue
		}
		found := false
		for _, f := range r.Findings {
			found = found || w.Finding == f.ID || w.Finding == f.Concept
		}
		if !found {
			out = append(out, w)
		}
	}
	return out
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
			// A waiver without a valid date waives nothing (gloss check GM610).
			exp, err := time.Parse(WaiverDate, w.Expires)
			if err != nil {
				f.Msg += fmt.Sprintf(" [waiver by %s ignored: expires %q is not a YYYY-MM-DD date]", w.Owner, w.Expires)
				continue
			}
			if WaiverExpired(exp, c.opt.Now) {
				f.Msg += fmt.Sprintf(" [waiver by %s expired %s]", w.Owner, w.Expires)
				continue
			}
			f.Waived = true
			f.Waiver = fmt.Sprintf("%s until %s", w.Owner, w.Expires)
		}
	}
}

// FindingKinds are the kinds a finding can have, in the order --fail-on lists them.
var FindingKinds = []string{KindViolation, KindGap, KindValueDrift, KindDangling, KindOverdue, KindPlannedClosed, KindPlanOverdue, KindOutOfRange, KindDeprecatedUse, KindRogue, KindOrphan, KindUntested, KindPresentedOnly, KindOverlap, KindStale, KindProposed, KindPlanned, KindConsistent}

// ParseFailOn splits a --fail-on list and rejects unknown kinds, so a typo
// cannot silently disable part of a gate. "none" (or nothing) fails on nothing.
func ParseFailOn(list string) ([]string, error) {
	var kinds []string
	for _, k := range strings.Split(list, ",") {
		k = strings.TrimSpace(k)
		if k == "" || k == "none" {
			continue
		}
		known := false
		for _, fk := range FindingKinds {
			known = known || k == fk
		}
		if !known {
			return nil, fmt.Errorf("--fail-on: unknown finding kind %q; valid kinds: %s, none", k, strings.Join(FindingKinds, ", "))
		}
		kinds = append(kinds, k)
	}
	return kinds, nil
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

func countNoun(n int, s string) string {
	if n == 1 {
		return "1 " + s
	}
	return fmt.Sprintf("%d %ss", n, s)
}
