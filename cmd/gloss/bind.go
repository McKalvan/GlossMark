// @gloss implements gmi:BindCommand scope=file

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/ontoedit"
)

func init() {
	register(command{"bind", "bind a constant in the code to a published value: gloss bind <path>:<symbol> --as <ns:Name> reads the value from the code, writes value:, value-from: and published: into the ontology file, puts a @gloss publishes-value marker above the line and records the baseline, so a later change to the line alone is drift and HARD", cmdBind})
}

// binding is what gloss bind resolved before it writes anything.
type binding struct {
	rel, symbol string // the file (repo-relative) and the symbol in it
	abs         string // the file on disk
	line        int    // the 1-based line that assigns the symbol
	text        string // that line, trimmed
	value       string // what value: will say
	ns, name    string // the concept
	exists      bool   // the concept is already in the ontology file
	ontology    string // the ontology file (absolute)
	spec        ontoedit.ConceptSpec
	markerLine  string   // the marker to insert above line, "" for none
	markerIndex int      // the 0-based line it goes before
	why         []string // reasons the marker is left out
	srcLines    []string // the file's lines
	eol         string
	manifest    *model.Manifest
}

func cmdBind(args []string) int {
	fs, root := newFlags("bind")
	as := fs.String("as", "", "the concept the value belongs to (ns:Name); created when it does not exist")
	realizes := fs.String("realizes", "", "core concept the new implementation concept realizes (default: --publish-as)")
	publishAs := fs.String("publish-as", "", "core concept the value is published as (published: { as: ... }); implies --realizes")
	kind := fs.String("kind", "constraint", "kind of a new concept: constraint or measure")
	value := fs.String("value", "", "what value: says (default: the literal the line assigns, 5_000 read as 5000)")
	doc := fs.String("doc", "", "one sentence for a new concept (default: '<symbol> in <path>')")
	internal := fs.Bool("internal", false, "a new implementation concept that realizes nothing (internal: true)")
	noMarker := fs.Bool("no-marker", false, "write value-from: only; no @gloss publishes-value marker above the line")
	dry := fs.Bool("n", false, "dry run: print what would be written")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || *as == "" {
		fmt.Fprintln(os.Stderr, "usage: gloss bind <path>:<symbol> --as <ns:Name> [--publish-as core:X] [--realizes core:X] [--kind constraint|measure] [--value '...'] [--doc '...'] [--internal] [--no-marker] [-n]")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	b, err := resolveBinding(p, fs.Arg(0), *as, *realizes, *publishAs, *kind, *value, *doc, *internal, *noMarker)
	if err != nil {
		return fail(err)
	}
	curie := b.ns + ":" + b.name
	ontRel := relOrAbs(p.WS.Root, b.ontology)
	verb := map[bool]string{true: "would bind", false: "bind"}[*dry]
	fmt.Printf("gloss bind: %s %s = %q, bound to %s:%d (%s)\n", verb, curie, b.value, b.rel, b.line, b.text)
	if b.exists {
		fmt.Printf("  %s: %s gets value: %s and value-from: %s\n", ontRel, curie, ontoedit.Quote(b.value), b.spec.ValueFrom.Render())
	} else {
		fmt.Printf("  %s: adds %s (%s)\n", ontRel, curie, describeSpec(b.spec))
	}
	if b.markerLine != "" {
		fmt.Printf("  %s:%d: %s\n", b.rel, b.line, strings.TrimSpace(b.markerLine))
	} else if *noMarker {
		fmt.Printf("  no marker (--no-marker): value-from: alone binds the line\n")
	} else {
		fmt.Printf("  no marker: %s; value-from: alone binds the line\n", strings.Join(b.why, "; "))
	}
	if *dry {
		return 0
	}
	if err := writeBinding(b); err != nil {
		return fail(err)
	}
	// Build with the ledger so the line becomes the baseline now; a GM360
	// here means the file changed under us between the read and the write.
	after, err := buildGraph(*root, true)
	if err != nil {
		return fail(err)
	}
	for _, d := range after.Diags {
		if d.Severity == "error" && strings.Contains(d.Msg, curie) {
			return fail(fmt.Errorf("%s", d))
		}
	}
	n := after.Graph.NodeByCurie(curie)
	if n == nil || n.ValueFrom == "" {
		return fail(fmt.Errorf("%s is not bound after the write; run gloss check", curie))
	}
	fmt.Printf("  baseline %s recorded in %s; commit it with the ontology and the code\n", n.ValueFP, relOrAbs(p.WS.Root, p.WS.StatePath("ledger.jsonl")))
	fmt.Printf("  a change to %s:%d without a change to value: is now drift (gloss drift) and HARD (gloss diff); gloss check confirms the entry\n", b.rel, b.line+markerShift(b))
	return 0
}

// resolveBinding reads the symbol's line and value and decides what to write,
// without writing. Errors say what to pass instead.
func resolveBinding(p *pipeline, target, as, realizes, publishAs, kind, value, doc string, internal, noMarker bool) (*binding, error) {
	i := strings.LastIndex(target, ":")
	if i <= 0 || i == len(target)-1 {
		return nil, fmt.Errorf("%q is not <path>:<symbol> (services/assessment/settings.py:AUTO_APPROVE_LIMIT_EUR)", target)
	}
	path, symbol := target[:i], target[i+1:]
	rel, err := workspacePath(p, path)
	if err != nil {
		return nil, err
	}
	if _, err := p.WS.Within(rel); err != nil {
		return nil, err
	}
	b := &binding{rel: rel, symbol: symbol, abs: p.WS.Abs(rel)}
	b.line, b.text, err = extract.ResolveValueFrom(p.WS, model.ValueFrom{Path: rel, Symbol: symbol})
	if err != nil {
		return nil, fmt.Errorf("%s: %v", target, err)
	}
	b.value = value
	if b.value == "" {
		if b.value = extract.ValueLiteral(b.text, lastSegment(symbol)); b.value == "" {
			return nil, fmt.Errorf("%s:%d assigns nothing readable (%q); pass --value", rel, b.line, b.text)
		}
	}
	b.ns, b.name, err = splitCurie(as)
	if err != nil {
		return nil, err
	}
	if b.ontology, err = ontologyPath(p, b.ns); err != nil {
		return nil, err
	}
	curie := b.ns + ":" + b.name
	if publishAs != "" && realizes == "" {
		realizes = publishAs
	}
	tier := p.WS.Tier(b.ns)
	for _, c := range []struct{ flag, cur string }{{"--realizes", realizes}, {"--publish-as", publishAs}} {
		if c.cur == "" {
			continue
		}
		if tier == model.TierCore {
			return nil, fmt.Errorf("%s is a core concept and realizes nothing; drop %s, or bind an implementation concept and publish it as %s", curie, c.flag, curie)
		}
		n := p.Graph.NodeByCurie(c.cur)
		if n == nil {
			return nil, fmt.Errorf("%s %s: no such concept; a published value names an existing core concept (propose it first, gloss propose concept %s --kind %s)", c.flag, c.cur, c.cur, kind)
		}
		if n.Tier != model.TierCore {
			return nil, fmt.Errorf("%s %s must name a core concept; %s is %s", c.flag, c.cur, c.cur, n.Tier)
		}
	}
	if existing := p.Graph.NodeByCurie(curie); existing != nil {
		b.exists = true
		if existing.Kind != "constraint" && existing.Kind != "measure" {
			return nil, fmt.Errorf("%s is a %s; a published value belongs to a constraint or a measure", curie, existing.Kind)
		}
		if existing.ValueFrom != "" && !noMarker {
			b.why = append(b.why, fmt.Sprintf("%s is already bound to %s", curie, existing.ValueFrom))
		}
	} else {
		if kind != "constraint" && kind != "measure" {
			return nil, fmt.Errorf("--kind %s: a published value belongs to a constraint or a measure", kind)
		}
		if tier == model.TierImpl && realizes == "" && !internal {
			return nil, fmt.Errorf("%s would be a new implementation concept: say what business rule it is with --publish-as <core:Concept> (or --realizes <core:Concept>, or --internal for an engineering-private setting)", curie)
		}
		if internal && publishAs != "" {
			return nil, fmt.Errorf("--internal and --publish-as: a concept cannot be both internal and published")
		}
		if doc == "" {
			doc = fmt.Sprintf("%s in %s.", symbol, rel)
		}
	}
	b.spec = ontoedit.ConceptSpec{Kind: kind, Doc: doc, Internal: internal, PublishedAs: publishAs, Value: b.value, ValueFrom: &ontoedit.ValueFrom{Path: rel, Symbol: symbol}}
	if realizes != "" {
		b.spec.Realizes = []string{realizes}
	}
	b.manifest = p.WS.ManifestFor(rel)
	if !noMarker && len(b.why) == 0 {
		b.renderMarker(p)
	}
	return b, nil
}

// renderMarker prepares the publishes-value marker line above the bound
// line, in the file's comment style and the line's indentation, or records
// why there is none.
func (b *binding) renderMarker(p *pipeline) {
	style, ok := marker.StyleFor(b.rel, p.WS.Config.CommentStyles)
	if !ok {
		b.why = append(b.why, fmt.Sprintf("%s has no comment syntax GlossMark knows", filepath.Ext(b.rel)))
		return
	}
	raw, err := os.ReadFile(b.abs)
	if err != nil {
		b.why = append(b.why, err.Error())
		return
	}
	b.eol = "\n"
	if strings.Contains(string(raw), "\r\n") {
		b.eol = "\r\n"
	}
	b.srcLines = strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	target := b.ns + ":" + b.name
	if b.manifest != nil && b.manifest.DefaultNamespace == b.ns {
		target = "." + b.name
	}
	line := "@gloss publishes-value " + target + " scope=line"
	switch {
	case len(style.Line) > 0 && style.Line[0] != "":
		line = style.Line[0] + " " + line
	case len(style.Block) > 0:
		line = style.Block[0][0] + " " + line + " " + style.Block[0][1]
	default:
		b.why = append(b.why, "sidecar files take no publishes-value marker")
		return
	}
	src := b.srcLines[b.line-1]
	indent := src[:len(src)-len(strings.TrimLeft(src, " \t"))]
	// A marker already there, by hand or by an earlier bind, is kept.
	if b.line >= 2 {
		if above := strings.TrimSpace(b.srcLines[b.line-2]); strings.Contains(above, "@gloss publishes-value ") && (strings.Contains(above, " "+target+" ") || strings.Contains(above, " "+b.ns+":"+b.name+" ")) {
			b.why = append(b.why, fmt.Sprintf("line %d already carries it", b.line-1))
			return
		}
	}
	b.markerLine = indent + line
	b.markerIndex = b.line - 1
}

// writeBinding writes the ontology entry (or the fields on an existing one)
// and the marker.
func writeBinding(b *binding) error {
	if b.exists {
		for _, f := range []struct{ key, val string }{
			{"value", ontoedit.Quote(b.value)},
			{"value-from", b.spec.ValueFrom.Render()},
		} {
			if err := ontoedit.SetField(b.ontology, b.name, f.key, f.val); err != nil {
				return err
			}
		}
		if b.spec.PublishedAs != "" {
			if err := ontoedit.SetField(b.ontology, b.name, "published", "{ as: "+b.spec.PublishedAs+" }"); err != nil {
				return err
			}
		}
		if len(b.spec.Realizes) > 0 {
			cur, _, err := ontoedit.Get(b.ontology, b.name, "realizes")
			if err != nil {
				return err
			}
			if cur == "" {
				if err := ontoedit.SetField(b.ontology, b.name, "realizes", b.spec.Realizes[0]); err != nil {
					return err
				}
			} else if cur != b.spec.Realizes[0] && !strings.Contains(cur, b.spec.Realizes[0]) {
				return fmt.Errorf("%s already realizes %s; add %s to realizes: by hand", b.name, cur, b.spec.Realizes[0])
			}
		}
	} else if err := ontoedit.AddConcept(b.ontology, b.name, b.spec); err != nil {
		return err
	}
	if b.markerLine == "" {
		return nil
	}
	lines := append([]string{}, b.srcLines[:b.markerIndex]...)
	lines = append(lines, b.markerLine)
	lines = append(lines, b.srcLines[b.markerIndex:]...)
	return os.WriteFile(b.abs, []byte(strings.Join(lines, b.eol)), 0o644)
}

// workspacePath turns a path as typed (relative to the working directory,
// absolute, or already repo-relative) into the slash-separated repo-relative
// form the graph uses.
func workspacePath(p *pipeline, path string) (string, error) {
	if abs, err := filepath.Abs(path); err == nil {
		if _, statErr := os.Stat(abs); statErr == nil {
			if rel, err := filepath.Rel(p.WS.Root, abs); err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel), nil
			}
		}
	}
	rel := filepath.ToSlash(filepath.Clean(path))
	if _, err := os.Stat(p.WS.Abs(rel)); err != nil {
		return "", fmt.Errorf("%s: no such file under %s", path, p.WS.Root)
	}
	return rel, nil
}

// lastSegment is the key a dotted key path ends with (a.b.c → c), the
// symbol itself otherwise.
func lastSegment(symbol string) string {
	if i := strings.LastIndex(symbol, "."); i >= 0 {
		return symbol[i+1:]
	}
	return symbol
}

func describeSpec(spec ontoedit.ConceptSpec) string {
	parts := []string{spec.Kind}
	if len(spec.Realizes) > 0 {
		parts = append(parts, "realizes "+strings.Join(spec.Realizes, ", "))
	}
	if spec.PublishedAs != "" {
		parts = append(parts, "published as "+spec.PublishedAs)
	}
	if spec.Internal {
		parts = append(parts, "internal")
	}
	parts = append(parts, "value "+ontoedit.Quote(spec.Value))
	return strings.Join(parts, ", ")
}

// markerShift is how far the bound line moved down: one when a marker was
// inserted above it.
func markerShift(b *binding) int {
	if b.markerLine != "" {
		return 1
	}
	return 0
}
