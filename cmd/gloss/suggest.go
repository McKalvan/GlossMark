// @gloss implements gmi:MarkerSuggester scope=file

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/mckalvan/glossmark/internal/suggest"
)

func init() {
	register(command{"suggest", "propose @gloss? markers for unmarked declarations from names, docstrings, tables touched and calls; prints a patch (--write to apply, --format json, --eval labels.tsv, --concepts)", cmdSuggest})
}

func cmdSuggest(args []string) int {
	fs, root := newFlags("suggest")
	write := fs.Bool("write", false, "insert the proposals into the files instead of printing a patch")
	format := fs.String("format", "patch", "patch | json | text")
	minConf := fs.Float64("min-conf", suggest.DefaultMinConf, "lowest conf that becomes a proposal; conf is the precision the proposal's evidence measured on the calibration set")
	minScore := fs.Int("min-score", 0, "deprecated: --min-conf score/110")
	eval := fs.String("eval", "", "score the proposals against a labels file (path, symbol or *, relation, target; tab-separated) and print precision and auto-accept rate per confidence band")
	labels := fs.Bool("labels", false, "print the workspace's own links as a labels file for --eval on a marker-stripped copy")
	concepts := fs.Bool("concepts", false, "propose new concepts (ns:NEW:Name) from clusters of declarations that share tables and docstring words, for gloss propose --from-new")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: gloss suggest [flags] [paths]\n\n%s\n\nflags:\n", suggest.Coverage)
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if *minScore > 0 {
		*minConf = math.Round(float64(*minScore)/110*100) / 100
	}
	if *minConf <= 0 || *minConf > 1 {
		fmt.Fprintln(os.Stderr, "gloss suggest: --min-conf must be in (0, 1]")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if *labels {
		for _, l := range suggest.LabelsFromGraph(p.Graph) {
			fmt.Println(l.String())
		}
		return 0
	}
	for _, h := range suggest.Hints(p.WS, p.Graph) {
		fmt.Fprintln(os.Stderr, "gloss suggest: "+h)
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
	opt := suggest.Options{MinConf: *minConf, Ledger: p.Ledger.Entries}
	if *eval != "" {
		opt.MinConf = 0.01 // the report shows every band
	}
	ps, skipped, err := suggest.Run(p.WS, p.Graph, fs.Args(), opt)
	if err != nil {
		return fail(err)
	}
	switch {
	case *eval != "":
		f, err := os.Open(*eval)
		if err != nil {
			return fail(err)
		}
		ls, err := suggest.ReadLabels(f)
		f.Close()
		if err != nil {
			return fail(err)
		}
		rep := suggest.Evaluate(*eval, suggest.Score(p.WS, ps, ls), ls, p.WS.Config.EffectivePolicy().SoftConfThreshold)
		fmt.Print(rep.String())
		fmt.Print(rep.BasisTable())
	case *write:
		if err := suggest.Write(p.WS.Root, ps); err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "gloss suggest: %s written as @gloss? proposals%s; review them, then gloss check && gloss build\n", plural(len(ps), "marker"), skippedNote(skipped))
	case *format == "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(ps)
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
			fmt.Printf("%s:%d  %s  %s %s%s  conf=%.2f  [%s]%s\n", s.Path, s.Line, s.Symbol, s.Relation, s.Target, scope, s.Conf, s.Matched, ctx)
		}
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
	}
	return 0
}

func skippedNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(", %d skipped (rejected before)", n)
}
