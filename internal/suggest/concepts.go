// @gloss implements gmi:MarkerSuggester scope=file

package suggest

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/mcp"
	"github.com/mckalvan/glossmark/internal/model"
)

// minConcepts is the size below which a tier is too thin for suggest to
// find much: it says so instead of printing nothing (#64).
const minConcepts = 3

// Hints explain an empty or thin ontology: one line per owned namespace
// with fewer than minConcepts concepts.
func Hints(ws *load.Workspace, g *graph.Graph) []string {
	count := map[string]int{}
	for _, n := range g.NodesOf(model.NodeConcept) {
		if !n.Deprecated {
			count[n.NS]++
		}
	}
	var nss []string
	for ns := range ws.Config.Owns {
		nss = append(nss, ns)
	}
	sort.Strings(nss)
	var out []string
	for _, ns := range nss {
		if count[ns] >= minConcepts {
			continue
		}
		what := "write the core tier first (adopt-glossmark step 3)"
		if ws.Config.Owns[ns].Tier == model.TierImpl {
			what = "write the implementation tier (adopt-glossmark step 4)"
		}
		out = append(out, fmt.Sprintf("%s: has %s; %s, or draft candidates from the code with gloss suggest --concepts", ns, plural(count[ns], "concept"), what))
	}
	return out
}

func plural(n int, s string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, s)
	}
	return fmt.Sprintf("%d %ss", n, s)
}

// ConceptProposal is a cluster of declarations that look like one domain
// concept nobody has named yet.
type ConceptProposal struct {
	Target   string   `json:"target"` // ns:NEW:Name, for gloss propose --from-new
	Members  []string `json:"members"`
	Tables   []string `json:"tables,omitempty"`
	Words    []string `json:"words,omitempty"`
	Internal bool     `json:"internal,omitempty"` // no domain word: an internal: true candidate
}

func (c ConceptProposal) String() string {
	s := c.Target
	if c.Internal {
		s += "  (internal: true candidate, no domain word)"
	}
	s += "\n  members: " + strings.Join(c.Members, ", ")
	if len(c.Tables) > 0 {
		s += "\n  tables: " + strings.Join(c.Tables, ", ")
	}
	if len(c.Words) > 0 {
		s += "\n  words: " + strings.Join(c.Words, ", ")
	}
	return s
}

// Concepts clusters declarations by the tables they touch (a declaration
// joins the tables its body names; tables join when their names share a
// first domain word, tbl_rma and tbl_rma_vendor) and names each cluster
// after the domain words of its members' docstrings, else of its tables.
// Generic programming words never name a cluster (#59).
func Concepts(ws *load.Workspace, g *graph.Graph, paths []string) []ConceptProposal {
	var stop []string
	if ws.Config.Suggest != nil {
		stop = ws.Config.Suggest.Stopwords
	}
	m := newMatcher(ws, g, append(stop, "tbl", "tab", "table", "log", "tmp", "temp", "master", "line", "detail", "history", "queue"))
	type decl struct {
		id, doc string
		body    string
	}
	var decls []decl
	tables := map[string]string{} // lower-case table name → member id
	for _, rel := range ws.Files {
		ext := strings.ToLower(filepath.Ext(rel))
		if !Under(rel, paths) || !codeExts[ext] || ext == ".md" || ext == ".yaml" || ext == ".yml" || ws.IsTestPath(rel) {
			continue
		}
		style, ok := marker.StyleFor(rel, ws.Config.CommentStyles)
		if !ok {
			continue
		}
		lines, err := readLines(ws.Abs(rel))
		if err != nil {
			continue
		}
		for _, d := range marker.Declarations(rel, lines, style) {
			if _, ok := surface(d, lines, rel, false); !ok {
				continue
			}
			id := rel + "#" + d.Symbol
			if isTableSym(d.Symbol) {
				tables[strings.ToLower(lastPart(d.Symbol))] = id
				continue
			}
			decls = append(decls, decl{id, d.Doc, bodyOf(lines, d)})
		}
	}
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if p, ok := parent[x]; ok && p != x {
			r := find(p)
			parent[x] = r
			return r
		}
		parent[x] = x
		return x
	}
	union := func(a, b string) { parent[find(a)] = find(b) }
	words := func(s string) []string {
		var out []string
		for _, t := range mcp.Tokens(strings.ReplaceAll(s, "_", " ")) {
			if !m.stop[t] && len(t) >= 3 {
				out = append(out, t)
			}
		}
		return out
	}
	byWord := map[string]string{}
	var tnames []string
	for t := range tables {
		tnames = append(tnames, t)
	}
	sort.Strings(tnames)
	for _, t := range tnames {
		find(tables[t])
		if w := words(t); len(w) > 0 {
			if o, ok := byWord[w[0]]; ok {
				union(tables[t], o)
			} else {
				byWord[w[0]] = tables[t]
			}
		}
	}
	touched := map[string][]string{}
	for _, d := range decls {
		for _, id := range ident.FindAllString(d.body, -1) {
			if t, ok := tables[strings.ToLower(id)]; ok {
				union(d.id, t)
				touched[d.id] = append(touched[d.id], strings.ToLower(id))
			}
		}
	}
	groups := map[string][]string{}
	for x := range parent {
		groups[find(x)] = append(groups[find(x)], x)
	}
	ns := "ns"
	for p, o := range ws.Config.Owns {
		if o.Tier == model.TierCore && (ns == "ns" || p < ns) {
			ns = p
		}
	}
	docOf := map[string]string{}
	for _, d := range decls {
		docOf[d.id] = d.doc
	}
	var out []ConceptProposal
	for _, members := range groups {
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		var cp ConceptProposal
		count := map[string]int{}
		for _, id := range members {
			cp.Members = append(cp.Members, id)
			for _, w := range words(docOf[id]) {
				count[w] += 2 // docstrings say what the code is for
			}
		}
		for _, t := range tnames {
			if find(tables[t]) == find(members[0]) {
				cp.Tables = append(cp.Tables, t)
				for _, w := range words(t) {
					count[w]++
				}
			}
		}
		for w := range count {
			cp.Words = append(cp.Words, w)
		}
		sort.Slice(cp.Words, func(i, j int) bool {
			if count[cp.Words[i]] != count[cp.Words[j]] {
				return count[cp.Words[i]] > count[cp.Words[j]]
			}
			return cp.Words[i] < cp.Words[j]
		})
		if len(cp.Words) > 6 {
			cp.Words = cp.Words[:6]
		}
		name := "Cluster"
		if len(cp.Words) > 0 {
			name = strings.ToUpper(cp.Words[0][:1]) + cp.Words[0][1:]
		} else {
			cp.Internal = true
			name += fmt.Sprint(len(out) + 1)
		}
		cp.Target = ns + ":NEW:" + name
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target+out[i].Members[0] < out[j].Target+out[j].Members[0] })
	return out
}
