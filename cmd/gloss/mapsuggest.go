// @gloss implements gmi:MappingCommands scope=file

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/calib"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/mapping"
	"github.com/mckalvan/glossmark/internal/model"
)

// skipPairs reports pairs a suggestion must not repeat: already in a set
// (any predicate, either direction, negations included) or rejected.
func skipPairs(p *pipeline) func(s, o string) bool {
	seen := map[string]bool{}
	for _, fm := range p.Res.Mappings {
		seen[fm.SubjectCURIE+"|"+fm.ObjectCURIE] = true
		seen[fm.ObjectCURIE+"|"+fm.SubjectCURIE] = true
	}
	for _, e := range p.Ledger.MappingEntries() {
		if len(e.Targets) == 2 && e.Status == model.StatusRejected {
			seen[e.Targets[0]+"|"+e.Targets[1]] = true
			seen[e.Targets[1]+"|"+e.Targets[0]] = true
		}
	}
	return func(s, o string) bool { return seen[s+"|"+o] }
}

func mapSuggest(args []string) int {
	fs, root := newFlags("map suggest")
	against := fs.String("against", "", "namespaces to match against, comma-separated (default: every loaded namespace this repo does not own)")
	minConf := fs.Float64("min-conf", calib.DefaultMinConf, "do not suggest below this confidence")
	write := fs.Bool("write", false, "add the suggestions to a mapping set as proposed rows (semapv:LexicalMatching)")
	setName := fs.String("set", "", "mapping set to write to (with --write)")
	evalPath := fs.String("eval", "", "score the suggestions against a labels file (subject, predicate, object per line) and print precision per basis")
	format := fs.String("format", "text", "text | json")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	opt := mapping.SuggestOptions{Owned: map[string]bool{}, Skip: skipPairs(p)}
	for ns, ok := range p.WS.Owned {
		opt.Owned[ns] = ok
	}
	if *against != "" {
		opt.Against = map[string]bool{}
		for _, ns := range splitList(*against) {
			ns = strings.TrimSuffix(ns, ":")
			opt.Against[ns] = true
			delete(opt.Owned, ns) // a namespace matched against is never a subject
		}
	}
	if *evalPath != "" {
		opt.Skip = nil // score every suggestion, mapped pairs included
	}
	all := mapping.Suggest(p.Graph, opt)
	var cands []mapping.Candidate
	for _, c := range all {
		if c.Conf >= *minConf || *evalPath != "" {
			cands = append(cands, c)
		}
	}
	if *evalPath != "" {
		b, err := os.ReadFile(*evalPath)
		if err != nil {
			return fail(err)
		}
		return printMapEval(os.Stdout, cands, mapping.ReadLabels(string(b)))
	}
	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(cands)
	} else {
		if len(cands) == 0 {
			fmt.Println("gloss map suggest: no suggestions at or above --min-conf")
		}
		for _, c := range cands {
			fmt.Printf("%.2f  %s %s %s  [%s: %s]\n", c.Conf, c.Subject, c.Predicate, c.Object, c.Basis, c.Why)
		}
	}
	if !*write || len(cands) == 0 {
		return 0
	}
	s, err := pickSet(p.WS, *setName, true)
	if err != nil {
		return fail(err)
	}
	actor := reviewer(p.WS.Root)
	for _, c := range cands {
		r := &mapping.Row{Cells: map[string]string{}}
		r.Set("subject_id", c.Subject)
		r.Set("predicate_id", c.Predicate)
		r.Set("object_id", c.Object)
		r.Set("mapping_justification", "semapv:LexicalMatching")
		r.Set("author_id", actor)
		r.Set("mapping_tool", "gloss map suggest")
		r.Set("mapping_date", today())
		r.Set("confidence", fmt.Sprintf("%.2f", c.Conf))
		r.Set("comment", c.Basis+": "+c.Why)
		r.Set("subject_label", labelOf(p, c.Subject))
		r.Set("object_label", labelOf(p, c.Object))
		s.Rows = append(s.Rows, r)
		for _, x := range []string{c.Subject, c.Object, actor} {
			if base, ok := p.WS.Prefixes[nsOf(x)]; ok {
				s.AddCurie(nsOf(x), base)
			} else if pre := nsOf(x); pre == "agent" || pre == "human" || pre == "team" {
				s.AddCurie(pre, "urn:gloss:actor:"+pre+":")
			}
		}
	}
	if err := s.Save(p.WS.Abs(s.Path)); err != nil {
		return fail(err)
	}
	if _, err := buildGraph(p.WS.Root, true); err != nil {
		return fail(err)
	}
	fmt.Printf("gloss map suggest: %s proposed in %s; review with gloss map status, accept or reject each\n", plural(len(cands), "mapping"), s.Path)
	return 0
}

func printMapEval(w io.Writer, cands []mapping.Candidate, labels []mapping.Label) int {
	t := mapping.Score(cands, labels)
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(w, "%-28s %5s %8s %10s %8s\n", "basis", "n", "correct", "precision", "conf")
	conf := mapping.ShippedCalibration.Confs()
	total := calib.Count{}
	for _, k := range keys {
		c := t[k]
		total.N += c.N
		total.Correct += c.Correct
		fmt.Fprintf(w, "%-28s %5d %8d %9.0f%% %8.2f\n", k, c.N, c.Correct, 100*float64(c.Correct)/float64(max(c.N, 1)), conf(k))
	}
	found := map[string]bool{}
	for _, c := range cands {
		found[c.Subject+"|"+c.Object] = true
		found[c.Object+"|"+c.Subject] = true
	}
	recalled := 0
	for _, l := range labels {
		if found[l.Subject+"|"+l.Object] {
			recalled++
		}
	}
	fmt.Fprintf(w, "\n%d suggestions, %d correct (%.0f%% precision); %d of %d labels found (%.0f%% recall)\n", total.N, total.Correct,
		100*float64(total.Correct)/float64(max(total.N, 1)), recalled, len(labels), 100*float64(recalled)/float64(max(len(labels), 1)))
	return 0
}

// mapExport writes every set as JSON records or as RDF (Turtle) following the
// SSSOM/RDF shape: each mapping an owl:Axiom on its subject, predicate and
// object, its slots as sssom: properties.
func mapExport(args []string) int {
	fs, root := newFlags("map export")
	format := fs.String("format", "json", "json | rdf")
	out := fs.String("o", "", "output file (default stdout)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	var w io.Writer = os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		w = f
	}
	switch *format {
	case "json":
		type set struct {
			ID       string              `json:"mapping_set_id"`
			Version  string              `json:"mapping_set_version,omitempty"`
			License  string              `json:"license,omitempty"`
			Creators []string            `json:"creator_id,omitempty"`
			CurieMap map[string]string   `json:"curie_map,omitempty"`
			Mappings []map[string]string `json:"mappings"`
		}
		var sets []set
		for _, s := range p.WS.MappingSets {
			x := set{ID: s.ID, Version: s.Version, License: s.License, Creators: s.Creators, CurieMap: s.CurieMap}
			for _, r := range s.Rows {
				x.Mappings = append(x.Mappings, r.Cells)
			}
			sets = append(sets, x)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(sets); err != nil {
			return fail(err)
		}
	case "rdf":
		if err := writeTurtle(w, p.WS.MappingSets, p.WS.Prefixes, p.Res.Mappings); err != nil {
			return fail(err)
		}
	default:
		return fail(fmt.Errorf("--format must be json or rdf"))
	}
	return 0
}

func writeTurtle(w io.Writer, sets []*mapping.Set, prefixes map[string]string, found []extract.FoundMapping) error {
	pm := map[string]string{"sssom": mapping.Builtin["sssom"], "owl": mapping.Builtin["owl"], "rdf": mapping.Builtin["rdf"], "xsd": mapping.Builtin["xsd"]}
	var names []string
	for k := range pm {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(w, "@prefix %s: <%s> .\n", k, pm[k])
	}
	fmt.Fprintln(w)
	iri := func(s *mapping.Set, curie string) string {
		if x, _, ok := s.Expand(curie, prefixes); ok {
			return "<" + x + ">"
		}
		return turtleString(curie)
	}
	entity := map[string]bool{"subject_id": true, "predicate_id": true, "object_id": true, "mapping_justification": true, "author_id": true,
		"reviewer_id": true, "creator_id": true, "subject_source": true, "object_source": true, "mapping_provider": true, "subject_category": true, "object_category": true}
	for _, s := range sets {
		fmt.Fprintf(w, "<%s> a sssom:MappingSet ;\n", s.ID)
		if s.Version != "" {
			fmt.Fprintf(w, "  sssom:mapping_set_version %s ;\n", turtleString(s.Version))
		}
		fmt.Fprintf(w, "  sssom:license <%s> .\n\n", s.License)
		for _, r := range s.Rows {
			fmt.Fprintf(w, "[] a owl:Axiom ;\n  owl:annotatedSource %s ;\n  owl:annotatedProperty %s ;\n  owl:annotatedTarget %s",
				iri(s, r.Get("subject_id")), iri(s, r.Get("predicate_id")), iri(s, r.Get("object_id")))
			keys := make([]string, 0, len(r.Cells))
			for k := range r.Cells {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if k == "subject_id" || k == "predicate_id" || k == "object_id" {
					continue
				}
				for _, v := range r.Multi(k) {
					val := turtleString(v)
					switch {
					case entity[k]:
						val = iri(s, v)
					case k == "confidence":
						val = turtleString(v) + "^^xsd:double"
					case k == "mapping_date" || k == "review_date":
						val = turtleString(v) + "^^xsd:date"
					}
					fmt.Fprintf(w, " ;\n  sssom:%s %s", k, val)
				}
			}
			fmt.Fprintf(w, " ;\n  sssom:mapping_set <%s> .\n\n", s.ID)
		}
	}
	_ = found
	return nil
}

func turtleString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}
