// @gloss implements gmi:VocabularyImporter scope=file

// Package importers reads vocabularies a repository already keeps (a
// markdown glossary, an enums file) into ontology entries, so a team that
// already cares about its vocabulary does not type it twice.
package importers

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/ontoedit"
)

// Entry is one concept read from a source.
type Entry struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Doc        string   `json:"doc,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	IsA        []string `json:"is_a,omitempty"`
	Proposed   bool     `json:"proposed,omitempty"`
	Deprecated bool     `json:"deprecated,omitempty"`
	// ReplacedBy is read from a deprecated status line ("use Patron",
	// "replaced by Patron").
	ReplacedBy string `json:"replaced_by,omitempty"`
	// KindFrom says where Kind came from: "source" (a Kind: line or an
	// enum), "definition" (the doc reads like a process or a constraint) or
	// "name" (the name's ending).
	KindFrom string `json:"kind_from,omitempty"`
	Line     int    `json:"line"`
	// Enum is set on an enum value named by its bare value: when the
	// ontology already holds a concept of that name that is not is-a the
	// enum (a glossary term, say), the value is written as <Enum><Value>
	// instead of overwriting it.
	Enum string `json:"enum,omitempty"`
}

var (
	headingRe = regexp.MustCompile(`^(#{2,4})\s+(.+?)\s*#*\s*$`)
	// A field line: "**Def.** text", "- **Aliases:** a, b", "Status: planned", "| Kind | process |".
	fieldRe = regexp.MustCompile(`^\s*[-*]?\s*\**\s*(Def(?:inition)?|Aliases?|Also known as|AKA|Status|Kind|Type|Is[- ]a)\s*\.?\s*:?\s*\**\s*:?\s*(.*)$`)
)

// ParseGlossary reads a markdown glossary: each ##-#### heading is a concept;
// "Def." (or "Definition") lines give the doc, "Aliases" the aliases,
// "Status" whether it is implemented, "Kind" its kind. A heading's first
// plain paragraph is the doc when no Def. line exists. A heading with no
// field line that is followed by a deeper heading is a section ("## Retired
// terms" above "### Borrower"), not a concept. Without a Kind line the kind
// is guessed from the definition, then from the name.
func ParseGlossary(text string) []Entry {
	var out []Entry
	var cur *Entry
	var para []string
	level, fields := 0, 0
	flush := func(next int) {
		if cur == nil {
			return
		}
		if fields == 0 && next > level {
			cur, para = nil, nil // a section heading
			return
		}
		if cur.Doc == "" && len(para) > 0 {
			cur.Doc = firstSentence(strings.Join(para, " "))
		}
		if cur.Kind == "" {
			if k := kindFromDoc(cur.Doc); k != "" {
				cur.Kind, cur.KindFrom = k, "definition"
			} else {
				cur.Kind, cur.KindFrom = guessKind(cur.Name), "name"
			}
		}
		out = append(out, *cur)
		cur, para = nil, nil
	}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	n := 0
	inFence := false
	for sc.Scan() {
		n++
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := headingRe.FindStringSubmatch(line); m != nil {
			flush(len(m[1]))
			name := Pascal(stripMarkup(m[2]))
			if name == "" {
				continue
			}
			cur = &Entry{Name: name, Line: n}
			level, fields = len(m[1]), 0
			continue
		}
		if cur == nil {
			continue
		}
		if m := fieldRe.FindStringSubmatch(line); m != nil && strings.TrimSpace(m[2]) != "" {
			val := strings.TrimSpace(stripMarkup(m[2]))
			fields++
			switch strings.ToLower(strings.TrimSuffix(m[1], ".")) {
			case "def", "definition":
				cur.Doc = val
			case "alias", "aliases", "also known as", "aka":
				for _, a := range strings.Split(val, ",") {
					if a = strings.Trim(strings.TrimSpace(a), "`\"'"); a != "" {
						cur.Aliases = append(cur.Aliases, a)
					}
				}
			case "status":
				switch s := strings.ToLower(val); {
				case strings.Contains(s, "deprecat") || strings.Contains(s, "retired") || strings.Contains(s, "removed"):
					cur.Deprecated = true
					if m := replacedRe.FindStringSubmatch(val); m != nil {
						cur.ReplacedBy = Pascal(m[1])
					}
				case strings.Contains(s, "implemented") && !strings.Contains(s, "not implemented") && !strings.Contains(s, "unimplemented"),
					strings.Contains(s, "live"), strings.Contains(s, "shipped"), strings.Contains(s, "done"), strings.Contains(s, "stable"):
				default:
					cur.Proposed = true
				}
			case "kind", "type":
				if k := strings.ToLower(strings.Trim(val, "`")); validKind(k) {
					cur.Kind, cur.KindFrom = k, "source"
				}
			case "is a", "is-a":
				cur.IsA = append(cur.IsA, Pascal(val))
			}
			continue
		}
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "|") && !strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "*") && !strings.HasPrefix(t, ">") {
			para = append(para, stripMarkup(t))
		} else if t == "" && len(para) > 0 && cur.Doc == "" {
			cur.Doc = firstSentence(strings.Join(para, " "))
		}
	}
	flush(0)
	return out
}

// replacedRe finds the successor in a deprecated status line: "use Patron",
// "replaced by Patron", "superseded by Patron", "see Patron".
var replacedRe = regexp.MustCompile(`(?i)\b(?:use|replaced by|superseded by|see|now)\s+(?:the\s+)?([A-Za-z][\w-]*(?:\s+[A-Z][\w-]*)*)`)

// constraintWords in a definition mark a rule rather than a thing.
var constraintWords = regexp.MustCompile(`(?i)\b(never|cannot|can't|must not|may not|at most|at least|no more than|no longer than|how long|how many|maximum|minimum|exceeds?)\b`)

// kindFromDoc reads a kind from a definition: one that starts with a gerund
// ("Lending an item ...", "The process of ...") is a process; one that
// states a bound or a prohibition ("never", "at most", "how long") is a
// constraint. It returns "" when the definition says neither.
func kindFromDoc(doc string) string {
	d := strings.TrimSpace(doc)
	if d == "" {
		return ""
	}
	low := strings.ToLower(d)
	for _, p := range []string{"the process of ", "the act of ", "a process ", "the step ", "a step "} {
		if strings.HasPrefix(low, p) {
			return "process"
		}
	}
	first := strings.Fields(low)[0]
	first = strings.Trim(first, ",.;:")
	if len(first) > 5 && strings.HasSuffix(first, "ing") && first != "nothing" && first != "something" && first != "anything" && first != "everything" && first != "string" {
		return "process"
	}
	if constraintWords.MatchString(d) {
		return "constraint"
	}
	return ""
}

// ParseEnums reads an enums file: a mapping (optionally under enums:) of
// enum name to either a list of values, a mapping value → description, or
// { description|doc, values }. Each enum becomes an entity; with values, each
// value becomes a concept that is-a the enum, named <Value> (or
// <Enum><Value> when the bare name would collide, here or with an unrelated
// concept already in the ontology; see Entry.Enum).
func ParseEnums(b []byte, withValues bool) ([]Entry, error) {
	return ParseEnumsNamed(b, withValues, false)
}

// ParseEnumsNamed is ParseEnums with a choice of value names: prefixed
// names every value <Enum><Value> (LoanStatusActive), so a bare value never
// lands in the namespace without its enum.
func ParseEnumsNamed(b []byte, withValues, prefixed bool) ([]Entry, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping of enum name to values")
	}
	m := root.Content[0]
	if v := get(m, "enums"); v != nil && v.Kind == yaml.MappingNode {
		m = v
	}
	var out []Entry
	names := map[string]bool{}
	type val struct {
		name, doc string
		line      int
	}
	vals := map[string][]val{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		e := Entry{Name: Pascal(k.Value), Kind: "entity", KindFrom: "source", Line: k.Line}
		if e.Name == "" {
			continue
		}
		var list *yaml.Node
		switch v.Kind {
		case yaml.SequenceNode:
			list = v
		case yaml.MappingNode:
			if d := firstOf(v, "description", "doc", "desc"); d != nil {
				e.Doc = strings.TrimSpace(d.Value)
			}
			list = firstOf(v, "values", "members", "options")
			if list == nil && e.Doc == "" {
				list = v // value → description map
			}
		}
		if list != nil {
			switch list.Kind {
			case yaml.SequenceNode:
				for _, it := range list.Content {
					switch it.Kind {
					case yaml.ScalarNode:
						vals[e.Name] = append(vals[e.Name], val{name: it.Value, line: it.Line})
					case yaml.MappingNode:
						nm := firstOf(it, "name", "value", "id", "key")
						if nm == nil {
							continue
						}
						d := firstOf(it, "description", "doc", "desc", "label")
						vv := val{name: nm.Value, line: nm.Line}
						if d != nil {
							vv.doc = d.Value
						}
						vals[e.Name] = append(vals[e.Name], vv)
					}
				}
			case yaml.MappingNode:
				for j := 0; j+1 < len(list.Content); j += 2 {
					vv := val{name: list.Content[j].Value, line: list.Content[j].Line}
					if list.Content[j+1].Kind == yaml.ScalarNode {
						vv.doc = list.Content[j+1].Value
					} else if d := firstOf(list.Content[j+1], "description", "doc", "desc", "label"); d != nil {
						vv.doc = d.Value
					}
					vals[e.Name] = append(vals[e.Name], vv)
				}
			}
		}
		if e.Doc == "" {
			var vs []string
			for _, x := range vals[e.Name] {
				vs = append(vs, x.name)
			}
			if len(vs) > 0 {
				e.Doc = "One of: " + strings.Join(vs, ", ") + "."
			}
		}
		names[e.Name] = true
		out = append(out, e)
	}
	if withValues {
		enums := make([]string, 0, len(vals))
		for e := range vals {
			enums = append(enums, e)
		}
		sort.Strings(enums)
		for _, en := range enums {
			for _, v := range vals[en] {
				name := Pascal(v.name)
				if name == "" {
					continue
				}
				e := Entry{Name: name, IsA: []string{en}, Doc: strings.TrimSpace(v.doc), Line: v.line, Enum: en, KindFrom: "source"}
				if names[name] || prefixed {
					e.Name, e.Enum = en+name, ""
				}
				names[e.Name] = true
				out = append(out, e)
			}
		}
	}
	return out, nil
}

func get(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func firstOf(m *yaml.Node, keys ...string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for _, k := range keys {
		if v := get(m, k); v != nil {
			return v
		}
	}
	return nil
}

// Result reports what an import did.
type Result struct {
	Added, Updated, Unchanged []string
	Skipped                   []string // with the reason
	// Kinds maps each added name to its kind and where it came from
	// ("process, from the definition").
	Kinds map[string]string
	// Kept lists names whose doc or aliases were edited by hand since the
	// last import while the source stayed the same: the edit is kept.
	Kept []string
}

// Apply writes entries into an ontology file without an import record.
func Apply(path string, entries []Entry, by string, dry bool) (*Result, error) {
	return ApplyRecorded(path, "", entries, by, dry, nil)
}

// ApplyRecorded writes entries into the ontology file of namespace ns: new
// concepts are appended in block style; for existing ones only doc and
// aliases are updated, so a re-run is a no-op and hand edits elsewhere in
// the entry survive. proposed:, deprecated: and replaced-by: are set on new
// entries only.
//
// With a record (what earlier imports wrote), a doc or alias list is
// updated only when the source changed since the last import, so a doc
// improved by hand stays; and a name the record has but the ontology does
// not (deleted or renamed by hand) is not added again. The record is
// updated in memory; the caller saves it.
func ApplyRecorded(path, ns string, entries []Entry, by string, dry bool, rec *Record) (*Result, error) {
	r := &Result{Kinds: map[string]string{}}
	for _, e := range entries {
		if !validName(e.Name) {
			r.Skipped = append(r.Skipped, fmt.Sprintf("%s (line %d): not a usable concept name", e.Name, e.Line))
			continue
		}
		_, ok, err := ontoedit.Get(path, e.Name, "kind")
		if err != nil {
			return r, err
		}
		if ok && e.Enum != "" {
			// The bare value name is taken. Unless the existing concept is
			// this enum's value from an earlier run, it belongs to someone
			// else: use <Enum><Value>, as ParseEnums does for collisions
			// inside one file.
			isa, _, _ := ontoedit.Get(path, e.Name, "is-a")
			if !hasParent(isa, e.Enum) {
				e.Name = e.Enum + e.Name
				if _, ok, err = ontoedit.Get(path, e.Name, "kind"); err != nil {
					return r, err
				}
			}
		}
		last := rec.get(ns, e.Name)
		if !ok {
			if last != nil {
				r.Skipped = append(r.Skipped, fmt.Sprintf("%s: removed from the ontology after an earlier import; not added again (drop it from %s to re-import it)", e.Name, RecordName))
				continue
			}
			r.Added = append(r.Added, e.Name)
			if e.KindFrom != "source" && len(e.IsA) == 0 {
				r.Kinds[e.Name] = e.Kind + ", guessed from the " + e.KindFrom
			} else if e.Kind != "" {
				r.Kinds[e.Name] = e.Kind
			}
			rec.put(ns, e)
			if dry {
				continue
			}
			spec := ontoedit.ConceptSpec{Kind: e.Kind, Doc: e.Doc, IsA: e.IsA, Aliases: e.Aliases, Proposed: e.Proposed, ProposedBy: by}
			if len(e.IsA) > 0 && e.Kind == "" {
				spec.Kind = ""
			}
			if !e.Proposed {
				spec.ProposedBy = ""
			}
			if err := ontoedit.AddConcept(path, e.Name, spec); err != nil {
				return r, err
			}
			if e.Deprecated {
				if err := ontoedit.SetField(path, e.Name, "deprecated", "true"); err != nil {
					return r, err
				}
				if e.ReplacedBy != "" {
					if err := ontoedit.SetField(path, e.Name, "replaced-by", e.ReplacedBy); err != nil {
						return r, err
					}
				}
			}
			continue
		}
		changed, kept := false, false
		if e.Doc != "" {
			old, _, _ := ontoedit.Get(path, e.Name, "doc")
			switch {
			case old == e.Doc:
			case last != nil && last.Doc == e.Doc:
				kept = true // edited by hand; the source has not changed since
			default:
				changed = true
				if !dry {
					if err := ontoedit.SetField(path, e.Name, "doc", ontoedit.Quote(e.Doc)); err != nil {
						r.Skipped = append(r.Skipped, fmt.Sprintf("%s: %v", e.Name, err))
						continue
					}
				}
			}
		}
		if len(e.Aliases) > 0 {
			old, _, _ := ontoedit.Get(path, e.Name, "aliases")
			switch {
			case old == strings.Join(e.Aliases, ", "):
			case last != nil && strings.Join(last.Aliases, ", ") == strings.Join(e.Aliases, ", "):
				kept = true
			default:
				changed = true
				if !dry {
					if err := ontoedit.SetField(path, e.Name, "aliases", ontoedit.List(e.Aliases)); err != nil {
						r.Skipped = append(r.Skipped, fmt.Sprintf("%s: %v", e.Name, err))
						continue
					}
				}
			}
		}
		rec.put(ns, e)
		switch {
		case changed:
			r.Updated = append(r.Updated, e.Name)
		case kept:
			r.Kept = append(r.Kept, e.Name)
			r.Unchanged = append(r.Unchanged, e.Name)
		default:
			r.Unchanged = append(r.Unchanged, e.Name)
		}
	}
	return r, nil
}

// hasParent reports whether a rendered is-a value ("A, ns:B") names parent,
// with or without a namespace prefix.
func hasParent(isa, parent string) bool {
	for _, p := range strings.Split(isa, ",") {
		p = strings.TrimSpace(p)
		if i := strings.LastIndex(p, ":"); i >= 0 {
			p = p[i+1:]
		}
		if p == parent {
			return true
		}
	}
	return false
}

// Pascal turns "item returned", "item_returned", "ITEM-RETURNED" or "itemReturned" into ItemReturned.
func Pascal(s string) string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if unicode.IsUpper(r) && i > 0 && unicode.IsLower(rs[i-1]) {
				flush()
			}
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	var sb strings.Builder
	for _, w := range words {
		lw := strings.ToLower(w)
		if w == strings.ToUpper(w) && len(w) <= 3 && len(words) > 1 {
			sb.WriteString(w) // keep short acronyms: API, UI
			continue
		}
		sb.WriteString(strings.ToUpper(lw[:1]) + lw[1:])
	}
	out := sb.String()
	if out != "" && unicode.IsDigit(rune(out[0])) {
		out = "N" + out
	}
	return out
}

func stripMarkup(s string) string {
	s = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`).ReplaceAllString(s, "$1")
	return strings.TrimSpace(strings.NewReplacer("**", "", "__", "", "`", "").Replace(s))
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ". "); i > 0 && i < 300 {
		return s[:i+1]
	}
	if len(s) > 300 {
		return s[:297] + "..."
	}
	return s
}

func guessKind(name string) string {
	switch {
	case strings.HasSuffix(name, "ing"), strings.HasSuffix(name, "tion"), strings.HasSuffix(name, "Process"), strings.HasSuffix(name, "Pipeline"):
		return "process"
	case strings.HasSuffix(name, "ed") && len(name) > 4, strings.HasSuffix(name, "Event"):
		return "event"
	case strings.HasSuffix(name, "Rule"), strings.HasSuffix(name, "Policy"), strings.HasSuffix(name, "Limit"), strings.HasSuffix(name, "Invariant"):
		return "constraint"
	case strings.HasSuffix(name, "Rate"), strings.HasSuffix(name, "Count"), strings.HasSuffix(name, "Score"), strings.HasSuffix(name, "Ratio"):
		return "measure"
	}
	return "entity"
}

func validKind(k string) bool {
	for _, x := range model.ConceptKinds {
		if x == k {
			return true
		}
	}
	return false
}

func validName(n string) bool {
	return n != "" && unicode.IsUpper(rune(n[0])) && !strings.ContainsAny(n, " :-_")
}
