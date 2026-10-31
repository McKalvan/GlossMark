// @gloss implements gmi:Linter scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
)

func init() {
	register(command{"check", "lint: syntax, dangling refs, domain/range, own-subject rule, tiers, allowed lists", cmdCheck})
}

func cmdCheck(args []string) int {
	fs, root := newFlags("check")
	warnErr := fs.Bool("strict", false, "treat warnings as errors")
	explain := fs.Bool("explain", false, "print the effective allowed set of every manifest (declared, or derived from what it declares) and why each entry is in it")
	format := fs.String("format", "text", "--explain output: text | json")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if *format != "text" && *format != "json" {
		return fail(fmt.Errorf("--format: unknown format %q; valid formats: text, json", *format))
	}
	if *explain {
		if *format == "json" {
			b, _ := json.MarshalIndent(check.ExplainAllowedRecords(p.WS, p.Graph), "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Print(check.ExplainAllowed(p.WS, p.Graph))
		}
	}
	diags := append([]marker.Diagnostic{}, p.Diags...)
	diags = append(diags, check.Run(p.WS, p.Graph)...)
	printDiags(diags)
	errs, warns, hints := 0, 0, 0
	for _, d := range diags {
		switch d.Severity {
		case "error":
			errs++
		case "hint":
			// A hint (GM250, the starter relation profile) names a switch,
			// not a defect: --strict leaves it alone.
			hints++
		default:
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
	if hints > 0 {
		summary += ", " + plural(hints, "hint")
	}
	if proposals > 0 || rogues > 0 {
		summary += fmt.Sprintf("; %s, %s (run gloss status)", plural(proposals, "proposal"), plural(rogues, "rogue"))
	}
	fmt.Fprintln(os.Stderr, summary)
	if hint := emptyOntologyHint(p.Graph); hint != "" {
		fmt.Fprintln(os.Stderr, "gloss check: "+hint)
	}
	if errs > 0 || (*warnErr && warns > 0) {
		// At adoption level 0 the vocabulary is all there is: errors are
		// reported, nothing fails, unless --strict asked for it.
		if level := p.WS.Config.AdoptionLevel(); !model.AdoptionChecksFail(level) && !*warnErr {
			fmt.Fprintln(os.Stderr, "gloss check: "+model.AdoptionNote(level, fmt.Sprintf("%s reported, none fails", plural(errs, "error"))))
			return 0
		}
		return 1
	}
	return 0
}

// emptyOntologyHint names the next command when the workspace defines no
// concepts of its own, the state gloss init leaves (issue #227); "" otherwise.
func emptyOntologyHint(g *graph.Graph) string {
	for _, n := range g.NodesOf(model.NodeConcept) {
		if !n.Imported {
			return ""
		}
	}
	return "0 concepts defined. Draft some with gloss suggest --concepts (or the adopt-glossmark skill: gloss hook install --skill)"
}
