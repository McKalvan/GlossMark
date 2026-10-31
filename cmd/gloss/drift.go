// @gloss implements gmi:DriftComparator scope=file
// @gloss produces gm:Drift, gm:Coverage, gm:DecisionRequest, gm:PlanItem scope=file
// @gloss publishes gm:DriftDetected, gm:DecisionRequested scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func init() {
	register(command{"drift", "compare declared boundaries with what the code derives: violations, gaps, orphans, rogues, stale, untested", cmdDrift})
	register(command{"coverage", "concepts with an artifact and a verifier (headline), files with an accepted link; --fail-under N", cmdCoverage})
}

func cmdDrift(args []string) int {
	fs, root := newFlags("drift")
	format := fs.String("format", "text", "text | json | markdown")
	failOn := fs.String("fail-on", "violation,value-drift,gap,dangling,overdue,planned-closed,plan-overdue", "comma-separated finding kinds that fail the command")
	all := fs.Bool("all", false, "also list consistent pairs")
	ids := fs.Bool("ids", false, "end each finding with its id, the key policy.waivers names (text output; JSON always has it)")
	explain := fs.Bool("explain", false, "print each finding's precise message, with the derived and declared sets (text output; JSON always has it)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	kinds, err := rollup.ParseFailOn(*failOn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gloss drift:", err)
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	rep := rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: p.WS.Config.EffectivePolicy(), Now: time.Now()})
	rollup.AddDeprecatedUse(rep, p.Graph, p.WS.Config.EffectivePolicy(), time.Now())
	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(rep)
	case "markdown":
		fmt.Print(rollup.RenderMarkdown(rep, *all))
	default:
		manifests := map[string]string{}
		for _, m := range p.WS.Manifests {
			manifests[m.Boundary] = m.Path
		}
		fmt.Print(rollup.RenderTextOpts(rep, rollup.TextOptions{All: *all, IDs: *ids, Guide: true, Explain: *explain, Manifests: manifests, Color: colorOn(os.Stdout)}))
		if ownConcepts(p.Graph) == 0 {
			fmt.Println("\nNo concepts are defined yet, so there is nothing to compare. Draft some with gloss suggest --concepts.")
		}
	}
	if failing := rep.Failing(kinds); len(failing) > 0 {
		// The adoption level caps what fails: below level 2 drift reports
		// only, at level 2 only findings on critical or high concepts fail.
		level := p.WS.Config.AdoptionLevel()
		enforced := rollup.FailingAtLevel(failing, p.Graph, level)
		if len(enforced) == 0 {
			what := fmt.Sprintf("%d finding(s) would fail (fail-on: %s), reported only", len(failing), strings.Join(kinds, ", "))
			if model.AdoptionDriftFails(level) {
				what = fmt.Sprintf("%d finding(s) would fail (fail-on: %s), none on a %s or critical concept, so none fails", len(failing), strings.Join(kinds, ", "), model.AdoptionMinCriticality(level))
			}
			fmt.Fprintln(os.Stderr, "gloss drift: "+model.AdoptionNote(level, what))
			return 0
		}
		msg := "gloss drift: " + rollup.FailSummary(enforced, kinds)
		if n := len(failing) - len(enforced); n > 0 {
			msg += fmt.Sprintf("; %d more below %s criticality are advisory at adoption %s", n, model.AdoptionMinCriticality(level), model.AdoptionName(level))
		}
		if colorOn(os.Stderr) {
			msg = "\x1b[1;31m" + msg + "\x1b[0m"
		}
		fmt.Fprintln(os.Stderr, msg)
		return 1
	}
	return 0
}

// ownConcepts counts the concepts the workspace defines, imports left out.
func ownConcepts(g *graph.Graph) int {
	n := 0
	for _, c := range g.NodesOf(model.NodeConcept) {
		if !c.Imported {
			n++
		}
	}
	return n
}

func cmdCoverage(args []string) int {
	fs, root := newFlags("coverage")
	failUnder := fs.String("fail-under", "", "fail when concept coverage is below this percentage, or below the base's when 'current' (ratchet; default from policy.coverage-fail-under)")
	failUnderFiles := fs.Float64("fail-under-files", 0, "fail when file coverage is below this percentage")
	base := fs.String("base", "origin/main", "git ref the ratchet (--fail-under current) and --changed-only compare against")
	changedOnly := fs.Bool("changed-only", false, "score file coverage over the files changed since --base only, so a repo can adopt gradually")
	format := fs.String("format", "text", "text | json")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	pol := p.WS.Config.EffectivePolicy()
	opts := rollup.Options{Ledger: p.Ledger.Entries, Policy: pol, Now: time.Now()}
	if *changedOnly {
		if err := verifyBase(p.WS.Root, *base); err != nil {
			return fail(err)
		}
		changed, err := changedFiles(p.WS.Root, *base)
		if err != nil {
			return fail(err)
		}
		inTree := map[string]bool{}
		for _, f := range p.WS.Files {
			inTree[f] = true
		}
		opts.Files = []string{}
		for _, f := range changed {
			if inTree[f] {
				opts.Files = append(opts.Files, f)
			}
		}
	}
	rep := rollup.Compare(p.Graph, p.WS, opts)
	m := rep.Metrics
	threshold := -1.0
	if *failUnder == "" {
		*failUnder = string(pol.CoverageFailUnder)
	}
	switch strings.ToLower(*failUnder) {
	case "":
	case "current":
		if err := verifyBase(p.WS.Root, *base); err != nil {
			return fail(err)
		}
		// The change that adopts GlossMark has nothing to ratchet against:
		// say so rather than pass without a word.
		if ok, at := baseHasWorkspace(p.WS, *base); !ok {
			fmt.Fprintf(os.Stderr, "gloss coverage: base %s has no workspace at %s (no %s); ratchet skipped\n", *base, at, load.ConfigName)
			break
		}
		bg, err := loadBaseGraph(p.WS, *base)
		if err != nil {
			return fail(err)
		}
		bm := rollup.Compare(bg, p.WS, rollup.Options{Policy: pol, Now: time.Now()}).Metrics
		if bm.ConceptsInScope == 0 {
			fmt.Fprintf(os.Stderr, "gloss coverage: base %s has no concepts in scope (coverage n/a); ratchet skipped\n", *base)
			break
		}
		threshold = bm.ConceptCoveragePct
	default:
		v, err := strconv.ParseFloat(*failUnder, 64)
		if err != nil {
			return fail(fmt.Errorf("--fail-under: %q is neither a number nor 'current'", *failUnder))
		}
		threshold = v
	}
	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(m)
	} else {
		// Whole percentages, as in gloss drift; JSON keeps the precise value.
		fmt.Printf("concepts %s (%d of %d concepts in scope have an artifact and a verifier: implementation concepts, and core concepts code implements directly)\n", m.ConceptCoverage("%.0f%%"), m.ConceptsCovered, m.ConceptsInScope)
		scope := "in-scope files"
		if *changedOnly {
			scope = "files changed since " + *base
		}
		fmt.Printf("files %.0f%% (%d of %d %s carry an accepted link)\n", m.CoveragePct, m.CoveredFiles, m.InScopeFiles, scope)
		backlog := ""
		if m.Proposed > 0 {
			backlog = fmt.Sprintf("; review backlog oldest %d days", m.BacklogOldestDay)
		}
		fmt.Printf("markers %d (%d targets): accepted %d, proposed %d, stale %d, rejected %d; proposed concepts %d%s\n", m.Markers, m.Targets, m.Accepted, m.Proposed, m.Stale, m.Rejected, m.ProposedConcepts, backlog)
		fmt.Printf("orphans %d, rogues %d, violations %d, gaps %d, untested %d, dangling %d; open plan items %d (planned concepts are out of the denominator)\n", m.Orphans, m.Rogues, m.Violations, m.Gaps, m.Untested, m.Dangling, m.PlanItems)
		if hint := emptyOntologyHint(p.Graph); hint != "" {
			fmt.Fprintln(os.Stderr, "gloss coverage: "+hint)
		}
	}
	// With nothing in scope coverage is n/a, which no threshold fails.
	if threshold > 0 && m.ConceptsInScope > 0 && m.ConceptCoveragePct < threshold {
		what := fmt.Sprintf("--fail-under %.1f%%", threshold)
		if strings.EqualFold(*failUnder, "current") {
			what = fmt.Sprintf("the base's %.1f%% (--fail-under current)", threshold)
		}
		fmt.Fprintf(os.Stderr, "gloss coverage: concept coverage %.1f%% is below %s\n", m.ConceptCoveragePct, what)
		return 1
	}
	if *failUnderFiles > 0 && m.CoveragePct < *failUnderFiles {
		fmt.Fprintf(os.Stderr, "gloss coverage: file coverage %.1f%% is below --fail-under-files %.1f%%\n", m.CoveragePct, *failUnderFiles)
		return 1
	}
	return 0
}
