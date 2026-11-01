// @gloss implements gmi:Extractor, gmi:LinkMapper scope=file
// @gloss produces gm:Graph scope=file

// Package extract is pass 1 of the extractor: ontology files, manifests,
// inline markers and manifest links become nodes and edges of the canonical
// graph. Derived edges are added afterwards by the rollup package.
package extract

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	// Markers lists every marker found, in file order, with its edge, then
	// every link a manifest's links: entries map (ADR 0045).
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
	// LegacyID is the id an earlier release gave the marker, so its ledger
	// entry moves instead of being orphaned: a JSON-sidecar range before #116.
	LegacyID string
	Edges    []*graph.Edge
	// Source is "" for an inline marker, which is written at
	// Path:Marker.Line, or graph.SourceManifest for a link a links: entry
	// maps, which is written at From:FromLine and spans to FromEnd; Path
	// and Marker.Line then name the code it anchors to.
	Source   string
	From     string
	FromLine int
	FromEnd  int
}

// Mapped reports a link a manifest's links: entry maps, as opposed to an
// inline marker.
func (fm FoundMarker) Mapped() bool { return fm.Source == graph.SourceManifest }

// Stated returns the file and line where the link is written: the marker
// line, or the links: entry in the manifest.
func (fm FoundMarker) Stated() (string, int) {
	if fm.Mapped() {
		return fm.From, fm.FromLine
	}
	return fm.Path, fm.Marker.Line
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
	if err := ex.mappings(); err != nil {
		return nil, err
	}
	return res, nil
}

type extractor struct {
	ws  *load.Workspace
	g   *graph.Graph
	res *Result
	ids map[string]string // marker id → path:line of its first marker
	// bindings are the lines each published value is bound to.
	bindings map[*graph.Node][]valueBinding
	// inline records the relations inline markers state on each artifact
	// (artifact IRI → relation set), which a mapped link does not restate.
	inline map[string]map[string]bool
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
		n := &graph.Node{NodeType: model.NodeNamespace, ID: o.IRI, CURIE: p + ":", NS: p, Tier: o.Tier, Version: o.Version, Doc: o.Doc, Imported: !ex.ws.Owned[p], DeclsFP: declsFingerprint(o)}
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

// declsFingerprint hashes an ontology file's non-concept content: its iri,
// tier, doc and relation declarations. The gate compares it between base
// and head, so an edit to relations: cannot hide behind a bookkeeping change
// to a concept in the same file.
func declsFingerprint(o *model.Ontology) string {
	b, _ := json.Marshal(struct {
		IRI, Tier, Doc string
		Relations      map[string]model.RelationDecl
	}{o.IRI, o.Tier, o.Doc, o.Relations})
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:8])
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
			if c.Planned != nil {
				n.Planned, n.PlanBy, n.PlanTicket = true, c.Planned.By, c.Planned.Ticket
			}
			if c.Criticality != "" {
				n.Criticality, n.CriticalityFrom = c.Criticality, "declared"
			}
			if c.ReplacedBy != "" {
				// A CURIE, or a full IRI shown as a CURIE when a prefix
				// matches; the namespace need not be imported (ADR 0020).
				riri, cur := ex.resolveRef(c.ReplacedBy, ns)
				if strings.Contains(cur, "://") {
					cur = ex.g.Curie(riri)
				}
				n.ReplacedBy = cur
			}
			if c.Published != nil {
				_, cur := ex.resolveRef(c.Published.As, ns)
				n.Published = cur
			}
			if ex.ws.Owned[ns] {
				for i := range c.ValueFrom {
					ex.bindValue(n, &c.ValueFrom[i], file, c.Line)
				}
			}
			ex.g.AddNode(n)
			add := func(p string, targets []string) {
				alias := ""
				if p == "applies-to" {
					p, alias = "guards", "applies-to"
				}
				for _, t := range targets {
					oiri, raw := ex.resolveRef(t, ns)
					ex.g.AddEdge(&graph.Edge{S: iri, P: p, O: oiri, Source: graph.SourceDeclared, Prov: &graph.Prov{Path: file, Line: c.Line, Raw: raw, Alias: alias}})
				}
			}
			add("is-a", c.IsA)
			add("part-of", c.PartOf)
			add("has-part", c.HasPart)
			add("realizes", c.Realizes)
			if c.ReplacedBy != "" {
				add("replaced-by", []string{c.ReplacedBy})
			}
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
		// A manifest that names no part-of is part of the nearest manifest
		// in an enclosing directory: nesting is containment unless part-of
		// says otherwise, and part-of: [] opts out (ADR 0026).
		if m.ImplicitPartOf() {
			if p := ex.ws.EnclosingManifest(m); p != nil {
				ex.g.AddEdge(&graph.Edge{S: iri, P: "part-of", O: ex.boundaryIRI(p.Boundary, m.Path, 1), Source: graph.SourceDeclared, Prov: &graph.Prov{Path: m.Path, Line: 1, Rule: "nearest-manifest"}})
			}
		}
		if m.ReplacedBy != "" {
			// The successor is often in another repository: no node is made.
			riri, _ := ex.g.Expand(m.ReplacedBy)
			ex.g.AddEdge(&graph.Edge{S: iri, P: "replaced-by", O: riri, Source: graph.SourceDeclared, Prov: &graph.Prov{Path: m.Path, Line: 1, Raw: m.ReplacedBy}})
		}
		for rel, targets := range m.DeclaredSets() {
			for _, t := range targets {
				oiri, _ := ex.g.Expand(t)
				prov := &graph.Prov{Path: m.Path, Line: 1, Raw: t}
				if m.Plans(rel, t) {
					prov.Rule = graph.RulePlanned
				}
				ex.g.AddEdge(&graph.Edge{S: iri, P: rel, O: oiri, Source: graph.SourceDeclared, Prov: prov})
			}
		}
		for _, t := range m.GuardedBy {
			oiri, _ := ex.g.Expand(t)
			ex.g.AddEdge(&graph.Edge{S: iri, P: "guarded-by", O: oiri, Source: graph.SourceDeclared, Prov: &graph.Prov{Path: m.Path, Line: 1, Raw: t}})
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
		src, err := ex.ws.ReadFile(rel)
		if err != nil {
			return err
		}
		found, diags, lines := marker.Scan(bytes.NewReader(src), style)
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
			b, err := ex.ws.ReadFile(target)
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
		fa := marker.FileAnchor(strings.Split(strings.TrimSuffix(string(tsrc), "\n"), "\n"))
		fa.Legacy = sidecarLegacy(a)
		return fa
	case "range":
		if !json.Valid(tsrc) {
			return a
		}
		ptr := mk.Keys["note"]
		if pa, ok := marker.PointerAnchor(tsrc, ptr, a.Scope); ok {
			pa.Legacy = sidecarLegacy(a)
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

// sidecarLegacy is the fingerprint v0.1.0 stored for a sidecar marker: the
// sidecar's own anchor, as that release computed it (#171).
func sidecarLegacy(a marker.Anchor) string {
	if a.Legacy != "" {
		return a.Legacy
	}
	return a.Fingerprint
}

func (ex *extractor) marker(rel, target string, fd marker.Found, anchor marker.Anchor, lines []string, m *model.Manifest, boundary string) {
	mk := fd.Marker
	if mk.Scope() == "symbol" && anchor.Kind == "unresolved-symbol" {
		ex.diag(rel, mk.Line, "warning", "GM122", "scope=symbol requested but no declaration was detected after the marker; anchored to the next block")
	}
	// Artifact node.
	aid, label, kind := artifactID(target, anchor)
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
	if mk.Keys["id"] == "" && anchor.Kind == "json-pointer" {
		fm.LegacyID = MarkerID(target, mk.Relation, mk.Targets, "", anchor.Scope)
	}
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
	pred, alias := canonical(mk.Relation)
	if ex.inline == nil {
		ex.inline = map[string]map[string]bool{}
	}
	if ex.inline[aid] == nil {
		ex.inline[aid] = map[string]bool{}
	}
	ex.inline[aid][pred] = true
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
				ex.addBinding(cn, fmt.Sprintf("%s:%d", target, anchor.StartLine), strings.TrimSpace(lines[anchor.StartLine-1]))
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

// artifactID names the artifact an anchor binds: the file, or the file
// plus the symbol, pointer or first line it anchors inside it.
func artifactID(target string, anchor marker.Anchor) (aid, label, kind string) {
	aid, label, kind = graph.ArtifactPrefix+target, target, "file"
	switch {
	case anchor.Scope == "symbol" && anchor.Symbol != "":
		aid += "#" + anchor.Symbol
		label = target + ":" + anchor.Symbol
		kind = anchor.Kind
	case anchor.Scope == "range" && anchor.Kind == "json-pointer":
		aid += "#" + anchor.Symbol
		label = target + ":" + anchor.Symbol
		kind = anchor.Scope
	case anchor.Scope == "block" || anchor.Scope == "line" || anchor.Scope == "range":
		aid += fmt.Sprintf("#L%d", anchor.StartLine)
		label = fmt.Sprintf("%s:%d", target, anchor.StartLine)
		kind = anchor.Scope
	}
	return aid, label, kind
}

// canonical returns the relation an alias is stored as, and the alias: a
// query for verifies finds every test link, and the written form stays in
// prov.alias.
func canonical(relation string) (pred, alias string) {
	if relation == "tests" {
		return "verifies", "tests"
	}
	return relation, ""
}

// mappings runs every links: entry of the manifests, then of glossmark.yaml,
// after the inline markers, so a marker on an artifact overrides a mapping
// of the same relation onto it (ADR 0045).
func (ex *extractor) mappings() error {
	for _, m := range ex.ws.Manifests {
		for _, l := range m.Links {
			if err := ex.mapping(m.Path, m.Dir, l, m); err != nil {
				return err
			}
		}
	}
	for _, l := range ex.ws.Config.Links {
		if err := ex.mapping(load.ConfigName, "", l, nil); err != nil {
			return err
		}
	}
	return nil
}

// mapping links the files beneath dir that a links: entry names: every
// scanned file the path glob matches at file scope, or, with symbol:, the
// declaration of that name in each of them, anchored as a marker above it
// would be. m is the manifest holding the entry (nil for glossmark.yaml).
func (ex *extractor) mapping(file, dir string, l *model.Link, m *model.Manifest) error {
	if l == nil {
		ex.diag(file, 1, "error", "GM125", "links: entry is empty; write { path: \"<glob>\", <relation>: <concept> } or { symbol: \"<name>\", <relation>: <concept> }")
		return nil
	}
	rels := l.RelationNames()
	switch {
	case l.Path == "" && l.Symbol == "":
		ex.diag(file, l.Line, "error", "GM125", "links: entry names neither path: (a glob over the files beneath this directory) nor symbol: (a declaration name)")
		return nil
	case len(rels) == 0:
		ex.diag(file, l.Line, "error", "GM125", fmt.Sprintf("links: entry for %s names no relation; add the relation a marker would carry (implements: ns:Concept, verifies: ..., represents: ...)", l.Where()))
		return nil
	case l.Conf != nil && (*l.Conf < 0 || *l.Conf > 1):
		ex.diag(file, l.Line, "error", "GM125", fmt.Sprintf("links: entry for %s: conf must be between 0 and 1, got %v", l.Where(), *l.Conf))
		return nil
	}
	for _, rel := range rels {
		if len(l.Relations[rel]) == 0 {
			ex.diag(file, l.Line, "error", "GM125", fmt.Sprintf("links: entry for %s: %s names no concept", l.Where(), rel))
			return nil
		}
	}
	matched, files := 0, 0
	for _, f := range ex.ws.Files {
		if marker.IsSidecar(f) || (dir != "" && !strings.HasPrefix(f, dir+"/")) {
			continue
		}
		sub := f
		if dir != "" {
			sub = strings.TrimPrefix(f, dir+"/")
		}
		if l.Path != "" && !model.MatchGlob(l.Path, sub) {
			continue
		}
		files++
		b, err := ex.ws.ReadFile(f)
		if err != nil {
			return err
		}
		lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
		if l.Symbol == "" {
			ex.mapped(file, l, f, marker.FileAnchor(lines), lines, m)
			matched++
			continue
		}
		style, _ := marker.StyleFor(f, ex.ws.Config.CommentStyles)
		var anchors []marker.Anchor
		for _, d := range marker.Declarations(f, lines, style) {
			if d.Symbol == l.Symbol || (d.Parent != "" && d.Parent+"."+d.Symbol == l.Symbol) {
				anchors = append(anchors, d.Anchor(lines))
			}
		}
		// Same-named declarations in one file must not share an artifact.
		marker.Disambiguate(f, lines, anchors)
		for _, a := range anchors {
			ex.mapped(file, l, f, a, lines, m)
			matched++
		}
	}
	where := "the repository"
	if dir != "" {
		where = dir + "/"
	}
	switch {
	case matched > 0:
	case files == 0 && l.Path != "":
		ex.diag(file, l.Line, "warning", "GM126", fmt.Sprintf("links: %s matches no scanned file beneath %s (globs are relative to this directory; a file in a language without a comment style is not scanned)", l.Where(), where))
	case files == 0:
		ex.diag(file, l.Line, "warning", "GM126", fmt.Sprintf("links: %s: no scanned file beneath %s to look in", l.Where(), where))
	default:
		ex.diag(file, l.Line, "warning", "GM126", fmt.Sprintf("links: %s is declared in none of the %d files it covers beneath %s (the name a marker's anchor would record: a Go method is Type.Method, a resource resource.type.name)", l.Where(), files, where))
	}
	return nil
}

// mapped adds one mapped link: the artifact the anchor names in target, and
// an edge per relation and concept of the links: entry, with the entry's
// location in prov.from. A relation an inline marker already states on the
// artifact is left to the marker, and a mapping that repeats another one's
// link (two overlapping globs) adds nothing.
func (ex *extractor) mapped(file string, l *model.Link, target string, anchor marker.Anchor, lines []string, m *model.Manifest) {
	bm := ex.ws.ManifestFor(target)
	boundary := ""
	if bm != nil {
		boundary = ex.boundaryIRI(bm.Boundary, bm.Path, 1)
	}
	if m == nil {
		m = bm // .Local in glossmark.yaml resolves in the file's own boundary
	}
	aid, label, kind := artifactID(target, anchor)
	a := anchor
	art := ex.g.AddNode(&graph.Node{NodeType: model.NodeArtifact, ID: aid, Kind: kind, Label: label, Path: target, Anchor: &a, Boundary: boundary, File: target, Line: anchor.StartLine})
	if art.Boundary == "" {
		art.Boundary = boundary
	}
	from := fmt.Sprintf("%s:%d", file, l.Line)
	for _, rel := range l.RelationNames() {
		targets := []string(l.Relations[rel])
		pred, alias := canonical(rel)
		if ex.inline[aid][pred] {
			continue
		}
		id := MarkerID(target, rel, targets, anchor.Symbol, anchor.Scope)
		if _, dup := ex.ids[id]; dup {
			continue
		}
		if ex.ids == nil {
			ex.ids = map[string]string{}
		}
		ex.ids[id] = from
		mk := &marker.Marker{Relation: rel, Targets: targets, Proposed: l.Proposed, Keys: map[string]string{}, Line: anchor.StartLine,
			Raw: fmt.Sprintf("%s: %s", rel, strings.Join(targets, ", "))}
		if l.Conf != nil {
			mk.Keys["conf"] = strconv.FormatFloat(*l.Conf, 'g', -1, 64)
		}
		if l.Note != "" {
			mk.Keys["note"] = l.Note
		}
		fm := FoundMarker{Path: target, Marker: mk, Anchor: anchor, ID: id, Source: graph.SourceManifest, From: file, FromLine: l.Line, FromEnd: l.EndLine}
		for _, t := range targets {
			oiri, raw := ex.resolveTarget(t, m, file, l.Line)
			prov := &graph.Prov{Path: target, Line: anchor.StartLine, Scope: anchor.Scope, Symbol: anchor.Symbol, Fingerprint: anchor.Fingerprint,
				Proposed: l.Proposed, ID: id, Note: l.Note, Raw: raw, Alias: alias, From: from, Conf: l.Conf}
			e := &graph.Edge{S: aid, P: pred, O: oiri, Source: graph.SourceManifest, Prov: prov}
			ex.g.AddEdge(e)
			if rel == "publishes-value" {
				if cn := ex.g.Node(oiri); cn != nil && cn.NodeType == model.NodeConcept && anchor.StartLine > 0 && anchor.StartLine <= len(lines) {
					ex.addBinding(cn, fmt.Sprintf("%s:%d", target, anchor.StartLine), strings.TrimSpace(lines[anchor.StartLine-1]))
				}
			}
			fm.Edges = append(fm.Edges, e)
			if (rel == "implements" || rel == "represents") && ex.ws.IsTestPath(target) {
				ex.g.AddEdge(&graph.Edge{S: aid, P: "verifies", O: oiri, Source: graph.SourceInferred, Prov: &graph.Prov{Path: target, Line: anchor.StartLine, Rule: "test-path", ID: id, Proposed: l.Proposed, From: from}})
			}
		}
		ex.res.Markers = append(ex.res.Markers, fm)
	}
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
	at, text, err := ResolveValueFrom(ex.ws, *vf)
	if err != nil {
		ex.diag(file, line, "error", "GM360", fmt.Sprintf("%s: %v", n.CURIE, err))
		return
	}
	ex.addBinding(n, fmt.Sprintf("%s:%d", filepath.ToSlash(vf.Path), at), text)
}

// ResolveValueFrom finds the line of code a value-from names: its 1-based
// line and the line's text, trimmed. The error reads as GM360 does, after
// the concept's name: "value-from ..." and what to write instead. gloss bind
// uses it to read a constant before it writes the binding.
func ResolveValueFrom(ws *load.Workspace, vf model.ValueFrom) (int, string, error) {
	rel := filepath.ToSlash(vf.Path)
	abs, err := ws.Within(rel)
	if err != nil {
		return 0, "", fmt.Errorf("value-from %v", err)
	}
	b, err := ws.ReadFile(ws.Rel(abs))
	if err != nil {
		return 0, "", fmt.Errorf("value-from %s does not exist", rel)
	}
	lines := strings.Split(string(b), "\n")
	at := 0
	switch {
	case vf.Line > 0:
		if vf.Line > len(lines) {
			return 0, "", fmt.Errorf("value-from %s has no line %d", rel, vf.Line)
		}
		at = vf.Line
	case vf.Symbol != "" && keyPathFile(rel):
		var cands []string
		at, cands = findKey(rel, lines, vf.Symbol)
		switch {
		case len(cands) > 1:
			return 0, "", fmt.Errorf("value-from symbol %q matches %d keys in %s (%s); write the dotted key path", vf.Symbol, len(cands), rel, strings.Join(cands, ", "))
		case at == 0:
			return 0, "", fmt.Errorf("value-from symbol %q names no key in %s; write the dotted key path from the top (a.b.c), or use line:", vf.Symbol, rel)
		}
	case vf.Symbol != "":
		re := symbolLine(vf.Symbol)
		for i, l := range lines {
			if re.MatchString(l) {
				at = i + 1
				break
			}
		}
		if at == 0 {
			return 0, "", fmt.Errorf("value-from symbol %q is not assigned anywhere in %s", vf.Symbol, rel)
		}
	default:
		return 0, "", fmt.Errorf("value-from needs symbol: or line:")
	}
	return at, strings.TrimSpace(lines[at-1]), nil
}

// ValueLiteral reads the value a line assigns to symbol, as the ontology
// would publish it: the right-hand side of the first =, := or : after the
// symbol, without a trailing comment or semicolon; a quoted string without
// its quotes; a number as its first numeric literal with digit separators
// dropped (5_000 is 5000), so gloss accept --value finds it in the line. A
// line that assigns nothing readable returns "".
func ValueLiteral(line, symbol string) string {
	re := symbolLine(symbol)
	loc := re.FindStringIndex(line)
	if loc == nil {
		// A key in a configuration file: "key: value" or "key = value".
		i := strings.Index(line, symbol)
		if i < 0 {
			return ""
		}
		loc = []int{i, i + len(symbol)}
		rest := strings.TrimLeft(line[loc[1]:], "\"' \t")
		if !strings.HasPrefix(rest, ":") && !strings.HasPrefix(rest, "=") {
			return ""
		}
		loc[1] += len(line[loc[1]:]) - len(rest) + 1
	}
	rhs := strings.TrimSpace(line[loc[1]:])
	if rhs == "" || rhs == "{" || rhs == "[" {
		return ""
	}
	if q := rhs[0]; q == '"' || q == '\'' || q == '`' {
		if j := strings.IndexByte(rhs[1:], q); j >= 0 {
			return rhs[1 : j+1]
		}
	}
	for _, c := range []string{" #", " //", "//", ";", " --"} {
		if j := strings.Index(rhs, c); j > 0 {
			rhs = rhs[:j]
		}
	}
	rhs = strings.TrimRight(strings.TrimSpace(rhs), ",;")
	if m := numLiteral.FindStringIndex(rhs); m != nil && (m[0] == 0 || !isIdent(rhs[m[0]-1])) {
		return strings.NewReplacer("_", "", ",", "").Replace(rhs[m[0]:m[1]])
	}
	return rhs
}

// numLiteral matches a number as code writes it: digit groups (1_000),
// a fraction, an exponent.
var numLiteral = regexp.MustCompile(`[-+]?(?:\d[\d_]*(?:\.\d+)?|\.\d+)(?:[eE][-+]?\d+)?`)

func isIdent(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// valueBinding is one line of code a published value is bound to.
type valueBinding struct{ at, text string }

// addBinding records that concept n's published value is held at location
// at (path:line). A value bound in several places (a value-from list, or
// publishes-value markers in two languages) keeps every location: the node's
// value-from lists them in order, live-value joins their lines with " | ",
// and the fingerprint covers all of them, so a change to any one is drift.
func (ex *extractor) addBinding(n *graph.Node, at, text string) {
	if ex.bindings == nil {
		ex.bindings = map[*graph.Node][]valueBinding{}
	}
	for _, b := range ex.bindings[n] {
		if b.at == at {
			return
		}
	}
	bs := append(ex.bindings[n], valueBinding{at, text})
	sort.Slice(bs, func(i, j int) bool { return bs[i].at < bs[j].at })
	ex.bindings[n] = bs
	var ats, texts []string
	for _, b := range bs {
		ats, texts = append(ats, b.at), append(texts, b.text)
	}
	n.ValueFrom, n.LiveValue = strings.Join(ats, ", "), strings.Join(texts, " | ")
	if len(bs) == 1 {
		n.ValueFP = valueFP(text)
	} else {
		n.ValueFP = valueFP(strings.Join(texts, "\n"))
	}
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
