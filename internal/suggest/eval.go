// @gloss implements gmi:SuggestCalibration scope=file

package suggest

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
)

// Label is one link a reviewer accepted: the truth a batch of proposals is
// scored against. Symbol is the anchored declaration, or "*" for a link on
// the whole file (scope=file).
type Label struct {
	Path, Symbol, Relation, Target string
}

// Verdict of one proposal against the labels.
const (
	Correct = "correct" // a label has the same file, symbol (or the whole file), relation and target
	Near    = "near"    // a label sits on the same declaration with another relation or target
	Wrong   = "wrong"
)

// LabelsFromGraph lists the inline links of a built workspace as labels:
// the markers a reviewed repository carries are the truth its stripped copy
// is scored against.
func LabelsFromGraph(g *graph.Graph) []Label {
	var out []Label
	seen := map[Label]bool{}
	for _, e := range g.Edges() {
		if e.Source != graph.SourceInline || e.Prov == nil {
			continue
		}
		on := g.Node(e.O)
		if on == nil || on.CURIE == "" {
			continue
		}
		sym := e.Prov.Symbol
		if e.Prov.Scope == "file" || sym == "" {
			sym = "*"
		}
		l := Label{e.Prov.Path, sym, e.P, on.CURIE}
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func (l Label) String() string {
	return strings.Join([]string{l.Path, l.Symbol, l.Relation, l.Target}, "\t")
}

// ReadLabels reads labels as tab-separated lines: path, symbol (or * for
// the file), relation, target CURIE. Blank lines and # comments are skipped.
func ReadLabels(r io.Reader) ([]Label, error) {
	var out []Label
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		t := strings.TrimSpace(sc.Text())
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		f := strings.Split(t, "\t")
		if len(f) != 4 {
			return nil, fmt.Errorf("labels line %d: want 4 tab-separated fields (path, symbol or *, relation, target), got %d", n, len(f))
		}
		out = append(out, Label{f[0], f[1], f[2], f[3]})
	}
	return out, sc.Err()
}

// Scored is a proposal with its verdict.
type Scored struct {
	Proposal
	Full    string `json:"full_target"` // Target with the .Name shorthand expanded
	Verdict string `json:"verdict"`
}

// Score labels each proposal correct, near or wrong. Targets written with
// the default-namespace shorthand (.Name) are expanded through ws.
func Score(ws *load.Workspace, ps []Proposal, labels []Label) []Scored {
	type key struct{ path, sym string }
	at := map[key][]Label{}
	for _, l := range labels {
		at[key{l.Path, l.Symbol}] = append(at[key{l.Path, l.Symbol}], l)
	}
	inFile := map[string][]Label{}
	for _, l := range labels {
		inFile[l.Path] = append(inFile[l.Path], l)
	}
	out := make([]Scored, 0, len(ps))
	for _, p := range ps {
		target := fullTarget(ws, p.Path, p.Target)
		same := func(l Label) bool { return l.Relation == p.Relation && l.Target == target }
		v := Wrong
		sym := p.Symbol
		if p.Scope == "file" {
			sym = "*"
		}
		switch {
		case anyLabel(at[key{p.Path, sym}], same), anyLabel(at[key{p.Path, "*"}], same),
			p.Scope != "symbol" && p.Scope != "" && anyLabel(inFile[p.Path], same):
			v = Correct
		case len(at[key{p.Path, sym}]) > 0:
			v = Near
		}
		out = append(out, Scored{p, target, v})
	}
	return out
}

func anyLabel(ls []Label, f func(Label) bool) bool {
	for _, l := range ls {
		if f(l) {
			return true
		}
	}
	return false
}

func fullTarget(ws *load.Workspace, path, target string) string {
	if strings.HasPrefix(target, ".") && ws != nil {
		if m := ws.ManifestFor(path); m != nil && m.DefaultNamespace != "" {
			return m.DefaultNamespace + ":" + target[1:]
		}
	}
	return target
}

// Bands are the confidence bands a report groups proposals into, lowest
// first; each is [lo, next lo).
var Bands = []float64{0, 0.5, 0.6, 0.7, 0.8, 0.9}

// Report is the calibration table of one scored batch.
type Report struct {
	Name      string
	Threshold float64 // soft-conf-threshold: proposals at or above it are auto-accepted
	Rows      []BandRow
	All       BandRow
	Auto      BandRow // conf >= Threshold
	Labels    int     // labels in the truth set
	Recalled  int     // labels some correct proposal states
	ByBasis   map[string]*BandRow
}

// BandRow counts verdicts in one band.
type BandRow struct {
	Lo, Hi           float64
	N, Correct, Near int
	SumConf          float64
	Wrong            int
}

// Precision is the share of correct proposals; -1 when the row is empty.
func (r BandRow) Precision() float64 {
	if r.N == 0 {
		return -1
	}
	return float64(r.Correct) / float64(r.N)
}

// MeanConf is the mean conf of the row's proposals.
func (r BandRow) MeanConf() float64 {
	if r.N == 0 {
		return 0
	}
	return r.SumConf / float64(r.N)
}

func (r *BandRow) add(s Scored) {
	r.N++
	r.SumConf += s.Conf
	switch s.Verdict {
	case Correct:
		r.Correct++
	case Near:
		r.Near++
	default:
		r.Wrong++
	}
}

// Evaluate builds the calibration report of a scored batch.
func Evaluate(name string, scored []Scored, labels []Label, threshold float64) Report {
	rep := Report{Name: name, Threshold: threshold, Labels: len(labels), ByBasis: map[string]*BandRow{}}
	for i, lo := range Bands {
		hi := 1.01
		if i+1 < len(Bands) {
			hi = Bands[i+1]
		}
		rep.Rows = append(rep.Rows, BandRow{Lo: lo, Hi: hi})
	}
	for _, s := range scored {
		rep.All.add(s)
		for i := range rep.Rows {
			if s.Conf >= rep.Rows[i].Lo && s.Conf < rep.Rows[i].Hi {
				rep.Rows[i].add(s)
			}
		}
		if s.Conf >= threshold {
			rep.Auto.add(s)
		}
		b := rep.ByBasis[s.Basis]
		if b == nil {
			b = &BandRow{}
			rep.ByBasis[s.Basis] = b
		}
		b.add(s)
	}
	recalled := map[Label]bool{}
	for _, l := range labels {
		for _, s := range scored {
			if s.Verdict != Correct || s.Path != l.Path || s.Relation != l.Relation || s.Full != l.Target {
				continue
			}
			if l.Symbol == "*" || s.Symbol == l.Symbol || s.Scope == "file" {
				recalled[l] = true
			}
		}
	}
	rep.Recalled = len(recalled)
	return rep
}

// GateMet reports the Phase 2 gate: at least 80 % of proposals
// auto-accepted, at least 90 % of those correct.
func (r Report) GateMet() bool {
	return r.All.N > 0 && float64(r.Auto.N) >= 0.8*float64(r.All.N) && r.Auto.Precision() >= 0.9
}

func pct(a, b int) string {
	if b == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(a)/float64(b))
}

// String renders the report as the table gloss suggest --eval prints.
func (r Report) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s: %d proposals, %d labels\n", r.Name, r.All.N, r.Labels)
	fmt.Fprintf(&sb, "  %-9s %5s %7s %5s %5s %9s %9s\n", "band", "n", "correct", "near", "wrong", "precision", "mean conf")
	row := func(name string, b BandRow) {
		p := "-"
		if b.N > 0 {
			p = pct(b.Correct, b.N)
		}
		mc := "-"
		if b.N > 0 {
			mc = fmt.Sprintf("%.2f", b.MeanConf())
		}
		fmt.Fprintf(&sb, "  %-9s %5d %7d %5d %5d %9s %9s\n", name, b.N, b.Correct, b.Near, b.Wrong, p, mc)
	}
	for _, b := range r.Rows {
		name := fmt.Sprintf("%.1f-%.1f", b.Lo, b.Hi)
		switch {
		case b.Lo == 0:
			name = fmt.Sprintf("<%.1f", b.Hi)
		case b.Hi > 1:
			name = fmt.Sprintf(">=%.1f", b.Lo)
		}
		row(name, b)
	}
	row("all", r.All)
	fmt.Fprintf(&sb, "  recall: %d of %d labels (%s)\n", r.Recalled, r.Labels, pct(r.Recalled, r.Labels))
	fmt.Fprintf(&sb, "  auto-accept (conf >= %.2f): %d of %d (%s), precision %s\n", r.Threshold, r.Auto.N, r.All.N, pct(r.Auto.N, r.All.N), pct(r.Auto.Correct, r.Auto.N))
	met := "NOT MET"
	if r.GateMet() {
		met = "MET"
	}
	fmt.Fprintf(&sb, "  Phase 2 gate (>= 80%% auto-accept at >= 90%% precision): %s\n", met)
	return sb.String()
}

// BasisTable renders precision per calibration basis, the table conf is
// fitted from.
func (r Report) BasisTable() string {
	var sb strings.Builder
	keys := make([]string, 0, len(r.ByBasis))
	for k := range r.ByBasis {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(&sb, "  %-28s %5s %7s %9s %9s\n", "basis", "n", "correct", "precision", "conf")
	for _, k := range keys {
		b := r.ByBasis[k]
		fmt.Fprintf(&sb, "  %-28s %5d %7d %9s %9.2f\n", k, b.N, b.Correct, pct(b.Correct, b.N), b.MeanConf())
	}
	return sb.String()
}
