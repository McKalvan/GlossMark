// @gloss implements gmi:McpServer scope=file
// @gloss publishes gm:LinkProposed scope=file

package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mckalvan/glossmark/internal/ontoedit"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/mcp"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/review"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/version"
)

func init() {
	register(command{"mcp", "serve the graph to agents over MCP on stdio (lookup_concept, search_concepts, concepts_for_path, propose_link, impact_of, slice_of, drift, and the review tools review_queue, review_item, review_decide, review_undo, write_link, edit_concept_doc, browse_node)", cmdMCP})
}

type mcpBackend struct {
	root   string
	mu     sync.Mutex
	p      *pipeline
	at     time.Time
	engine *review.Engine
}

// ReviewEngine serves the review tools (ADR 0033). A decision changes the
// tree, so the cached build is dropped for the next tool.
func (b *mcpBackend) ReviewEngine() (*review.Engine, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.p = nil
	if b.engine == nil {
		e, err := newEngine(b.root)
		if err != nil {
			return nil, err
		}
		b.engine = e
	}
	return b.engine, nil
}

func (b *mcpBackend) Current() (*load.Workspace, *graph.Graph, map[string]*model.LedgerEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Rebuild at most every 2 seconds so a burst of calls does not re-scan.
	if b.p == nil || time.Since(b.at) > 2*time.Second {
		p, err := buildGraph(b.root, false)
		if err != nil {
			return nil, nil, nil, err
		}
		b.p, b.at = p, time.Now()
	}
	return b.p.WS, b.p.Graph, b.p.Ledger.Entries, nil
}

func (b *mcpBackend) Impact(props []mcp.Proposal) (any, error) {
	ws, g, _, err := b.Current()
	if err != nil {
		return nil, err
	}
	pol := ws.Config.EffectivePolicy()
	overall := gate.AUTO
	var items []map[string]any
	for _, p := range props {
		rel := ws.Vocab.Get(p.Relation)
		item := map[string]any{"path": p.Path, "relation": p.Relation, "target": p.Target}
		if rel == nil || !rel.Inline {
			item["error"] = fmt.Sprintf("relation %q cannot be used inline", p.Relation)
			item["gate"] = "HARD"
			overall = gate.HARD
			items = append(items, item)
			continue
		}
		// Render the marker line in the file's comment style.
		style, ok := marker.StyleFor(p.Path, ws.Config.CommentStyles)
		tok := "#"
		if ok && len(style.Line) > 0 && style.Line[0] != "" {
			tok = style.Line[0]
		}
		var kv []string
		if p.Scope != "" {
			kv = append(kv, "scope="+p.Scope)
		}
		if p.Conf > 0 {
			kv = append(kv, fmt.Sprintf("conf=%.2f", p.Conf))
		}
		if p.Note != "" {
			kv = append(kv, fmt.Sprintf("note=%q", p.Note))
		}
		line := fmt.Sprintf("@gloss? %s %s", p.Relation, p.Target)
		if len(kv) > 0 {
			line += " " + strings.Join(kv, " ")
		}
		if !ok {
			item["sidecar"] = p.Path + ".gloss"
			item["marker"] = line
		} else if len(style.Line) == 0 && len(style.Block) > 0 {
			item["marker"] = style.Block[0][0] + " " + line + " " + style.Block[0][1]
		} else {
			item["marker"] = tok + " " + line
		}
		// Resolve the target like the extractor would and classify.
		m := ws.ManifestFor(p.Path)
		target := p.Target
		if strings.HasPrefix(target, ".") {
			if m == nil || m.DefaultNamespace == "" {
				item["error"] = "local target needs a default-namespace in an enclosing .gloss.yaml"
				item["gate"] = "HARD"
				overall = gate.HARD
				items = append(items, item)
				continue
			}
			target = m.DefaultNamespace + ":" + target[1:]
		}
		var oiri string
		switch {
		case marker.IsNewTarget(target):
			oiri = "urn:gloss:new:" + target
		default:
			oiri, _ = g.Expand(target)
		}
		conf := p.Conf
		aid := graph.ArtifactPrefix + p.Path
		g.AddNode(&graph.Node{NodeType: model.NodeArtifact, ID: aid, Kind: "file", Path: p.Path, Label: p.Path})
		e := &graph.Edge{S: aid, P: p.Relation, O: oiri, Source: graph.SourceInline, Prov: &graph.Prov{Path: p.Path, Proposed: true, Actor: os.Getenv("GLOSS_ACTOR"), ID: extract.MarkerID(p.Path, p.Relation, []string{target}, "", "file")}}
		if conf > 0 {
			e.Prov.Conf = &conf
		}
		cls, why := gate.ClassifyLink(g, ws, e, pol)
		item["gate"] = string(cls)
		item["reasons"] = why
		if on := g.Node(oiri); on != nil {
			item["target_kind"] = on.Kind
			if len(rel.RangeKinds) > 0 && on.Kind != "" && !containsStr(rel.RangeKinds, on.Kind) {
				item["warning"] = check.RangeKindMsg(p.Relation, rel.RangeKinds, on.CURIE, on.Kind)
			}
		} else if !marker.IsNewTarget(target) {
			item["error"] = fmt.Sprintf("concept %s does not exist; use %s:NEW:%s to propose it", target, strings.SplitN(target, ":", 2)[0], lastPart(target))
		}
		overall = gate.Max(overall, cls)
		items = append(items, item)
	}
	// Invalidate the cached graph since we added scratch nodes.
	b.mu.Lock()
	b.p = nil
	b.mu.Unlock()
	rep := rollup.Compare(g, ws, rollup.Options{Policy: pol})
	return map[string]any{"gate": string(overall), "proposals": items, "current_drift": map[string]any{"violations": rep.Metrics.Violations, "gaps": rep.Metrics.Gaps, "orphans": rep.Metrics.Orphans, "rogues": rep.Metrics.Rogues}}, nil
}

func (b *mcpBackend) ImpactOfChange(base string, paths []string) (any, error) {
	ws, g, lg, err := b.Current()
	if err != nil {
		return nil, err
	}
	files := append([]string{}, paths...)
	if base != "" {
		ch, err := changedFiles(ws.Root, base)
		if err != nil {
			return nil, err
		}
		files = append(files, ch...)
	}
	touched := gate.Touched(g, ws, uniqStrings(files))
	out := map[string]any{"files": touched}
	if ss := slicesTouched(g, ws, touched); len(ss) > 0 {
		out["slices"] = ss
	}
	if base != "" {
		bg, err := loadBaseGraph(ws, base)
		if err != nil {
			return nil, err
		}
		pol := ws.Config.EffectivePolicy()
		after := rollup.Compare(g, ws, rollup.Options{Ledger: lg, Policy: pol})
		im := gate.Classify(bg, g, ws, gate.Options{Base: base, Policy: pol, HeadLedger: lg, ChangedFiles: files, AfterReport: after})
		out["gate"] = im.Gate
		out["changes"] = im.Changes
		out["views_changed"] = im.ViewsChanged
		out["rollup"] = im.Rollup
	}
	return out, nil
}

func (b *mcpBackend) ProposeConcept(cp mcp.ConceptProposal) (any, error) {
	b.mu.Lock()
	b.p = nil
	b.mu.Unlock()
	p, err := buildGraph(b.root, false)
	if err != nil {
		return nil, err
	}
	ns, name, err := splitCurie(cp.Curie)
	if err != nil {
		return nil, err
	}
	actor := os.Getenv("GLOSS_ACTOR")
	if actor == "" {
		actor = "agent:mcp"
	}
	spec := ontoedit.ConceptSpec{Kind: cp.Kind, Doc: cp.Doc, Realizes: cp.Realizes, IsA: cp.IsA, Internal: cp.Internal, Aliases: cp.Aliases, Proposed: true, ProposedBy: actor}
	if msg := validateSpec(p, ns, spec); msg != "" {
		return nil, fmt.Errorf("%s", msg)
	}
	path, err := ontologyPath(p, ns)
	if err != nil {
		return nil, err
	}
	if err := ontoedit.AddConcept(path, name, spec); err != nil {
		return nil, err
	}
	curie := ns + ":" + name
	rewritten, _ := ontoedit.RewriteNewMarkers(absFiles(p), curie)
	if rewritten > 0 {
		if err := carryMarkerIDs(p); err != nil {
			return nil, err
		}
	}
	if _, err := buildGraph(b.root, true); err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.p = nil
	b.mu.Unlock()
	return map[string]any{"curie": curie, "file": relOrAbs(p.WS.Root, path), "status": "proposed", "markers_rewritten": rewritten,
		"next": "link to it with @gloss? ... " + curie + "; a human accepts it with: gloss accept --concept " + curie}, nil
}

func (b *mcpBackend) Context(dir string, budget int, changed bool) (string, error) {
	p, err := buildGraph(b.root, false)
	if err != nil {
		return "", err
	}
	return briefing(p, dir, 0, budget, changed), nil
}

func containsStr(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func lastPart(c string) string {
	p := strings.Split(c, ":")
	return p[len(p)-1]
}

func cmdMCP(args []string) int {
	fs, root := newFlags("mcp")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	r := *root
	if r == "" {
		cwd, _ := os.Getwd()
		rr, err := load.FindRoot(cwd)
		if err != nil {
			return fail(err)
		}
		r = rr
	}
	srv := mcp.New(&mcpBackend{root: r}, os.Stdin, os.Stdout, version.String())
	if err := srv.Serve(); err != nil {
		return fail(err)
	}
	return 0
}
