// @gloss implements gmi:Linter scope=file

package main

import (
	"fmt"
	"os"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/marker"
)

func init() {
	register(command{"check", "lint: syntax, dangling refs, domain/range, own-subject rule, tiers, allowed lists", cmdCheck})
}

func cmdCheck(args []string) int {
	fs, root := newFlags("check")
	warnErr := fs.Bool("strict", false, "treat warnings as errors")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	diags := append([]marker.Diagnostic{}, p.Diags...)
	diags = append(diags, check.Run(p.WS, p.Graph)...)
	printDiags(diags)
	errs, warns := 0, 0
	for _, d := range diags {
		if d.Severity == "error" {
			errs++
		} else {
			warns++
		}
	}
	fmt.Fprintf(os.Stderr, "gloss check: %s, %s\n", plural(errs, "error"), plural(warns, "warning"))
	if errs > 0 || (*warnErr && warns > 0) {
		return 1
	}
	return 0
}
