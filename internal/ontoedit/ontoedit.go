// Package ontoedit makes small, formatting-preserving edits to *.gloss.yaml
// files: add a proposed concept, accept it (drop proposed:), remove it, and
// rewrite ns:NEW:Name markers once the concept exists.
package ontoedit

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConceptSpec is what a proposal carries.
type ConceptSpec struct {
	Kind       string
	Doc        string
	Realizes   []string
	IsA        []string
	Internal   bool
	Aliases    []string
	ProposedBy string
	Proposed   bool
}

func loadDoc(path string) (*yaml.Node, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			var doc yaml.Node
			if err := yaml.Unmarshal([]byte("concepts: {}\n"), &doc); err != nil {
				return nil, err
			}
			return &doc, nil
		}
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		if err := yaml.Unmarshal([]byte("concepts: {}\n"), &doc); err != nil {
			return nil, err
		}
	}
	return &doc, nil
}

func saveDoc(path string, doc *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	enc.Close()
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// mapGet returns the value node for key in a mapping node, or nil.
func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mapSet(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, val)
}

func mapDel(m *yaml.Node, key string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return true
		}
	}
	return false
}

func scalar(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: v} }

func boolNode(v bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(v)}
}

func seq(vals []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, v := range vals {
		n.Content = append(n.Content, scalar(v))
	}
	return n
}

func conceptsMap(doc *yaml.Node) *yaml.Node {
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		root = &yaml.Node{Kind: yaml.MappingNode}
		doc.Content[0] = root
	}
	cm := mapGet(root, "concepts")
	if cm == nil || cm.Kind != yaml.MappingNode {
		cm = &yaml.Node{Kind: yaml.MappingNode}
		mapSet(root, "concepts", cm)
	}
	return cm
}

// AddConcept appends a concept entry. It fails when the name exists.
func AddConcept(path, name string, spec ConceptSpec) error {
	doc, err := loadDoc(path)
	if err != nil {
		return err
	}
	cm := conceptsMap(doc)
	if mapGet(cm, name) != nil {
		return fmt.Errorf("%s: concept %s already exists", path, name)
	}
	entry := &yaml.Node{Kind: yaml.MappingNode}
	if spec.Kind != "" {
		mapSet(entry, "kind", scalar(spec.Kind))
	}
	if len(spec.IsA) > 0 {
		mapSet(entry, "is-a", listOrScalar(spec.IsA))
	}
	if len(spec.Realizes) > 0 {
		mapSet(entry, "realizes", listOrScalar(spec.Realizes))
	}
	if spec.Internal {
		mapSet(entry, "internal", boolNode(true))
	}
	if spec.Doc != "" {
		// Quoted: inside a { } flow mapping an unquoted doc ends at its first comma.
		mapSet(entry, "doc", &yaml.Node{Kind: yaml.ScalarNode, Style: yaml.DoubleQuotedStyle, Value: spec.Doc})
	}
	if len(spec.Aliases) > 0 {
		mapSet(entry, "aliases", seq(spec.Aliases))
	}
	if spec.Proposed {
		mapSet(entry, "proposed", boolNode(true))
		if spec.ProposedBy != "" {
			mapSet(entry, "proposed-by", scalar(spec.ProposedBy))
		}
	}
	mapSet(cm, name, entry)
	return saveDoc(path, doc)
}

func listOrScalar(vals []string) *yaml.Node {
	if len(vals) == 1 {
		return scalar(vals[0])
	}
	return seq(vals)
}

// SetProposed accepts (false) or re-proposes (true) a concept.
func SetProposed(path, name string, proposed bool, by string) error {
	doc, err := loadDoc(path)
	if err != nil {
		return err
	}
	cm := conceptsMap(doc)
	entry := mapGet(cm, name)
	if entry == nil {
		return fmt.Errorf("%s: no concept %s", path, name)
	}
	if entry.Kind != yaml.MappingNode {
		entry = &yaml.Node{Kind: yaml.MappingNode}
		mapSet(cm, name, entry)
	}
	if proposed {
		mapSet(entry, "proposed", boolNode(true))
		if by != "" {
			mapSet(entry, "proposed-by", scalar(by))
		}
	} else {
		mapDel(entry, "proposed")
		mapDel(entry, "proposed-by")
	}
	return saveDoc(path, doc)
}

// RemoveConcept deletes an entry.
func RemoveConcept(path, name string) error {
	doc, err := loadDoc(path)
	if err != nil {
		return err
	}
	cm := conceptsMap(doc)
	if !mapDel(cm, name) {
		return fmt.Errorf("%s: no concept %s", path, name)
	}
	return saveDoc(path, doc)
}

// IsProposed reports whether the entry carries proposed: true.
func IsProposed(path, name string) (bool, error) {
	doc, err := loadDoc(path)
	if err != nil {
		return false, err
	}
	entry := mapGet(conceptsMap(doc), name)
	if entry == nil {
		return false, fmt.Errorf("%s: no concept %s", path, name)
	}
	p := mapGet(entry, "proposed")
	return p != nil && p.Value == "true", nil
}

// RewriteNewMarkers replaces ns:NEW:Name with ns:Name in the given files.
// It returns the number of lines changed.
func RewriteNewMarkers(files []string, curie string) (int, error) {
	i := strings.Index(curie, ":")
	if i <= 0 {
		return 0, fmt.Errorf("bad curie %q", curie)
	}
	from := curie[:i] + ":NEW:" + curie[i+1:]
	re := regexp.MustCompile(`(@gloss\??\s+[a-z][a-z0-9-]*\s+(?:[^\n]*?[,\s])?)` + regexp.QuoteMeta(from) + `\b`)
	n := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return n, err
		}
		if !bytes.Contains(b, []byte(from)) {
			continue
		}
		out := re.ReplaceAllString(string(b), "${1}"+curie)
		if out != string(b) {
			n += strings.Count(string(b), from) - strings.Count(out, from)
			if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}
