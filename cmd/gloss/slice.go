// @gloss implements gmi:ViewProjector scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/slice"
	"github.com/mckalvan/glossmark/internal/views"
)

func init() {
	register(command{"slice", "everything relevant to a feature: the concepts, code, tests, boundaries and context around a concept, boundary or path (ADR 0023)", cmdSlice})
}

func cmdSlice(args []string) int {
	fs, root := newFlags("slice")
	context := fs.Int("context", 1, "1 keeps the faded context ring (stage 5), 0 drops it")
	follow := fs.String("follow", "", "flow relations that repeat with the member stages, comma-separated: produces, consumes, publishes, subscribes")
	format := fs.String("format", "text", "text | json | md")
	var repos []string
	fs.Func("repo", "slice the landscape of these repository roots instead of one workspace (repeatable)", func(s string) error { repos = append(repos, s); return nil })
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gloss slice <curie|path>... [--context 0|1] [--follow rels] [--format text|json|md] [--repo root]...")
		return 2
	}
	if *context != 0 && *context != 1 {
		fmt.Fprintln(os.Stderr, "gloss slice: --context takes 0 or 1")
		return 2
	}
	if *format != "text" && *format != "json" && *format != "md" {
		fmt.Fprintln(os.Stderr, "gloss slice: --format takes text, json or md")
		return 2
	}
	fl, err := slice.ParseFollow(*follow)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gloss slice:", err)
		return 2
	}
	var g *graph.Graph
	var rep *rollup.Report
	base := ""
	if len(repos) > 0 {
		ws, lg, err := assembleLandscape(repos)
		if err != nil {
			return fail(err)
		}
		g = lg
		rep = rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy(), Now: time.Now()})
	} else {
		p, err := buildGraph(*root, false)
		if err != nil {
			return fail(err)
		}
		g, base = p.Graph, p.WS.Root
		rep = rollup.Compare(g, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: p.WS.Config.EffectivePolicy(), Now: time.Now()})
	}
	s, err := slice.Of(g, base, fs.Args(), slice.Options{Context: *context == 1, Follow: fl}, rep.Findings)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gloss slice:", err)
		return 1
	}
	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(s)
	default:
		fmt.Print(slice.Render(s, *format == "md"))
	}
	return 0
}

// slicesTouched names the saved slices (views.yaml views with slice:) that
// changed files touch: a file is in a slice when one of its artifacts or one
// of the concepts it links to is a node of the slice. Sharing a boundary with
// the slice is not enough: the boundaries are drawn for context.
func slicesTouched(g *graph.Graph, ws *load.Workspace, touched []gate.FileTouch) []string {
	if ws.Views == nil {
		return nil
	}
	var names []string
	for name, v := range ws.Views.Views {
		if len(v.Slice) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var out []string
	for _, name := range names {
		set := views.SliceMembers(g, ws.Views.Views[name])
		var why []string
		for _, ft := range touched {
			hit := ""
			for _, c := range ft.Concepts {
				if set[c.Curie] {
					hit = c.Curie
					break
				}
			}
			if hit == "" {
				for _, a := range g.NodesOf(model.NodeArtifact) {
					if a.Path == ft.Path && set[g.Curie(a.ID)] {
						hit = g.Curie(a.ID)
						break
					}
				}
			}
			if hit != "" {
				why = append(why, ft.Path+" ("+hit+")")
			}
		}
		if len(why) > 0 {
			out = append(out, fmt.Sprintf("%s [%s]: %s", name, strings.Join(ws.Views.Views[name].Slice, ", "), strings.Join(why, ", ")))
		}
	}
	return out
}

// renderSlicesTouched prints the saved slices a change touches, if any.
func renderSlicesTouched(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nSaved slices touched\n")
	for _, s := range ss {
		b.WriteString("  " + s + "\n")
	}
	return b.String()
}

// conceptBriefing is gloss context --concept: the slice of a feature,
// rendered as the briefing an agent reads before working on it, cut to the
// token budget (about four characters a token).
func conceptBriefing(p *pipeline, seeds []string, budget int) (string, error) {
	rep := rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: p.WS.Config.EffectivePolicy(), Now: time.Now()})
	s, err := slice.Of(p.Graph, p.WS.Root, seeds, slice.Options{Context: true}, rep.Findings)
	if err != nil {
		return "", err
	}
	txt := "GlossMark briefing: the feature slice of " + strings.Join(s.Seeds, ", ") + ". Members belong to the feature; context explains it. " +
		"Link new code to a member with @gloss? (gloss mcp propose_link gives the exact line).\n\n" + slice.Render(s, false)
	if budget > 0 && len(txt) > budget*4 {
		cut := strings.LastIndex(txt[:budget*4], "\n")
		if cut < 0 {
			cut = budget * 4
		}
		txt = txt[:cut] + "\n… cut to the budget; run gloss slice " + strings.Join(seeds, " ") + " for the rest\n"
	}
	return txt, nil
}
