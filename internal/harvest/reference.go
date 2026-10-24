// @gloss implements gmi:EvidenceHarvester scope=file

package harvest

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// A reference vocabulary is one the owner supplies (FIBO, schema.org, a
// partner's ontology): it breaks naming ties and proposes structure between
// concepts that have evidence of their own, and never proposes a concept
// (ADR 0025, section 6.3). Nothing is bundled or fetched; gloss harvest
// --reference reads a file in Turtle (or N-Triples), RDF/XML or JSON-LD,
// with the standard library only.

const (
	rdfNS  = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	rdfsNS = "http://www.w3.org/2000/01/rdf-schema#"
	owlNS  = "http://www.w3.org/2002/07/owl#"
	skosNS = "http://www.w3.org/2004/02/skos/core#"
)

// triple is one RDF statement; o is an IRI unless lit is set.
type triple struct {
	s, p, o string
	lit     bool
	lang    string
}

// RefTerm is a class or concept of a reference vocabulary.
type RefTerm struct {
	IRI     string
	Label   string
	Parents []string // is-a (rdfs:subClassOf, skos:broader), as IRIs
	Wholes  []string // part-of, as IRIs
}

// ReadReference parses a reference vocabulary and returns its terms,
// sorted by IRI. The format is chosen by extension: .ttl, .nt, .n3 Turtle;
// .rdf, .owl, .xml RDF/XML; .jsonld, .json JSON-LD.
func ReadReference(path string) ([]RefTerm, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ts []triple
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttl", ".nt", ".n3", ".turtle":
		ts, err = parseTurtle(string(b))
	case ".rdf", ".owl", ".xml":
		ts, err = parseRDFXML(b)
	case ".jsonld", ".json":
		ts, err = parseJSONLD(b)
	default:
		return nil, fmt.Errorf("%s: unknown reference format; use .ttl, .nt, .rdf, .owl or .jsonld", path)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return refTerms(ts), nil
}

// refTerms collects the classes and concepts the triples describe.
func refTerms(ts []triple) []RefTerm {
	by := map[string]*RefTerm{}
	labels := map[string]map[string]string{} // iri → predicate → label (English or untagged first)
	preferred := map[string]bool{}
	isTerm := map[string]bool{}
	get := func(iri string) *RefTerm {
		if by[iri] == nil {
			by[iri] = &RefTerm{IRI: iri}
		}
		return by[iri]
	}
	for _, t := range ts {
		if strings.HasPrefix(t.s, "_:") {
			continue
		}
		switch {
		case t.p == rdfNS+"type" && (t.o == owlNS+"Class" || t.o == rdfsNS+"Class" || t.o == skosNS+"Concept"):
			isTerm[t.s] = true
		case t.lit && (t.p == skosNS+"prefLabel" || t.p == rdfsNS+"label" || t.p == "http://schema.org/name" || t.p == "https://schema.org/name"):
			if labels[t.s] == nil {
				labels[t.s] = map[string]string{}
			}
			// The first label wins, unless a later one is English or
			// untagged and the first was neither.
			english := t.lang == "" || strings.HasPrefix(strings.ToLower(t.lang), "en")
			if _, ok := labels[t.s][t.p]; !ok || english && !preferred[t.s+"\x00"+t.p] {
				labels[t.s][t.p] = t.o
				preferred[t.s+"\x00"+t.p] = english
			}
		case !t.lit && (t.p == rdfsNS+"subClassOf" || t.p == skosNS+"broader"):
			if !strings.HasPrefix(t.o, "_:") {
				isTerm[t.s] = true
				get(t.s).Parents = append(get(t.s).Parents, t.o)
			}
		case !t.lit && partOf(t.p):
			if !strings.HasPrefix(t.o, "_:") {
				get(t.s).Wholes = append(get(t.s).Wholes, t.o)
			}
		}
	}
	var out []RefTerm
	for iri := range isTerm {
		t := get(iri)
		for _, p := range []string{skosNS + "prefLabel", rdfsNS + "label", "https://schema.org/name", "http://schema.org/name"} {
			if l := labels[iri][p]; l != "" {
				t.Label = l
				break
			}
		}
		if t.Label == "" {
			t.Label = localName(iri)
		}
		if t.Label == "" {
			continue
		}
		sort.Strings(t.Parents)
		sort.Strings(t.Wholes)
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IRI < out[j].IRI })
	return out
}

// partOf reports whether a predicate says part-of: BFO's part of,
// dcterms:isPartOf, schema:isPartOf or any predicate named part-of.
func partOf(p string) bool {
	if p == "http://purl.obolibrary.org/obo/BFO_0000050" {
		return true
	}
	l := strings.ToLower(localName(p))
	return l == "ispartof" || l == "partof" || l == "part_of" || l == "part-of"
}

// localName is the last segment of an IRI, after # or /.
func localName(iri string) string {
	if i := strings.LastIndexAny(iri, "#/"); i >= 0 {
		return iri[i+1:]
	}
	return iri
}

// referenceEvidence turns a vocabulary's terms into reference-term
// evidence: the label is the surface, the IRI the locator, and the parents
// and wholes become is-a: and part-of: context by label.
func referenceEvidence(uri string, terms []RefTerm) []Evidence {
	label := map[string]string{}
	for _, t := range terms {
		label[t.IRI] = t.Label
	}
	name := func(iri string) string {
		if l := label[iri]; l != "" {
			return l
		}
		return localName(iri)
	}
	var out []Evidence
	for _, t := range terms {
		ctx := []string{"iri:" + t.IRI}
		for _, p := range t.Parents {
			ctx = append(ctx, "is-a:"+name(p))
		}
		for _, w := range t.Wholes {
			ctx = append(ctx, "part-of:"+name(w))
		}
		e := Evidence{Signal: "reference-term", Surface: t.Label, Source: Source{Family: FamilyReference, URI: uri, Locator: t.IRI}, Context: ctx}
		e.ID = idFor(e)
		out = append(out, e)
	}
	return out
}

// Reference reads each file and returns its terms as evidence.
func Reference(paths []string) ([]Evidence, error) {
	var out []Evidence
	for _, p := range paths {
		terms, err := ReadReference(p)
		if err != nil {
			return nil, err
		}
		out = append(out, referenceEvidence(filepath.ToSlash(p), terms)...)
	}
	return out, nil
}

// --- Turtle and N-Triples ---------------------------------------------------

type ttl struct {
	src      []rune
	i        int
	prefixes map[string]string
	base     string
	out      []triple
	blank    int
}

func parseTurtle(src string) ([]triple, error) {
	p := &ttl{src: []rune(src), prefixes: map[string]string{}}
	for {
		p.ws()
		if p.i >= len(p.src) {
			return p.out, nil
		}
		if err := p.statement(); err != nil {
			line := 1 + strings.Count(string(p.src[:min(p.i, len(p.src))]), "\n")
			return nil, fmt.Errorf("turtle line %d: %w", line, err)
		}
	}
}

func (p *ttl) ws() {
	for p.i < len(p.src) {
		r := p.src[p.i]
		if r == '#' {
			for p.i < len(p.src) && p.src[p.i] != '\n' {
				p.i++
			}
			continue
		}
		if !unicode.IsSpace(r) {
			return
		}
		p.i++
	}
}

func (p *ttl) peek() rune {
	if p.i < len(p.src) {
		return p.src[p.i]
	}
	return 0
}

func (p *ttl) keyword(k string) bool {
	n := len([]rune(k))
	if p.i+n <= len(p.src) && strings.EqualFold(string(p.src[p.i:p.i+n]), k) && (p.i+n == len(p.src) || unicode.IsSpace(p.src[p.i+n])) {
		p.i += n
		return true
	}
	return false
}

func (p *ttl) statement() error {
	switch {
	case p.keyword("@prefix"), p.keyword("PREFIX"):
		p.ws()
		start := p.i
		for p.i < len(p.src) && p.src[p.i] != ':' {
			p.i++
		}
		name := strings.TrimSpace(string(p.src[start:p.i]))
		p.i++
		p.ws()
		iri, err := p.iriref()
		if err != nil {
			return err
		}
		p.prefixes[name] = iri
		p.ws()
		if p.peek() == '.' {
			p.i++
		}
		return nil
	case p.keyword("@base"), p.keyword("BASE"):
		p.ws()
		iri, err := p.iriref()
		if err != nil {
			return err
		}
		p.base = iri
		p.ws()
		if p.peek() == '.' {
			p.i++
		}
		return nil
	}
	s, err := p.subject()
	if err != nil {
		return err
	}
	p.ws()
	if p.peek() == '.' && strings.HasPrefix(s, "_:") {
		p.i++
		return nil // a bare [ ... ] .
	}
	if err := p.predicateObjects(s, '.'); err != nil {
		return err
	}
	p.ws()
	if p.peek() != '.' {
		return fmt.Errorf("expected '.'")
	}
	p.i++
	return nil
}

func (p *ttl) subject() (string, error) {
	p.ws()
	switch p.peek() {
	case '[':
		return p.blankList()
	case '(':
		return p.collection()
	}
	return p.iri()
}

func (p *ttl) predicateObjects(s string, end rune) error {
	for {
		p.ws()
		if p.peek() == end || p.peek() == ']' || p.peek() == '.' {
			return nil
		}
		var pred string
		if p.peek() == 'a' && p.i+1 < len(p.src) && (unicode.IsSpace(p.src[p.i+1]) || p.src[p.i+1] == '<' || p.src[p.i+1] == '[') {
			p.i++
			pred = rdfNS + "type"
		} else {
			var err error
			if pred, err = p.iri(); err != nil {
				return err
			}
		}
		for {
			p.ws()
			if err := p.object(s, pred); err != nil {
				return err
			}
			p.ws()
			if p.peek() != ',' {
				break
			}
			p.i++
		}
		p.ws()
		if p.peek() != ';' {
			return nil
		}
		for p.peek() == ';' {
			p.i++
			p.ws()
		}
	}
}

func (p *ttl) object(s, pred string) error {
	switch r := p.peek(); {
	case r == '"' || r == '\'':
		v, lang, err := p.literal()
		if err != nil {
			return err
		}
		p.out = append(p.out, triple{s: s, p: pred, o: v, lit: true, lang: lang})
	case r == '[':
		b, err := p.blankList()
		if err != nil {
			return err
		}
		p.out = append(p.out, triple{s: s, p: pred, o: b})
	case r == '(':
		b, err := p.collection()
		if err != nil {
			return err
		}
		p.out = append(p.out, triple{s: s, p: pred, o: b})
	case r == '-' || r == '+' || unicode.IsDigit(r):
		start := p.i
		for p.i < len(p.src) && strings.ContainsRune("+-.eE0123456789", p.src[p.i]) {
			p.i++
		}
		p.out = append(p.out, triple{s: s, p: pred, o: string(p.src[start:p.i]), lit: true})
	case p.keyword("true"), p.keyword("false"):
		p.out = append(p.out, triple{s: s, p: pred, o: "bool", lit: true})
	default:
		o, err := p.iri()
		if err != nil {
			return err
		}
		p.out = append(p.out, triple{s: s, p: pred, o: o})
	}
	return nil
}

func (p *ttl) newBlank() string {
	p.blank++
	return fmt.Sprintf("_:b%d", p.blank)
}

func (p *ttl) blankList() (string, error) {
	p.i++ // [
	b := p.newBlank()
	if err := p.predicateObjects(b, ']'); err != nil {
		return "", err
	}
	p.ws()
	if p.peek() != ']' {
		return "", fmt.Errorf("expected ']'")
	}
	p.i++
	return b, nil
}

// collection reads ( ... ) and keeps only that it was one: no term the
// harvest uses is a list.
func (p *ttl) collection() (string, error) {
	p.i++ // (
	b := p.newBlank()
	for {
		p.ws()
		if p.peek() == ')' {
			p.i++
			return b, nil
		}
		if p.i >= len(p.src) {
			return "", fmt.Errorf("unterminated collection")
		}
		if err := p.object(b, rdfNS+"first"); err != nil {
			return "", err
		}
	}
}

func (p *ttl) iriref() (string, error) {
	if p.peek() != '<' {
		return "", fmt.Errorf("expected <iri>")
	}
	start := p.i + 1
	for p.i < len(p.src) && p.src[p.i] != '>' {
		p.i++
	}
	if p.i >= len(p.src) {
		return "", fmt.Errorf("unterminated <iri>")
	}
	iri := string(p.src[start:p.i])
	p.i++
	if p.base != "" && !strings.Contains(iri, ":") {
		iri = p.base + iri
	}
	return iri, nil
}

func (p *ttl) iri() (string, error) {
	if p.peek() == '<' {
		return p.iriref()
	}
	start := p.i
	for p.i < len(p.src) {
		r := p.src[p.i]
		if unicode.IsSpace(r) || strings.ContainsRune(";,[]()<\"'", r) || r == '.' && (p.i+1 >= len(p.src) || unicode.IsSpace(p.src[p.i+1]) || p.src[p.i+1] == '#') {
			break
		}
		p.i++
	}
	name := string(p.src[start:p.i])
	if name == "" {
		return "", fmt.Errorf("expected an IRI or prefixed name near %q", string(p.src[start:min(start+20, len(p.src))]))
	}
	if strings.HasPrefix(name, "_:") {
		return name, nil
	}
	pre, local, ok := strings.Cut(name, ":")
	if !ok {
		return "", fmt.Errorf("%q is not a prefixed name", name)
	}
	ns, ok := p.prefixes[pre]
	if !ok {
		return "", fmt.Errorf("undeclared prefix %q", pre)
	}
	return ns + strings.ReplaceAll(local, `\`, ""), nil
}

func (p *ttl) literal() (string, string, error) {
	q := p.src[p.i]
	long := p.i+2 < len(p.src) && p.src[p.i+1] == q && p.src[p.i+2] == q
	if long {
		p.i += 3
	} else {
		p.i++
	}
	var sb strings.Builder
	for {
		if p.i >= len(p.src) {
			return "", "", fmt.Errorf("unterminated literal")
		}
		r := p.src[p.i]
		if r == '\\' && p.i+1 < len(p.src) {
			switch e := p.src[p.i+1]; e {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			default:
				sb.WriteRune(e)
			}
			p.i += 2
			continue
		}
		if r == q {
			if !long {
				p.i++
				break
			}
			if p.i+2 < len(p.src) && p.src[p.i+1] == q && p.src[p.i+2] == q {
				p.i += 3
				break
			}
		}
		sb.WriteRune(r)
		p.i++
	}
	lang := ""
	if p.peek() == '@' {
		start := p.i + 1
		for p.i++; p.i < len(p.src) && (unicode.IsLetter(p.src[p.i]) || p.src[p.i] == '-'); p.i++ {
		}
		lang = string(p.src[start:p.i])
	} else if p.peek() == '^' && p.i+1 < len(p.src) && p.src[p.i+1] == '^' {
		p.i += 2
		if _, err := p.iri(); err != nil {
			return "", "", err
		}
	}
	return sb.String(), lang, nil
}

// --- RDF/XML ------------------------------------------------------------------

func parseRDFXML(b []byte) ([]triple, error) {
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	var out []triple
	base := ""
	blank := 0
	newBlank := func() string {
		blank++
		return fmt.Sprintf("_:x%d", blank)
	}
	attr := func(se xml.StartElement, space, local string) string {
		for _, a := range se.Attr {
			if a.Name.Space == space && a.Name.Local == local {
				return a.Value
			}
		}
		return ""
	}
	subjectOf := func(se xml.StartElement) string {
		if v := attr(se, rdfNS, "about"); v != "" {
			if !strings.Contains(v, ":") {
				return base + v
			}
			return v
		}
		if v := attr(se, rdfNS, "ID"); v != "" {
			return base + "#" + v
		}
		if v := attr(se, rdfNS, "nodeID"); v != "" {
			return "_:" + v
		}
		return newBlank()
	}
	// node reads a node element whose start tag was just read, and returns
	// its subject.
	var node func(se xml.StartElement) (string, error)
	node = func(se xml.StartElement) (string, error) {
		s := subjectOf(se)
		if !(se.Name.Space == rdfNS && se.Name.Local == "Description") {
			out = append(out, triple{s: s, p: rdfNS + "type", o: se.Name.Space + se.Name.Local})
		}
		for {
			tok, err := dec.Token()
			if err != nil {
				return "", err
			}
			switch t := tok.(type) {
			case xml.EndElement:
				return s, nil
			case xml.StartElement:
				pred := t.Name.Space + t.Name.Local
				if r := attr(t, rdfNS, "resource"); r != "" {
					if !strings.Contains(r, ":") {
						r = base + r
					}
					out = append(out, triple{s: s, p: pred, o: r})
					if err := dec.Skip(); err != nil {
						return "", err
					}
					continue
				}
				lang := attr(t, "http://www.w3.org/XML/1998/namespace", "lang")
				// A property element holds text or one nested node element.
				var text strings.Builder
				for done := false; !done; {
					tok, err := dec.Token()
					if err != nil {
						return "", err
					}
					switch c := tok.(type) {
					case xml.CharData:
						text.Write(c)
					case xml.StartElement:
						o, err := node(c)
						if err != nil {
							return "", err
						}
						out = append(out, triple{s: s, p: pred, o: o})
						text.Reset()
						if err := dec.Skip(); err != nil {
							return "", err
						}
						done = true
					case xml.EndElement:
						if v := strings.TrimSpace(text.String()); v != "" {
							out = append(out, triple{s: s, p: pred, o: v, lit: true, lang: lang})
						}
						done = true
					}
				}
			}
		}
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Space == rdfNS && se.Name.Local == "RDF" {
			base = attr(se, "http://www.w3.org/XML/1998/namespace", "base")
			continue
		}
		if _, err := node(se); err != nil {
			return nil, err
		}
	}
}

// --- JSON-LD ------------------------------------------------------------------

// parseJSONLD reads the common shapes: a node, an array of nodes, or
// {"@context": ..., "@graph": [...]}, with prefixes and simple term
// definitions in the context (no remote contexts).
func parseJSONLD(b []byte) ([]triple, error) {
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	ctx := map[string]string{"rdf": rdfNS, "rdfs": rdfsNS, "owl": owlNS, "skos": skosNS}
	vocab := ""
	readCtx := func(c any) {
		var list []any
		switch v := c.(type) {
		case []any:
			list = v
		default:
			list = []any{v}
		}
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for k, v := range m {
				switch d := v.(type) {
				case string:
					if k == "@vocab" {
						vocab = d
					} else if !strings.HasPrefix(k, "@") {
						ctx[k] = d
					}
				case map[string]any:
					if id, ok := d["@id"].(string); ok {
						ctx[k] = id
					}
				}
			}
		}
	}
	expand := func(term string) string {
		if v, ok := ctx[term]; ok && !strings.Contains(term, ":") {
			return expandCurie(v, ctx)
		}
		return expandCurie(term, ctx)
	}
	expandKey := func(k string) string {
		if !strings.Contains(k, ":") && ctx[k] == "" && vocab != "" {
			return vocab + k
		}
		return expand(k)
	}
	var out []triple
	blank := 0
	var walk func(n map[string]any) string
	walk = func(n map[string]any) string {
		if c, ok := n["@context"]; ok {
			readCtx(c)
		}
		s, _ := n["@id"].(string)
		if s == "" {
			blank++
			s = fmt.Sprintf("_:j%d", blank)
		} else {
			s = expand(s)
		}
		for k, v := range n {
			switch k {
			case "@context", "@id":
				continue
			case "@graph":
				for _, x := range asList(v) {
					if m, ok := x.(map[string]any); ok {
						walk(m)
					}
				}
				continue
			case "@type":
				for _, x := range asList(v) {
					if t, ok := x.(string); ok {
						out = append(out, triple{s: s, p: rdfNS + "type", o: expandKey(t)})
					}
				}
				continue
			}
			pred := expandKey(k)
			for _, x := range asList(v) {
				switch o := x.(type) {
				case string:
					out = append(out, triple{s: s, p: pred, o: o, lit: true})
				case map[string]any:
					if val, ok := o["@value"]; ok {
						lang, _ := o["@language"].(string)
						out = append(out, triple{s: s, p: pred, o: fmt.Sprint(val), lit: true, lang: lang})
					} else if id, ok := o["@id"].(string); ok && len(o) == 1 {
						out = append(out, triple{s: s, p: pred, o: expand(id)})
					} else {
						out = append(out, triple{s: s, p: pred, o: walk(o)})
					}
				}
			}
		}
		return s
	}
	for _, x := range asList(doc) {
		if m, ok := x.(map[string]any); ok {
			walk(m)
		}
	}
	return out, nil
}

func asList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{v}
}

func expandCurie(s string, ctx map[string]string) string {
	if pre, local, ok := strings.Cut(s, ":"); ok && !strings.HasPrefix(local, "//") {
		if ns, ok := ctx[pre]; ok {
			return ns + local
		}
	}
	return s
}
