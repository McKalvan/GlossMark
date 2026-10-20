// @gloss implements gmi:Publisher, gmi:SqliteExport, gmi:AgentsSnippet scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/store"
)

func init() {
	register(command{"export", "export the graph: --sqlite path | --prov path (PROV-O JSON-LD ledger) | --status (implemented/untested/orphan/proposed per concept, JSON)", cmdExport})
	register(command{"publish", "write per-namespace ontology.jsonld and graph.jsonl for importers (default dist/glossmark/)", cmdPublish})
	register(command{"agents", "print the AGENTS.md vocabulary snippet for the repo or a directory (--dir)", cmdAgents})
}

func cmdExport(args []string) int {
	fs, root := newFlags("export")
	sqlitePath := fs.String("sqlite", "", "write a SQLite database with closure views")
	provPath := fs.String("prov", "", "write the ledger as PROV-O JSON-LD")
	status := fs.Bool("status", false, "write implemented / untested / orphan / proposed per owned concept as JSON (for an external checker)")
	out := fs.String("o", "", "with --status: write to this file instead of stdout")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if *sqlitePath == "" && *provPath == "" && !*status {
		fmt.Fprintln(os.Stderr, "usage: gloss export --sqlite graph.db | --prov ledger.prov.jsonld | --status [-o status.json]")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if *status {
		rep := rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: p.WS.Config.EffectivePolicy(), Now: time.Now()})
		b, err := json.MarshalIndent(rollup.ConceptStatus(p.Graph, rep), "", "  ")
		if err != nil {
			return fail(err)
		}
		b = append(b, '\n')
		if *out == "" {
			os.Stdout.Write(b)
		} else if err := os.WriteFile(*out, b, 0o644); err != nil {
			return fail(err)
		}
	}
	if *sqlitePath != "" {
		if err := store.Export(p.Graph, *sqlitePath); err != nil {
			return fail(err)
		}
		fmt.Printf("gloss export: sqlite → %s\n", *sqlitePath)
	}
	if *provPath != "" {
		b, err := p.Ledger.ExportPROV()
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(*provPath, b, 0o644); err != nil {
			return fail(err)
		}
		fmt.Printf("gloss export: PROV-O → %s\n", *provPath)
	}
	return 0
}

func cmdPublish(args []string) int {
	fs, root := newFlags("publish")
	out := fs.String("o", "dist/glossmark", "output directory (one subdirectory per owned namespace)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	g := p.Graph
	prefixes := make([]string, 0, len(p.WS.Owned))
	for ns := range p.WS.Owned {
		prefixes = append(prefixes, ns)
	}
	sort.Strings(prefixes)
	outDir := *out
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(p.WS.Root, outDir)
	}
	for _, ns := range prefixes {
		o := p.WS.Ontologies[ns]
		dir := filepath.Join(outDir, ns)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail(err)
		}
		// ontology.jsonld: the types. For implementation namespaces only published concepts cross repos.
		ctx := map[string]any{"gm": graph.VocabIRI, "label": "http://www.w3.org/2000/01/rdf-schema#label", "doc": "http://www.w3.org/2000/01/rdf-schema#comment",
			"is-a": map[string]any{"@id": "http://www.w3.org/2000/01/rdf-schema#subClassOf", "@type": "@id"}}
		for pre, iri := range g.Prefixes {
			ctx[pre] = iri
		}
		for _, r := range model.BuiltinRelations {
			if _, ok := ctx[r.Name]; !ok {
				ctx[r.Name] = map[string]any{"@id": "gm:" + r.Name, "@type": "@id"}
			}
		}
		var items []map[string]any
		for _, n := range g.NodesOf(model.NodeConcept) {
			if n.NS != ns {
				continue
			}
			if o.Tier == model.TierImpl && n.Published == "" {
				continue
			}
			item := map[string]any{"@id": n.CURIE, "@type": "gm:Concept", "gm:kind": n.Kind, "gm:tier": n.Tier}
			if n.Label != "" {
				item["label"] = n.Label
			}
			if n.Doc != "" {
				item["doc"] = n.Doc
			}
			if n.Published != "" {
				item["published-as"] = n.Published
			}
			if n.Deprecated {
				item["gm:deprecated"] = true
				if n.ReplacedBy != "" {
					item["replaced-by"] = n.ReplacedBy
				}
			}
			if n.Value != nil {
				item["gm:value"] = fmt.Sprint(n.Value)
			}
			for _, e := range g.Out(n.ID, "") {
				if e.Source != graph.SourceDeclared {
					continue
				}
				on := g.Node(e.O)
				if on != nil && on.NodeType == model.NodeConcept && on.Tier == model.TierImpl && on.Published == "" && on.NS == ns {
					continue // internal structure stays private
				}
				key := e.P
				cur := g.Curie(e.O)
				switch v := item[key].(type) {
				case nil:
					item[key] = cur
				case string:
					item[key] = []string{v, cur}
				case []string:
					item[key] = append(v, cur)
				}
			}
			items = append(items, item)
		}
		doc := map[string]any{"@context": ctx, "@id": o.IRI, "@type": "gm:Namespace", "gm:prefix": ns, "gm:tier": o.Tier, "gm:version": o.Version, "@graph": items}
		b, _ := json.MarshalIndent(doc, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "ontology.jsonld"), b, 0o644); err != nil {
			return fail(err)
		}
		// graph.jsonl for implementation namespaces: the assignments and boundaries that reference published concepts.
		if o.Tier == model.TierImpl {
			sub := graph.New(g.Prefixes)
			sub.Generator = g.Generator
			keep := map[string]bool{}
			for _, n := range g.NodesOf(model.NodeConcept) {
				if n.NS == ns && n.Published != "" {
					keep[n.ID] = true
				}
			}
			for _, n := range g.NodesOf(model.NodeBoundary) {
				keep[n.ID] = true
				sub.AddNode(n)
			}
			for id := range keep {
				if n := g.Node(id); n != nil {
					sub.AddNode(n)
				}
			}
			for _, e := range g.Edges() {
				if keep[e.S] && (keep[e.O] || g.Node(e.O) != nil && g.Node(e.O).Tier == model.TierCore) {
					if on := g.Node(e.O); on != nil {
						sub.AddNode(on)
					}
					sub.AddEdge(e)
				}
			}
			f, err := os.Create(filepath.Join(dir, "graph.jsonl"))
			if err != nil {
				return fail(err)
			}
			sub.WriteJSONL(f)
			f.Close()
		}
		// The source file itself, so importers can point source: at the directory.
		if src, err := os.ReadFile(o.Path); err == nil {
			os.WriteFile(filepath.Join(dir, ns+".gloss.yaml"), src, 0o644)
		}
		fmt.Printf("gloss publish: %s: (%s %s) → %s\n", ns, o.Tier, o.Version, relOrAbs(p.WS.Root, dir))
	}
	return 0
}

func cmdAgents(args []string) int {
	fs, root := newFlags("agents")
	dir := fs.String("dir", "", "directory whose nearest manifest scopes the vocabulary (default: whole repo)")
	write := fs.String("write", "", "write into this file between <!-- gloss:begin --> and <!-- gloss:end --> markers (e.g. AGENTS.md)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	var m *model.Manifest
	if *dir != "" {
		rel := *dir
		if filepath.IsAbs(rel) {
			rel, _ = filepath.Rel(p.WS.Root, rel)
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		m = p.WS.ManifestFor(rel + "/x")
	}
	snippet := agents.Snippet(p.WS, p.Graph, m)
	if *write == "" {
		fmt.Print(snippet)
		return 0
	}
	const begin, end = "<!-- gloss:begin -->", "<!-- gloss:end -->"
	existing, _ := os.ReadFile(*write)
	s := string(existing)
	block := begin + "\n" + snippet + end
	if i := strings.Index(s, begin); i >= 0 {
		if j := strings.Index(s, end); j > i {
			s = s[:i] + block + s[j+len(end):]
		} else {
			s = s[:i] + block
		}
	} else {
		if s != "" && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += "\n" + block + "\n"
	}
	if err := os.WriteFile(*write, []byte(s), 0o644); err != nil {
		return fail(err)
	}
	fmt.Printf("gloss agents: vocabulary written to %s\n", *write)
	return 0
}
