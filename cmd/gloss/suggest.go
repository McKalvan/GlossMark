// @gloss implements gmi:MarkerSuggester scope=file
// @gloss produces gm:ProposedLink scope=file

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/mckalvan/glossmark/internal/calib"
	"github.com/mckalvan/glossmark/internal/suggest"
)

func init() {
	register(command{"suggest", "propose @gloss? markers for unmarked declarations from names, docstrings, tables touched and calls; prints a patch (--write to apply, --format json, --eval labels.tsv, --calibrate, --concepts)", cmdSuggest})
}

// suggestReport is gloss suggest --format json: the auto-accept rule in
// force and the proposals.
type suggestReport struct {
	AutoAccept calib.AutoAccept   `json:"auto_accept"`
	Proposals  []suggest.Proposal `json:"proposals"`
}

func cmdSuggest(args []string) int {
	fs, root := newFlags("suggest")
	write := fs.Bool("write", false, "insert the proposals into the files instead of printing a patch; with --calibrate, write the measurement to suggest.calibration")
	format := fs.String("format", "patch", "patch | json | text")
	minConf := fs.Float64("min-conf", suggest.DefaultMinConf, "lowest conf that becomes a proposal; conf is the precision the proposal's evidence measured on the calibration set (default: suggest.min-conf in glossmark.yaml, else 0.3)")
	precision := fs.Float64("auto-accept-precision", 0, "precision auto-accepted proposals must reach; the auto-accept threshold is derived from it (default: suggest.auto-accept-precision, else policy.soft-conf-threshold as a fixed threshold, else 0.9)")
	minScore := fs.Int("min-score", 0, "deprecated: --min-conf score/110")
	eval := fs.String("eval", "", "score the proposals against a labels file (path, symbol or *, relation, target; tab-separated) and print precision and auto-accept rate per confidence band; with --calibrate, the labels to score against")
	calibrate := fs.Bool("calibrate", false, "measure suggest on this workspace: strip its markers in a temporary copy, suggest on it, and score against its own links (or suggest.labels, or --eval); --write saves the counts to suggest.calibration")
	labels := fs.Bool("labels", false, "print the workspace's own links as a labels file for --eval on a marker-stripped copy")
	concepts := fs.Bool("concepts", false, "propose new concepts (ns:NEW:Name) from clusters of declarations that share tables and docstring words, for gloss propose --from-new")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: gloss suggest [flags] [paths]\n\n%s\n\nRepository defaults for --min-conf, --auto-accept-precision and the labels and calibration files live under suggest: in glossmark.yaml; flags override them.\n\nflags:\n", suggest.Coverage)
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if *minScore > 0 {
		*minConf = math.Round(float64(*minScore)/110*100) / 100
		set["min-conf"] = true
	}
	if *minConf <= 0 || *minConf > 1 {
		fmt.Fprintln(os.Stderr, "gloss suggest: --min-conf must be in (0, 1]")
		return 2
	}
	if set["auto-accept-precision"] && (*precision <= 0 || *precision > 1) {
		fmt.Fprintln(os.Stderr, "gloss suggest: --auto-accept-precision must be in (0, 1]")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	cfg := p.WS.Config
	if !set["min-conf"] {
		*minConf = cfg.SuggestMinConf()
	}
	rule := cfg.AutoAcceptWith(*precision, *minConf)
	if *labels {
		for _, l := range suggest.LabelsFromGraph(p.Graph) {
			fmt.Println(l.String())
		}
		return 0
	}
	for _, h := range suggest.Hints(p.WS, p.Graph) {
		fmt.Fprintln(os.Stderr, "gloss suggest: "+h)
	}
	for _, n := range cfg.AutoAcceptNotes() {
		fmt.Fprintln(os.Stderr, "gloss suggest: "+n)
	}
	if *concepts {
		cs := suggest.Concepts(p.WS, p.Graph, fs.Args())
		if *format == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(cs)
			return 0
		}
		for _, c := range cs {
			fmt.Println(c.String())
		}
		fmt.Fprintf(os.Stderr, "gloss suggest: %s; write the ones you keep into the ontology, or mark code with them and run gloss propose --from-new\n", plural(len(cs), "concept proposal"))
		return 0
	}
	if *calibrate {
		return suggestCalibrate(p, *eval, *write, *format, rule)
	}
	opt := suggest.Options{MinConf: *minConf, Ledger: p.Ledger.Entries}
	if *eval != "" {
		opt.MinConf = 0.01 // the report shows every band
	}
	ps, skipped, err := suggest.Run(p.WS, p.Graph, fs.Args(), opt)
	if err != nil {
		return fail(err)
	}
	auto := 0
	for i := range ps {
		ps[i].AutoAccept = ps[i].AutoAccepts(rule.Threshold)
		if ps[i].AutoAccept {
			auto++
		}
	}
	autoNote := func() {
		fmt.Fprintf(os.Stderr, "gloss suggest: %d of %d at or above the threshold; %s\n", auto, len(ps), rule)
	}
	switch {
	case *eval != "":
		ls, err := readLabels(*eval)
		if err != nil {
			return fail(err)
		}
		rep := suggest.Evaluate(*eval, suggest.Score(p.WS, ps, ls), ls, rule)
		if *format == "json" {
			return printJSON(rep)
		}
		fmt.Print(rep.String())
		fmt.Print(rep.BasisTable())
	case *write:
		if err := suggest.Write(p.WS.Root, ps); err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "gloss suggest: %s written as @gloss? proposals%s; review them, then gloss check && gloss build\n", plural(len(ps), "marker"), skippedNote(skipped))
		autoNote()
	case *format == "json":
		return printJSON(suggestReport{AutoAccept: rule, Proposals: ps})
	case *format == "text":
		for _, s := range ps {
			scope := ""
			if s.Scope != "" {
				scope = " scope=" + s.Scope
			}
			ctx := ""
			if s.Context != "" {
				ctx = "  (" + s.Context + ")"
			}
			if s.Note != "" {
				ctx += "  (" + s.Note + ")"
			}
			tag := "review"
			if s.AutoAccept {
				tag = "auto"
			}
			fmt.Printf("%s:%d  %s  %s %s%s  conf=%.2f %s  [%s]%s\n", s.Path, s.Line, s.Symbol, s.Relation, s.Target, scope, s.Conf, tag, s.Matched, ctx)
		}
		autoNote()
	default:
		patch, err := suggest.Patch(p.WS.Root, ps)
		if err != nil {
			return fail(err)
		}
		fmt.Print(patch)
		files := map[string]bool{}
		for _, s := range ps {
			files[s.Path] = true
		}
		fmt.Fprintf(os.Stderr, "gloss suggest: %s in %s%s; apply with git apply, then review every one\n", plural(len(ps), "proposal"), plural(len(files), "file"), skippedNote(skipped))
		autoNote()
	}
	return 0
}

// suggestCalibrate is gloss suggest --calibrate: the calibration harness
// on this workspace.
func suggestCalibrate(p *pipeline, evalPath string, write bool, format string, rule calib.AutoAccept) int {
	cfg := p.WS.Config
	if evalPath == "" && cfg.Suggest != nil && cfg.Suggest.Labels != "" {
		evalPath = cfg.SuggestPath(cfg.Suggest.Labels)
	}
	var ls []suggest.Label
	name := "this workspace's own links"
	if evalPath != "" {
		var err error
		if ls, err = readLabels(evalPath); err != nil {
			return fail(err)
		}
		name = evalPath
	}
	m, err := suggest.SelfEval(p.WS, p.Graph, ls)
	if err != nil {
		return fail(err)
	}
	rep := suggest.Evaluate("calibration on "+name, m.Scored, m.Labels, rule)
	if write {
		if cfg.Suggest == nil || cfg.Suggest.Calibration == "" {
			fmt.Fprintln(os.Stderr, "gloss suggest: --calibrate --write needs a file to write: add\n  suggest:\n    calibration: glossmark.calibration.tsv\nto glossmark.yaml")
			return 2
		}
		var b bytes.Buffer
		header := fmt.Sprintf("gloss suggest --calibrate: %d proposals scored against %d labels (%s), %s.\nThe auto-accept threshold is derived from these counts (ADR 0024); conf still comes from the shipped table.", len(m.Scored), len(m.Labels), name, time.Now().UTC().Format("2006-01-02"))
		calib.Write(&b, suggest.Fit(m.Scored), header)
		if err := os.WriteFile(cfg.SuggestPath(cfg.Suggest.Calibration), b.Bytes(), 0o644); err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "gloss suggest: wrote %s; from now on %s\n", cfg.Suggest.Calibration, rep.Own)
	}
	if format == "json" {
		return printJSON(rep)
	}
	fmt.Print(rep.String())
	fmt.Print(rep.BasisTable())
	return 0
}

func readLabels(path string) ([]suggest.Label, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return suggest.ReadLabels(f)
}

func printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fail(err)
	}
	return 0
}

func skippedNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(", %d skipped (rejected before)", n)
}
