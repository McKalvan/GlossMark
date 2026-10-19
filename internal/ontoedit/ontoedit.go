// Package ontoedit makes small, formatting-preserving edits to *.gloss.yaml
// files: add a proposed concept, accept it (drop proposed:), remove it, and
// rewrite ns:NEW:Name markers once the concept exists.
//
// Edits are made on the file's lines, never by re-marshalling the document:
// alignment, quoting, comments and key order stay as the owner wrote them,
// so a proposal is a diff of the lines it adds and nothing else. The parsed
// node tree is used only to locate entries.
package ontoedit

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
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

// file is one ontology file: its lines plus the parsed tree for locating entries.
type file struct {
	path  string
	lines []string
	eol   string
	root  *yaml.Node // the top-level mapping, or nil for an empty file
}

func open(path string) (*file, error) {
	f := &file{path: path, eol: "\n"}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}
	if bytes.Contains(b, []byte("\r\n")) {
		f.eol = "\r\n"
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if text != "" {
		f.lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 && doc.Content[0].Kind == yaml.MappingNode {
		f.root = doc.Content[0]
	}
	return f, nil
}

func (f *file) save() error {
	var sb strings.Builder
	for _, l := range f.lines {
		sb.WriteString(l)
		sb.WriteString(f.eol)
	}
	return os.WriteFile(f.path, []byte(sb.String()), 0o644)
}

// mapGet returns the key and value nodes for key in a mapping node.
func mapGet(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// concepts returns the concepts: key and mapping nodes (either may be nil).
func (f *file) concepts() (*yaml.Node, *yaml.Node) {
	return mapGet(f.root, "concepts")
}

// entry returns the key and value nodes of a concept, or nils.
func (f *file) entry(name string) (*yaml.Node, *yaml.Node) {
	_, cm := f.concepts()
	return mapGet(cm, name)
}

// extent returns the 0-based first and last line of a concept entry: the key
// line through the last line of its value, including a multi-line flow
// mapping's closing brace and a block scalar's continuation lines.
func (f *file) extent(key, val *yaml.Node) (int, int) {
	start := key.Line - 1
	end := maxLine(key, val) - 1
	if val != nil && val.Kind != yaml.ScalarNode && val.Style&yaml.FlowStyle != 0 {
		// Walk the brackets from the key line until they close.
		depth := 0
		for k := start; k < len(f.lines); k++ {
			depth += bracketDelta(f.lines[k])
			if k >= end && depth <= 0 {
				end = k
				break
			}
		}
	} else {
		ind := indent(f.lines[start])
		for end+1 < len(f.lines) {
			t := strings.TrimSpace(f.lines[end+1])
			if t == "" || indent(f.lines[end+1]) <= ind {
				break
			}
			end++
		}
	}
	return start, end
}

func maxLine(nodes ...*yaml.Node) int {
	m := 0
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if n.Line > m {
			m = n.Line
		}
		if l := maxLine(n.Content...); l > m {
			m = l
		}
	}
	return m
}

// bracketDelta counts flow brackets opened minus closed on a line, outside
// quotes and comments.
func bracketDelta(line string) int {
	d := 0
	var quote byte
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if quote != 0 {
			if ch == '\\' {
				i++
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '#':
			if i == 0 || line[i-1] == ' ' || line[i-1] == '\t' {
				return d
			}
		case '{', '[':
			d++
		case '}', ']':
			d--
		}
	}
	return d
}

func indent(s string) int {
	n := 0
	for _, ch := range s {
		if ch == ' ' {
			n++
		} else if ch == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}

// plain renders a scalar unquoted when YAML would read it back unchanged,
// else double-quoted.
func plain(s string) string {
	if s == "" || strings.ContainsAny(s, ",[]{}#\"'\n\t") || strings.Contains(s, ": ") || strings.HasPrefix(s, "- ") ||
		strings.HasPrefix(s, "@") || strings.HasPrefix(s, "!") || strings.HasPrefix(s, "&") || strings.HasPrefix(s, "*") || strings.HasPrefix(s, "%") ||
		strings.TrimSpace(s) != s || strings.HasSuffix(s, ":") {
		return strconv.Quote(s)
	}
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off", "null", "~":
		return strconv.Quote(s)
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return strconv.Quote(s)
	}
	return s
}

func list(vals []string) string {
	if len(vals) == 1 {
		return plain(vals[0])
	}
	var out []string
	for _, v := range vals {
		out = append(out, plain(v))
	}
	return "[" + strings.Join(out, ", ") + "]"
}

// fields renders a spec as "key: value" pairs in the documented order.
func fields(spec ConceptSpec) []string {
	var out []string
	if spec.Kind != "" {
		out = append(out, "kind: "+spec.Kind)
	}
	if len(spec.IsA) > 0 {
		out = append(out, "is-a: "+list(spec.IsA))
	}
	if len(spec.Realizes) > 0 {
		out = append(out, "realizes: "+list(spec.Realizes))
	}
	if spec.Internal {
		out = append(out, "internal: true")
	}
	if spec.Doc != "" {
		// Always quoted: inside a { } flow mapping an unquoted doc ends at its first comma.
		out = append(out, "doc: "+strconv.Quote(spec.Doc))
	}
	if len(spec.Aliases) > 0 {
		out = append(out, "aliases: "+list(spec.Aliases))
	}
	if spec.Proposed {
		out = append(out, "proposed: true")
		if spec.ProposedBy != "" {
			out = append(out, "proposed-by: "+plain(spec.ProposedBy))
		}
	}
	return out
}

// AddConcept appends a block-style concept entry after the last existing one.
// It fails when the name exists.
func AddConcept(path, name string, spec ConceptSpec) error {
	f, err := open(path)
	if err != nil {
		return err
	}
	if k, _ := f.entry(name); k != nil {
		return fmt.Errorf("%s: concept %s already exists", path, name)
	}
	keyInd, at := 2, len(f.lines)
	ck, cm := f.concepts()
	switch {
	case ck == nil:
		if len(f.lines) > 0 && strings.TrimSpace(f.lines[len(f.lines)-1]) != "" {
			f.lines = append(f.lines, "")
		}
		f.lines = append(f.lines, "concepts:")
		at = len(f.lines)
	case cm.Kind == yaml.MappingNode && len(cm.Content) > 0:
		// After the last entry in file order.
		last := -1
		var lk, lv *yaml.Node
		for i := 0; i+1 < len(cm.Content); i += 2 {
			if cm.Content[i].Line > last {
				last, lk, lv = cm.Content[i].Line, cm.Content[i], cm.Content[i+1]
			}
		}
		keyInd = lk.Column - 1
		_, end := f.extent(lk, lv)
		at = end + 1
	default:
		// concepts: {} or concepts: (null): the key line becomes a block mapping.
		line := ck.Line - 1
		f.lines[line] = f.lines[line][:ck.Column-1] + "concepts:"
		keyInd = ck.Column - 1 + 2
		at = line + 1
	}
	pad := strings.Repeat(" ", keyInd)
	entry := []string{pad + plain(name) + ":"}
	for _, fl := range fields(spec) {
		entry = append(entry, pad+"  "+fl)
	}
	f.lines = append(f.lines[:at], append(entry, f.lines[at:]...)...)
	return f.save()
}

var proposedKey = regexp.MustCompile(`\s*,?\s*proposed(?:-by)?:\s*(?:"[^"]*"|'[^']*'|[^,}\s]*)`)

// SetProposed accepts (false) or re-proposes (true) a concept, editing only
// the entry's own lines.
func SetProposed(path, name string, proposed bool, by string) error {
	f, err := open(path)
	if err != nil {
		return err
	}
	k, v := f.entry(name)
	if k == nil {
		return fmt.Errorf("%s: no concept %s", path, name)
	}
	start, end := f.extent(k, v)
	flow := v != nil && v.Kind == yaml.MappingNode && v.Style&yaml.FlowStyle != 0
	switch {
	case !proposed && flow:
		for i := start; i <= end; i++ {
			f.lines[i] = proposedKey.ReplaceAllString(f.lines[i], "")
			f.lines[i] = strings.Replace(f.lines[i], "{,", "{", 1)
			f.lines[i] = strings.Replace(f.lines[i], "{ ,", "{", 1)
		}
	case !proposed:
		var kept []string
		for i := start; i <= end; i++ {
			t := strings.TrimSpace(f.lines[i])
			if i > start && (strings.HasPrefix(t, "proposed:") || strings.HasPrefix(t, "proposed-by:")) {
				continue
			}
			kept = append(kept, f.lines[i])
		}
		f.lines = append(f.lines[:start], append(kept, f.lines[end+1:]...)...)
	case proposed && flow:
		add := ", proposed: true"
		if by != "" {
			add += ", proposed-by: " + plain(by)
		}
		for i := end; i >= start; i-- {
			if j := strings.LastIndex(f.lines[i], "}"); j >= 0 {
				f.lines[i] = strings.TrimRight(f.lines[i][:j], " ") + add + " " + f.lines[i][j:]
				break
			}
		}
	default:
		pad := strings.Repeat(" ", k.Column-1+2)
		if v != nil && v.Kind == yaml.MappingNode && len(v.Content) > 0 {
			pad = strings.Repeat(" ", v.Content[0].Column-1)
		} else if v == nil || v.Kind != yaml.MappingNode {
			// A bare "Name:" with no fields: turn it into a block mapping.
			f.lines[start] = f.lines[start][:k.Column-1] + plain(name) + ":"
			end = start
		}
		add := []string{pad + "proposed: true"}
		if by != "" {
			add = append(add, pad+"proposed-by: "+plain(by))
		}
		f.lines = append(f.lines[:end+1], append(add, f.lines[end+1:]...)...)
	}
	return f.save()
}

// RemoveConcept deletes an entry's lines.
func RemoveConcept(path, name string) error {
	f, err := open(path)
	if err != nil {
		return err
	}
	k, v := f.entry(name)
	if k == nil {
		return fmt.Errorf("%s: no concept %s", path, name)
	}
	start, end := f.extent(k, v)
	f.lines = append(f.lines[:start], f.lines[end+1:]...)
	return f.save()
}

// IsProposed reports whether the entry carries proposed: true.
func IsProposed(path, name string) (bool, error) {
	f, err := open(path)
	if err != nil {
		return false, err
	}
	k, v := f.entry(name)
	if k == nil {
		return false, fmt.Errorf("%s: no concept %s", path, name)
	}
	_, p := mapGet(v, "proposed")
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

// Get returns the scalar value of key on a concept ("" when absent), and
// whether the concept exists.
func Get(path, name, key string) (string, bool, error) {
	f, err := open(path)
	if err != nil {
		return "", false, err
	}
	k, v := f.entry(name)
	if k == nil {
		return "", false, nil
	}
	_, val := mapGet(v, key)
	if val == nil {
		return "", true, nil
	}
	if val.Kind == yaml.SequenceNode {
		var parts []string
		for _, c := range val.Content {
			parts = append(parts, c.Value)
		}
		return strings.Join(parts, ", "), true, nil
	}
	return val.Value, true, nil
}

// SetField sets key to an already-rendered YAML value (use Quote or List) on
// an existing concept, editing only that key's line: replaced in place when
// present, added as the entry's last field otherwise. Flow and block entries
// keep their style, alignment and comments.
func SetField(path, name, key, rendered string) error {
	f, err := open(path)
	if err != nil {
		return err
	}
	k, v := f.entry(name)
	if k == nil {
		return fmt.Errorf("%s: no concept %s", path, name)
	}
	start, end := f.extent(k, v)
	if v == nil || v.Kind != yaml.MappingNode {
		f.lines[start] = f.lines[start][:k.Column-1] + plain(name) + ":"
		f.lines = append(f.lines[:start+1], append([]string{strings.Repeat(" ", k.Column+1) + key + ": " + rendered}, f.lines[start+1:]...)...)
		return f.save()
	}
	kn, vn := mapGet(v, key)
	flow := v.Style&yaml.FlowStyle != 0
	switch {
	case kn != nil && vn != nil && vn.Line == kn.Line:
		// Replace the value on its line: from after "key:" to the next field
		// separator (flow) or the comment / end of line (block).
		i := kn.Line - 1
		line := f.lines[i]
		col := kn.Column - 1 + len(key)
		rest := line[col:]
		if !strings.HasPrefix(rest, ":") {
			return fmt.Errorf("%s:%d: cannot locate %s", path, kn.Line, key)
		}
		valStart := col + 1
		for valStart < len(line) && line[valStart] == ' ' {
			valStart++
		}
		valEnd := scalarEnd(line, valStart, flow)
		f.lines[i] = line[:valStart] + rendered + line[valEnd:]
	case kn != nil:
		return fmt.Errorf("%s:%d: %s spans several lines; edit it by hand", path, kn.Line, key)
	case flow:
		for i := end; i >= start; i-- {
			if j := strings.LastIndex(f.lines[i], "}"); j >= 0 {
				f.lines[i] = strings.TrimRight(f.lines[i][:j], " ") + ", " + key + ": " + rendered + " " + f.lines[i][j:]
				break
			}
		}
	default:
		pad := strings.Repeat(" ", v.Content[0].Column-1)
		f.lines = append(f.lines[:end+1], append([]string{pad + key + ": " + rendered}, f.lines[end+1:]...)...)
	}
	return f.save()
}

// scalarEnd finds where a YAML value that starts at i ends on its line.
func scalarEnd(line string, i int, flow bool) int {
	if i >= len(line) {
		return i
	}
	switch line[i] {
	case '"', '\'':
		q := line[i]
		for j := i + 1; j < len(line); j++ {
			if line[j] == '\\' && q == '"' {
				j++
				continue
			}
			if line[j] == q {
				return j + 1
			}
		}
		return len(line)
	case '[', '{':
		depth := 0
		for j := i; j < len(line); j++ {
			switch line[j] {
			case '[', '{':
				depth++
			case ']', '}':
				depth--
				if depth == 0 {
					return j + 1
				}
			}
		}
		return len(line)
	}
	for j := i; j < len(line); j++ {
		if flow && (line[j] == ',' || line[j] == '}') {
			return strings.LastIndexFunc(line[:j], func(r rune) bool { return r != ' ' }) + 1
		}
		if line[j] == '#' && j > 0 && line[j-1] == ' ' {
			return strings.LastIndexFunc(line[:j], func(r rune) bool { return r != ' ' }) + 1
		}
	}
	return len(strings.TrimRight(line, " "))
}

// Quote renders a string as a double-quoted YAML scalar.
func Quote(s string) string { return strconv.Quote(s) }

// List renders a flow sequence of plain-or-quoted scalars.
func List(vals []string) string {
	var out []string
	for _, v := range vals {
		out = append(out, plain(v))
	}
	return "[" + strings.Join(out, ", ") + "]"
}
