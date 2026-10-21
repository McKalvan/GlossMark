// @gloss implements gmi:MappingCommands scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/mapping"
	"github.com/mckalvan/glossmark/internal/model"
)

func init() {
	register(command{"map", "SSSOM mapping sets between namespaces (ADR 0025): init, propose, accept, reject, repoint, status, suggest, export", cmdMap})
}

const mapUsage = `usage: gloss map <command> [flags]

  init <set> [--owner team] [--scope repo|landscape]   create mappings/<set>.sssom.tsv and declare it
  propose <subject> <predicate> <object>               add a proposed mapping (predicate: exact, close, broad, narrow, related, or a CURIE)
  accept <subject> <predicate> <object> | <id>         review a proposed or stale mapping (sets reviewer_id, review_date)
  reject <subject> <predicate> <object> --reason ...   reject it: a reviewed negation (predicate_modifier: Not)
  repoint <subject> <predicate> <object>               move a mapping off a deprecated concept to its replaced-by successor
  status [--format json]                               proposed, stale, rejected and dangling mappings
  suggest [--against ns] [--write]                     lexical mapping proposals between owned and other namespaces
  export [--format json|rdf] [-o file]                 every set as SSSOM/JSON-like records or RDF (Turtle)
`

func cmdMap(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stderr, mapUsage)
		return 2
	}
	switch args[0] {
	case "init":
		return mapInit(args[1:])
	case "propose":
		return mapPropose(args[1:])
	case "accept":
		return mapReview(args[1:], false)
	case "reject":
		return mapReview(args[1:], true)
	case "repoint":
		return mapRepoint(args[1:])
	case "status":
		return mapStatus(args[1:])
	case "suggest":
		return mapSuggest(args[1:])
	case "export":
		return mapExport(args[1:])
	}
	fmt.Fprintf(os.Stderr, "gloss map: unknown command %q\n\n%s", args[0], mapUsage)
	return 2
}

func today() string { return time.Now().UTC().Format("2006-01-02") }

// predicateCURIE normalises exact, close, ... or a CURIE to a supported predicate.
func predicateCURIE(p string) (mapping.Predicate, error) {
	if full, ok := mapping.ShortNames[p]; ok {
		p = full
	}
	if pr, ok := mapping.ByCURIE(p); ok {
		return pr, nil
	}
	return mapping.Predicate{}, fmt.Errorf("unknown predicate %q: use exact, close, broad, narrow, related, or skos:exactMatch, owl:equivalentClass, ...", p)
}

// inverse swaps a predicate's direction (broad ↔ narrow); the others are symmetric.
func inverse(p mapping.Predicate) mapping.Predicate {
	switch p.CURIE {
	case "skos:broadMatch":
		q, _ := mapping.ByCURIE("skos:narrowMatch")
		return q
	case "skos:narrowMatch":
		q, _ := mapping.ByCURIE("skos:broadMatch")
		return q
	}
	return p
}

func nsOf(curie string) string {
	if i := strings.Index(curie, ":"); i > 0 {
		return curie[:i]
	}
	return ""
}

// findMapping finds the rows for a mapping given as an id or as three words.
func findMapping(p *pipeline, args []string) ([]extract.FoundMapping, string, error) {
	var id string
	switch len(args) {
	case 1:
		id = args[0]
		if !strings.HasPrefix(id, "mapping:") {
			id = "mapping:" + id
		}
	case 3:
		pr, err := predicateCURIE(args[1])
		if err != nil {
			return nil, "", err
		}
		id = mapping.ID(args[0], pr.CURIE, args[2])
	default:
		return nil, "", fmt.Errorf("give the mapping as <subject> <predicate> <object>, or as its id from gloss map status")
	}
	var out []extract.FoundMapping
	for _, fm := range p.Res.Mappings {
		if fm.ID == id {
			out = append(out, fm)
		}
	}
	if len(out) == 0 {
		// Accept the reverse direction too (the CLI swaps on propose).
		if s, pr, o, ok := mapping.ParseID(id); ok {
			if pp, ok := mapping.ByCURIE(pr); ok {
				rid := mapping.ID(o, inverse(pp).CURIE, s)
				for _, fm := range p.Res.Mappings {
					if fm.ID == rid {
						out = append(out, fm)
					}
				}
				if len(out) > 0 {
					return out, rid, nil
				}
			}
		}
		return nil, id, fmt.Errorf("no mapping %s in any set (gloss map status lists them)", strings.TrimPrefix(id, "mapping:"))
	}
	return out, id, nil
}

// pickSet chooses the set a new mapping goes to.
func pickSet(ws *load.Workspace, name string, ownsOne bool) (*mapping.Set, error) {
	if name != "" {
		if s := ws.MappingSet(name); s != nil {
			return s, nil
		}
		return nil, fmt.Errorf("no mapping set %q under mappings: in %s", name, load.ConfigName)
	}
	want := model.MappingScopeLandscape
	if ownsOne {
		want = model.MappingScopeRepo
	}
	var cands []*mapping.Set
	for _, s := range ws.MappingSets {
		if s.Scope == want {
			cands = append(cands, s)
		}
	}
	switch len(cands) {
	case 1:
		return cands[0], nil
	case 0:
		return nil, fmt.Errorf("no mapping set with scope %s; create one with gloss map init <name>%s", want, map[bool]string{true: "", false: " --scope landscape"}[ownsOne])
	}
	var names []string
	for _, s := range cands {
		names = append(names, s.Name)
	}
	return nil, fmt.Errorf("several mapping sets could hold it (%s); choose one with --set", strings.Join(names, ", "))
}

func mapInit(args []string) int {
	fs, root := newFlags("map init")
	owner := fs.String("owner", "", "team that owns and approves the set")
	scope := fs.String("scope", model.MappingScopeRepo, "repo (one side of every row is owned here) or landscape (neither side need be)")
	license := fs.String("license", "https://creativecommons.org/publicdomain/zero/1.0/", "license IRI of the set")
	file := fs.String("file", "", "path of the set (default mappings/<set>.sssom.tsv)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: gloss map init <set> [--owner team] [--scope repo|landscape]")
		return 2
	}
	name := fs.Arg(0)
	if *scope != model.MappingScopeRepo && *scope != model.MappingScopeLandscape {
		return fail(fmt.Errorf("--scope must be repo or landscape"))
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if _, ok := p.WS.Config.Mappings[name]; ok {
		return fail(fmt.Errorf("mapping set %s is already declared in %s", name, load.ConfigName))
	}
	rel := *file
	if rel == "" {
		rel = "mappings/" + name + ".sssom.tsv"
	}
	base := p.WS.Config.RepoIRI
	if base == "" {
		var prefixes []string
		for pre := range p.WS.Config.Owns {
			prefixes = append(prefixes, pre)
		}
		sort.Strings(prefixes)
		if len(prefixes) > 0 {
			d := p.WS.Config.Owns[prefixes[0]]
			base = strings.TrimSuffix(strings.TrimSuffix(d.IRI, "/"), "/"+nsLast(d.IRI)) + "/"
		}
	}
	if base == "" {
		base = "https://onto.example.com/"
	}
	s := &mapping.Set{Name: name, ID: strings.TrimSuffix(base, "/") + "/mappings/" + name, Version: "0.1.0", License: *license, CurieMap: map[string]string{}}
	// Declare every namespace a row can name, so hand-written rows are valid
	// SSSOM for other tools too.
	for pre, d := range p.WS.Config.Owns {
		if d.IRI != "" {
			s.CurieMap[pre] = d.IRI
		}
	}
	for pre, d := range p.WS.Config.Imports {
		if d.IRI != "" {
			s.CurieMap[pre] = d.IRI
		}
	}
	if *owner != "" {
		s.Creators = []string{"team:" + strings.TrimPrefix(*owner, "team:")}
		s.CurieMap["team"] = "urn:gloss:actor:team:"
	}
	if _, err := os.Stat(p.WS.Abs(rel)); err == nil {
		return fail(fmt.Errorf("%s already exists", rel))
	}
	if err := s.SaveNew(p.WS.Abs(rel)); err != nil {
		return fail(err)
	}
	if err := declareSet(p.WS.Abs(load.ConfigName), name, rel, *owner, *scope); err != nil {
		return fail(err)
	}
	fmt.Printf("gloss map init: %s (%s) created and declared under mappings: in %s\n", rel, *scope, load.ConfigName)
	fmt.Printf("  propose a mapping: gloss map propose <owned:Concept> exact <other:Concept> --set %s\n", name)
	return 0
}

func nsLast(iri string) string {
	parts := strings.Split(strings.TrimSuffix(iri, "/"), "/")
	return parts[len(parts)-1]
}

// declareSet adds name under mappings: in glossmark.yaml, keeping the file's layout.
func declareSet(cfgPath, name, file, owner, scope string) error {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	entry := fmt.Sprintf("  %s: { file: %s", name, file)
	if owner != "" {
		entry += ", owner: " + owner
	}
	if scope != model.MappingScopeRepo {
		entry += ", scope: " + scope
	}
	entry += " }"
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "mappings:") {
			if strings.TrimSpace(strings.TrimPrefix(l, "mappings:")) != "" {
				return fmt.Errorf("%s: mappings: is written inline; add %s: { file: %s } to it by hand", filepath.Base(cfgPath), name, file)
			}
			out := append(append(append([]string{}, lines[:i+1]...), entry), lines[i+1:]...)
			return os.WriteFile(cfgPath, []byte(strings.Join(out, "\n")), 0o644)
		}
	}
	text := strings.TrimRight(string(b), "\n") + "\n# SSSOM mapping sets (ADR 0025).\nmappings:\n" + entry + "\n"
	return os.WriteFile(cfgPath, []byte(text), 0o644)
}

func mapPropose(args []string) int {
	fs, root := newFlags("map propose")
	setName := fs.String("set", "", "mapping set to add the row to (default: the only set that can hold it)")
	just := fs.String("justification", "", "manual, lexical, llm (semapv:UnspecifiedMatching), review, structural, background, ... or a semapv term SSSOM accepts (default llm for agents, manual for people)")
	conf := fs.Float64("conf", -1, "confidence in [0,1]")
	comment := fs.String("comment", "", "why the concepts match")
	force := fs.Bool("force", false, "propose a pair that was rejected (removes the negation row)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 3 {
		fmt.Fprintln(os.Stderr, "usage: gloss map propose <subject> <predicate> <object> [--set s] [--justification j] [--conf c] [--comment ...]")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	res, err := proposeMapping(p, MappingRequest{Subject: fs.Arg(0), Predicate: fs.Arg(1), Object: fs.Arg(2), Set: *setName, Justification: *just,
		Conf: *conf, Comment: *comment, Force: *force, Actor: reviewer(p.WS.Root)})
	if err != nil {
		return fail(err)
	}
	if res.Swapped {
		fmt.Printf("gloss map propose: written as %s %s %s (the owned concept is the subject)\n", res.Subject, res.Predicate, res.Object)
	}
	fmt.Printf("gloss map propose: %s %s %s proposed in %s [%s: %s]\n", res.Subject, res.Predicate, res.Object, res.Path, res.Gate, strings.Join(res.Reasons, "; "))
	fmt.Printf("  the set owner accepts it with: gloss map accept %s %s %s\n", res.Subject, res.Predicate, res.Object)
	return 0
}

// MappingRequest is a mapping to propose (CLI and MCP).
type MappingRequest struct {
	Subject, Predicate, Object string
	Set, Justification         string
	Conf                       float64 // -1 when absent
	Comment, Actor             string
	Force                      bool
}

// MappingResult is what proposing a mapping did.
type MappingResult struct {
	ID        string   `json:"id"`
	Subject   string   `json:"subject"`
	Predicate string   `json:"predicate"`
	Object    string   `json:"object"`
	Set       string   `json:"set"`
	Path      string   `json:"path"`
	Swapped   bool     `json:"swapped,omitempty"`
	Gate      string   `json:"gate"`
	Reasons   []string `json:"reasons"`
	Next      string   `json:"next"`
}

// proposeMapping validates and writes a proposed row, then rebuilds the ledger.
func proposeMapping(p *pipeline, req MappingRequest) (*MappingResult, error) {
	subj, obj := req.Subject, req.Object
	pr, err := predicateCURIE(req.Predicate)
	if err != nil {
		return nil, err
	}
	for _, c := range []string{subj, obj} {
		iri, ok := p.Graph.Expand(c)
		if !ok {
			return nil, fmt.Errorf("%s: unknown prefix; declare it under owns:, imports: or prefixes:", c)
		}
		if _, loaded := p.WS.Ontologies[nsOf(c)]; loaded && p.Graph.Node(iri) == nil {
			return nil, fmt.Errorf("%s does not exist (search_concepts, or propose the concept first)", c)
		}
	}
	if req.Conf != -1 && (req.Conf < 0 || req.Conf > 1) {
		return nil, fmt.Errorf("confidence must be in [0,1]")
	}
	res := &MappingResult{}
	ownS, ownO := p.WS.Owned[nsOf(subj)], p.WS.Owned[nsOf(obj)]
	if !ownS && ownO {
		subj, obj, pr = obj, subj, inverse(pr)
		res.Swapped = true
	}
	s, err := pickSet(p.WS, req.Set, ownS || ownO)
	if err != nil {
		return nil, err
	}
	id := mapping.ID(subj, pr.CURIE, obj)
	for _, fm := range p.Res.Mappings {
		if fm.ID != id || fm.Set != s {
			continue
		}
		if fm.Negative {
			if !req.Force {
				return nil, fmt.Errorf("%s %s %s was rejected (%s); pass --force to propose it again", subj, pr.CURIE, obj, firstNonEmpty(fm.Row.Get("comment"), "no reason recorded"))
			}
			removeRow(s, fm.Row)
			continue
		}
		return nil, fmt.Errorf("%s %s %s is already in %s (%s)", subj, pr.CURIE, obj, s.Path, map[bool]string{true: "accepted", false: "proposed"}[fm.Row.Reviewed()])
	}
	if le := p.Ledger.Entry(id); le != nil && le.Status == model.StatusRejected && !req.Force {
		return nil, fmt.Errorf("%s %s %s was rejected by %s: %s; pass --force to propose it again", subj, pr.CURIE, obj, le.ReviewedBy, firstNonEmpty(le.ReviewReason, "no reason recorded"))
	}
	j := req.Justification
	if j == "" {
		j = "manual"
		if strings.HasPrefix(req.Actor, "agent:") {
			j = "llm"
		}
	}
	if full, ok := mapping.Justifications[j]; ok {
		j = full
	}
	if !mapping.SemapvJustifications[j] {
		return nil, fmt.Errorf("justification %q: use manual, lexical, llm, review, logical, structural, background, composite, instance, unspecified, or one of the semapv terms SSSOM accepts", req.Justification)
	}
	r := &mapping.Row{Cells: map[string]string{}}
	r.Set("subject_id", subj)
	r.Set("predicate_id", pr.CURIE)
	r.Set("object_id", obj)
	r.Set("mapping_justification", j)
	r.Set("author_id", req.Actor)
	r.Set("mapping_date", today())
	r.Set("subject_label", labelOf(p, subj))
	r.Set("object_label", labelOf(p, obj))
	if req.Conf >= 0 {
		r.Set("confidence", fmt.Sprintf("%.3g", req.Conf))
	}
	r.Set("comment", req.Comment)
	s.Rows = append(s.Rows, r)
	for _, c := range []string{subj, obj, req.Actor} {
		if base, ok := p.WS.Prefixes[nsOf(c)]; ok {
			s.AddCurie(nsOf(c), base)
		} else if pre := nsOf(c); pre == "agent" || pre == "human" || pre == "team" {
			s.AddCurie(pre, "urn:gloss:actor:"+pre+":")
		}
	}
	if err := s.Save(p.WS.Abs(s.Path)); err != nil {
		return nil, err
	}
	if req.Force {
		p.Ledger.ReopenMapping(id)
		if err := p.Ledger.Save(); err != nil {
			return nil, err
		}
	}
	q, err := buildGraph(p.WS.Root, true)
	if err != nil {
		return nil, err
	}
	res.ID, res.Subject, res.Predicate, res.Object, res.Set, res.Path = id, subj, pr.CURIE, obj, s.Name, s.Path
	res.Gate = "?"
	for _, e := range q.Graph.Edges() {
		if e.Prov != nil && e.Prov.ID == id {
			cls, why := gate.ClassifyMapping(q.Graph, q.WS, e, q.WS.Config.EffectivePolicy())
			res.Gate, res.Reasons = string(cls), why
		}
	}
	res.Next = fmt.Sprintf("the set owner (%s) reviews it: gloss map accept %s %s %s", firstNonEmpty(s.Owner, "see glossmark.yaml"), subj, pr.CURIE, obj)
	return res, nil
}

func labelOf(p *pipeline, curie string) string {
	if iri, ok := p.Graph.Expand(curie); ok {
		if n := p.Graph.Node(iri); n != nil && n.Label != "" {
			return n.Label
		}
	}
	return ""
}

func removeRow(s *mapping.Set, r *mapping.Row) {
	for i, x := range s.Rows {
		if x == r {
			s.Rows = append(s.Rows[:i], s.Rows[i+1:]...)
			return
		}
	}
}

// mapReview accepts or rejects a mapping in its set file and the ledger.
func mapReview(args []string, reject bool) int {
	name := "map accept"
	if reject {
		name = "map reject"
	}
	fs, root := newFlags(name)
	reason := fs.String("reason", "", "why the concepts do not match (required to reject)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if reject && *reason == "" {
		fmt.Fprintln(os.Stderr, "gloss map reject: --reason is required; a rejection is a reviewed negation and travels with the set")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	rows, id, err := findMapping(p, fs.Args())
	if err != nil {
		return fail(err)
	}
	rev := reviewer(p.WS.Root)
	if strings.HasPrefix(rev, "agent:") {
		fmt.Fprintln(os.Stderr, "gloss map: warning: the reviewer is an agent; a mapping review is meant to be a person's decision (ADR 0025 §3)")
	}
	now := time.Now().UTC()
	touched := map[*mapping.Set]bool{}
	var sfp, ofp string
	for _, fm := range rows {
		r := fm.Row
		if reject {
			r.Set("predicate_modifier", "Not")
			r.Set("comment", *reason)
		} else {
			if fm.Negative {
				continue
			}
			r.Set("predicate_modifier", "")
		}
		r.Set("reviewer_id", rev)
		r.Set("review_date", today())
		if n := p.WS.Ontologies[nsOf(fm.SubjectCURIE)]; n != nil {
			r.Set("subject_source_version", n.Version)
		}
		if n := p.WS.Ontologies[nsOf(fm.ObjectCURIE)]; n != nil {
			r.Set("object_source_version", n.Version)
		}
		if base, ok := p.WS.Prefixes[nsOf(rev)]; ok {
			fm.Set.AddCurie(nsOf(rev), base)
		} else if p := nsOf(rev); p == "agent" || p == "human" || p == "team" {
			fm.Set.AddCurie(p, "urn:gloss:actor:"+p+":")
		}
		touched[fm.Set] = true
		sfp, ofp = mapping.ConceptFingerprint(p.Graph, fm.Subject), mapping.ConceptFingerprint(p.Graph, fm.Object)
	}
	if len(touched) == 0 {
		return fail(fmt.Errorf("%s is a negation; to accept the pair, gloss map propose it with --force", strings.TrimPrefix(id, "mapping:")))
	}
	for s := range touched {
		if err := s.Save(p.WS.Abs(s.Path)); err != nil {
			return fail(err)
		}
	}
	q, err := buildGraph(p.WS.Root, false)
	if err != nil {
		return fail(err)
	}
	if !reject {
		q.Ledger.ReviewMapping(id, rev, sfp, ofp, now)
	} else if e := q.Ledger.Entry(id); e != nil {
		e.ReviewReason = *reason
		q.Ledger.Touch()
	}
	if err := q.Ledger.Save(); err != nil {
		return fail(err)
	}
	verb := "accepted"
	if reject {
		verb = "rejected"
	}
	fmt.Printf("gloss map: %s %s by %s\n", strings.TrimPrefix(id, "mapping:"), verb, rev)
	return 0
}

func mapRepoint(args []string) int {
	fs, root := newFlags("map repoint")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	rows, id, err := findMapping(p, fs.Args())
	if err != nil {
		return fail(err)
	}
	moved := 0
	for _, fm := range rows {
		for _, side := range []struct{ col, iri string }{{"subject_id", fm.Subject}, {"object_id", fm.Object}} {
			n := p.Graph.Node(side.iri)
			if n == nil || !n.Deprecated || n.ReplacedBy == "" {
				continue
			}
			old := fm.Row.Get(side.col)
			fm.Row.Set(side.col, n.ReplacedBy)
			label := strings.Replace(side.col, "_id", "_label", 1)
			fm.Row.Set(label, labelOf(p, n.ReplacedBy))
			// A moved mapping is a new claim: it needs a review again.
			for _, c := range []string{"reviewer_id", "reviewer_label", "review_date", "subject_source_version", "object_source_version"} {
				fm.Row.Set(c, "")
			}
			fm.Row.Set("comment", strings.TrimSpace(fm.Row.Get("comment")+" (re-pointed from "+old+")"))
			fm.Row.Set("mapping_date", today())
			if base, ok := p.WS.Prefixes[nsOf(n.ReplacedBy)]; ok {
				fm.Set.AddCurie(nsOf(n.ReplacedBy), base)
			}
			moved++
			fmt.Printf("gloss map repoint: %s → %s in %s (now proposed; review it with gloss map accept)\n", old, n.ReplacedBy, fm.Set.Path)
		}
		if err := fm.Set.Save(p.WS.Abs(fm.Set.Path)); err != nil {
			return fail(err)
		}
	}
	if moved == 0 {
		return fail(fmt.Errorf("neither endpoint of %s is deprecated with a replaced-by successor", strings.TrimPrefix(id, "mapping:")))
	}
	if _, err := buildGraph(p.WS.Root, true); err != nil {
		return fail(err)
	}
	return 0
}

// MappingStatus is one row of gloss map status.
type MappingStatus struct {
	ID        string   `json:"id"`
	Set       string   `json:"set"`
	Path      string   `json:"path"`
	Line      int      `json:"line"`
	Subject   string   `json:"subject"`
	Predicate string   `json:"predicate"`
	Object    string   `json:"object"`
	Status    string   `json:"status"`
	Author    string   `json:"author,omitempty"`
	Reviewer  string   `json:"reviewer,omitempty"`
	Conf      float64  `json:"conf,omitempty"`
	Changed   []string `json:"changed,omitempty"` // endpoints whose fingerprint moved since review
	Repoint   []string `json:"repoint,omitempty"` // deprecated endpoint → successor
	Dangling  []string `json:"dangling,omitempty"`
	Note      string   `json:"note,omitempty"`
}

// MappingStatuses lists every mapping with its state.
func MappingStatuses(p *pipeline) []MappingStatus {
	var out []MappingStatus
	for _, fm := range p.Res.Mappings {
		st := MappingStatus{ID: fm.ID, Set: fm.Set.Name, Path: fm.Set.Path, Line: fm.Row.Line, Subject: fm.SubjectCURIE, Predicate: fm.Predicate.CURIE, Object: fm.ObjectCURIE,
			Author: fm.Row.Get("author_id"), Reviewer: fm.Row.Get("reviewer_id"), Note: fm.Row.Get("comment")}
		if c := fm.Row.Confidence(); c >= 0 {
			st.Conf = c
		}
		le := p.Ledger.Entry(fm.ID)
		switch {
		case !fm.Supported:
			st.Status = "ignored"
		case len(fm.Unresolved) > 0:
			st.Status = "unresolved"
		case le != nil:
			st.Status = le.Status
			if le.Status == model.StatusStale {
				if changed(le.SubjectFingerprint, mapping.ConceptFingerprint(p.Graph, fm.Subject)) {
					st.Changed = append(st.Changed, fm.SubjectCURIE)
				}
				if changed(le.ObjectFingerprint, mapping.ConceptFingerprint(p.Graph, fm.Object)) {
					st.Changed = append(st.Changed, fm.ObjectCURIE)
				}
			}
		}
		for _, iri := range []string{fm.Subject, fm.Object} {
			if iri == "" {
				continue
			}
			n := p.Graph.Node(iri)
			if n == nil {
				if _, loaded := p.WS.Ontologies[nsOf(p.Graph.Curie(iri))]; loaded {
					st.Dangling = append(st.Dangling, p.Graph.Curie(iri))
				}
				continue
			}
			if n.Deprecated && n.ReplacedBy != "" && !fm.Negative {
				st.Repoint = append(st.Repoint, n.CURIE+" → "+n.ReplacedBy)
			}
		}
		out = append(out, st)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if rankStatus(out[i].Status) != rankStatus(out[j].Status) {
			return rankStatus(out[i].Status) < rankStatus(out[j].Status)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func changed(recorded, current string) bool {
	return recorded != "" && current != "" && recorded != current
}

func rankStatus(s string) int {
	for i, x := range []string{"stale", "proposed", "unresolved", "rejected", "accepted", "ignored"} {
		if s == x {
			return i
		}
	}
	return 9
}

func mapStatus(args []string) int {
	fs, root := newFlags("map status")
	format := fs.String("format", "text", "text | json")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	sts := MappingStatuses(p)
	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(sts)
		return 0
	}
	if len(sts) == 0 {
		fmt.Println("gloss map status: no mappings (gloss map init <set>, then gloss map propose)")
		return 0
	}
	counts := map[string]int{}
	for _, st := range sts {
		counts[st.Status]++
		line := fmt.Sprintf("%-9s %s %s %s  [%s:%d]", st.Status, st.Subject, st.Predicate, st.Object, st.Path, st.Line)
		if st.Reviewer != "" {
			line += "  reviewed by " + st.Reviewer
		} else if st.Author != "" {
			line += "  by " + st.Author
		}
		if st.Conf > 0 {
			line += fmt.Sprintf(" conf=%.2f", st.Conf)
		}
		fmt.Println(line)
		if len(st.Changed) > 0 {
			fmt.Printf("          changed since review: %s\n", strings.Join(st.Changed, ", "))
		}
		for _, r := range st.Repoint {
			fmt.Printf("          deprecated: %s (gloss map repoint %s %s %s)\n", r, st.Subject, st.Predicate, st.Object)
		}
		for _, d := range st.Dangling {
			fmt.Printf("          dangling: %s no longer exists\n", d)
		}
	}
	var parts []string
	for _, s := range []string{"accepted", "proposed", "stale", "rejected", "unresolved", "ignored"} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	fmt.Printf("\n%s\n", strings.Join(parts, ", "))
	return 0
}
