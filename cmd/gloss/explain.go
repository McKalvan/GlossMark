// @gloss implements gmi:Diagnostics scope=file

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mckalvan/glossmark/internal/diag"
)

func init() {
	register(command{"explain", "explain a diagnostic code: severity, meaning, fix and doc page (GM305, a range like GM3xx, or --all; --format markdown writes docs/DIAGNOSTICS.md)", cmdExplain})
}

func cmdExplain(args []string) int {
	fs, _ := newFlags("explain")
	all := fs.Bool("all", false, "every code")
	format := fs.String("format", "text", "text | markdown | json")
	if err := fs.Parse(flagsLast(args)); err != nil {
		return 2
	}
	var codes []diag.Code
	switch {
	case *all && fs.NArg() == 0:
		codes = diag.Codes
	case !*all && fs.NArg() == 1:
		arg := fs.Arg(0)
		if c, ok := diag.Lookup(arg); ok {
			codes = []diag.Code{c}
		} else if codes = diag.Family(arg); len(codes) == 0 {
			hint := "run gloss explain --all to list every code"
			if r := diag.RangeOf(diag.Normalize(arg)); r.Prefix != "" {
				hint = fmt.Sprintf("run gloss explain %sxx for the %s codes, or --all for every code", r.Prefix, strings.ToLower(r.Title))
			}
			fmt.Fprintf(os.Stderr, "gloss explain: unknown diagnostic code %q; codes look like GM305 (%s)\n", arg, hint)
			return 1
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: gloss explain <code> | <range like GM3xx> | --all [--format text|markdown|json]")
		return 2
	}
	switch *format {
	case "text":
		fmt.Print(diag.Text(codes))
	case "json":
		fmt.Print(diag.JSON(codes))
	case "markdown":
		if *all {
			fmt.Print(diag.Markdown())
		} else {
			fmt.Print(diag.Table(codes))
		}
	default:
		fmt.Fprintf(os.Stderr, "gloss explain: unknown format %q (text, markdown or json)\n", *format)
		return 2
	}
	return 0
}
