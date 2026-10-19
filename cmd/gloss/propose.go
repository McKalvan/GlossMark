// @gloss implements gmi:OntologyEditor scope=file

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/ontoedit"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func init() {
	register(command{"propose", "add a proposed concept to an ontology file (gloss propose concept ns:Name --kind ...), or --from-new to lift every ns:NEW:Name marker into one", cmdPropose})
}

// ontologyPath returns the owned ontology file for a namespace prefix.
func ontologyPath(p *pipeline, ns string) (string, error) {
	d, ok := p.WS.Config.Owns[ns]
	if !ok {
		if _, imported := p.WS.Imported[ns]; imported {
			return "", fmt.Errorf("namespace %s: is imported; propose upstream (decision request) or extend it in a namespace this repo owns", ns)
		}
		return "", fmt.Errorf("namespace %s: is not declared under owns: in %s", ns, "glossmark.yaml")
	}
	return filepath.Join(p.WS.Root, filepath.FromSlash(d.File)), nil
}

func splitCurie(c string) (string, string, error) {
	c = strings.Replace(c, ":NEW:", ":", 1)
	i := strings.Index(c, ":")
	if i <= 0 || i == len(c)-1 {
		return "", "", fmt.Errorf("%q is not a CURIE (ns:Name)", c)
	}
	return c[:i], c[i+1:], nil
}

func cmdPropose(args []string) int {
	fs, root := newFlags("propose")
	kind := fs.String("kind", "", "concept kind: "+strings.Join(model.ConceptKinds, ", "))
	realizes := fs.String("realizes", "", "core concept(s) an implementation concept realizes (comma-separated)")
	isA := fs.String("is-a", "", "parent concept(s) (comma-separated)")
	doc := fs.String("doc", "", "one sentence: what the concept is (with --from-new: applies when exactly one concept is lifted)")
	internal := fs.Bool("internal", false, "engineering-internal mechanism (realizes nothing)")
	aliases := fs.String("aliases", "", "other names people use (comma-separated)")
	fromNew := fs.Bool("from-new", false, "create a proposed concept for every ns:NEW:Name marker target and rewrite the markers")
	by := fs.String("by", "", "proposer (default GLOSS_ACTOR or git user)")
	if err := fs.Parse(flagsLast(args)); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	actor := *by
	if actor == "" {
		actor = reviewer(p.WS.Root)
	}
	if *fromNew {
		return proposeFromNew(p, actor, *doc)
	}
	rest := fs.Args()
	if len(rest) == 2 && rest[0] == "concept" {
		rest = rest[1:]
	}
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: gloss propose concept <ns:Name> --kind <kind> [--realizes core:X] [--is-a ns:Y] [--doc '...'] [--internal] [--aliases a,b]\n       gloss propose --from-new")
		return 2
	}
	ns, name, err := splitCurie(rest[0])
	if err != nil {
		return fail(err)
	}
	spec := ontoedit.ConceptSpec{Kind: *kind, Doc: *doc, Internal: *internal, Proposed: true, ProposedBy: actor, Realizes: splitList(*realizes), IsA: splitList(*isA), Aliases: splitList(*aliases)}
	if msg := validateSpec(p, ns, spec); msg != "" {
		return fail(fmt.Errorf("%s", msg))
	}
	path, err := ontologyPath(p, ns)
	if err != nil {
		return fail(err)
	}
	if err := ontoedit.AddConcept(path, name, spec); err != nil {
		return fail(err)
	}
	curie := ns + ":" + name
	n, _ := ontoedit.RewriteNewMarkers(absFiles(p), curie)
	fmt.Printf("gloss propose: %s proposed in %s", curie, relOrAbs(p.WS.Root, path))
	if n > 0 {
		fmt.Printf("; %d marker(s) rewritten from %s:NEW:%s", n, ns, name)
	}
	fmt.Println()
	fmt.Printf("  a human accepts it with: gloss accept --concept %s\n", curie)
	// Record in the ledger now so status shows it before the next build.
	if _, err := buildGraph(*root, true); err != nil {
		return fail(err)
	}
	return 0
}

// flagsLast moves leading positional arguments after the flags so that
// "gloss propose concept ns:Name --kind x" parses (the flag package stops at
// the first positional).
func flagsLast(args []string) []string {
	var pos, rest []string
	i := 0
	for i < len(args) && !strings.HasPrefix(args[i], "-") {
		pos = append(pos, args[i])
		i++
	}
	rest = append(rest, args[i:]...)
	return append(rest, pos...)
}

func validateSpec(p *pipeline, ns string, spec ontoedit.ConceptSpec) string {
	tier := p.WS.Tier(ns)
	if spec.Kind == "" && len(spec.IsA) == 0 {
		return "--kind is required (or --is-a to inherit it): " + strings.Join(model.ConceptKinds, ", ")
	}
	if spec.Kind != "" {
		ok := false
		for _, k := range model.ConceptKinds {
			if k == spec.Kind {
				ok = true
			}
		}
		if !ok {
			return fmt.Sprintf("unknown kind %q; allowed: %s", spec.Kind, strings.Join(model.ConceptKinds, ", "))
		}
	}
	for _, r := range spec.Realizes {
		n := p.Graph.NodeByCurie(r)
		if n == nil {
			return fmt.Sprintf("--realizes %s: no such concept", r)
		}
		if n.Tier != model.TierCore {
			return fmt.Sprintf("--realizes %s must name a core concept", r)
		}
	}
	for _, r := range spec.IsA {
		if p.Graph.NodeByCurie(r) == nil {
			return fmt.Sprintf("--is-a %s: no such concept", r)
		}
	}
	if tier == model.TierImpl && len(spec.Realizes) == 0 && !spec.Internal && len(spec.IsA) == 0 {
		fmt.Fprintf(os.Stderr, "gloss propose: note: implementation concepts need --realizes <core> or --internal before they can be accepted\n")
	}
	if tier == model.TierCore && len(spec.Realizes) > 0 {
		return "core concepts realize nothing; drop --realizes"
	}
	return ""
}

func absFiles(p *pipeline) []string {
	var out []string
	for _, f := range p.WS.Files {
		out = append(out, p.WS.Abs(f))
	}
	return out
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// proposeFromNew lifts every ns:NEW:Name marker into a proposed concept, using
// the marker's relation to guess the kind. The marker's note= is why the link
// was written, not what the concept is, so it goes to the ledger as the
// rationale; the doc comes from --doc (one concept) or a later edit.
func proposeFromNew(p *pipeline, actor, doc string) int {
	rep := rollup.Compare(p.Graph, p.WS, rollup.Options{Policy: p.WS.Config.EffectivePolicy(), Now: time.Now()})
	done := map[string]bool{}
	notes := map[string]string{}
	for _, d := range rep.Decisions {
		if d.Kind == "new-concept" && !done[d.Concept] {
			done[d.Concept] = true
			notes[d.Concept] = d.Note
		}
	}
	if doc != "" && len(done) > 1 {
		return fail(fmt.Errorf("--doc applies to one concept but %d ns:NEW:Name targets were found; propose them one at a time with gloss propose concept <ns:Name> --doc '...'", len(done)))
	}
	done = map[string]bool{}
	n := 0
	for _, d := range rep.Decisions {
		if d.Kind != "new-concept" || done[d.Concept] {
			continue
		}
		done[d.Concept] = true
		ns, name, err := splitCurie(d.Concept)
		if err != nil {
			continue
		}
		path, err := ontologyPath(p, ns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gloss propose: %s: %v\n", d.Concept, err)
			continue
		}
		kind := kindForRelation(d.Relation)
		spec := ontoedit.ConceptSpec{Kind: kind, Doc: doc, Proposed: true, ProposedBy: firstNonEmpty(d.Actor, actor)}
		if err := ontoedit.AddConcept(path, name, spec); err != nil {
			fmt.Fprintf(os.Stderr, "gloss propose: %v\n", err)
			continue
		}
		rw, _ := ontoedit.RewriteNewMarkers(absFiles(p), d.Concept)
		fmt.Printf("gloss propose: %s (%s) proposed in %s; %d marker(s) rewritten\n", d.Concept, kind, relOrAbs(p.WS.Root, path), rw)
		if doc == "" {
			fmt.Printf("  no doc yet: add doc: \"...\" to the entry (one sentence: what it is, not why the link was written)\n")
		}
		n++
	}
	if n == 0 {
		fmt.Println("gloss propose: no ns:NEW:Name markers found")
		return 0
	}
	fmt.Printf("gloss propose: %s; implementation concepts still need realizes: or internal: true before acceptance\n", plural(n, "concept"))
	p2, err := buildGraph(p.WS.Root, true)
	if err != nil {
		return fail(err)
	}
	// The marker's note is the rationale of the proposal.
	for curie, note := range notes {
		if e := p2.Ledger.Entry("concept:" + curie); e != nil && note != "" && e.Rationale == "" {
			e.Rationale = note
			p2.Ledger.Touch()
		}
	}
	if err := p2.Ledger.Save(); err != nil {
		return fail(err)
	}
	return 0
}

func kindForRelation(rel string) string {
	switch rel {
	case "represents":
		return "entity"
	case "provisions", "configures":
		return "resource"
	case "deploys":
		return "system-type"
	case "publishes", "subscribes":
		return "event"
	}
	return "process"
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// acceptConcepts flips proposed: true off for each CURIE.
func acceptConcepts(p *pipeline, curies []string, rev string, dry bool) (int, error) {
	n := 0
	for _, c := range curies {
		ns, name, err := splitCurie(c)
		if err != nil {
			return n, err
		}
		path, err := ontologyPath(p, ns)
		if err != nil {
			return n, err
		}
		prop, err := ontoedit.IsProposed(path, name)
		if err != nil {
			return n, err
		}
		if !prop {
			fmt.Fprintf(os.Stderr, "%s is not proposed; nothing to accept\n", c)
			continue
		}
		// A proposed implementation concept must be complete before acceptance.
		if node := p.Graph.NodeByCurie(c); node != nil && node.Tier == model.TierImpl && !node.Internal && len(p.Graph.Objects(node.ID, "realizes")) == 0 && len(p.Graph.Objects(node.ID, "is-a")) == 0 {
			return n, fmt.Errorf("%s: add realizes: <core concept> or internal: true before accepting", c)
		}
		fmt.Printf("%s concept %s\n", map[bool]string{true: "would accept", false: "accept"}[dry], c)
		if dry {
			continue
		}
		if err := ontoedit.SetProposed(path, name, false, ""); err != nil {
			return n, err
		}
		p.Ledger.Accept("concept:"+c, rev, "", time.Now().UTC())
		n++
	}
	return n, nil
}

// rejectConcept records the rejection and optionally removes the entry.
func rejectConcept(p *pipeline, c, rev, reason string, remove bool) error {
	ns, name, err := splitCurie(c)
	if err != nil {
		return err
	}
	path, err := ontologyPath(p, ns)
	if err != nil {
		return err
	}
	if !p.Ledger.Reject("concept:"+c, rev, reason, time.Now().UTC()) {
		p.Ledger.Entries["concept:"+c] = &model.LedgerEntry{MarkerID: "concept:" + c, Path: relOrAbs(p.WS.Root, path), Relation: "concept", Targets: []string{c}, Created: time.Now().UTC().Format(time.RFC3339), Status: model.StatusRejected, ReviewedBy: rev, Rationale: reason}
		p.Ledger.Touch()
	}
	if remove {
		if err := ontoedit.RemoveConcept(path, name); err != nil {
			return err
		}
		// Markers that pointed at it become ns:NEW:Name proposals again.
		for _, f := range absFiles(p) {
			b, err := os.ReadFile(f)
			if err != nil || !strings.Contains(string(b), c) {
				continue
			}
			style, _ := marker.StyleFor(f, p.WS.Config.CommentStyles)
			_ = style
			out := strings.ReplaceAll(string(b), "@gloss "+" ", "@gloss ")
			out = strings.ReplaceAll(out, c, ns+":NEW:"+name)
			os.WriteFile(f, []byte(out), 0o644)
		}
	}
	return nil
}
