// @gloss implements gmi:SuggestCalibration scope=file

// Package calib holds what gloss suggest's conf is read from and what the
// auto-accept threshold is derived from: the calibration counts, per
// evidence basis, of how many proposals the labelled calibration set made
// and how many were correct (ADR 0021, ADR 0024). It has no dependencies,
// so the gate policy (internal/model) and suggest both read it.
package calib

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Count is what one basis measured: N proposals, Correct of them right.
type Count struct{ N, Correct int }

// Table maps a basis (evidence/role, plus /tie) to its count.
type Table map[string]Count

// MinBasisN is how often a basis must occur to get its own conf; a rarer
// basis is read from its evidence row (all bases with that evidence).
const MinBasisN = 4

// Unmeasured is the conf of evidence the calibration set never saw.
const Unmeasured = 0.3

// DefaultMinConf is gloss suggest's floor: evidence wrong more often than
// right on the calibration set is not proposed.
const DefaultMinConf = 0.3

// DefaultPrecision is the auto-accept precision target when glossmark.yaml
// sets neither suggest.auto-accept-precision nor policy.soft-conf-threshold.
const DefaultPrecision = 0.9

// Never is the threshold when no basis meets the target: above every conf,
// so nothing is auto-accepted.
const Never = 1.01

// Evidence is the evidence part of a basis.
func Evidence(basis string) string { return strings.SplitN(basis, "/", 2)[0] }

// Smoothed is the Laplace estimate of a precision, to two decimals.
func Smoothed(correct, n int) float64 {
	return math.Round(float64(correct+1)/float64(n+2)*100) / 100
}

// Conf is the conf a basis gets from t: its own row when seen MinBasisN
// times, else its evidence row, else Unmeasured.
func (t Table) Conf(basis string) float64 {
	if c, ok := t[basis]; ok && c.N >= MinBasisN {
		return Smoothed(c.Correct, c.N)
	}
	return t.evidenceConf(Evidence(basis))
}

func (t Table) evidenceConf(ev string) float64 {
	var sum Count
	for b, c := range t {
		if Evidence(b) == ev {
			sum.N += c.N
			sum.Correct += c.Correct
		}
	}
	if sum.N > 0 {
		return Smoothed(sum.Correct, sum.N)
	}
	return Unmeasured
}

// Confs precomputes Conf for every basis of t and for every evidence row,
// for lookups in the matcher's inner loop.
func (t Table) Confs() func(string) float64 {
	by, ev := map[string]float64{}, map[string]float64{}
	for b := range t {
		by[b] = t.Conf(b)
		ev[Evidence(b)] = t.evidenceConf(Evidence(b))
	}
	return func(basis string) float64 {
		if c, ok := by[basis]; ok {
			return c
		}
		if c, ok := ev[Evidence(basis)]; ok {
			return c
		}
		return Unmeasured
	}
}

// Total sums the table.
func (t Table) Total() Count {
	var s Count
	for _, c := range t {
		s.N += c.N
		s.Correct += c.Correct
	}
	return s
}

// AutoAccept is the auto-accept rule in force and what it measures: the
// conf at or above which a proposal is an auto-accept candidate (SOFT),
// the precision proposals at or above it had on the calibration data, and
// the share of proposals (at or above min-conf) it auto-accepts.
type AutoAccept struct {
	// Target is the precision auto-accept must meet; 0 when the threshold
	// is policy.soft-conf-threshold (Fixed).
	Target float64 `json:"target,omitempty"`
	// Fixed: the threshold is policy.soft-conf-threshold, not derived.
	Fixed bool `json:"fixed,omitempty"`
	// Met: some threshold meets Target (always true when Fixed). When
	// false, Threshold is Never and nothing is auto-accepted.
	Met       bool    `json:"met"`
	Threshold float64 `json:"threshold"`
	// Precision is the measured precision at Threshold (-1 when nothing
	// reaches it); Best is the highest precision any threshold reached.
	Precision float64 `json:"precision"`
	Best      float64 `json:"best_precision"`
	// Auto of N calibration proposals at or above min-conf reach Threshold.
	Auto    int     `json:"auto"`
	N       int     `json:"n"`
	Share   float64 `json:"share"`
	MinConf float64 `json:"min_conf"`
	// Source names the calibration data: "shipped" or the repository's
	// calibration file.
	Source string `json:"source"`
}

type bucket struct {
	conf float64
	Count
}

func buckets(conf func(string) float64, t Table, minConf float64) []bucket {
	by := map[float64]*bucket{}
	for b, c := range t {
		v := conf(b)
		if v < minConf-1e-9 {
			continue
		}
		k := math.Round(v * 100)
		if by[k] == nil {
			by[k] = &bucket{conf: v}
		}
		by[k].N += c.N
		by[k].Correct += c.Correct
	}
	out := make([]bucket, 0, len(by))
	for _, b := range by {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].conf > out[j].conf })
	return out
}

// Derive finds the lowest conf at or above minConf whose cumulative
// precision (correct over all proposals at or above it) meets target on t.
// conf gives each basis the conf its proposals carry.
func Derive(conf func(string) float64, t Table, target, minConf float64) AutoAccept {
	a := AutoAccept{Target: target, Threshold: Never, Precision: -1, Best: -1, MinConf: minConf}
	var cum Count
	bs := buckets(conf, t, minConf)
	for _, b := range bs {
		a.N += b.N
	}
	for _, b := range bs {
		cum.N += b.N
		cum.Correct += b.Correct
		if cum.N == 0 {
			continue
		}
		p := float64(cum.Correct) / float64(cum.N)
		if p > a.Best {
			a.Best = p
		}
		if p >= target-1e-9 {
			a.Met, a.Threshold, a.Precision, a.Auto = true, b.conf, p, cum.N
		}
	}
	if a.N > 0 {
		a.Share = float64(a.Auto) / float64(a.N)
	}
	return a
}

// At measures a fixed threshold on t.
func At(conf func(string) float64, t Table, threshold, minConf float64) AutoAccept {
	a := AutoAccept{Fixed: true, Met: true, Threshold: threshold, Precision: -1, Best: -1, MinConf: minConf}
	var auto Count
	for _, b := range buckets(conf, t, minConf) {
		a.N += b.N
		if b.conf >= threshold-1e-9 {
			auto.N += b.N
			auto.Correct += b.Correct
		}
	}
	a.Auto = auto.N
	if auto.N > 0 {
		a.Precision = float64(auto.Correct) / float64(auto.N)
		a.Best = a.Precision
	}
	if a.N > 0 {
		a.Share = float64(a.Auto) / float64(a.N)
	}
	return a
}

// String is the one-line summary gloss suggest, --eval and doctor print.
func (a AutoAccept) String() string {
	src := "the shipped calibration"
	if a.Source != "" && a.Source != "shipped" {
		src = a.Source
	}
	pct := func(f float64) string { return fmt.Sprintf("%.1f%%", 100*f) }
	switch {
	case a.Fixed:
		p := "-"
		if a.Precision >= 0 {
			p = pct(a.Precision)
		}
		return fmt.Sprintf("auto-accept: conf >= %.2f (policy.soft-conf-threshold); on %s: precision %s, %s of proposals (%d of %d)", a.Threshold, src, p, pct(a.Share), a.Auto, a.N)
	case !a.Met:
		best := "no proposals"
		if a.Best >= 0 {
			best = "the best any threshold reaches is " + pct(a.Best)
		}
		return fmt.Sprintf("auto-accept: nothing; no conf reaches the target precision %s on %s (%s), so every proposal needs review", pct(a.Target), src, best)
	}
	return fmt.Sprintf("auto-accept: conf >= %.2f, derived from the target precision %s; on %s: precision %s, %s of proposals (%d of %d)", a.Threshold, pct(a.Target), src, pct(a.Precision), pct(a.Share), a.Auto, a.N)
}

// Read parses a calibration file: tab-separated lines of basis, proposals,
// correct. Blank lines and # comments are skipped.
func Read(r io.Reader) (Table, error) {
	t := Table{}
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("line %d: want 3 tab-separated fields (basis, proposals, correct), got %d", n, len(f))
		}
		pn, err1 := strconv.Atoi(f[1])
		pc, err2 := strconv.Atoi(f[2])
		if err1 != nil || err2 != nil || pn < 0 || pc < 0 || pc > pn {
			return nil, fmt.Errorf("line %d: proposals and correct must be counts with correct <= proposals", n)
		}
		c := t[f[0]]
		c.N += pn
		c.Correct += pc
		t[f[0]] = c
	}
	return t, sc.Err()
}

// Write renders t as a calibration file, sorted by basis.
func Write(w io.Writer, t Table, header string) error {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, l := range strings.Split(strings.TrimSpace(header), "\n") {
		if l != "" {
			sb.WriteString("# " + l + "\n")
		}
	}
	sb.WriteString("# basis\tproposals\tcorrect\n")
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s\t%d\t%d\n", k, t[k].N, t[k].Correct)
	}
	_, err := io.WriteString(w, sb.String())
	return err
}
