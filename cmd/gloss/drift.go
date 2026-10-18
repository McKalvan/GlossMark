package main

import (
	"encoding/json"
	"fmt"
	"os"
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
	failOn := fs.String("fail-on", "violation,gap,dangling", "comma-separated finding kinds that fail the command")
	all := fs.Bool("all", false, "also list consistent pairs")
	if err := fs.Parse(args); err != nil {
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
	kinds := strings.Split(*failOn, ",")
	if n := len(rep.Failing(kinds)); n > 0 {
		fmt.Fprintf(os.Stderr, "gloss drift: %s (%s)\n", plural(n, "failing finding"), *failOn)
		return 1
	}
	return 0
}

func cmdCoverage(args []string) int {
	fs, root := newFlags("coverage")
	failUnder := fs.Float64("fail-under", -1, "fail when concept coverage is below this percentage (default from policy.coverage-fail-under)")
	failUnderFiles := fs.Float64("fail-under-files", 0, "fail when file coverage is below this percentage")
	format := fs.String("format", "text", "text | json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	pol := p.WS.Config.EffectivePolicy()
	rep := rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: pol, Now: time.Now()})
	m := rep.Metrics
	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(m)
	} else {
		fmt.Printf("concepts %.1f%% (%d of %d implementation concepts have an artifact and a verifier)\n", m.ConceptCoveragePct, m.ConceptsCovered, m.ConceptsInScope)
		fmt.Printf("files %.1f%% (%d of %d in-scope files carry an accepted link)\n", m.CoveragePct, m.CoveredFiles, m.InScopeFiles)
		fmt.Printf("markers %d (%d targets): accepted %d, proposed %d, stale %d, rejected %d; proposed concepts %d; review backlog oldest %d days\n", m.Markers, m.Targets, m.Accepted, m.Proposed, m.Stale, m.Rejected, m.ProposedConcepts, m.BacklogOldestDay)
		fmt.Printf("orphans %d, rogues %d, violations %d, gaps %d, untested %d, dangling %d\n", m.Orphans, m.Rogues, m.Violations, m.Gaps, m.Untested, m.Dangling)
	}
	threshold := *failUnder
	if threshold < 0 {
		threshold = pol.CoverageFailUnder
	}
	if threshold > 0 && m.ConceptCoveragePct < threshold {
		fmt.Fprintf(os.Stderr, "gloss coverage: concept coverage %.1f%% is below --fail-under %.1f%%\n", m.ConceptCoveragePct, threshold)
		return 1
	}
	if *failUnderFiles > 0 && m.CoveragePct < *failUnderFiles {
		fmt.Fprintf(os.Stderr, "gloss coverage: file coverage %.1f%% is below --fail-under-files %.1f%%\n", m.CoveragePct, *failUnderFiles)
		return 1
	}
	return 0
}
