// @gloss implements gmi:MarkerSuggester scope=file

package suggest

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/mcp"
	"github.com/mckalvan/glossmark/internal/model"
)

// Evidence kinds, strongest first. A proposal names its evidence in matched
// and its calibration basis, so a reviewer can see why it was made.
const (
	evNamedInDoc   = "named-in-doc"   // the concept's doc names the declaration (Service.Book)
	evNameExact    = "name-exact"     // the declaration's words are the concept name's words
	evAliasExact   = "alias-exact"    // ... or one of its aliases
	evCalls        = "calls"          // a test calls a symbol linked to the concept; beats a word in the test's name
	evNameContains = "name-contains"  // the declaration's name holds the whole concept name (RenewalHandler: Renewal)
	evAliasContain = "alias-contains" // ... or a whole multi-word alias
	evTopic        = "topic"          // a listener or producer names the event's topic
	evDoc          = "doc"            // the docstring or comment names the concept
	evData         = "data"           // the body touches a table that represents an entity the concept works on
	evNamePartial  = "name-partial"   // the declaration's words are part of the concept name (appointment: AppointmentReminding)
	evNameOverlap  = "name-overlap"   // some words shared, neither name holds the other
	// Evidence read from a spec (#260, ADR 0041).
	evAsyncAPI = "asyncapi" // the code names a channel or a message of the AsyncAPI contract
	evOpenAPI  = "openapi"  // the code declares a route the OpenAPI spec maps to a process
	evRoute    = "route"    // the code declares a route whose path names the concept
	evSchema   = "schema"   // the type maps to a table that represents the entity
)

var strength = map[string]int{
	evNamedInDoc: 110, evNameExact: 100, evAliasExact: 95, evSchema: 92, evAsyncAPI: 90, evOpenAPI: 88, evCalls: 85,
	evNameContains: 80, evAliasContain: 75, evTopic: 65, evDoc: 60, evRoute: 50, evData: 45, evNamePartial: 40, evNameOverlap: 30,
}

// DefaultStopwords is programming vocabulary that never matches a concept
// on its own (#59): Handler, Manager, process, run. A concept-name word on
// the list still counts when a word off the list matches too, and an alias
// is never filtered, so a team that writes aliases: [handler] gets the
// match. glossmark.yaml extends the list under suggest: stopwords:.
var DefaultStopwords = []string{
	"handle", "handler", "handling", "process", "processor", "processing", "manage", "manager", "management",
	"run", "runner", "do", "data", "util", "utils", "helper", "helpers", "common", "misc", "get", "set", "stuff",
	"thing", "obj", "object", "new", "service", "controller", "impl", "base", "default", "main", "app", "api",
	"client", "server", "info", "item", "list", "map", "value", "result", "request", "response", "error", "err",
	"config", "context", "ctx", "init", "test", "mock", "fake", "stub", "make", "load", "save", "read", "write",
	"parse", "format", "render", "view", "index", "id", "key", "name", "no", "is", "has", "all", "core", "lib",
	"tool", "exec", "type", "model", "store", "check", "build", "create", "update", "delete", "func", "fn",
}

// concept is one target candidate, tokenized once per run.
type concept struct {
	n        *graph.Node
	curie    string
	name     string
	toks     []string
	raw      string // lower-case name, letters and digits only
	aliases  [][]string
	aliasRaw []string
	alias    []string
	doc      string
	impl     bool
	realizes map[string]bool // core concepts this one realizes
}

type matcher struct {
	g        *graph.Graph
	ws       *load.Workspace
	concepts []*concept
	byCurie  map[string]*concept
	stop     map[string]bool
}

func newMatcher(ws *load.Workspace, g *graph.Graph, extraStop []string) *matcher {
	m := &matcher{g: g, ws: ws, byCurie: map[string]*concept{}, stop: map[string]bool{}}
	for _, w := range append(append([]string{}, DefaultStopwords...), extraStop...) {
		for _, t := range mcp.Tokens(w) {
			m.stop[t] = true
		}
		m.stop[strings.ToLower(w)] = true
	}
	for _, n := range g.NodesOf(model.NodeConcept) {
		// A proposed concept awaits review, its names and aliases with it,
		// and is no target until accepted. A deprecated one stays a target:
		// brownfield code still says its name, and the proposal names the
		// replacement for the reviewer (ADR 0030).
		if n.Proposed || n.Imported && n.Tier == model.TierImpl && n.Published == "" {
			continue
		}
		name := n.CURIE[strings.Index(n.CURIE, ":")+1:]
		c := &concept{n: n, curie: n.CURIE, name: name, toks: mcp.Tokens(name), raw: rawJoin(name), doc: n.Doc, impl: n.Tier == model.TierImpl, realizes: map[string]bool{}}
		for _, a := range n.Aliases {
			c.aliases = append(c.aliases, mcp.Tokens(a))
			c.aliasRaw = append(c.aliasRaw, rawJoin(a))
			c.alias = append(c.alias, a)
		}
		for _, o := range g.Objects(n.ID, "realizes") {
			c.realizes[g.Curie(o)] = true
		}
		if len(c.toks) == 0 {
			continue
		}
		m.concepts = append(m.concepts, c)
		m.byCurie[c.curie] = c
	}
	sort.Slice(m.concepts, func(i, j int) bool { return m.concepts[i].curie < m.concepts[j].curie })
	return m
}

// hit is one piece of evidence for one concept.
type hit struct {
	c     *concept
	kind  string
	terms string
}

func (h hit) strength() int { return strength[h.kind] }

func (h hit) matched() string {
	if h.terms == "" {
		return h.kind
	}
	return h.kind + ":" + h.terms
}

// nonStop reports whether any of toks is off the stoplist and at least
// three letters long, so it can carry a match.
func (m *matcher) nonStop(toks []string) bool {
	for _, t := range toks {
		if !m.stop[t] && len(t) >= 3 {
			return true
		}
	}
	return false
}

// nameHits scores a declaration's name against every concept. Matching is
// on whole words only: a symbol that is a fragment of a concept-name word
// (ent, plen in Replenishment) or shorter than four letters is no match
// unless it equals an alias.
func (m *matcher) nameHits(sym string) []hit {
	toks := mcp.Tokens(sym)
	raw := rawJoin(sym)
	var out []hit
	for _, c := range m.concepts {
		best := hit{}
		set := func(k, terms string) {
			if strength[k] > strength[best.kind] {
				best = hit{c, k, terms}
			}
		}
		// Aliases bypass the stoplist; a short alias must be the whole symbol.
		for i, at := range c.aliases {
			switch {
			case c.aliasRaw[i] != "" && c.aliasRaw[i] == raw, len(at) > 0 && equal(at, toks):
				set(evAliasExact, c.alias[i])
			case len(c.aliasRaw[i]) >= 4 && len(at) > 0 && containsSeq(toks, at):
				set(evAliasContain, c.alias[i])
			}
		}
		if len(raw) >= 4 && len(toks) > 0 {
			shared := intersect(toks, c.toks)
			switch {
			case equal(toks, c.toks) && m.nonStop(toks):
				set(evNameExact, c.name)
			case len(toks) > len(c.toks) && containsSeq(toks, c.toks) && m.nonStop(c.toks):
				set(evNameContains, strings.Join(c.toks, ","))
			case len(c.toks) > len(toks) && containsSeq(c.toks, toks) && m.nonStop(toks):
				set(evNamePartial, strings.Join(toks, ","))
			case len(shared) > 0 && m.nonStop(shared):
				set(evNameOverlap, strings.Join(shared, ","))
			}
		}
		if best.c != nil {
			out = append(out, best)
		}
	}
	return out
}

// docHits finds concepts a declaration's docstring or comment names: the
// whole concept name, or a whole alias, as a phrase.
func (m *matcher) docHits(doc string) []hit {
	if doc == "" {
		return nil
	}
	toks := mcp.Tokens(doc)
	var out []hit
	for _, c := range m.concepts {
		switch {
		case len(c.raw) >= 5 && containsSeq(toks, c.toks) && m.nonStop(c.toks):
			out = append(out, hit{c, evDoc, strings.Join(c.toks, ",")})
		default:
			for i, at := range c.aliases {
				if len(c.aliasRaw[i]) >= 4 && len(at) > 0 && containsSeq(toks, at) {
					out = append(out, hit{c, evDoc, c.alias[i]})
					break
				}
			}
		}
	}
	return out
}

// namedInDoc finds concepts whose doc names the declaration itself
// ("Service.Book: checks the slot..."): the strongest evidence there is,
// and it picks the implementation concept over a core one the name merely
// resembles.
func (m *matcher) namedInDoc(sym, parent string) []hit {
	var names []string
	if parent != "" {
		names = append(names, parent+"."+lastPart(sym))
	}
	if strings.Contains(sym, ".") {
		names = append(names, sym)
	}
	if b := lastPart(sym); distinctive(b) {
		names = append(names, b)
	}
	var out []hit
	for _, c := range m.concepts {
		if c.doc == "" {
			continue
		}
		for _, n := range names {
			if wordIn(c.doc, n) {
				out = append(out, hit{c, evNamedInDoc, n})
				break
			}
		}
	}
	return out
}

// distinctive reports an identifier unlikely to appear in prose by chance:
// CamelCase or snake_case, at least six letters.
func distinctive(s string) bool {
	if len(s) < 6 {
		return false
	}
	if strings.Contains(s, "_") {
		return true
	}
	for i := 1; i < len(s); i++ {
		if unicode.IsUpper(rune(s[i])) && unicode.IsLower(rune(s[i-1])) {
			return true
		}
	}
	return false
}

func wordIn(text, w string) bool {
	// Not as the qualifier of a member: StringUtils.check names check.
	re, err := regexp.Compile(`(^|[^\w.])` + regexp.QuoteMeta(w) + `($|[^\w.]|\.(\s|$))`)
	return err == nil && re.MatchString(text)
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// containsSeq reports whether needle appears in hay as consecutive words.
func containsSeq(hay, needle []string) bool {
	if len(needle) == 0 || len(needle) > len(hay) {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		if equal(hay[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

func intersect(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, x := range a {
		if set[x] && !seen[x] {
			out = append(out, x)
			seen[x] = true
		}
	}
	return out
}

func rawJoin(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
