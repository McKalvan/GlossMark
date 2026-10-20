// @gloss implements gmi:MarkerSuggester scope=file

// Package suggest proposes `@gloss?` markers for unmarked declarations by
// matching their names and docstrings against the ontology, so the first
// pass of an adoption is a reviewable patch instead of hand-written markers.
package suggest

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/mcp"
	"github.com/mckalvan/glossmark/internal/model"
)

// Proposal is one marker to insert.
type Proposal struct {
	Path     string  `json:"path"`
	Line     int     `json:"line"` // 1-based line the marker is inserted above
	Symbol   string  `json:"symbol"`
	Relation string  `json:"relation"`
	Target   string  `json:"target"`
	Conf     float64 `json:"conf"`
	Matched  string  `json:"matched"`
	Marker   string  `json:"marker"` // the full comment line, indented
}

// Options tunes the matcher.
type Options struct {
	// MinScore is the lowest score that becomes a proposal, after the
	// docstring penalty (0 means DefaultMinScore).
	MinScore int
	// Ledger, when set, holds review decisions: a link it records as
	// rejected (same marker id, or same anchor fingerprint, relation and
	// target) is not proposed again.
	Ledger map[string]*model.LedgerEntry
}

// DefaultMinScore is at least two words of the name, or the whole of a
// one-word name.
const DefaultMinScore = 60

// docPenalty is taken off a match found through the docstring rather than
// the name: weaker evidence, so it needs a stronger hit to be proposed.
const docPenalty = 20

// codeExts are the files suggest proposes markers in.
var codeExts = map[string]bool{".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".go": true, ".rs": true,
	".java": true, ".kt": true, ".rb": true, ".cs": true, ".php": true, ".swift": true, ".scala": true, ".tf": true, ".sql": true,
	".proto": true, ".feature": true}

// Run proposes markers for the files under paths (all workspace files when
// empty). skipped counts the proposals left out because the ledger records
// them as rejected.
func Run(ws *load.Workspace, g *graph.Graph, paths []string, opt Options) (out []Proposal, skipped int, err error) {
	if opt.MinScore <= 0 {
		opt.MinScore = DefaultMinScore
	}
	type link struct{ fp, rel, target string }
	rejected := map[link]bool{}
	for _, e := range opt.Ledger {
		if e.Status == model.StatusRejected && len(e.Targets) == 1 && e.AnchorFingerprint != "" {
			rejected[link{e.AnchorFingerprint, e.Relation, e.Targets[0]}] = true
		}
	}
	marked := map[string]bool{} // path#line of anchored declarations
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if a.Anchor == nil {
			continue
		}
		if a.Anchor.Scope == "file" {
			marked[a.Path+"#file"] = true
		}
		marked[fmt.Sprintf("%s#%d", a.Path, a.Anchor.StartLine)] = true
	}
	for _, rel := range ws.Files {
		if !under(rel, paths) || !codeExts[strings.ToLower(filepath.Ext(rel))] || marked[rel+"#file"] {
			continue
		}
		style, ok := marker.StyleFor(rel, ws.Config.CommentStyles)
		if !ok {
			continue
		}
		lines, err := readLines(ws.Abs(rel))
		if err != nil {
			return nil, 0, err
		}
		m := ws.ManifestFor(rel)
		test := ws.IsTestPath(rel)
		for _, d := range marker.Declarations(rel, lines, style) {
			if marked[fmt.Sprintf("%s#%d", rel, d.Line)] || private(d.Symbol) {
				continue
			}
			query := strings.TrimPrefix(strings.TrimPrefix(lastPart(d.Symbol), "test_"), "Test")
			hits := mcp.Search(g, query, "", "", "", 5)
			best, how := pick(hits, ws, g, m, d.Kind, test, rel)
			score := 0
			if best != nil {
				score = best.Score
			}
			if score < opt.MinScore && d.Doc != "" {
				// The docstring is weaker evidence; it needs to name the concept.
				if dh := mcp.Search(g, d.Doc, "", "", "", 5); len(dh) > 0 {
					if b, h := pick(dh, ws, g, m, d.Kind, test, rel); b != nil && strings.HasPrefix(b.Matched, "name") && b.Score-docPenalty > score {
						best, how, score = b, "doc "+h, b.Score-docPenalty
					}
				}
			}
			// MinScore is the floor of what is emitted, conf included.
			if best == nil || score < opt.MinScore {
				continue
			}
			relName := relation(d.Kind, best.Kind, test, rel)
			target := best.Curie
			if m != nil && m.DefaultNamespace != "" && strings.HasPrefix(target, m.DefaultNamespace+":") {
				target = "." + strings.TrimPrefix(target, m.DefaultNamespace+":")
			}
			if le := opt.Ledger[extract.MarkerID(rel, relName, []string{target}, d.Symbol, "symbol")]; (le != nil && le.Status == model.StatusRejected) ||
				rejected[link{d.Fingerprint, relName, target}] {
				skipped++
				continue
			}
			conf := math.Min(0.95, math.Round(float64(score)/110*100)/100)
			tok := commentLine(style)
			line := fmt.Sprintf("%s%s @gloss? %s %s conf=%.2f", d.Indent, tok[0], relName, target, conf)
			if tok[1] != "" {
				line += " " + tok[1]
			}
			out = append(out, Proposal{Path: rel, Line: d.Insert, Symbol: d.Symbol, Relation: relName, Target: target, Conf: conf, Matched: how, Marker: line})
		}
	}
	return out, skipped, nil
}

// pick returns the best hit a boundary may use: allowed by the manifest's
// written list, not deprecated, and of a kind the relation it would get
// accepts (a function does not implement an entity; GM221 would flag it).
func pick(hits []mcp.Hit, ws *load.Workspace, g *graph.Graph, m *model.Manifest, declKind string, test bool, path string) (*mcp.Hit, string) {
	for i := range hits {
		h := hits[i]
		if m != nil && len(m.Allowed) > 0 && !model.MatchAnyCurie(m.Allowed, h.Curie) {
			continue
		}
		if n := g.NodeByCurie(h.Curie); n == nil || n.Deprecated {
			continue
		}
		if r := ws.Vocab.Get(relation(declKind, h.Kind, test, path)); r != nil && len(r.RangeKinds) > 0 && h.Kind != "" && !contains(r.RangeKinds, h.Kind) {
			continue
		}
		return &hits[i], h.Matched
	}
	return nil, ""
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// relation chooses the starter-five relation for a declaration.
func relation(declKind, conceptKind string, test bool, path string) string {
	switch {
	case test || strings.HasSuffix(path, ".feature"):
		return "verifies"
	case declKind == "sql" || declKind == "proto":
		return "represents"
	case declKind == "resource":
		return "provisions"
	case (declKind == "class" || declKind == "interface" || declKind == "type" || declKind == "enum" || declKind == "struct") &&
		(conceptKind == "entity" || conceptKind == "event" || conceptKind == "measure" || conceptKind == "resource"):
		return "represents"
	}
	return "implements"
}

func commentLine(style marker.CommentStyle) [2]string {
	for _, t := range style.Line {
		if t != "" {
			return [2]string{t, ""}
		}
	}
	if len(style.Block) > 0 {
		return style.Block[0]
	}
	return [2]string{"#", ""}
}

func private(sym string) bool {
	s := lastPart(sym)
	return strings.HasPrefix(s, "_") || s == "main" || s == "init"
}

func lastPart(sym string) string {
	if i := strings.LastIndexAny(sym, ".:/"); i >= 0 {
		return sym[i+1:]
	}
	return sym
}

func under(rel string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, p := range paths {
		p = strings.Trim(filepath.ToSlash(p), "/")
		if p == "" || p == "." || rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}

// Patch renders proposals as a unified diff against the files under root,
// applicable with git apply or patch -p1.
func Patch(root string, ps []Proposal) (string, error) {
	byFile := map[string][]Proposal{}
	for _, p := range ps {
		byFile[p.Path] = append(byFile[p.Path], p)
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)
	var sb strings.Builder
	for _, f := range files {
		lines, err := readLines(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return "", err
		}
		ins := byFile[f]
		sort.Slice(ins, func(i, j int) bool { return ins[i].Line < ins[j].Line })
		fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", f, f)
		const ctx = 3
		offset := 0
		for i := 0; i < len(ins); {
			// Merge insertions whose context windows overlap into one hunk.
			start := max(0, ins[i].Line-1-ctx)
			j := i
			end := min(len(lines), ins[j].Line-1+ctx)
			for j+1 < len(ins) && ins[j+1].Line-1-ctx <= end {
				j++
				end = min(len(lines), ins[j].Line-1+ctx)
			}
			n := j - i + 1
			fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", start+1, end-start, start+1+offset, end-start+n)
			k := i
			for l := start; l < end; l++ {
				for k <= j && ins[k].Line-1 == l {
					sb.WriteString("+" + ins[k].Marker + "\n")
					k++
				}
				sb.WriteString(" " + lines[l] + "\n")
			}
			for ; k <= j; k++ {
				sb.WriteString("+" + ins[k].Marker + "\n")
			}
			offset += n
			i = j + 1
		}
	}
	return sb.String(), nil
}

// Write inserts the proposals into the files.
func Write(root string, ps []Proposal) error {
	byFile := map[string][]Proposal{}
	for _, p := range ps {
		byFile[p.Path] = append(byFile[p.Path], p)
	}
	for f, ins := range byFile {
		path := filepath.Join(root, filepath.FromSlash(f))
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.SplitAfter(string(b), "\n")
		sort.Slice(ins, func(i, j int) bool { return ins[i].Line > ins[j].Line })
		for _, p := range ins {
			at := p.Line - 1
			if at > len(lines) {
				at = len(lines)
			}
			lines = append(lines[:at], append([]string{p.Marker + "\n"}, lines[at:]...)...)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644); err != nil {
			return err
		}
	}
	return nil
}
