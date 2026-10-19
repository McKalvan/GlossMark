// @gloss implements gmi:Linter scope=file

package main

import (
	"fmt"
	"os"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
)

func init() {
	register(command{"check", "lint: syntax, dangling refs, domain/range, own-subject rule, tiers, allowed lists", cmdCheck})
}

func cmdCheck(args []string) int {
	fs, root := newFlags("check")
	warnErr := fs.Bool("strict", false, "treat warnings as errors")
	explain := fs.Bool("explain", false, "print the effective allowed set of every manifest (declared, or derived from what it declares)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if *explain {
		fmt.Print(check.ExplainAllowed(p.WS, p.Graph))
	}
	diags := append([]marker.Diagnostic{}, p.Diags...)
	diags = append(diags, check.Run(p.WS, p.Graph)...)
	printDiags(diags)
	errs, warns := 0, 0
	for _, d := range diags {
		if d.Severity == "error" {
			errs++
		} else {
			warns++
		}
	}
	// Proposals and rogues pass check (they are decisions, not defects) but a
	// reviewer running the hook must see that they exist.
	proposals, rogues := 0, 0
	for _, fm := range p.Res.Markers {
		if fm.Marker.Proposed {
			proposals++
		}
		for _, t := range fm.Marker.Targets {
			if marker.IsNewTarget(t) {
				rogues++
				break
			}
		}
	}
	for _, n := range p.Graph.NodesOf(model.NodeConcept) {
		if n.Proposed && !n.Imported {
			proposals++
		}
	}
	summary := fmt.Sprintf("gloss check: %s, %s", plural(errs, "error"), plural(warns, "warning"))
	if proposals > 0 || rogues > 0 {
		summary += fmt.Sprintf("; %s, %s (run gloss status)", plural(proposals, "proposal"), plural(rogues, "rogue"))
	}
	fmt.Fprintln(os.Stderr, summary)
	if errs > 0 || (*warnErr && warns > 0) {
		return 1
	}
	return 0
}
