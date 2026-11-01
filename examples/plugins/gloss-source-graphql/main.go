// Command gloss-source-graphql is a reference link source for gloss
// suggest (docs/PLUGINS.md): a GraphQL schema (SDL) read through the
// plugin protocol, contract v1. It is its own module with no
// dependencies and imports nothing from gloss: what it shares with the
// core is the JSON on stdin and stdout.
//
// What it proposes:
//
//   - an object type in a schema file represents the entity (or event,
//     measure) it is named for: `type Order {` → @gloss? represents
//     hp:Order scope=block, anchored above the type;
//   - a field of Query, Mutation or Subscription documents the process it
//     names: `placeOrder(input: ...)` → @gloss? documents hp:OrderPlacement,
//     anchored on the field's line;
//   - code that resolves an operation implements that process: a
//     declaration whose body names the field (placeOrder, or place_order
//     in a snake_case language) gets an implements hint; a declaration
//     named like a type (class Order) gets a represents hint.
//
// The plugin never names a concept itself: it hands the core the names
// (split into words) and the kinds they may match, and the core's
// matcher, calibration, dedupe and anchoring do the rest. Its bases are
// graphql/type/*, graphql/operation/* and graphql/field/*; until a
// repository measures them (gloss suggest --calibrate --write) they carry
// the declared prior, capped below the auto-accept threshold.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
)

// The protocol, as docs/PLUGINS.md fixes it. Only the fields this plugin
// reads or writes are declared; unknown fields are ignored either way.
const contract = 1

type message struct {
	Type     string `json:"type"`
	Contract int    `json:"contract,omitempty"`
	// index
	Root  string `json:"root,omitempty"`
	Files []file `json:"files,omitempty"`
	// read / files
	Globs    []string  `json:"globs,omitempty"`
	Contents []content `json:"contents,omitempty"`
	// indexed
	Name   string             `json:"name,omitempty"`
	Items  map[string][]item  `json:"items,omitempty"`
	Priors map[string]float64 `json:"priors,omitempty"`
	// hints
	Decl  *decl  `json:"decl,omitempty"`
	Hints []hint `json:"hints,omitempty"`
	// error
	Message string `json:"message,omitempty"`
}

type file struct {
	Path string `json:"path"`
}

type content struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type target struct {
	Concept string   `json:"concept,omitempty"`
	Names   []string `json:"names,omitempty"`
	Kinds   []string `json:"kinds,omitempty"`
	Basis   string   `json:"basis,omitempty"`
	Terms   string   `json:"terms,omitempty"`
}

type item struct {
	Symbol   string `json:"symbol"`
	Role     string `json:"role"`
	Relation string `json:"relation"`
	Line     int    `json:"line"`
	End      int    `json:"end"`
	Indent   string `json:"indent,omitempty"`
	Doc      string `json:"doc,omitempty"`
	target
}

type decl struct {
	Path   string `json:"path"`
	Symbol string `json:"symbol"`
	Role   string `json:"role"`
	Line   int    `json:"line"`
	Body   string `json:"body"`
}

type hint struct {
	target
	Relation string `json:"relation,omitempty"`
	Via      string `json:"via,omitempty"`
	Line     int    `json:"line,omitempty"`
}

var (
	entityKinds  = []string{"entity", "event", "measure"}
	processKinds = []string{"process", "capability"}
	schemaExts   = map[string]bool{".graphql": true, ".graphqls": true, ".gql": true}
	// priors are what the author expects of each basis, by role; the core
	// caps them below the auto-accept threshold until measured.
	priors = map[string]float64{"type": 0.7, "operation": 0.6, "field": 0.6}
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Fprintf(os.Stderr, "gloss-source-graphql: a gloss suggest link source for GraphQL schemas (contract v%d).\nPut it on PATH and run gloss suggest; gloss source list shows it. It reads no flags: gloss speaks to it on stdin/stdout.\n", contract)
		return
	}
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gloss-source-graphql:", err)
		os.Exit(1)
	}
}

func serve(in io.Reader, out io.Writer) error {
	dec, enc := json.NewDecoder(in), json.NewEncoder(out)
	fail := func(err error) error {
		enc.Encode(message{Type: "error", Message: err.Error()})
		return err
	}
	var idx message
	if err := dec.Decode(&idx); err != nil {
		return fmt.Errorf("reading the index message: %w", err)
	}
	if idx.Type != "index" {
		return fail(fmt.Errorf("expected index, got %q", idx.Type))
	}
	if idx.Contract != contract {
		// Answer with our version so the core can name both.
		enc.Encode(message{Type: "indexed", Contract: contract, Name: "graphql"})
		return fmt.Errorf("gloss speaks contract v%d, this plugin v%d", idx.Contract, contract)
	}
	// Ask the core for the schema files' content rather than reading the
	// disk: the plugin then needs no file access of its own.
	var globs []string
	for ext := range schemaExts {
		globs = append(globs, "**/*"+ext)
	}
	if err := enc.Encode(message{Type: "read", Globs: globs}); err != nil {
		return err
	}
	var files message
	if err := dec.Decode(&files); err != nil {
		return err
	}
	if files.Type != "files" {
		return fail(fmt.Errorf("expected files, got %q", files.Type))
	}
	s := &schema{}
	items := map[string][]item{}
	for _, f := range files.Contents {
		if !schemaExts[strings.ToLower(path.Ext(f.Path))] {
			continue
		}
		if its := s.read(f.Content); len(its) > 0 {
			items[f.Path] = its
		}
	}
	if err := enc.Encode(message{Type: "indexed", Contract: contract, Name: "graphql", Items: items, Priors: priors}); err != nil {
		return err
	}
	for {
		var q message
		if err := dec.Decode(&q); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if q.Type != "hints" || q.Decl == nil {
			return fail(fmt.Errorf("expected hints with a decl, got %q", q.Type))
		}
		if err := enc.Encode(message{Type: "hints", Hints: s.hints(*q.Decl)}); err != nil {
			return err
		}
	}
}

// schema is what every schema file declared: object types and the
// operations (fields of Query, Mutation, Subscription).
type schema struct {
	types      map[string]string // lower-case name → name as written
	operations map[string]string // field name → "Mutation.placeOrder"
}

var (
	defLine   = regexp.MustCompile(`^\s*(?:extend\s+)?(type|interface|input|enum|union|scalar)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	fieldLine = regexp.MustCompile(`^(\s*)([A-Za-z_][A-Za-z0-9_]*)\s*(?:\(|:)`)
	defined   = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*[:(]`)
	operation = map[string]bool{"Query": true, "Mutation": true, "Subscription": true}
)

// read indexes one schema's text and returns its items: one per object
// type (represents) and one per operation field (documents). The doc is
// the description above the definition: a """block""" or # comments.
func (s *schema) read(text string) []item {
	if s.types == nil {
		s.types, s.operations = map[string]string{}, map[string]string{}
	}
	lines := strings.Split(text, "\n")
	var items []item
	for i := 0; i < len(lines); i++ {
		m := defLine.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		kind, name := m[1], m[2]
		if kind == "scalar" || kind == "union" {
			continue
		}
		end := blockEnd(lines, i)
		switch {
		case operation[name]:
			for j := i + 1; j <= end && j < len(lines); j++ {
				f := fieldLine.FindStringSubmatch(lines[j])
				if f == nil || strings.HasPrefix(strings.TrimSpace(lines[j]), "#") {
					continue
				}
				sym := name + "." + f[2]
				s.operations[f[2]] = sym
				// A mutation does work: a process. A query reads: a process,
				// or the entity it returns (order(id): Order).
				kinds := processKinds
				if name == "Query" {
					kinds = append(append([]string{}, processKinds...), entityKinds...)
				}
				items = append(items, item{Symbol: sym, Role: "operation", Relation: "documents", Line: j + 1, End: j + 1, Indent: f[1], Doc: docAbove(lines, j),
					target: target{Names: []string{words(f[2])}, Kinds: kinds, Basis: "operation", Terms: f[2]}})
			}
		case kind == "type" || kind == "interface":
			s.types[strings.ToLower(name)] = name
			items = append(items, item{Symbol: name, Role: "type", Relation: "represents", Line: i + 1, End: end + 1, Doc: docAbove(lines, i),
				target: target{Names: []string{words(name)}, Kinds: entityKinds, Basis: "type", Terms: name}})
		}
		i = end
	}
	return items
}

// hints: a declaration whose body names an operation field (as written,
// or in snake_case) implements the process the field names; a
// declaration named like a type represents what the type represents.
func (s *schema) hints(d decl) []hint {
	var out []hint
	seen := map[string]bool{}
	if d.Role == "type" {
		if name, ok := s.types[strings.ToLower(last(d.Symbol))]; ok {
			out = append(out, hint{target: target{Names: []string{words(name)}, Kinds: entityKinds, Basis: "type", Terms: name}, Relation: "represents", Via: last(d.Symbol), Line: d.Line})
		}
		return out
	}
	for i, ln := range strings.Split(d.Body, "\n") {
		// An identifier that is defined or called (place_order(, placeOrder:)
		// rather than merely mentioned: `order` is also a word.
		for _, m := range defined.FindAllStringSubmatch(ln, -1) {
			id := m[1]
			field, ok := s.fieldFor(id)
			if !ok || seen[field] {
				continue
			}
			seen[field] = true
			out = append(out, hint{target: target{Names: []string{words(field)}, Kinds: processKinds, Basis: "field", Terms: field}, Relation: "implements", Via: id, Line: d.Line + i})
		}
	}
	return out
}

// fieldFor is the operation field an identifier names: itself, or its
// camelCase form (place_order → placeOrder).
func (s *schema) fieldFor(id string) (string, bool) {
	if _, ok := s.operations[id]; ok {
		return id, true
	}
	if strings.Contains(id, "_") {
		camel := camelCase(id)
		if _, ok := s.operations[camel]; ok {
			return camel, true
		}
	}
	return "", false
}

// blockEnd is the index of the line that closes the block opened at i
// (the line itself when it opens none).
func blockEnd(lines []string, i int) int {
	depth := 0
	for j := i; j < len(lines); j++ {
		depth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
		if depth <= 0 && (j > i || strings.Contains(lines[j], "}")) {
			return j
		}
		if depth <= 0 && j == i && !strings.Contains(lines[j], "{") {
			return j
		}
	}
	return len(lines) - 1
}

// docAbove is the description right above line i: a """block""" or a
// run of # comments.
func docAbove(lines []string, i int) string {
	var doc []string
	j := i - 1
	if j >= 0 && strings.HasSuffix(strings.TrimSpace(lines[j]), `"""`) {
		for k := j; k >= 0; k-- {
			t := strings.TrimSpace(lines[k])
			if inner := strings.TrimSpace(strings.Trim(t, `"`)); inner != "" {
				doc = append([]string{inner}, doc...)
			}
			// The opening """: a line that starts with it, unless it is
			// the closing line itself (a bare """), or the one-line form.
			if strings.HasPrefix(t, `"""`) && (k < j || len(t) > 3) {
				break
			}
		}
		return strings.Join(doc, " ")
	}
	for ; j >= 0; j-- {
		t := strings.TrimSpace(lines[j])
		if !strings.HasPrefix(t, "#") || strings.Contains(t, "@gloss") {
			break
		}
		doc = append([]string{strings.TrimSpace(strings.TrimPrefix(t, "#"))}, doc...)
	}
	return strings.Join(doc, " ")
}

var camelSplit = regexp.MustCompile(`([a-z0-9])([A-Z])|([A-Z]+)([A-Z][a-z])`)

// words splits an identifier into lower-case words: placeOrder → place
// order, OrderLine → order line, place_order → place order.
func words(id string) string {
	s := camelSplit.ReplaceAllString(id, "$1$3 $2$4")
	s = strings.NewReplacer("_", " ", "-", " ").Replace(s)
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func camelCase(snake string) string {
	parts := strings.Split(snake, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func last(sym string) string {
	if i := strings.LastIndexAny(sym, ".:/"); i >= 0 {
		return sym[i+1:]
	}
	return sym
}
