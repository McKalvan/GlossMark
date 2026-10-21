// @gloss implements gmi:McpServer scope=file

// Package mcp serves the graph to agents over the Model Context Protocol
// (JSON-RPC 2.0 on stdio): lookup_concept, search_concepts, concepts_for_path,
// propose_link, impact_of, propose_concept, context, slice_of and drift.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/slice"
)

// Protocol version implemented.
const ProtocolVersion = "2024-11-05"

// Backend supplies the graph and workspace, rebuilt on demand so edits are seen.
type Backend interface {
	Current() (*load.Workspace, *graph.Graph, map[string]*model.LedgerEntry, error)
	// Impact classifies a proposed set of markers (as marker lines) against the current graph.
	Impact(proposals []Proposal) (any, error)
	// ImpactOfChange reports what changed files touch, optionally classified against a git base.
	ImpactOfChange(base string, paths []string) (any, error)
	// ProposeConcept adds a proposed concept to the owning ontology file.
	ProposeConcept(p ConceptProposal) (any, error)
	// Context renders the session briefing for a directory.
	Context(dir string, budget int, changed bool) (string, error)
	// ProposeMapping adds a proposed row to a mapping set (ADR 0025).
	ProposeMapping(m MappingProposal) (any, error)
}

// MappingProposal is an equivalence an agent proposes between two concepts.
type MappingProposal struct {
	Subject       string   `json:"subject"`
	Predicate     string   `json:"predicate"`
	Object        string   `json:"object"`
	Set           string   `json:"set,omitempty"`
	Justification string   `json:"justification,omitempty"`
	Conf          *float64 `json:"conf,omitempty"`
	Comment       string   `json:"comment,omitempty"`
}

// ConceptProposal is a new concept an agent wants to add with proposed: true.
type ConceptProposal struct {
	Curie    string   `json:"curie"`
	Kind     string   `json:"kind,omitempty"`
	Doc      string   `json:"doc,omitempty"`
	Realizes []string `json:"realizes,omitempty"`
	IsA      []string `json:"is_a,omitempty"`
	Internal bool     `json:"internal,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
}

// Proposal is a link an agent wants to add.
type Proposal struct {
	Path     string  `json:"path"`
	Relation string  `json:"relation"`
	Target   string  `json:"target"`
	Conf     float64 `json:"conf,omitempty"`
	Note     string  `json:"note,omitempty"`
	Scope    string  `json:"scope,omitempty"`
}

// Server is one MCP session over a reader/writer.
type Server struct {
	backend Backend
	in      io.Reader
	out     io.Writer
	mu      sync.Mutex
	version string
}

// New returns a server.
func New(b Backend, in io.Reader, out io.Writer, version string) *Server {
	return &Server{backend: b, in: in, out: out, version: version}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve runs until stdin closes.
func (s *Server) Serve() error {
	sc := bufio.NewScanner(s.in)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.send(response{JSONRPC: "2.0", Error: &rpcError{-32700, "parse error: " + err.Error()}})
			continue
		}
		if req.ID == nil {
			// Notification: nothing to answer.
			continue
		}
		res, rerr := s.handle(req)
		if rerr != nil {
			s.send(response{JSONRPC: "2.0", ID: req.ID, Error: rerr})
		} else {
			s.send(response{JSONRPC: "2.0", ID: req.ID, Result: res})
		}
	}
	return sc.Err()
}

func (s *Server) send(r response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(r)
	s.out.Write(b)
	s.out.Write([]byte("\n"))
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func obj(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var tools = []tool{
	{"lookup_concept", "Return a concept (by CURIE such as lib:Book) with its kind, tier, doc, relations, realizations and the artifacts linking to it.",
		obj(map[string]any{"curie": str("Concept CURIE, e.g. lib:Renewal")}, "curie")},
	{"search_concepts", "Search concepts by meaning: CamelCase names split into words, aliases, docs, stems, synonyms and small typos all match; a kind word in the query (event, constraint) boosts that kind; one hop through realizes/is-a is followed. Hits carry the doc, the artifact count and which field and terms matched. Optionally restrict to a namespace, kind or tier.",
		obj(map[string]any{"query": str("Substring or words to match"), "namespace": str("Prefix to restrict to (optional)"), "kind": str("Concept kind (optional)"), "tier": str("core | implementation (optional)"), "limit": map[string]any{"type": "integer", "default": 20}}, "query")},
	{"concepts_for_path", "The concepts an artifact at this repo-relative path is linked to, plus the boundary manifest context: default namespace, allowed targets, and what the boundary realizes.",
		obj(map[string]any{"path": str("Repo-relative file path")}, "path")},
	{"propose_link", "Render the exact @gloss? marker line to insert for a proposal and classify its gate (AUTO/SOFT/HARD) against policy, allowed lists and remit. Does not edit files. " + agents.ProposalSentence,
		obj(map[string]any{"path": str("Repo-relative file the marker will live in"), "relation": str("implements | represents | verifies | ..."), "target": str("Concept CURIE, .Local, or ns:NEW:Name for a new concept"), "conf": map[string]any{"type": "number"}, "note": str("Rationale (optional)"), "scope": str("file | symbol | block | line | range (optional)")}, "path", "relation", "target")},
	{"impact_of", "Impact of a change. Pass proposals (links you intend to add) to get each link's gate; and/or pass paths (files you changed) or base (a git ref) to learn what those files touch: the concepts, whether they sit on the published seam, what constrains them, which boundaries declare them, their tests, and the other files sharing them. With base, the AUTO/SOFT/HARD classification of the whole working tree is included.",
		obj(map[string]any{"proposals": map[string]any{"type": "array", "items": obj(map[string]any{"path": str(""), "relation": str(""), "target": str(""), "conf": map[string]any{"type": "number"}, "note": str("")}, "path", "relation", "target")},
			"paths": map[string]any{"type": "array", "items": str("Repo-relative file you changed or will change")},
			"base":  str("Git ref to diff the working tree against (e.g. origin/main)")})},
	{"propose_concept", "Add a concept to the owning ontology file with proposed: true (the ontology's @gloss?) and rewrite ns:NEW:Name markers to it. Implementation concepts need realizes (a core concept), presents/hosts, or internal. " + agents.ProposalSentence,
		obj(map[string]any{"curie": str("ns:Name in a namespace this repo owns"), "kind": str("entity | process | capability | event | measure | constraint | resource | system-type"), "doc": str("One sentence: what it is and why it is needed"),
			"realizes": map[string]any{"type": "array", "items": str("core concept CURIE")}, "is_a": map[string]any{"type": "array", "items": str("parent CURIE")}, "internal": map[string]any{"type": "boolean"}, "aliases": map[string]any{"type": "array", "items": str("")}}, "curie", "kind")},
	{"propose_mapping", "Propose that two concepts of different namespaces match (an SSSOM mapping, ADR 0025): adds a row without reviewer_id to the repository's mapping set and returns its gate. Predicates: exact, close, broad, narrow, related. Use it when another team's concept means what you need instead of proposing a new concept. The set owner accepts it with gloss map accept.",
		obj(map[string]any{"subject": str("Concept CURIE (the owned side becomes the subject)"), "predicate": str("exact | close | broad | narrow | related, or skos:exactMatch ..."), "object": str("Concept CURIE in another namespace"),
			"set": str("Mapping set name (optional when there is one)"), "justification": str("manual | lexical | llm | review | structural | background | unspecified, or a semapv term (default llm for agents, written as semapv:UnspecifiedMatching)"), "conf": map[string]any{"type": "number"}, "comment": str("Why they match")}, "subject", "predicate", "object")},
	{"context", "The compact briefing for a directory: its boundary, allowed targets, your remit, the concepts most relevant to the directory first, and current state. Call it once when you start working in a directory.",
		obj(map[string]any{"dir": str("Repo-relative directory (default: repo root)"), "budget": map[string]any{"type": "integer", "description": "Approximate token budget (default 2000)"}, "changed": map[string]any{"type": "boolean", "description": "Open with what the uncommitted changes touch"}})},
	{"slice_of", "Everything relevant to a feature (docs/adr/0023-feature-slices.md): from seeds (concept or boundary CURIEs, artifacts path#symbol, or paths) the kinds, parts, realizations, artifacts, tests and boundaries that belong to it, plus a faded context ring. Each node carries its role and the via path of edges that brought it in, its criticality and its drift findings; the summary lists what is untested. Same JSON as gloss slice --format json.",
		obj(map[string]any{"seeds": map[string]any{"type": "array", "items": str("CURIE, path#symbol or repo-relative path")}, "context": map[string]any{"type": "boolean", "description": "Keep the context ring (default true)"},
			"follow": map[string]any{"type": "array", "items": str("produces | consumes | publishes | subscribes: flow relations to follow end to end")}}, "seeds")},
	{"drift", "Run the drift comparison on the current tree and return violations, gaps, orphans, rogues and metrics.", obj(map[string]any{})},
}

func (s *Server) handle(req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "glossmark", "version": s.version},
			"instructions":    "GlossMark serves the repository's code ontology. Call concepts_for_path before writing code in a directory, search_concepts to find the right concept, propose_link to get the exact @gloss? line and its gate. " + agents.ProposalSentence,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		res, err := s.call(p.Name, p.Arguments)
		if err != nil {
			return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": err.Error()}}}, nil
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return map[string]any{"content": []map[string]any{{"type": "text", "text": string(b)}}, "structuredContent": res}, nil
	case "resources/list", "prompts/list":
		return map[string]any{strings.TrimSuffix(req.Method, "/list"): []any{}}, nil
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

func (s *Server) call(name string, args json.RawMessage) (any, error) {
	ws, g, lg, err := s.backend.Current()
	if err != nil {
		return nil, err
	}
	switch name {
	case "lookup_concept":
		var a struct {
			Curie string `json:"curie"`
		}
		json.Unmarshal(args, &a)
		n := g.NodeByCurie(a.Curie)
		if n == nil {
			return nil, fmt.Errorf("no concept %q; try search_concepts", a.Curie)
		}
		return describe(g, n), nil
	case "search_concepts":
		var a struct {
			Query, Namespace, Kind, Tier string
			Limit                        int
		}
		json.Unmarshal(args, &a)
		if a.Limit <= 0 {
			a.Limit = 20
		}
		return Search(g, a.Query, a.Namespace, a.Kind, a.Tier, a.Limit), nil
	case "concepts_for_path":
		var a struct {
			Path string `json:"path"`
		}
		json.Unmarshal(args, &a)
		return conceptsForPath(ws, g, a.Path), nil
	case "propose_link":
		var p Proposal
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return s.backend.Impact([]Proposal{p})
	case "impact_of":
		var a struct {
			Proposals []Proposal `json:"proposals"`
			Paths     []string   `json:"paths"`
			Base      string     `json:"base"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		out := map[string]any{}
		if len(a.Proposals) > 0 {
			r, err := s.backend.Impact(a.Proposals)
			if err != nil {
				return nil, err
			}
			out["proposals"] = r
		}
		if len(a.Paths) > 0 || a.Base != "" {
			r, err := s.backend.ImpactOfChange(a.Base, a.Paths)
			if err != nil {
				return nil, err
			}
			out["change"] = r
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("pass proposals, paths or base")
		}
		return out, nil
	case "propose_mapping":
		var mp MappingProposal
		if err := json.Unmarshal(args, &mp); err != nil {
			return nil, err
		}
		return s.backend.ProposeMapping(mp)
	case "propose_concept":
		var cp ConceptProposal
		if err := json.Unmarshal(args, &cp); err != nil {
			return nil, err
		}
		return s.backend.ProposeConcept(cp)
	case "context":
		var a struct {
			Dir     string `json:"dir"`
			Budget  int    `json:"budget"`
			Changed bool   `json:"changed"`
		}
		json.Unmarshal(args, &a)
		if a.Budget <= 0 {
			a.Budget = 2000
		}
		txt, err := s.backend.Context(a.Dir, a.Budget, a.Changed)
		if err != nil {
			return nil, err
		}
		return map[string]any{"context": txt}, nil
	case "slice_of":
		a := struct {
			Seeds   []string `json:"seeds"`
			Context *bool    `json:"context"`
			Follow  []string `json:"follow"`
		}{}
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		fl, err := slice.ParseFollow(strings.Join(a.Follow, ","))
		if err != nil {
			return nil, err
		}
		rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg, Policy: ws.Config.EffectivePolicy(), Now: time.Now()})
		return slice.Of(g, ws.Root, a.Seeds, slice.Options{Context: a.Context == nil || *a.Context, Follow: fl}, rep.Findings)
	case "drift":
		rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg, Policy: ws.Config.EffectivePolicy()})
		return rep, nil
	}
	return nil, fmt.Errorf("unknown tool %q", name)
}

func describe(g *graph.Graph, n *graph.Node) map[string]any {
	out := map[string]any{"curie": n.CURIE, "iri": n.ID, "kind": n.Kind, "tier": n.Tier, "namespace": n.NS}
	if n.Label != "" {
		out["label"] = n.Label
	}
	if n.Doc != "" {
		out["doc"] = n.Doc
	}
	if n.Published != "" {
		out["published_as"] = n.Published
	}
	if n.Internal {
		out["internal"] = true
	}
	if n.Criticality != "" {
		out["criticality"] = n.Criticality
		out["criticality_from"] = n.CriticalityFrom
	}
	if n.Deprecated {
		out["deprecated"] = true
		out["replaced_by"] = n.ReplacedBy
	}
	if n.Value != nil {
		out["value"] = n.Value
	}
	rels := map[string][]string{}
	for _, e := range g.Out(n.ID, "") {
		if e.Source == graph.SourceDeclared {
			rels[e.P] = append(rels[e.P], g.Curie(e.O))
		}
	}
	out["relations"] = rels
	var realizedBy, subclasses []string
	for _, s := range g.Subjects(n.ID, "realizes") {
		if sn := g.Node(s); sn != nil && sn.NodeType == model.NodeConcept {
			realizedBy = append(realizedBy, sn.CURIE)
		}
	}
	for _, s := range g.Subjects(n.ID, "is-a") {
		subclasses = append(subclasses, g.Curie(s))
	}
	out["realized_by"] = realizedBy
	out["subclasses"] = subclasses
	out["ancestors"] = curies(g, g.Ancestors(n.ID))
	var arts []map[string]any
	for _, e := range g.In(n.ID, "") {
		if a := g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact && e.Prov != nil {
			arts = append(arts, map[string]any{"path": e.Prov.Path, "line": e.Prov.Line, "symbol": e.Prov.Symbol, "relation": e.P, "proposed": e.Prov.Proposed, "marker_id": e.Prov.ID})
		}
	}
	out["artifacts"] = arts
	var bounds []string
	for _, e := range g.In(n.ID, "") {
		if b := g.Node(e.S); b != nil && b.NodeType == model.NodeBoundary {
			bounds = append(bounds, b.CURIE+" "+e.P+" ("+e.Source+")")
		}
	}
	sort.Strings(bounds)
	out["boundaries"] = uniq(bounds)
	return out
}

func curies(g *graph.Graph, ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, g.Curie(id))
	}
	return out
}

func uniq(xs []string) []string {
	m := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !m[x] {
			m[x] = true
			out = append(out, x)
		}
	}
	return out
}

func conceptsForPath(ws *load.Workspace, g *graph.Graph, path string) map[string]any {
	out := map[string]any{"path": path}
	var links []map[string]any
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if a.Path != path {
			continue
		}
		for _, e := range g.Out(a.ID, "") {
			if e.Source != graph.SourceInline || e.Prov == nil {
				continue
			}
			links = append(links, map[string]any{"relation": e.P, "target": g.Curie(e.O), "line": e.Prov.Line, "symbol": e.Prov.Symbol, "proposed": e.Prov.Proposed, "marker_id": e.Prov.ID})
		}
	}
	out["links"] = links
	if m := ws.ManifestFor(path); m != nil {
		allowed := rollup.AllowedFor(g, ws, m)
		b := map[string]any{"boundary": m.Boundary, "level": m.Level, "manifest": m.Path, "default_namespace": m.DefaultNamespace, "allowed": allowed.Patterns, "allowed_derived": allowed.Derived, "realizes": m.Realizes, "produces": m.Produces, "consumes": m.Consumes}
		if bn := g.NodeByCurie(m.Boundary); bn != nil {
			var derived []string
			for _, e := range g.Out(bn.ID, "") {
				if e.Source == graph.SourceDerived && e.P != "part-of" {
					derived = append(derived, e.P+" "+g.Curie(e.O))
				}
			}
			sort.Strings(derived)
			b["derived"] = uniq(derived)
		}
		out["boundary"] = b
		// Suggested vocabulary: allowed concepts, or everything owned when no list.
		var vocab []string
		for _, n := range g.NodesOf(model.NodeConcept) {
			if n.Deprecated || (n.Imported && n.Tier == model.TierImpl && n.Published == "") {
				continue
			}
			if allowed.Matches(n.CURIE) {
				vocab = append(vocab, n.CURIE+" ("+n.Kind+")")
			}
		}
		out["vocabulary"] = vocab
	} else {
		out["boundary"] = nil
		out["hint"] = "no .gloss.yaml manifest encloses this path; .Local targets will not resolve"
	}
	_ = os.Stderr
	return out
}
