// @gloss implements gmi:LandscapeAssembler scope=file
// @gloss produces gm:Landscape scope=file

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/backstage"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/version"
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
		fmt.Fprintln(os.Stderr, "usage: gloss landscape [--format text|json|markdown] [--view out.html] [--graph out.jsonl] [--backstage dir] <repo-root>...")
		return 2
	}
	failKinds, err := rollup.ParseFailOn(fs.failOn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gloss landscape:", err)
		return 2
	}
	ws, g, wss, ledger, err := assembleLandscapeRepos(repos)
	if err != nil {
		return fail(err)
	}
	rep := rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy(), Now: time.Now()})
	// An overdue plan is the owning repository's to fix: a warning here,
	// not in the default --fail-on (ADR 0034 section 2).
	for i := range rep.Findings {
		if rep.Findings[i].Kind == rollup.KindPlanOverdue {
			rep.Findings[i].Severity = "warning"
		}
	}
	// Version state per repository, and who still uses deprecated concepts.
	entries, imports, outOfRange := landscapeVersions(repos, wss)
	rep.Findings = append(rep.Findings, outOfRange...)
	rollup.AddDeprecatedUse(rep, g, ws.Config.EffectivePolicy(), time.Now())
	// Context-map edges: implied depends-on (consumer → producer) between
	// boundaries, except between a boundary and one that contains it.
	var ctx []string
	var ctxEdges []backstage.ContextEdge
	anc := boundaryAncestors(g, ws)
	for _, e := range g.Edges() {
		if e.P == "depends-on" && e.Source == graph.SourceDerived && !anc[e.S][e.O] && !anc[e.O][e.S] {
			ctx = append(ctx, fmt.Sprintf("%s → %s via %s", g.Curie(e.S), g.Curie(e.O), strings.Join(e.Prov.Via, ", ")))
			ctxEdges = append(ctxEdges, backstage.ContextEdge{From: g.Curie(e.S), To: g.Curie(e.O), Via: e.Prov.Via})
		}
	}
	sort.Strings(ctx)
	sort.Slice(ctxEdges, func(i, j int) bool {
		if ctxEdges[i].From != ctxEdges[j].From {
			return ctxEdges[i].From < ctxEdges[j].From
		}
		return ctxEdges[i].To < ctxEdges[j].To
	})
	switch fs.format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"repos": entries, "imports": imports, "context_map": ctx, "drift": rep, "stats": g.Stats()})
	case "markdown":
		fmt.Printf("### Landscape: %s\n\n", strings.Join(baseNames(repos), ", "))
		if len(ctx) > 0 {
			fmt.Print("**Context map** (consumer → producer)\n\n")
			for _, c := range ctx {
				fmt.Printf("- %s\n", c)
			}
			fmt.Println()
		}
		if len(imports) > 0 {
			fmt.Print("**Imports**\n\n| importer | namespace | range | locked | resolved | status |\n| --- | --- | --- | --- | --- | --- |\n")
			for _, r := range imports {
				st := r.Status
				if r.Detail != "" {
					st += " (" + r.Detail + ")"
				}
				fmt.Printf("| %s | %s | %s | %s | %s | %s |\n", r.Repo, r.Namespace, orNoneV(r.Range), orNoneV(r.Locked), orNoneV(r.Resolved), st)
			}
			fmt.Println()
		}
		fmt.Print(rollup.RenderMarkdown(rep, false))
	default:
		fmt.Printf("Landscape: %s\n", strings.Join(baseNames(repos), ", "))
		if len(ctx) > 0 {
			fmt.Println("\nContext map (consumer → producer: depends on what it subscribes to or consumes)")
			for _, c := range ctx {
				fmt.Printf("  %s\n", c)
			}
		}
		fmt.Print(renderImports(imports))
		fmt.Println()
		fmt.Print(rollup.RenderText(rep, fs.all))
	}
	if fs.graphOut != "" {
		if err := writeGraph(g, fs.graphOut); err != nil {
			return fail(err)
		}
	}
	if fs.viewOut != "" {
		data := viewer.Data{Generated: time.Now().UTC().Format(time.RFC3339), Repo: "landscape: " + strings.Join(baseNames(repos), ", "), Views: views.ProjectAll(g, ws.Views), Slices: viewer.Slices(g, viewer.SliceBudget)}
		b, err := viewer.Render(data)
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(fs.viewOut, b, 0o644); err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "gloss landscape: viewer → %s\n", fs.viewOut)
	}
	if fs.backstageOut != "" {
		// One document per boundary of every repository, each linked to its
		// own repository's source-url, and the union's landscape.json.
		urls := map[string]string{}
		for _, w := range wss {
			urls[filepath.Base(w.Root)] = w.Config.SourceURL
		}
		docs, l := backstage.Build(g, ws, rep, backstage.Options{Landscape: true, SourceURLs: urls, Ledger: ledger, Generator: "gloss " + version.String(), ContextMap: ctxEdges})
		if err := backstage.Write(fs.backstageOut, docs, l); err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "gloss landscape: backstage: %s + %s → %s\n", countBoundaries(len(docs)), backstage.LandscapeFile, fs.backstageOut)
	}
	if failing := rep.Failing(failKinds); len(failing) > 0 {
		fmt.Fprintf(os.Stderr, "gloss landscape: %s\n", rollup.FailSummary(failing, failKinds))
		return 1
	}
	return 0
}

// assembleLandscape builds each repository, prefixes its artifact paths with
// the repository name so they stay unique, unions the graphs by IRI and
// re-derives the rollups on the union.
func assembleLandscape(repos []string) (*load.Workspace, *graph.Graph, error) {
	ws, g, _, _, err := assembleLandscapeRepos(repos)
	return ws, g, err
}

// assembleLandscapeRepos is assembleLandscape that also returns each
// repository's own workspace (for its imports, lock and versions) and the
// union of their ledgers, keyed by marker id with repo-prefixed paths.
func assembleLandscapeRepos(repos []string) (*load.Workspace, *graph.Graph, []*load.Workspace, map[string]*model.LedgerEntry, error) {
	var wss []*load.Workspace
	var graphs []*graph.Graph
	ledger := map[string]*model.LedgerEntry{}
	for _, r := range repos {
		p, err := buildGraph(r, false)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%s: %w", r, err)
		}
		// Prefix every path with the repo name so it stays unique in the
		// union and every finding says which repository it is in. An
		// imported ontology's ../sibling/ path cleans to sibling/.
		name := filepath.Base(p.WS.Root)
		for _, n := range p.Graph.Nodes() {
			if n.NodeType == model.NodeArtifact {
				n.Path = name + "/" + n.Path
				n.Label = name + "/" + n.Label
			}
			n.File = inRepo(name, n.File)
		}
		for _, e := range p.Graph.Edges() {
			if e.Prov != nil {
				e.Prov.Path = inRepo(name, e.Prov.Path)
			}
		}
		for id, le := range p.Ledger.Entries {
			if _, ok := ledger[id]; ok {
				continue // one marker id, one row: the first repository's
			}
			ll := *le
			ll.Path = inRepo(name, le.Path)
			ledger[id] = &ll
		}
		wss = append(wss, p.WS)
		graphs = append(graphs, p.Graph)
	}
	ws, err := load.Merge(wss)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	g := union(graphs, ws)
	rollup.Derive(g, ws.Vocab)
	return ws, g, wss, ledger, nil
}

type landscapeFlags struct {
	fs           *flag.FlagSet
	repos        []string
	format       string
	viewOut      string
	graphOut     string
	backstageOut string
	failOn       string
	all          bool
}

func newLandscapeFlags() *landscapeFlags {
	f := &landscapeFlags{}
	fs, _ := newFlags("landscape")
	fs.Func("repo", "repository root (repeatable)", func(s string) error { f.repos = append(f.repos, s); return nil })
	fs.StringVar(&f.format, "format", "text", "text | json | markdown")
	fs.StringVar(&f.viewOut, "view", "", "write the landscape viewer HTML here")
	fs.StringVar(&f.graphOut, "graph", "", "write the union graph.jsonl here")
	fs.StringVar(&f.backstageOut, "backstage", "", "write one JSON document per boundary of every repository plus landscape.json into this directory (gloss export --backstage over the union)")
	fs.StringVar(&f.failOn, "fail-on", "violation,dangling,overdue", "finding kinds that fail the command")
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

// boundaryAncestors maps each boundary IRI to the boundaries that contain it:
// what it is part-of, and the boundary of every manifest whose directory
// encloses its own (merged manifest directories carry the repo name, so
// nesting never crosses repositories), transitively.
func boundaryAncestors(g *graph.Graph, ws *load.Workspace) map[string]map[string]bool {
	byCurie := map[string]string{}
	for _, b := range g.NodesOf(model.NodeBoundary) {
		byCurie[b.CURIE] = b.ID
	}
	parents := map[string][]string{}
	for _, b := range g.NodesOf(model.NodeBoundary) {
		parents[b.ID] = append(parents[b.ID], g.Objects(b.ID, "part-of")...)
	}
	for _, m := range ws.Manifests {
		id := byCurie[m.Boundary]
		for _, o := range ws.Manifests {
			if oid := byCurie[o.Boundary]; id != "" && oid != "" && oid != id && strings.HasPrefix(m.Dir, o.Dir+"/") {
				parents[id] = append(parents[id], oid)
			}
		}
	}
	out := map[string]map[string]bool{}
	for id := range parents {
		seen := map[string]bool{}
		stack := append([]string{}, parents[id]...)
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if p == id || seen[p] {
				continue
			}
			seen[p] = true
			stack = append(stack, parents[p]...)
		}
		out[id] = seen
	}
	return out
}

// inRepo prefixes a workspace-relative path with the repository name.
func inRepo(name, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return path.Clean(name + "/" + filepath.ToSlash(p))
}

func baseNames(paths []string) []string {
	var out []string
	for _, p := range paths {
		abs, _ := filepath.Abs(p)
		out = append(out, filepath.Base(abs))
	}
	return out
}
