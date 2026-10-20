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
	minScore := fs.Int("min-score", 60, "lowest search score that becomes a proposal")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	ps, skipped, err := suggest.Run(p.WS, p.Graph, fs.Args(), suggest.Options{MinScore: *minScore, Ledger: p.Ledger.Entries})
	if err != nil {
		return fail(err)
	}
	switch {
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
