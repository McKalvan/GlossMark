// @gloss implements gmi:Extractor scope=file

// Package extract is pass 1 of the extractor: ontology files, manifests and
// inline markers become nodes and edges of the canonical graph. Derived edges
// are added afterwards by the rollup package.
package extract

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	ex.inheritKinds()
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
	ids map[string]string // marker id → path:line of its first marker
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
		file := ex.ws.Rel(o.Path)
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
			if c.ValueFrom != nil && ex.ws.Owned[ns] {
				ex.bindValue(n, c.ValueFrom, file, c.Line)
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

// inheritKinds gives a concept with no kind of its own the kind of its
// nearest is-a ancestor that declares one, and records that ancestor in
// KindFrom, so every kind-dependent rule (the holds and infrastructure lifts,
// domain and range, impact output) sees a subtype as what it is. Among
// ancestors at the same distance the first by IRI wins; gloss check reports
// parents that disagree (GM306). The walk visits each ancestor once, so an
// is-a cycle ends it.
func (ex *extractor) inheritKinds() {
	for _, n := range ex.g.NodesOf(model.NodeConcept) {
		if n.Kind != "" {
			continue
		}
		seen := map[string]bool{n.ID: true}
		level := []string{n.ID}
		for len(level) > 0 && n.Kind == "" {
			var next []string
			for _, x := range level {
				for _, p := range ex.g.Objects(x, "is-a") {
					pn := ex.g.Node(p)
					if seen[p] || pn == nil || pn.NodeType != model.NodeConcept {
						continue
					}
					seen[p] = true
					if n.Kind == "" && pn.Kind != "" && pn.KindFrom == "" {
						n.Kind, n.KindFrom = pn.Kind, pn.CURIE
					}
					next = append(next, p)
				}
			}
			level = next
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
	if _, ok := ex.g.Prefixes[prefix]; !ok && ex.ws.Config.RepoIRI != "" {
		base := ex.ws.Config.RepoIRI
		if !strings.HasSuffix(base, "/") && !strings.HasSuffix(base, "#") {
			base += "/"
		}
		ex.g.Prefixes[prefix] = base + "boundaries/" + prefix + "/"
	}
	if _, ok := ex.g.Prefixes[prefix]; !ok {
		ex.g.Prefixes[prefix] = "urn:gloss:boundary:" + prefix + ":"
		ex.diag(path, line, "warning", "GM112", fmt.Sprintf("boundary prefix %q is not declared under prefixes: in %s; using urn:gloss:boundary:%s:", prefix, load.ConfigName, prefix))
	}
	iri, _ := ex.g.Expand(curie)
	return iri
}

func (ex *extractor) manifests() {
	// Every manifest's own node first: a part-of reference to a boundary
	// declared in a sibling directory must never create that node from the
	// child's manifest, which would give it the child's file and no dir.
	iris := make([]string, len(ex.ws.Manifests))
	for i, m := range ex.ws.Manifests {
		iri := ex.boundaryIRI(m.Boundary, m.Path, 1)
		iris[i] = iri
		n := &graph.Node{NodeType: model.NodeBoundary, ID: iri, CURIE: m.Boundary, Kind: m.Level, Label: m.Label, Doc: m.Doc, Dir: m.Dir, File: m.Path, Line: 1, Owner: m.Owner}
		if m.Criticality != "" {
			n.Criticality, n.CriticalityFrom = m.Criticality, "declared"
		}
		n.CriticalityFloor = m.CriticalityFloor
		n.Deprecated, n.ReplacedBy, n.RetireBy = m.Deprecated, m.ReplacedBy, m.RetireBy
		if i := strings.Index(m.Boundary, ":"); i > 0 {
			n.NS = m.Boundary[:i]
		}
		ex.g.AddNode(n)
	}
	for i, m := range ex.ws.Manifests {
		iri := iris[i]
		for _, p := range m.PartOf {
			piri := ex.boundaryIRI(p, m.Path, 1)
			if ex.g.Node(piri) == nil {
				// Implicit parent boundary: declared only by reference. It has
				// no level of its own; views place it above its children.
				pn := &graph.Node{NodeType: model.NodeBoundary, ID: piri, CURIE: p, File: m.Path, Line: 1}
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
		var tsrc []byte // the annotated file, for a sidecar
		if marker.IsSidecar(rel) {
			target = marker.SidecarTarget(rel)
			b, err := os.ReadFile(ex.ws.Abs(target))
			if err != nil {
				ex.diag(rel, 1, "warning", "GM121", fmt.Sprintf("sidecar annotates %s, which does not exist", target))
			}
			tsrc = b
		}
		m := ex.ws.ManifestFor(rel)
		var boundary string
		if m != nil {
			boundary = ex.boundaryIRI(m.Boundary, m.Path, 1)
		}
		anchors := make([]marker.Anchor, len(found))
		for i, fd := range found {
			anchors[i] = marker.Resolve(rel, fd, lines, style)
			if marker.IsSidecar(rel) {
				anchors[i] = ex.sidecarAnchor(rel, target, tsrc, fd.Marker, anchors[i])
			}
		}
		// Same-named declarations in one file must not share an artifact.
		marker.Disambiguate(rel, lines, anchors)
		for i, fd := range found {
			ex.marker(rel, target, fd, anchors[i], lines, m, boundary)
		}
	}
	return nil
}

// sidecarAnchor anchors a sidecar marker to the file it annotates, not to
// the sidecar: scope=file fingerprints the target, and scope=range in a JSON
// sidecar resolves the JSON Pointer in note= to the value's lines. Editing
// the sidecar then never makes a link stale; editing the value does.
func (ex *extractor) sidecarAnchor(rel, target string, tsrc []byte, mk *marker.Marker, a marker.Anchor) marker.Anchor {
	if tsrc == nil {
		return a
	}
	switch a.Scope {
	case "symbol", "file":
		return marker.FileAnchor(strings.Split(strings.TrimSuffix(string(tsrc), "\n"), "\n"))
	case "range":
		if !json.Valid(tsrc) {
			return a
		}
		ptr := mk.Keys["note"]
		if pa, ok := marker.PointerAnchor(tsrc, ptr, a.Scope); ok {
			return pa
		}
		if !strings.HasPrefix(ptr, "/") {
			ex.diag(rel, mk.Line, "warning", "GM123", fmt.Sprintf("scope=range in a JSON sidecar needs a JSON Pointer in note= (such as note=\"/fields/1\"), got %q; anchored to the sidecar", ptr))
		} else {
			ex.diag(rel, mk.Line, "warning", "GM123", fmt.Sprintf("JSON Pointer %s does not resolve in %s; anchored to the sidecar", ptr, target))
		}
	}
	return a
}

func (ex *extractor) marker(rel, target string, fd marker.Found, anchor marker.Anchor, lines []string, m *model.Manifest, boundary string) {
	mk := fd.Marker
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
	} else if anchor.Scope == "range" && anchor.Kind == "json-pointer" {
		aid += "#" + anchor.Symbol
		label = target + ":" + anchor.Symbol
		kind = anchor.Scope
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
	// Two markers with one id state one link twice; review commands act on
	// the id, so both lines are kept in Markers and accepted together.
	if at, dup := ex.ids[id]; dup {
		ex.diag(rel, mk.Line, "warning", "GM132", fmt.Sprintf("duplicate marker: %s %s repeats %s (same relation, targets and anchor, marker id %s); delete one", mk.Relation, strings.Join(mk.Targets, ", "), at, id))
	} else {
		if ex.ids == nil {
			ex.ids = map[string]string{}
		}
		ex.ids[id] = fmt.Sprintf("%s:%d", rel, mk.Line)
	}
	// An alias is stored as its canonical relation, so a query for verifies
	// finds every test link; the written form stays in prov.alias.
	pred, alias := mk.Relation, ""
	if pred == "tests" {
		pred, alias = "verifies", "tests"
	}
	for _, t := range mk.Targets {
		oiri, raw := ex.resolveTarget(t, m, rel, mk.Line)
		prov := &graph.Prov{Path: rel, Line: mk.Line, Col: mk.Col, Scope: anchor.Scope, Symbol: anchor.Symbol, Fingerprint: anchor.Fingerprint,
			Proposed: mk.Proposed, ID: id, Note: mk.Keys["note"], Sugar: mk.Sugar, Raw: raw, Alias: alias}
		if c := mk.Conf(); c >= 0 {
			cc := c
			prov.Conf = &cc
		}
		e := &graph.Edge{S: aid, P: pred, O: oiri, Source: graph.SourceInline, Prov: prov}
		ex.g.AddEdge(e)
		if mk.Relation == "publishes-value" {
			if cn := ex.g.Node(oiri); cn != nil && cn.NodeType == model.NodeConcept && anchor.StartLine > 0 && anchor.StartLine <= len(lines) {
				text := strings.TrimSpace(lines[anchor.StartLine-1])
				cn.ValueFrom, cn.LiveValue, cn.ValueFP = fmt.Sprintf("%s:%d", target, anchor.StartLine), text, valueFP(text)
			}
		}
		fm.Edges = append(fm.Edges, e)
		// Inferrer: a test-path artifact that implements/represents also verifies the concept.
		if (mk.Relation == "implements" || mk.Relation == "represents") && ex.ws.IsTestPath(rel) {
			ex.g.AddEdge(&graph.Edge{S: aid, P: "verifies", O: oiri, Source: graph.SourceInferred, Prov: &graph.Prov{Path: rel, Line: mk.Line, Rule: "test-path", ID: id, Proposed: mk.Proposed}})
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

// symbolLine matches the first line that assigns, annotates or defaults a
// symbol: "swr: float = 0.04", "SWR = 0.04", "const swr = 0.04", "swr=0.04".
func symbolLine(symbol string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^\w.])` + regexp.QuoteMeta(symbol) + `\s*(:[^=,)]*)?\s*(=|:=)`)
}

// bindValue resolves a concept's value-from to a line of code and records it.
// @gloss implements gmi:ValueBinder
func (ex *extractor) bindValue(n *graph.Node, vf *model.ValueFrom, file string, line int) {
	rel := filepath.ToSlash(vf.Path)
	b, err := os.ReadFile(ex.ws.Abs(rel))
	if err != nil {
		ex.diag(file, line, "error", "GM360", fmt.Sprintf("%s: value-from %s does not exist", n.CURIE, rel))
		return
	}
	lines := strings.Split(string(b), "\n")
	at := 0
	switch {
	case vf.Line > 0:
		if vf.Line > len(lines) {
			ex.diag(file, line, "error", "GM360", fmt.Sprintf("%s: value-from %s has no line %d", n.CURIE, rel, vf.Line))
			return
		}
		at = vf.Line
	case vf.Symbol != "":
		re := symbolLine(vf.Symbol)
		for i, l := range lines {
			if re.MatchString(l) {
				at = i + 1
				break
			}
		}
		if at == 0 {
			ex.diag(file, line, "error", "GM360", fmt.Sprintf("%s: value-from symbol %q is not assigned anywhere in %s", n.CURIE, vf.Symbol, rel))
			return
		}
	default:
		ex.diag(file, line, "error", "GM360", fmt.Sprintf("%s: value-from needs symbol: or line:", n.CURIE))
		return
	}
	text := strings.TrimSpace(lines[at-1])
	n.ValueFrom, n.LiveValue, n.ValueFP = fmt.Sprintf("%s:%d", rel, at), text, valueFP(text)
}

func valueFP(text string) string {
	h := sha256.Sum256([]byte(strings.Join(strings.Fields(text), " ")))
	return "sha256:" + hex.EncodeToString(h[:])[:16]
}

// MarkerID derives a stable marker id from what the marker says and where it
// binds, independent of line numbers.
func MarkerID(path, relation string, targets []string, symbol, scope string) string {
	ts := append([]string{}, targets...)
	sort.Strings(ts)
	h := sha1.Sum([]byte(path + "\x00" + relation + "\x00" + strings.Join(ts, ",") + "\x00" + symbol + "\x00" + scope))
	return "m-" + hex.EncodeToString(h[:])[:10]
}
