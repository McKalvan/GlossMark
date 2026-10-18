package mcp

import (
	"sort"
	"strings"
	"unicode"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// Hit is one search result with the reason it matched.
type Hit struct {
	Curie       string   `json:"curie"`
	Kind        string   `json:"kind"`
	Tier        string   `json:"tier"`
	Doc         string   `json:"doc,omitempty"`
	PublishedAs string   `json:"published_as,omitempty"`
	Realizes    []string `json:"realizes,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Proposed    bool     `json:"proposed,omitempty"`
	Score       int      `json:"score"`
	Matched     string   `json:"matched"` // what the query hit: name, alias, doc, fuzzy
}

// Search ranks concepts by meaning rather than substring: it splits CamelCase
// names into words, matches aliases and docs, strips common suffixes, and
// tolerates small typos with edit distance. It also follows is-a and realizes
// one hop so "settlement cutoff" finds sw:CutoffClock through its core concept.
func Search(g *graph.Graph, q, ns, kind, tier string, limit int) []Hit {
	words := tokens(q)
	if len(words) == 0 {
		return nil
	}
	raw := rawJoin(q)
	var hits []Hit
	for _, n := range g.NodesOf(model.NodeConcept) {
		if ns != "" && n.NS != ns || kind != "" && n.Kind != kind || tier != "" && n.Tier != tier {
			continue
		}
		if n.Imported && n.Tier == model.TierImpl && n.Published == "" {
			continue
		}
		score, how := scoreConcept(g, n, words, raw)
		if score <= 0 {
			continue
		}
		h := Hit{Curie: n.CURIE, Kind: n.Kind, Tier: n.Tier, Doc: n.Doc, PublishedAs: n.Published, Aliases: n.Aliases, Proposed: n.Proposed, Score: score, Matched: how}
		if r := g.Objects(n.ID, "realizes"); len(r) > 0 {
			for _, x := range r {
				h.Realizes = append(h.Realizes, g.Curie(x))
			}
		}
		hits = append(hits, h)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		// Shorter names first on ties: Capture before CaptureHandler.
		if len(hits[i].Curie) != len(hits[j].Curie) {
			return len(hits[i].Curie) < len(hits[j].Curie)
		}
		return hits[i].Curie < hits[j].Curie
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// scoreConcept scores a concept against query tokens.
func scoreConcept(g *graph.Graph, n *graph.Node, words []string, raw string) (int, string) {
	name := n.CURIE[strings.Index(n.CURIE, ":")+1:]
	nameToks := tokens(name)
	best, how := 0, ""
	add := func(s int, h string) {
		if s > best {
			best, how = s, h
		}
	}
	// Exact (as typed) beats exact (after stemming) beats substring.
	joined := strings.ToLower(name)
	qjoined := strings.Join(words, "")
	switch {
	case raw != "" && joined == raw:
		add(110, "name")
	case strings.Join(nameToks, "") == qjoined:
		add(100, "name")
	case strings.Contains(joined, raw) && raw != "", strings.Contains(strings.Join(nameToks, ""), qjoined):
		add(60, "name")
	}
	// Token overlap with the name (all query words present → strong).
	if s := overlap(words, nameToks); s > 0 {
		add(40+s*10, "name")
	}
	// Aliases.
	for _, a := range n.Aliases {
		at := tokens(a)
		if rawJoin(a) == raw || strings.Join(at, "") == qjoined {
			add(95, "alias:"+a)
		} else if s := overlap(words, at); s > 0 {
			add(35+s*10, "alias:"+a)
		}
	}
	// Label and doc.
	if n.Label != "" {
		if s := overlap(words, tokens(n.Label)); s > 0 {
			add(30+s*10, "label")
		}
	}
	if n.Doc != "" {
		if s := overlap(words, tokens(n.Doc)); s > 0 {
			add(15+s*5, "doc")
		}
	}
	// Fuzzy: each query word within edit distance 2 of a name/alias token.
	fuzzy := 0
	for _, w := range words {
		if len(w) < 4 {
			continue
		}
		for _, t := range append(nameToks, aliasTokens(n)...) {
			if lev(w, t) <= maxDist(w) {
				fuzzy++
				break
			}
		}
	}
	if fuzzy > 0 && fuzzy == len(words) {
		add(25, "fuzzy")
	}
	// One hop: a core concept this realizes, or a superclass, matching strongly.
	for _, p := range []string{"realizes", "published-as", "is-a"} {
		for _, o := range g.Objects(n.ID, p) {
			on := g.Node(o)
			if on == nil {
				continue
			}
			oname := on.CURIE[strings.Index(on.CURIE, ":")+1:]
			if strings.ToLower(oname) == qjoined || overlap(words, tokens(oname)) == len(words) {
				add(30, "via "+p+" "+on.CURIE)
			}
			for _, a := range on.Aliases {
				if strings.Join(tokens(a), "") == qjoined {
					add(28, "via "+p+" "+on.CURIE)
				}
			}
		}
	}
	return best, how
}

func aliasTokens(n *graph.Node) []string {
	var out []string
	for _, a := range n.Aliases {
		out = append(out, tokens(a)...)
	}
	return out
}

func maxDist(w string) int {
	if len(w) >= 8 {
		return 2
	}
	return 1
}

// overlap counts query words that appear in the candidate tokens (after stemming).
func overlap(q, cand []string) int {
	set := map[string]bool{}
	for _, c := range cand {
		set[c] = true
	}
	n := 0
	for _, w := range q {
		if set[w] {
			n++
		}
	}
	return n
}

// tokens splits CamelCase, snake_case, kebab-case and prose into lower-case
// stems, dropping stop words.
func tokens(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			w := stem(strings.ToLower(string(cur)))
			if w != "" && !stop[w] {
				out = append(out, w)
			}
			cur = nil
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if unicode.IsUpper(r) && len(cur) > 0 && (unicode.IsLower(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]) && unicode.IsUpper(rs[i-1]))) {
				flush()
			}
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	return out
}

var stop = map[string]bool{"the": true, "a": true, "an": true, "of": true, "and": true, "or": true, "to": true, "for": true, "in": true, "on": true, "is": true, "it": true, "that": true, "this": true, "with": true, "by": true, "from": true, "as": true}

// rawJoin lower-cases and strips everything but letters and digits.
func rawJoin(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// stem strips common English suffixes and a trailing e, so "settlements"
// matches "Settlement", "refunding" matches "Refund" and "captured" matches
// "Capture".
func stem(w string) string {
	for _, suf := range []string{"ations", "ation", "ing", "ers", "ies", "ed", "es", "s", "er"} {
		if strings.HasSuffix(w, suf) && len(w)-len(suf) >= 3 {
			w = strings.TrimSuffix(w, suf)
			if suf == "ies" {
				w += "y"
			}
			break
		}
	}
	if strings.HasSuffix(w, "e") && len(w) > 4 {
		w = strings.TrimSuffix(w, "e")
	}
	return w
}

// lev is the Levenshtein distance.
func lev(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
