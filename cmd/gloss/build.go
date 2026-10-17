package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func init() {
	register(command{"build", "extract markers, manifests and ontologies into .glossmark/graph.jsonl", cmdBuild})
	register(command{"version", "print the version", func([]string) int { fmt.Println("gloss", extract.Version); return 0 }})
}

// pipeline is the shared load → extract → derive → ledger-sync sequence.
type pipeline struct {
	WS     *load.Workspace
	Res    *extract.Result
	Graph  *graph.Graph
	Ledger *ledger.Ledger
	Diags  []marker.Diagnostic
	Took   time.Duration
}

// buildGraph runs the pipeline. When writeLedger is true the ledger is synced
// to disk; otherwise it is only read (so read-only commands stay read-only).
func buildGraph(root string, writeLedger bool) (*pipeline, error) {
	start := time.Now()
	if root == "" {
		cwd, _ := os.Getwd()
		r, err := load.FindRoot(cwd)
		if err != nil {
			return nil, err
		}
		root = r
	}
	ws, err := load.Load(root)
	if err != nil {
		return nil, err
	}
	res, err := extract.Build(ws)
	if err != nil {
		return nil, err
	}
	g := res.Graph
	rollup.Derive(g, ws.Vocab)
	lg, err := ledger.Open(ws.StatePath(ledger.FileName))
	if err != nil {
		return nil, err
	}
	actor := os.Getenv("GLOSS_ACTOR")
	if actor == "" {
		actor = ledger.DefaultActor(root)
	}
	lg.Sync(g, res.Markers, actor, time.Now().UTC())
	if writeLedger {
		if err := lg.Save(); err != nil {
			return nil, err
		}
	}
	return &pipeline{WS: ws, Res: res, Graph: g, Ledger: lg, Diags: res.Diags, Took: time.Since(start)}, nil
}

func cmdBuild(args []string) int {
	fs, root := newFlags("build")
	out := fs.String("o", "", "output path (default .glossmark/graph.jsonl under the root)")
	quiet := fs.Bool("q", false, "print only diagnostics")
	noLedger := fs.Bool("no-ledger", false, "do not update .glossmark/ledger.jsonl")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, !*noLedger)
	if err != nil {
		return fail(err)
	}
	path := *out
	if path == "" {
		path = p.WS.StatePath("graph.jsonl")
	}
	if err := writeGraph(p.Graph, path); err != nil {
		return fail(err)
	}
	printDiags(p.Diags)
	if !*quiet {
		s := p.Graph.Stats()
		fmt.Printf("gloss build: %d files scanned, %s, %d nodes (%d concepts, %d boundaries, %d artifacts), %d edges (%d declared, %d inline, %d derived) in %s → %s\n",
			p.Res.ScannedFiles, plural(len(p.Res.Markers), "marker"), s["nodes"], s["nodes.concept"], s["nodes.boundary"], s["nodes.artifact"],
			s["edges"], s["edges.declared"], s["edges.inline"], s["edges.derived"], p.Took.Round(time.Millisecond), relOrAbs(p.WS.Root, path))
	}
	if hasErrors(p.Diags) {
		return 1
	}
	return 0
}

func writeGraph(g *graph.Graph, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := g.WriteJSONL(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// JSON-LD context beside the graph makes the same file RDF-loadable.
	return os.WriteFile(filepath.Join(filepath.Dir(path), "context.jsonld"), []byte(graph.JSONLDContext(g)), 0o644)
}

func printDiags(diags []marker.Diagnostic) {
	for _, d := range diags {
		fmt.Fprintln(os.Stderr, d.String())
	}
}

func hasErrors(diags []marker.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

func relOrAbs(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return r
	}
	return path
}
