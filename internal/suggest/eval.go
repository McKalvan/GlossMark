// @gloss implements gmi:SuggestCalibration scope=file

package suggest

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/calib"
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

// LabelsFromGraph lists the accepted inline links of a built workspace as
// labels: the markers a reviewed repository carries are the truth its
// stripped copy is scored against. A `@gloss?` proposal is not a label.
func LabelsFromGraph(g *graph.Graph) []Label {
	var out []Label
	seen := map[Label]bool{}
	for _, e := range g.Edges() {
		if !graph.LinkSource(e.Source) || e.Prov == nil || e.Prov.Proposed {
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
	Name string
	// Rule is the auto-accept rule in force (glossmark.yaml, else the
	// default target on the shipped calibration); Auto counts this batch's
	// proposals at or above its threshold.
	Rule calib.AutoAccept
	// Own is the rule this batch alone derives for the same target: what
	// gloss suggest --calibrate --write would put in force.
	Own      calib.AutoAccept
	Rows     []BandRow
	All      BandRow
	Kept     BandRow // conf >= Rule.MinConf: what gloss suggest outputs
	Auto     BandRow // auto-accepted under Rule: conf >= Threshold, target not deprecated
	Labels   int     // labels in the truth set
	Recalled int     // labels some correct proposal states
	ByBasis  map[string]*BandRow
	// BySource counts per inference source (code, asyncapi, openapi,
	// route, topic, schema; #260): its proposals, the labels its correct
	// proposals recall, and what the rule in force auto-accepts of it.
	BySource map[string]*SourceRow
}

// SourceRow counts one source's proposals.
type SourceRow struct {
	BandRow
	Recalled int
	Auto     BandRow
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

// Evaluate builds the calibration report of a scored batch under the
// auto-accept rule in force.
func Evaluate(name string, scored []Scored, labels []Label, rule calib.AutoAccept) Report {
	rep := Report{Name: name, Rule: rule, Labels: len(labels), ByBasis: map[string]*BandRow{}, BySource: map[string]*SourceRow{}}
	target := rule.Target
	if rule.Fixed || target <= 0 {
		target = calib.DefaultPrecision
	}
	carried := map[string]float64{}
	for _, s := range scored {
		carried[s.Basis] = s.Conf
	}
	rep.Own = calib.Derive(func(b string) float64 { return carried[b] }, Fit(scored), target, rule.MinConf)
	rep.Own.Source = "this eval"
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
		if s.Conf >= rule.MinConf-1e-9 {
			rep.Kept.add(s)
		}
		if s.AutoAccepts(rule.Threshold) {
			rep.Auto.add(s)
		}
		b := rep.ByBasis[s.Basis]
		if b == nil {
			b = &BandRow{}
			rep.ByBasis[s.Basis] = b
		}
		b.add(s)
		src := rep.source(s)
		src.add(s)
		if s.AutoAccepts(rule.Threshold) {
			src.Auto.add(s)
		}
	}
	recalled := map[Label]bool{}
	bySource := map[string]map[Label]bool{}
	for _, l := range labels {
		for _, s := range scored {
			if s.Verdict != Correct || s.Path != l.Path || s.Relation != l.Relation || s.Full != l.Target {
				continue
			}
			if l.Symbol == "*" || s.Symbol == l.Symbol || s.Scope == "file" {
				recalled[l] = true
				if bySource[s.Source] == nil {
					bySource[s.Source] = map[Label]bool{}
				}
				bySource[s.Source][l] = true
			}
		}
	}
	rep.Recalled = len(recalled)
	for src, ls := range bySource {
		rep.BySource[src].Recalled = len(ls)
	}
	return rep
}

func (r *Report) source(s Scored) *SourceRow {
	name := s.Source
	if name == "" {
		name = SourceCode
	}
	row := r.BySource[name]
	if row == nil {
		row = &SourceRow{}
		r.BySource[name] = row
	}
	return row
}

// Target is the precision the report's gate checks: the configured
// auto-accept precision, or the default when the threshold is fixed by
// policy.soft-conf-threshold.
func (r Report) Target() float64 {
	if r.Rule.Fixed || r.Rule.Target <= 0 {
		return calib.DefaultPrecision
	}
	return r.Rule.Target
}

// GateMet reports the Phase 2 gate, per repository (ADR 0024): the
// proposals the rule in force auto-accepts are at least Target() correct
// on this batch. Auto-accepting nothing does not meet it.
func (r Report) GateMet() bool {
	return r.Auto.N > 0 && r.Auto.Precision() >= r.Target()-1e-9
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
	sb.WriteString(r.BandTable())
	fmt.Fprintf(&sb, "  recall: %d of %d labels (%s)\n", r.Recalled, r.Labels, pct(r.Recalled, r.Labels))
	fmt.Fprintf(&sb, "  in force: %s\n", r.Rule)
	if r.Rule.Threshold > 1 {
		fmt.Fprintf(&sb, "  auto-accept on this eval: nothing (no threshold in force)\n")
	} else {
		fmt.Fprintf(&sb, "  auto-accept on this eval (conf >= %.2f): %d of %d at conf >= %.2f (%s), precision %s\n", r.Rule.Threshold, r.Auto.N, r.Kept.N, r.Rule.MinConf, pct(r.Auto.N, r.Kept.N), pct(r.Auto.Correct, r.Auto.N))
	}
	fmt.Fprintf(&sb, "  this eval alone: %s\n", r.Own)
	met := "NOT MET"
	if r.GateMet() {
		met = "MET"
	}
	fmt.Fprintf(&sb, "  Phase 2 gate (auto-accepted proposals at least %.0f%% correct on this eval): %s\n", 100*r.Target(), met)
	return sb.String()
}

// BandTable renders the precision per confidence band.
func (r Report) BandTable() string {
	var sb strings.Builder
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
	return sb.String()
}

// SourceTable renders precision and recall per inference source (#260):
// what each kind of spec contributed, how often it was right, how many
// labels it recalled and how much of it the rule in force auto-accepts.
func (r Report) SourceTable() string {
	var sb strings.Builder
	keys := make([]string, 0, len(r.BySource))
	for k := range r.BySource {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(&sb, "  %-10s %5s %7s %5s %5s %9s %9s %9s %12s\n", "source", "n", "correct", "near", "wrong", "precision", "recalled", "auto", "auto prec.")
	for _, k := range keys {
		b := r.BySource[k]
		fmt.Fprintf(&sb, "  %-10s %5d %7d %5d %5d %9s %9s %9d %12s\n", k, b.N, b.Correct, b.Near, b.Wrong, pct(b.Correct, b.N), fmt.Sprintf("%d/%d", b.Recalled, r.Labels), b.Auto.N, pct(b.Auto.Correct, b.Auto.N))
	}
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
