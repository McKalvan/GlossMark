// @gloss implements gmi:ComplianceReporter scope=file

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/report"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func init() {
	register(command{"report", "audit reports from the graph, the ledger and drift: report compliance [--concept c]... [--criticality level] [--since date] [--format markdown|html|csv|json]", cmdReport})
}

// stringList is a repeatable flag; a value may also hold several items
// separated by commas.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			*s = append(*s, x)
		}
	}
	return nil
}

func cmdReport(args []string) int {
	fs, root := newFlags("report")
	var concepts stringList
	fs.Var(&concepts, "concept", "report on this concept (repeatable, or comma-separated); default: every constraint and every critical concept")
	crit := fs.String("criticality", "", "report on every concept at this level or above (low | medium | high | critical)")
	since := fs.String("since", "", "keep only ledger decisions on or after this date (YYYY-MM-DD)")
	format := fs.String("format", "markdown", "markdown | html | csv | json")
	out := fs.String("o", "", "write to this file instead of stdout")
	sourceURL := fs.String("source-url", "", "template paths link to, with {path} and {line} (default: source-url in glossmark.yaml)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || fs.Arg(0) != "compliance" {
		fmt.Fprintln(os.Stderr, "usage: gloss report compliance [--concept c]... [--criticality high|critical] [--since YYYY-MM-DD] [--format markdown|html|csv|json] [-o file]")
		return 2
	}
	opt := report.Options{Concepts: concepts, SourceURL: *sourceURL, Now: time.Now()}
	if *crit != "" {
		lvl := strings.TrimSuffix(strings.ToLower(*crit), "+")
		if lvl == "" || !model.ValidCriticality(lvl) {
			fmt.Fprintf(os.Stderr, "gloss report: --criticality %q is not a level; want one of %s\n", *crit, join(model.CriticalityLevels))
			return 2
		}
		opt.Criticality = lvl
	}
	if *since != "" {
		t, err := time.Parse(rollup.WaiverDate, *since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gloss report: --since %q is not a date; want YYYY-MM-DD\n", *since)
			return 2
		}
		opt.Since = t
	}
	ok := false
	for _, f := range report.Formats {
		ok = ok || f == *format
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "gloss report: unknown --format %q; want %s\n", *format, join(report.Formats))
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if opt.SourceURL == "" {
		opt.SourceURL = p.WS.Config.SourceURL
	}
	pol := p.WS.Config.EffectivePolicy()
	opt.Policy, opt.Ledger = pol, p.Ledger.Entries
	opt.Drift = rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: pol, Now: opt.Now})
	rollup.AddDeprecatedUse(opt.Drift, p.Graph, pol, opt.Now)
	rep, err := report.Compliance(p.Graph, p.WS, opt)
	if err != nil {
		return fail(err)
	}
	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		w = f
	}
	if err := report.Render(w, rep, *format); err != nil {
		return fail(err)
	}
	if *out != "" {
		fmt.Printf("gloss report: %s compliance report, %s → %s\n", *format, plural(len(rep.Concepts), "concept"), *out)
	}
	return 0
}
