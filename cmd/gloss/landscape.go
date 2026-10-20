// @gloss implements gmi:LandscapeAssembler scope=file

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/viewer"
	"github.com/mckalvan/glossmark/internal/views"
)

func init() {
	register(command{"landscape", "union several repositories' graphs by IRI, re-derive rollups and report cross-repo drift and context-map edges", cmdLandscape})
}

func cmdLandscape(args []string) int {
	fs := newLandscapeFlags()
	if err := parseFlags(fs.fs, args); err != nil {
		return 2
	}
	repos := append([]string{}, fs.repos...)
	repos = append(repos, fs.fs.Args()...)
	if len(repos) < 1 {
		fmt.Fprintln(os.Stderr, "usage: gloss landscape [--format text|json|markdown] [--view out.html] [--graph out.jsonl] <repo-root>...")
		return 2
	}
	var wss []*load.Workspace
	var graphs []*graph.Graph
	for _, r := range repos {
		p, err := buildGraph(r, false)
		if err != nil {
			return fail(fmt.Errorf("%s: %w", r, err))
		}
		// Prefix artifact paths with the repo name so they stay unique in the union.
		name := filepath.Base(p.WS.Root)
		for _, n := range p.Graph.NodesOf(model.NodeArtifact) {
			n.Path = name + "/" + n.Path
			n.Label = name + "/" + n.Label
			n.File = name + "/" + n.File
		}
		for _, e := range p.Graph.Edges() {
			if e.Prov != nil && e.Prov.Path != "" && e.Source != graph.SourceDeclared {
				e.Prov.Path = name + "/" + e.Prov.Path
			}
		}
		wss = append(wss, p.WS)
		graphs = append(graphs, p.Graph)
	}
	ws, err := load.Merge(wss)
	if err != nil {
		return fail(err)
	}
	g := union(graphs, ws)
	rollup.Derive(g, ws.Vocab)
	rep := rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy(), Now: time.Now()})
	// Context-map edges: implied depends-on between boundaries of different repos.
	var ctx []string
	for _, e := range g.Edges() {
		if e.P == "depends-on" && e.Source == graph.SourceDerived {
			ctx = append(ctx, fmt.Sprintf("%s → %s via %s", g.Curie(e.S), g.Curie(e.O), strings.Join(e.Prov.Via, ", ")))
		}
	}
	sort.Strings(ctx)
	switch fs.format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"repos": repos, "context_map": ctx, "drift": rep, "stats": g.Stats()})
	case "markdown":
		fmt.Printf("### Landscape: %s\n\n", strings.Join(baseNames(repos), ", "))
		if len(ctx) > 0 {
			fmt.Print("**Context map**\n\n")
			for _, c := range ctx {
				fmt.Printf("- %s\n", c)
			}
			fmt.Println()
		}
		fmt.Print(rollup.RenderMarkdown(rep, false))
	default:
		fmt.Printf("Landscape: %s\n", strings.Join(baseNames(repos), ", "))
		if len(ctx) > 0 {
			fmt.Println("\nContext map (publishes → subscribes, produces → consumes across boundaries)")
			for _, c := range ctx {
				fmt.Printf("  %s\n", c)
			}
		}
		fmt.Println()
		fmt.Print(rollup.RenderText(rep, fs.all))
	}
	if fs.graphOut != "" {
		if err := writeGraph(g, fs.graphOut); err != nil {
			return fail(err)
		}
	}
	if fs.viewOut != "" {
		data := viewer.Data{Generated: time.Now().UTC().Format(time.RFC3339), Repo: "landscape: " + strings.Join(baseNames(repos), ", "), Views: views.ProjectAll(g, ws.Views)}
		b, err := viewer.Render(data)
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(fs.viewOut, b, 0o644); err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "gloss landscape: viewer → %s\n", fs.viewOut)
	}
	if n := len(rep.Failing(strings.Split(fs.failOn, ","))); n > 0 {
		fmt.Fprintf(os.Stderr, "gloss landscape: %s (%s)\n", plural(n, "failing finding"), fs.failOn)
		return 1
	}
	return 0
}

type landscapeFlags struct {
	fs       *flag.FlagSet
	repos    []string
	format   string
	viewOut  string
	graphOut string
	failOn   string
	all      bool
}

func newLandscapeFlags() *landscapeFlags {
	f := &landscapeFlags{}
	fs, _ := newFlags("landscape")
	fs.Func("repo", "repository root (repeatable)", func(s string) error { f.repos = append(f.repos, s); return nil })
	fs.StringVar(&f.format, "format", "text", "text | json | markdown")
	fs.StringVar(&f.viewOut, "view", "", "write the landscape viewer HTML here")
	fs.StringVar(&f.graphOut, "graph", "", "write the union graph.jsonl here")
	fs.StringVar(&f.failOn, "fail-on", "violation,dangling", "finding kinds that fail the command")
	fs.BoolVar(&f.all, "all", false, "also list consistent pairs")
	f.fs = fs
	return f
}

// union merges graphs by IRI; later nodes fill blanks of earlier ones.
func union(gs []*graph.Graph, ws *load.Workspace) *graph.Graph {
	out := graph.New(ws.Prefixes)
	out.Generator = gs[0].Generator
	for _, g := range gs {
		for _, n := range g.Nodes() {
			nn := *n
			// A namespace imported by one repo and owned by another is owned in the landscape.
			if nn.NodeType == model.NodeConcept || nn.NodeType == model.NodeNamespace {
				nn.Imported = !ws.Owned[nn.NS]
			}
			if ex := out.Node(nn.ID); ex != nil && ex.Imported && !nn.Imported {
				*ex = nn
				continue
			}
			out.AddNode(&nn)
		}
		for _, e := range g.Edges() {
			if e.Source == graph.SourceDerived {
				continue
			}
			ee := *e
			out.AddEdge(&ee)
		}
	}
	return out
}

func baseNames(paths []string) []string {
	var out []string
	for _, p := range paths {
		abs, _ := filepath.Abs(p)
		out = append(out, filepath.Base(abs))
	}
	return out
}
