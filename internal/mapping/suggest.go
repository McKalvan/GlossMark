// @gloss implements gmi:MappingSets scope=file

package mapping

import (
	"sort"
	"strings"
	"unicode"

	"github.com/mckalvan/glossmark/internal/calib"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// Candidate is a suggested mapping from an owned concept to a concept of
// another namespace. Conf is the precision its Basis measured on the labelled
// mapping set (calibration.go), as gloss suggest's conf is (ADR 0021).
type Candidate struct {
	Subject, Object string // CURIEs; the subject is owned
	Predicate       string // skos:exactMatch or skos:closeMatch
	Basis           string // evidence, plus /kind-mismatch and /tie
	Conf            float64
	Why             string
}

// Evidence kinds, strongest first.
var evidenceRank = []string{"name-exact", "alias-exact", "name-fuzzy", "name-overlap", "doc-overlap"}

// SuggestOptions bound a suggestion run.
type SuggestOptions struct {
	Owned   map[string]bool // namespaces whose concepts are subjects
	Against map[string]bool // namespaces to match against (nil: every other loaded one)
	// Skip reports a pair that must not be suggested (already mapped, negated
	// or rejected), either direction.
	Skip func(subject, object string) bool
	// Table is the calibration table; nil uses the shipped one.
	Table calib.Table
}

// Suggest proposes, for each owned concept, its best lexical match in each
// other namespace: the same name, an alias naming the other concept, a name
// one or two edits away, shared name words, or shared definition words. A
// kind mismatch turns an exact match into a close one.
func Suggest(g *graph.Graph, opt SuggestOptions) []Candidate {
	table := opt.Table
	if table == nil {
		table = ShippedCalibration
	}
	conf := table.Confs()
	var owned, others []*graph.Node
	for _, n := range g.NodesOf(model.NodeConcept) {
		if n.Deprecated {
			continue
		}
		switch {
		case opt.Owned[n.NS]:
			owned = append(owned, n)
		case opt.Against == nil || opt.Against[n.NS]:
			if n.Imported && n.Tier == model.TierImpl && n.Published == "" {
				continue // the seam: unpublished implementation concepts do not cross
			}
			others = append(others, n)
		}
	}
	var out []Candidate
	for _, a := range owned {
		best := map[string][]Candidate{} // namespace → best candidates
		bestRank := map[string]int{}
		for _, b := range others {
			if b.NS == a.NS || (opt.Skip != nil && opt.Skip(a.CURIE, b.CURIE)) {
				continue
			}
			ev, why := evidence(a, b)
			if ev == "" {
				continue
			}
			r := rankOf(ev)
			c := Candidate{Subject: a.CURIE, Object: b.CURIE, Predicate: "skos:closeMatch", Basis: ev, Why: why}
			strong := ev == "name-exact" || ev == "alias-exact" || ev == "name-fuzzy"
			if a.Kind != "" && b.Kind != "" && a.Kind != b.Kind {
				c.Basis += "/kind-mismatch"
			} else if strong {
				c.Predicate = "skos:exactMatch"
			}
			cur, seen := bestRank[b.NS]
			switch {
			case !seen || r < cur:
				bestRank[b.NS] = r
				best[b.NS] = []Candidate{c}
			case r == cur:
				best[b.NS] = append(best[b.NS], c)
			}
		}
		for _, cs := range best {
			for _, c := range cs {
				if len(cs) > 1 {
					c.Basis += "/tie"
				}
				c.Conf = conf(c.Basis)
				out = append(out, c)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Conf != out[j].Conf {
			return out[i].Conf > out[j].Conf
		}
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		return out[i].Object < out[j].Object
	})
	return out
}

func rankOf(ev string) int {
	for i, e := range evidenceRank {
		if e == ev {
			return i
		}
	}
	return len(evidenceRank)
}

func localName(curie string) string {
	if i := strings.Index(curie, ":"); i >= 0 {
		return curie[i+1:]
	}
	return curie
}

// evidence returns the strongest lexical evidence that a and b match.
func evidence(a, b *graph.Node) (string, string) {
	an, bn := words(localName(a.CURIE)), words(localName(b.CURIE))
	aj, bj := strings.Join(an, ""), strings.Join(bn, "")
	if aj != "" && aj == bj {
		return "name-exact", "same name"
	}
	for _, x := range a.Aliases {
		if strings.Join(words(x), "") == bj {
			return "alias-exact", a.CURIE + " is also called " + x
		}
	}
	for _, x := range b.Aliases {
		if strings.Join(words(x), "") == aj {
			return "alias-exact", b.CURIE + " is also called " + x
		}
	}
	for _, x := range a.Aliases {
		for _, y := range b.Aliases {
			if j := strings.Join(words(x), ""); j != "" && j == strings.Join(words(y), "") {
				return "alias-exact", "both are also called " + x
			}
		}
	}
	if len(aj) >= 6 && len(bj) >= 6 && lev(aj, bj) <= 2 {
		return "name-fuzzy", "names one or two letters apart"
	}
	if shared := intersect(an, bn); len(shared) > 0 && float64(len(shared))/float64(len(union(an, bn))) >= 1.0/3 {
		return "name-overlap", "names share " + strings.Join(shared, ", ")
	}
	if a.Doc != "" && b.Doc != "" {
		if shared := intersect(content(a.Doc), content(b.Doc)); len(shared) >= 3 {
			return "doc-overlap", "definitions share " + strings.Join(shared, ", ")
		}
	}
	return "", ""
}

// words splits CamelCase, snake_case and prose into lower-case singular words.
func words(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			w := strings.ToLower(string(cur))
			switch {
			case strings.HasSuffix(w, "ies") && len(w) > 4:
				w = strings.TrimSuffix(w, "ies") + "y"
			case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && len(w) > 3:
				w = strings.TrimSuffix(w, "s")
			}
			out = append(out, w)
			cur = nil
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if unicode.IsUpper(r) && len(cur) > 0 && (unicode.IsLower(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]))) {
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

var stop = map[string]bool{"the": true, "a": true, "an": true, "of": true, "and": true, "or": true, "to": true, "for": true, "in": true, "on": true,
	"is": true, "it": true, "that": true, "this": true, "with": true, "by": true, "from": true, "as": true, "at": true, "be": true, "any": true, "its": true}

func content(doc string) []string {
	var out []string
	for _, w := range words(doc) {
		if len(w) > 2 && !stop[w] {
			out = append(out, w)
		}
	}
	return out
}

func intersect(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, x := range a {
		if set[x] && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func union(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range append(append([]string{}, a...), b...) {
		set[x] = true
	}
	out := make([]string, 0, len(set))
	for x := range set {
		out = append(out, x)
	}
	return out
}

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

// Label is a labelled mapping: the truth a suggestion is scored against.
type Label struct{ Subject, Predicate, Object string }

// Score counts suggestions per basis against labels: correct when the pair
// and the predicate match a label, either direction.
func Score(cands []Candidate, labels []Label) calib.Table {
	truth := map[string]string{}
	for _, l := range labels {
		truth[l.Subject+"|"+l.Object] = l.Predicate
		truth[l.Object+"|"+l.Subject] = l.Predicate
	}
	t := calib.Table{}
	for _, c := range cands {
		cnt := t[c.Basis]
		cnt.N++
		if truth[c.Subject+"|"+c.Object] == c.Predicate {
			cnt.Correct++
		}
		t[c.Basis] = cnt
	}
	return t
}

// ReadLabels reads a labels file: subject, predicate, object per line,
// tab-separated (an SSSOM/TSV mapping set's first three columns work too).
func ReadLabels(text string) []Label {
	var out []Label
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" || strings.HasPrefix(line, "subject_id") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		p := f[1]
		if full, ok := ShortNames[p]; ok {
			p = full
		}
		out = append(out, Label{Subject: strings.TrimSpace(f[0]), Predicate: p, Object: strings.TrimSpace(f[len(f)-1])})
		if len(f) > 3 {
			out[len(out)-1].Object = strings.TrimSpace(f[2])
		}
	}
	return out
}

// Duplicates finds existing concepts a proposed one may duplicate: the same
// lexical evidence Suggest uses, against every loaded concept outside the
// proposed concept's own name. A hit in another namespace is better recorded
// as a mapping; one in the same namespace is the concept to use instead.
func Duplicates(g *graph.Graph, curie, kind, doc string, aliases []string) []Candidate {
	ns := curie
	if i := strings.Index(curie, ":"); i > 0 {
		ns = curie[:i]
	}
	probe := &graph.Node{NodeType: model.NodeConcept, CURIE: curie, NS: ns, Kind: kind, Doc: doc, Aliases: aliases}
	conf := ShippedCalibration.Confs()
	var out []Candidate
	for _, n := range g.NodesOf(model.NodeConcept) {
		if n.CURIE == curie || n.Deprecated {
			continue
		}
		ev, why := evidence(probe, n)
		if ev == "" || rankOf(ev) > rankOf("name-fuzzy") {
			continue // only strong evidence: a name, an alias or a near spelling
		}
		c := Candidate{Subject: curie, Object: n.CURIE, Predicate: "skos:exactMatch", Basis: ev, Why: why}
		if kind != "" && n.Kind != "" && kind != n.Kind {
			c.Basis += "/kind-mismatch"
			c.Predicate = "skos:closeMatch"
		}
		c.Conf = conf(c.Basis)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Conf != out[j].Conf {
			return out[i].Conf > out[j].Conf
		}
		return out[i].Object < out[j].Object
	})
	return out
}
