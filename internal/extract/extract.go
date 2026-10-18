// Package extract is pass 1 of the extractor: ontology files, manifests and
// inline markers become nodes and edges of the canonical graph. Derived edges
// are added afterwards by the rollup package.
package extract

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/version"
)

// Result is the extractor output.
type Result struct {
	Graph *graph.Graph
	Diags []marker.Diagnostic
	// Markers lists every marker found, in file order, with its edge.
	Markers []FoundMarker
	// ScannedFiles is the number of files read.
	ScannedFiles int
}

// FoundMarker ties a marker to the edges it produced.
type FoundMarker struct {
	Path   string
	Marker *marker.Marker
	Anchor marker.Anchor
	ID     string
	Edges  []*graph.Edge
}

// Build runs pass 1 over a loaded workspace.
func Build(ws *load.Workspace) (*Result, error) {
	g := graph.New(ws.Prefixes)
	g.Generator = version.Generator()
	res := &Result{Graph: g}
	res.Diags = append(res.Diags, ws.Diags...)
	ex := &extractor{ws: ws, g: g, res: res}
	ex.namespaces()
	ex.ontologies()
	ex.manifests()
	if err := ex.files(); err != nil {
		return nil, err
	}
	return res, nil
}

type extractor struct {
	ws  *load.Workspace
	g   *graph.Graph
	res *Result
}

func (ex *extractor) diag(path string, line int, sev, code, msg string) {
	ex.res.Diags = append(ex.res.Diags, marker.Diagnostic{Path: path, Line: line, Severity: sev, Code: code, Msg: msg})
}

func (ex *extractor) namespaces() {
	names := make([]string, 0, len(ex.ws.Ontologies))
	for p := range ex.ws.Ontologies {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		o := ex.ws.Ontologies[p]
		n := &graph.Node{NodeType: model.NodeNamespace, ID: o.IRI, CURIE: p + ":", NS: p, Tier: o.Tier, Version: o.Version, Doc: o.Doc, Imported: !ex.ws.Owned[p]}
		if d, ok := ex.ws.Config.Owns[p]; ok {
			n.Owner = d.Owner
			n.File = d.File
		}
		if im, ok := ex.ws.Imported[p]; ok {
			n.File = im.Source
		}
		ex.g.AddNode(n)
	}
}

// resolveRef expands a reference written in an ontology file: bare names are
// in the file's namespace; CURIEs are expanded.
func (ex *extractor) resolveRef(ref, ns string) (string, string) {
	ref = strings.TrimSpace(ref)
	if !strings.Contains(ref, ":") {
		ref = ns + ":" + ref
	}
	iri, _ := ex.g.Expand(ref)
	return iri, ref
}

func (ex *extractor) ontologies() {
	names := make([]string, 0, len(ex.ws.Ontologies))
	for p := range ex.ws.Ontologies {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, ns := range names {
		o := ex.ws.Ontologies[ns]
		file := o.Path
		if rel, err := relPath(ex.ws.Root, file); err == nil {
			file = rel
		}
		cnames := make([]string, 0, len(o.Concepts))
		for c := range o.Concepts {
			cnames = append(cnames, c)
		}
		sort.Strings(cnames)
		for _, name := range cnames {
			c := o.Concepts[name]
			curie := ns + ":" + name
			iri, _ := ex.g.Expand(curie)
			n := &graph.Node{NodeType: model.NodeConcept, ID: iri, CURIE: curie, NS: ns, Tier: o.Tier, Kind: c.Kind, Label: c.Label, Doc: c.Doc,
				Internal: c.Internal, Deprecated: c.Deprecated, Value: c.Value, Imported: !ex.ws.Owned[ns], File: file, Line: c.Line,
				Proposed: c.Proposed, ProposedBy: c.ProposedBy, Aliases: []string(c.Aliases)}
			if c.Criticality != "" {
				n.Criticality, n.CriticalityFrom = c.Criticality, "declared"
			}
			if c.ReplacedBy != "" {
				_, cur := ex.resolveRef(c.ReplacedBy, ns)
				n.ReplacedBy = cur
			}
			if c.Published != nil {
				_, cur := ex.resolveRef(c.Published.As, ns)
				n.Published = cur
			}
			ex.g.AddNode(n)
			add := func(p string, targets []string) {
				for _, t := range targets {
					oiri, raw := ex.resolveRef(t, ns)
					ex.g.AddEdge(&graph.Edge{S: iri, P: p, O: oiri, Source: graph.SourceDeclared, Prov: &graph.Prov{Path: file, Line: c.Line, Raw: raw}})
				}
			}
			add("is-a", c.IsA)
			add("part-of", c.PartOf)
			add("has-part", c.HasPart)
			add("realizes", c.Realizes)
			if c.Published != nil {
				add("published-as", []string{c.Published.As})
			}
			for _, rel := range c.RelationNames() {
				add(rel, c.Relations[rel])
			}
		}
	}
}

// boundaryIRI expands a boundary CURIE, registering an urn prefix for unknown prefixes.
func (ex *extractor) boundaryIRI(curie, path string, line int) string {
	i := strings.Index(curie, ":")
	if i <= 0 {
		ex.diag(path, line, "error", "GM111", fmt.Sprintf("boundary %q must be written as prefix:name (org:name)", curie))
		return "urn:gloss:boundary::" + curie
	}
	prefix := curie[:i]
	if _, ok := ex.g.Prefixes[prefix]; !ok {
		ex.g.Prefixes[prefix] = "urn:gloss:boundary:" + prefix + ":"
		ex.diag(path, line, "warning", "GM112", fmt.Sprintf("boundary prefix %q is not declared under prefixes: in %s; using urn:gloss:boundary:%s:", prefix, load.ConfigName, prefix))
	}
	iri, _ := ex.g.Expand(curie)
	return iri
}

func (ex *extractor) manifests() {
	for _, m := range ex.ws.Manifests {
		iri := ex.boundaryIRI(m.Boundary, m.Path, 1)
		n := &graph.Node{NodeType: model.NodeBoundary, ID: iri, CURIE: m.Boundary, Kind: m.Level, Label: m.Label, Doc: m.Doc, Dir: m.Dir, File: m.Path, Line: 1, Owner: m.Owner}
		if m.Criticality != "" {
			n.Criticality, n.CriticalityFrom = m.Criticality, "declared"
		}
		if i := strings.Index(m.Boundary, ":"); i > 0 {
			n.NS = m.Boundary[:i]
		}
		ex.g.AddNode(n)
		for _, p := range m.PartOf {
			piri := ex.boundaryIRI(p, m.Path, 1)
			if ex.g.Node(piri) == nil {
				// Implicit parent boundary: declared only by reference.
				pn := &graph.Node{NodeType: model.NodeBoundary, ID: piri, CURIE: p, Kind: parentLevel(m.Level), File: m.Path, Line: 1}
				if i := strings.Index(p, ":"); i > 0 {
					pn.NS = p[:i]
				}
				ex.g.AddNode(pn)
			}
			ex.g.AddEdge(&graph.Edge{S: iri, P: "part-of", O: piri, Source: graph.SourceDeclared, Prov: &graph.Prov{Path: m.Path, Line: 1, Raw: p}})
		}
		for rel, targets := range m.DeclaredSets() {
			for _, t := range targets {
				oiri, _ := ex.g.Expand(t)
				ex.g.AddEdge(&graph.Edge{S: iri, P: rel, O: oiri, Source: graph.SourceDeclared, Prov: &graph.Prov{Path: m.Path, Line: 1, Raw: t}})
			}
		}
		if m.Owner != "" {
			actor := m.Owner
			if !strings.Contains(actor, ":") {
				actor = "team:" + actor
			}
			aid := "urn:gloss:actor:" + actor
			ex.g.AddNode(&graph.Node{NodeType: model.NodeActor, ID: aid, CURIE: actor, Label: m.Owner})
			ex.g.AddEdge(&graph.Edge{S: aid, P: "owns", O: iri, Source: graph.SourceDeclared, Prov: &graph.Prov{Path: m.Path, Line: 1}})
		}
	}
}

func parentLevel(level string) string {
	for i, l := range model.BoundaryLevels {
		if l == level && i > 0 {
			return model.BoundaryLevels[i-1]
		}
	}
	return "domain"
}

func (ex *extractor) files() error {
	for _, rel := range ex.ws.Files {
		style, _ := marker.StyleFor(rel, ex.ws.Config.CommentStyles)
		f, err := os.Open(ex.ws.Abs(rel))
		if err != nil {
			return err
		}
		found, diags, lines := marker.Scan(f, style)
		f.Close()
		ex.res.ScannedFiles++
		for _, d := range diags {
			d.Path = rel
			ex.res.Diags = append(ex.res.Diags, d)
		}
		if len(found) == 0 {
			continue
		}
		target := rel
		if marker.IsSidecar(rel) {
			target = marker.SidecarTarget(rel)
			if _, err := os.Stat(ex.ws.Abs(target)); err != nil {
				ex.diag(rel, 1, "warning", "GM121", fmt.Sprintf("sidecar annotates %s, which does not exist", target))
			}
		}
		m := ex.ws.ManifestFor(rel)
		var boundary string
		if m != nil {
			boundary = ex.boundaryIRI(m.Boundary, m.Path, 1)
		}
		for _, fd := range found {
			ex.marker(rel, target, fd, lines, style, m, boundary)
		}
	}
	return nil
}

func (ex *extractor) marker(rel, target string, fd marker.Found, lines []string, style marker.CommentStyle, m *model.Manifest, boundary string) {
	mk := fd.Marker
	anchor := marker.Resolve(rel, fd, lines, style)
	if marker.IsSidecar(rel) && anchor.Scope == "symbol" {
		anchor.Scope = "file"
		anchor.Symbol = ""
	}
	if mk.Scope() == "symbol" && anchor.Kind == "unresolved-symbol" {
		ex.diag(rel, mk.Line, "warning", "GM122", "scope=symbol requested but no declaration was detected after the marker; anchored to the next block")
	}
	// Artifact node.
	aid := graph.ArtifactPrefix + target
	label := target
	kind := "file"
	if anchor.Scope == "symbol" && anchor.Symbol != "" {
		aid += "#" + anchor.Symbol
		label = target + ":" + anchor.Symbol
		kind = anchor.Kind
	} else if anchor.Scope == "block" || anchor.Scope == "line" || anchor.Scope == "range" {
		aid += fmt.Sprintf("#L%d", anchor.StartLine)
		label = fmt.Sprintf("%s:%d", target, anchor.StartLine)
		kind = anchor.Scope
	}
	a := anchor
	art := ex.g.AddNode(&graph.Node{NodeType: model.NodeArtifact, ID: aid, Kind: kind, Label: label, Path: target, Anchor: &a, Boundary: boundary, File: rel, Line: mk.Line})
	if art.Boundary == "" {
		art.Boundary = boundary
	}
	id := mk.Keys["id"]
	if id == "" {
		id = MarkerID(target, mk.Relation, mk.Targets, anchor.Symbol, anchor.Scope)
	}
	fm := FoundMarker{Path: rel, Marker: mk, Anchor: anchor, ID: id}
	for _, t := range mk.Targets {
		oiri, raw := ex.resolveTarget(t, m, rel, mk.Line)
		prov := &graph.Prov{Path: rel, Line: mk.Line, Col: mk.Col, Scope: anchor.Scope, Symbol: anchor.Symbol, Fingerprint: anchor.Fingerprint,
			Proposed: mk.Proposed, ID: id, Note: mk.Keys["note"], Sugar: mk.Sugar, Raw: raw}
		if c := mk.Conf(); c >= 0 {
			cc := c
			prov.Conf = &cc
		}
		e := &graph.Edge{S: aid, P: mk.Relation, O: oiri, Source: graph.SourceInline, Prov: prov}
		ex.g.AddEdge(e)
		fm.Edges = append(fm.Edges, e)
		// Inferrer: a test-path artifact that implements/represents also tests the concept.
		if (mk.Relation == "implements" || mk.Relation == "represents") && ex.ws.IsTestPath(rel) {
			ex.g.AddEdge(&graph.Edge{S: aid, P: "tests", O: oiri, Source: graph.SourceInferred, Prov: &graph.Prov{Path: rel, Line: mk.Line, Rule: "test-path", ID: id, Proposed: mk.Proposed}})
		}
	}
	ex.res.Markers = append(ex.res.Markers, fm)
}

// resolveTarget maps a marker target to an IRI: .Local via the manifest's
// default namespace, prefix:NEW:Name to a proposal URN, globs to a glob URN.
func (ex *extractor) resolveTarget(t string, m *model.Manifest, path string, line int) (string, string) {
	raw := t
	if strings.HasPrefix(t, ".") {
		if m == nil || m.DefaultNamespace == "" {
			ex.diag(path, line, "error", "GM120", fmt.Sprintf("local target %q needs a default-namespace in an enclosing %s manifest", t, load.ManifestName))
			return "urn:gloss:unresolved:" + t, raw
		}
		t = m.DefaultNamespace + ":" + t[1:]
	}
	if marker.IsNewTarget(t) {
		return "urn:gloss:new:" + t, raw
	}
	if strings.Contains(t, "*") {
		return "urn:gloss:glob:" + t, raw
	}
	iri, _ := ex.g.Expand(t)
	return iri, raw
}

// MarkerID derives a stable marker id from what the marker says and where it
// binds, independent of line numbers.
func MarkerID(path, relation string, targets []string, symbol, scope string) string {
	ts := append([]string{}, targets...)
	sort.Strings(ts)
	h := sha1.Sum([]byte(path + "\x00" + relation + "\x00" + strings.Join(ts, ",") + "\x00" + symbol + "\x00" + scope))
	return "m-" + hex.EncodeToString(h[:])[:10]
}

func relPath(root, path string) (string, error) {
	if !strings.HasPrefix(path, root) {
		return path, fmt.Errorf("outside root")
	}
	return strings.TrimPrefix(strings.TrimPrefix(path, root), "/"), nil
}
