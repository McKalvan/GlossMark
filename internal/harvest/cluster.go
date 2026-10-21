// @gloss implements gmi:EvidenceHarvester scope=file

package harvest

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/mckalvan/glossmark/internal/mcp"
)

// Triangulation categories (ADR 0025, section 6).
const (
	Corroborated     = "corroborated"       // intent and built sources agree
	DocsOnly         = "docs-only"          // intended, not built, or a stale doc: a planned candidate
	CodeOnlyVisible  = "code-only-visible"  // built and user-visible, undocumented
	CodeOnlyInternal = "code-only-internal" // built, not visible: implementation or an undocumented rule
)

// intentFamilies say what the business means; builtFamilies say what exists.
var (
	intentFamilies = map[string]bool{FamilyDoc: true, FamilyDocConnector: true, FamilyTracker: true, FamilyVocabulary: true}
	builtFamilies  = map[string]bool{FamilyCode: true, FamilyData: true, FamilyBehaviour: true, FamilySurface: true}
)

// authority orders families for choosing a cluster's name: domain words win
// (ADR 0025, principle 4).
var authority = []string{FamilyVocabulary, FamilyDocConnector, FamilyDoc, FamilyTracker, FamilySurface, FamilyData, FamilyCode, FamilyBehaviour, FamilyHistory, FamilyReference}

// namingSignals propose a candidate concept; every other signal only
// mentions one (a test name, an error message, a function, a column).
var namingSignals = map[string]bool{"type-decl": true, "field": true, "enum-type": true, "enum-value": true, "module": true, "table": true, "schema-type": true,
	"doc-heading": true, "doc-definition": true, "glossary-entry": true, "rename": true, "ui-string": true, "reference-term": true, "issue-title": true}

// Cluster is a candidate concept: the surfaces that normalise to one key,
// and every observation that names or mentions it.
type Cluster struct {
	Key      string         `json:"key"`
	Name     string         `json:"name"`
	NamedBy  string         `json:"named_by"`
	Category string         `json:"category"`
	Families map[string]int `json:"families"`
	Signals  map[string]int `json:"signals"`
	Naming   int            `json:"naming"`
	NamedAs  []string       `json:"named_as"`
	Mentions int            `json:"mentions"`
	Units    []string       `json:"units,omitempty"`
	Surfaces []string       `json:"surfaces"`
	// Related are the keys a source links to this one without deciding
	// they are the same concept: "renamed-to:patron" from the history,
	// "alias:member" from a glossary. Merging is a judgment (ADR 0025).
	Related  []string `json:"related,omitempty"`
	Evidence []string `json:"evidence"`
}

// maxEvidence caps the ids a cluster lists; the counts stay exact.
const maxEvidence = 20

var routeParam = regexp.MustCompile(`\{[^}]*\}|:\w+|<[^>]*>`)

// Tokens are the stems a surface is compared by, the ones search_concepts and
// gloss suggest use, with route parameters removed.
func Tokens(surface string) []string {
	return mcp.Tokens(routeParam.ReplaceAllString(surface, " "))
}

func stemSet(words string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(words) {
		for _, t := range mcp.Tokens(w) {
			out[t] = true
		}
	}
	return out
}

// Engineering affixes: SessionRow, LoanRecord and BaseExercise name the
// session, the loan and the exercise.
var (
	leadingAffixes  = stemSet("base abstract default generic is has can should num max min total how what why when where")
	trailingAffixes = stemSet("row record model schema handler service manager repo repository dto request response table impl controller router view page store factory mixin config settings client util utils helper error exception payload in out create update props state context provider hook id ids at url uri count ms seconds")
	genericKeys     = stemSet("test tests main init app util helper data info type value config error list get set new id name file path line string text number count result option param arg args self none true false todo readme overview introduction usage example examples install setup summary background notes contents appendix decision decisions consequences alternatives motivation goal goals scope status section step steps how what why faq license changelog contributing design architecture api done next now risk risks default index other misc see also references context problem solution format the run cli yes no ok created updated deleted start end detail details")
	genericPhrases  = map[string]bool{}
)

func init() {
	for _, p := range []string{"open questions", "next steps", "getting started", "how it works", "what is in this repository", "quick start", "see also", "design in one paragraph", "alternatives considered", "out of scope"} {
		genericPhrases[strings.Join(mcp.Tokens(p), " ")] = true
	}
}

// Key normalises a naming surface into a candidate key: its stems without
// engineering affixes. It returns "" for a surface that names no concept: a
// generic word, a heading that is a sentence.
func Key(surface string) string {
	toks := Tokens(surface)
	if genericPhrases[strings.Join(toks, " ")] {
		return ""
	}
	for len(toks) > 1 && leadingAffixes[toks[0]] {
		toks = toks[1:]
	}
	for len(toks) > 1 && trailingAffixes[toks[len(toks)-1]] {
		toks = toks[:len(toks)-1]
	}
	if len(toks) == 0 || len(toks) > 4 {
		return ""
	}
	if len(toks) == 1 && (genericKeys[toks[0]] || len(toks[0]) < 3) {
		return ""
	}
	return strings.Join(toks, " ")
}

// minPhraseFiles is how many documents must use a prose phrase before it
// proposes a concept on its own.
const minPhraseFiles = 3

// namingKeys returns, per observation, the key it proposes, or "". A prose
// phrase proposes one only when it recurs across documents or code, data or
// the UI also uses it as a run of words in a name: "catch rate" in three docs,
// or once in a doc and as catch_rate in a model.
func namingKeys(ev []Evidence) []string {
	files := map[string]map[string]bool{}
	named := map[string]bool{}
	for _, e := range ev {
		switch {
		case e.Signal == "doc-phrase":
			k := Key(e.Surface)
			if files[k] == nil {
				files[k] = map[string]bool{}
			}
			files[k][e.Source.Path] = true
		case builtFamilies[e.Source.Family]:
			toks := Tokens(e.Surface)
			for i := range toks {
				for n := 2; n <= 3 && i+n <= len(toks); n++ {
					named[strings.Join(toks[i:i+n], " ")] = true
				}
			}
		}
	}
	keys := make([]string, len(ev))
	for i, e := range ev {
		switch {
		case e.Signal == "doc-phrase":
			if k := Key(e.Surface); k != "" && (len(files[k]) >= minPhraseFiles || named[k]) {
				keys[i] = k
			}
		case !namingSignals[e.Signal]:
		case e.Signal == "ui-string" && len(strings.Fields(e.Surface)) > 3:
		default:
			keys[i] = Key(e.Surface)
		}
	}
	return keys
}

// Clusters groups evidence into candidate concepts, ranked by how many ways
// each is named and how many source families observe it.
func Clusters(ev []Evidence) []Cluster {
	byKey := map[string]*Cluster{}
	surfaces := map[string]map[string]*surfaceTally{}
	units := map[string]map[string]bool{}
	note := func(c *Cluster, e Evidence) {
		c.Families[e.Source.Family]++
		c.Signals[e.Signal]++
		if len(c.Evidence) < maxEvidence {
			c.Evidence = append(c.Evidence, e.ID)
		}
		if e.Unit != "" {
			units[c.Key][e.Unit] = true
		}
	}
	keys := namingKeys(ev)
	for i, e := range ev {
		k := keys[i]
		if k == "" {
			continue
		}
		c := byKey[k]
		if c == nil {
			c = &Cluster{Key: k, Families: map[string]int{}, Signals: map[string]int{}}
			byKey[k] = c
			surfaces[k] = map[string]*surfaceTally{}
			units[k] = map[string]bool{}
		}
		c.Naming++
		if !contains(c.NamedAs, e.Signal) {
			c.NamedAs = append(c.NamedAs, e.Signal)
		}
		note(c, e)
		t := surfaces[k][e.Surface]
		if t == nil {
			t = &surfaceTally{surface: e.Surface, rank: len(authority), exact: len(words(e.Surface)) == strings.Count(k, " ")+1}
			surfaces[k][e.Surface] = t
		}
		t.count++
		if r := authorityRank(e.Source.Family); r < t.rank {
			t.rank, t.family = r, e.Source.Family
		}
	}
	// Index keys by their first stem, then count every observation that
	// mentions a key as a contiguous run of stems.
	first := map[string][]string{}
	for k := range byKey {
		f := strings.SplitN(k, " ", 2)[0]
		first[f] = append(first[f], k)
	}
	for i, e := range ev {
		own := keys[i]
		texts := []string{e.Surface}
		for _, c := range e.Context {
			if to, ok := strings.CutPrefix(c, "renamed-to:"); ok {
				texts = append(texts, to)
			}
		}
		hit := map[string]bool{}
		for _, s := range texts {
			toks := Tokens(s)
			for i := range toks {
				for _, k := range first[toks[i]] {
					if k != own && !hit[k] && runAt(toks, i, strings.Split(k, " ")) {
						hit[k] = true
					}
				}
			}
		}
		for k := range hit {
			c := byKey[k]
			c.Mentions++
			note(c, e)
		}
	}
	for i, e := range ev {
		from := byKey[keys[i]]
		if from == nil {
			continue
		}
		for _, c := range e.Context {
			rel, target, ok := strings.Cut(c, ":")
			if !ok || (rel != "renamed-to" && rel != "alias") {
				continue
			}
			to := byKey[Key(target)]
			if to == nil || to == from {
				continue
			}
			addRelated(from, rel+":"+to.Key)
			back := "alias:" + from.Key
			if rel == "renamed-to" {
				back = "renamed-from:" + from.Key
			}
			addRelated(to, back)
		}
	}
	out := make([]Cluster, 0, len(byKey))
	for k, c := range byKey {
		var ts []*surfaceTally
		for _, t := range surfaces[k] {
			ts = append(ts, t)
		}
		sort.Slice(ts, func(i, j int) bool {
			if ts[i].exact != ts[j].exact {
				return ts[i].exact
			}
			if ts[i].rank != ts[j].rank {
				return ts[i].rank < ts[j].rank
			}
			if ts[i].count != ts[j].count {
				return ts[i].count > ts[j].count
			}
			return ts[i].surface < ts[j].surface
		})
		c.Name, c.NamedBy = ts[0].surface, ts[0].family
		for _, t := range ts {
			c.Surfaces = append(c.Surfaces, t.surface)
		}
		for u := range units[k] {
			c.Units = append(c.Units, u)
		}
		sort.Strings(c.Units)
		sort.Strings(c.NamedAs)
		c.Category = categorize(c.Families)
		out = append(out, *c)
	}
	// Rank by how many kinds of signal name a candidate, then by how many
	// families observe it: on AgentArena a candidate named one way only was
	// a glossary term about one time in eighty, and this order put half the
	// glossary in the first 300 candidates (ADR 0025, section 12).
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if len(a.NamedAs) != len(b.NamedAs) {
			return len(a.NamedAs) > len(b.NamedAs)
		}
		if len(a.Families) != len(b.Families) {
			return len(a.Families) > len(b.Families)
		}
		if a.Naming+a.Mentions != b.Naming+b.Mentions {
			return a.Naming+a.Mentions > b.Naming+b.Mentions
		}
		return a.Key < b.Key
	})
	return out
}

// surfaceTally counts one written form of a key. A form is exact when it
// has no word the key dropped (an affix, a stop word): "Drill" names the
// drill better than "In the drill" or "DrillRow".
type surfaceTally struct {
	surface, family string
	rank, count     int
	exact           bool
}

var camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// words splits a surface into its words, CamelCase and snake_case included.
func words(s string) []string {
	s = routeParam.ReplaceAllString(s, " ")
	s = camelBoundary.ReplaceAllString(s, "$1 $2")
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func authorityRank(family string) int {
	for i, f := range authority {
		if f == family {
			return i
		}
	}
	return len(authority)
}

func addRelated(c *Cluster, r string) {
	if !contains(c.Related, r) {
		c.Related = append(c.Related, r)
		sort.Strings(c.Related)
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func runAt(toks []string, i int, key []string) bool {
	if i+len(key) > len(toks) {
		return false
	}
	for j, k := range key {
		if toks[i+j] != k {
			return false
		}
	}
	return true
}

func categorize(families map[string]int) string {
	intent, built := false, false
	for f := range families {
		intent = intent || intentFamilies[f]
		built = built || builtFamilies[f]
	}
	switch {
	case intent && built:
		return Corroborated
	case intent:
		return DocsOnly
	case families[FamilySurface] > 0:
		return CodeOnlyVisible
	default:
		return CodeOnlyInternal
	}
}
