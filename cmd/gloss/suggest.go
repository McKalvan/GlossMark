// @gloss implements gmi:MarkerSuggester scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mckalvan/glossmark/internal/suggest"
)

func init() {
	register(command{"suggest", "propose @gloss? markers for unmarked declarations by matching names and docstrings to concepts; prints a patch (--write to apply, --format json)", cmdSuggest})
}

func cmdSuggest(args []string) int {
	fs, root := newFlags("suggest")
	write := fs.Bool("write", false, "insert the proposals into the files instead of printing a patch")
	format := fs.String("format", "patch", "patch | json | text")
	minScore := fs.Int("min-score", suggest.DefaultMinScore, "lowest score that becomes a proposal (conf is score/110; a docstring match scores 20 below its search score)")
	if err := fs.Parse(flagsLast(args)); err != nil {
		return 2
	}
	if *minScore < 1 {
		fmt.Fprintln(os.Stderr, "gloss suggest: --min-score must be at least 1")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	ps, err := suggest.Run(p.WS, p.Graph, fs.Args(), suggest.Options{MinScore: *minScore})
	if err != nil {
		return fail(err)
	}
	switch {
	case *write:
		if err := suggest.Write(p.WS.Root, ps); err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "gloss suggest: %s written as @gloss? proposals; review them, then gloss check && gloss build\n", plural(len(ps), "marker"))
	case *format == "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(ps)
	case *format == "text":
		for _, s := range ps {
			fmt.Printf("%s:%d  %s  %s %s  conf=%.2f  [%s]\n", s.Path, s.Line, s.Symbol, s.Relation, s.Target, s.Conf, s.Matched)
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
		fmt.Fprintf(os.Stderr, "gloss suggest: %s in %s; apply with git apply, then review every one\n", plural(len(ps), "proposal"), plural(len(files), "file"))
	}
	return 0
}
