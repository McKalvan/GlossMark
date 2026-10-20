// @gloss implements gmi:DriftComparator scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/rollup"
)

func init() {
	register(command{"drift", "compare declared boundaries with what the code derives: violations, gaps, orphans, rogues, stale, untested", cmdDrift})
	register(command{"coverage", "concepts with an artifact and a verifier (headline), files with an accepted link; --fail-under N", cmdCoverage})
}

func cmdDrift(args []string) int {
	fs, root := newFlags("drift")
	format := fs.String("format", "text", "text | json | markdown")
	failOn := fs.String("fail-on", "violation,value-drift,gap,dangling,overdue", "comma-separated finding kinds that fail the command")
	all := fs.Bool("all", false, "also list consistent pairs")
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
	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(rep)
	case "markdown":
		fmt.Print(rollup.RenderMarkdown(rep, *all))
	default:
		fmt.Print(rollup.RenderText(rep, *all))
	}
	if n := len(rep.Failing(kinds)); n > 0 {
		fmt.Fprintf(os.Stderr, "gloss drift: %s (%s)\n", plural(n, "failing finding"), *failOn)
		return 1
	}
	return 0
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
		bg, err := loadBaseGraph(p.WS, *base)
		if err != nil {
			return fail(err)
		}
		threshold = rollup.Compare(bg, p.WS, rollup.Options{Policy: pol, Now: time.Now()}).Metrics.ConceptCoveragePct
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
		fmt.Printf("concepts %s (%d of %d concepts in scope have an artifact and a verifier: implementation concepts, and core concepts code implements directly)\n", m.ConceptCoverage("%.1f%%"), m.ConceptsCovered, m.ConceptsInScope)
		scope := "in-scope files"
		if *changedOnly {
			scope = "files changed since " + *base
		}
		fmt.Printf("files %.1f%% (%d of %d %s carry an accepted link)\n", m.CoveragePct, m.CoveredFiles, m.InScopeFiles, scope)
		fmt.Printf("markers %d (%d targets): accepted %d, proposed %d, stale %d, rejected %d; proposed concepts %d; review backlog oldest %d days\n", m.Markers, m.Targets, m.Accepted, m.Proposed, m.Stale, m.Rejected, m.ProposedConcepts, m.BacklogOldestDay)
		fmt.Printf("orphans %d, rogues %d, violations %d, gaps %d, untested %d, dangling %d\n", m.Orphans, m.Rogues, m.Violations, m.Gaps, m.Untested, m.Dangling)
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
