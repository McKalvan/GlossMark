// @gloss implements gmi:SpecInference scope=file

package infer

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// openAPI reads OpenAPI 3.x and Swagger 2.0 documents. An operation
// documents the process its operationId, path words and summary name; a
// schema represents the entity it is named for; a handler whose route
// the document covers implements the operation's process. The names are
// handed to the runner's matcher, so the spec and the code resolve alike.
type openAPI struct {
	ops   []*operation
	files map[string][]Item
}

type operation struct {
	method, path, id string
	norm             string // path with every parameter as {}
	target           Target
}

// processKinds are what an operation or a route can implement; an
// implementation concept that realizes one counts too.
var processKinds = []string{"process", "capability"}

var httpMethods = map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true, "head": true, "options": true}

func (s *openAPI) Name() string { return "openapi" }

func (s *openAPI) Index(files Files, v Vocabulary) error {
	s.ops, s.files = nil, map[string][]Item{}
	for _, f := range files.List {
		rel := f.Path
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		lines, root := readSpec(files.Abs(rel), "openapi")
		if root == nil {
			continue
		}
		items := s.read(root, lines)
		if ext != ".json" {
			s.files[rel] = items
		}
	}
	return nil
}

// read indexes one document's operations and returns its items.
func (s *openAPI) read(root *yaml.Node, lines []string) []Item {
	var items []Item
	if paths := mapValue(root, "paths"); paths != nil && paths.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(paths.Content); i += 2 {
			p, pi := paths.Content[i], paths.Content[i+1]
			if pi.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(pi.Content); j += 2 {
				mk, op := pi.Content[j], pi.Content[j+1]
				if !httpMethods[strings.ToLower(mk.Value)] || op.Kind != yaml.MappingNode {
					continue
				}
				o := &operation{method: strings.ToLower(mk.Value), path: p.Value, norm: NormPath(p.Value), id: scalar(mapValue(op, "operationId"))}
				doc := strings.TrimSpace(scalar(mapValue(op, "summary")) + " " + scalar(mapValue(op, "description")))
				if tags := mapValue(op, "tags"); tags != nil {
					for _, t := range tags.Content {
						doc += " " + t.Value
					}
				}
				sym := o.id
				if sym == "" {
					sym = o.method + " " + o.path
				}
				// The matcher's evidence kind is the basis (openapi/name-exact/operation),
				// so the names decide between a process and the mechanism that realizes it.
				o.target = Target{Names: uniq([]string{o.id, PathWords(o.path)}), Kinds: processKinds, Terms: strings.TrimSpace(o.method + " " + o.path)}
				s.ops = append(s.ops, o)
				items = append(items, item(lines, mk.Line, sym, "operation", "documents", doc, o.target))
			}
		}
	}
	schemas := mapValue(mapValue(root, "components"), "schemas")
	if schemas == nil {
		schemas = mapValue(root, "definitions")
	}
	if schemas != nil && schemas.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(schemas.Content); i += 2 {
			k, v := schemas.Content[i], schemas.Content[i+1]
			items = append(items, item(lines, k.Line, k.Value, "schema", "represents", scalar(mapValue(v, "description")), Target{Names: []string{k.Value}, Kinds: []string{"entity", "event", "measure"}}))
		}
	}
	return items
}

func (s *openAPI) Items(path string) []Item { return s.files[path] }

// operationFor finds the operation a route declares: same method (when
// known) and a document path that is, or ends with, the route's path.
func (s *openAPI) operationFor(r Route) *operation {
	norm := NormPath(r.Path)
	var best *operation
	for _, o := range s.ops {
		if r.Method != "" && o.method != r.Method {
			continue
		}
		if o.norm == norm || strings.HasSuffix(o.norm, norm) && norm != "" && norm != "/" {
			if best == nil || len(o.norm) < len(best.norm) {
				best = o
			}
		}
	}
	return best
}

// Hints: a handler whose route the document covers implements the
// operation's process.
func (s *openAPI) Hints(d Decl) []Hint {
	if len(s.ops) == 0 || d.Role != "func" && d.Role != "member" {
		return nil
	}
	var out []Hint
	for _, rt := range Routes(d.Decorators, d.Body) {
		o := s.operationFor(rt)
		if o == nil {
			continue
		}
		t := o.target
		t.Terms = strings.TrimSpace(rt.Method + " " + rt.Path)
		out = append(out, Hint{Target: t, Relation: "implements", Via: rt.Path, Line: d.Line})
	}
	return out
}
