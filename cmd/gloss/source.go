// @gloss implements gmi:SourcePlugin scope=file

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mckalvan/glossmark/internal/infer"
	"github.com/mckalvan/glossmark/internal/infer/wire"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

func init() {
	register(command{"source", "the link sources gloss suggest reads specs through: list shows the built-in ones and the external plugins found (gloss-source-<name> on PATH, sources: in glossmark.yaml); serve runs a built-in source over the plugin protocol on stdin/stdout, contract v" + fmt.Sprint(wire.Contract) + " (docs/PLUGINS.md)", cmdSource})
}

func cmdSource(args []string) int {
	fs, root := newFlags("source")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: gloss source list\n       gloss source serve <name>\n\nlist   the built-in sources and the external ones a run here would use\nserve  speak the plugin protocol (contract v%d) for a built-in source: %s\n\nflags:\n", wire.Contract, builtinNames())
		fs.PrintDefaults()
	}
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 2
	}
	switch rest[0] {
	case "list":
		return sourceList(*root)
	case "serve":
		if len(rest) != 2 {
			fmt.Fprintf(os.Stderr, "gloss source serve: name a built-in source: %s\n", builtinNames())
			return 2
		}
		src := infer.BuiltinNamed(rest[1])
		if src == nil {
			fmt.Fprintf(os.Stderr, "gloss source serve: %q is not a built-in source; one of %s\n", rest[1], builtinNames())
			return 2
		}
		if err := wire.Serve(src, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "gloss source serve:", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(os.Stderr, "gloss source: unknown subcommand %q; list or serve\n", rest[0])
	return 2
}

func builtinNames() string {
	var names []string
	for _, s := range infer.Builtin() {
		names = append(names, s.Name())
	}
	return strings.Join(names, ", ")
}

// sourceList prints the sources a suggest run from root uses: the core
// evidence, the built-in spec sources and the external plugins, with
// where each was found. It needs no graph, only glossmark.yaml.
func sourceList(root string) int {
	if root == "" {
		cwd, _ := os.Getwd()
		r, err := load.FindRoot(cwd)
		if err != nil {
			return fail(err)
		}
		root = r
	}
	cfg, err := model.LoadConfig(root + "/" + load.ConfigName)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("%-12s %-10s %s\n", "source", "kind", "where")
	for _, name := range []string{"code", "route", "topic", "schema"} {
		fmt.Printf("%-12s %-10s %s\n", name, "core", "the runner's own evidence")
	}
	ext, err := wire.Discover(cfg, root)
	if err != nil {
		return fail(err)
	}
	replaced := map[string]bool{}
	for _, c := range ext {
		replaced[c.Name()] = true
	}
	for _, s := range infer.Builtin() {
		where := "built in"
		if replaced[s.Name()] {
			where = "built in, replaced by the external source below"
		}
		fmt.Printf("%-12s %-10s %s\n", s.Name(), "builtin", where)
	}
	for _, c := range ext {
		fmt.Printf("%-12s %-10s %s: %s\n", c.Name(), "external", c.Origin(), strings.Join(c.Command(), " "))
	}
	if len(ext) == 0 {
		fmt.Fprintln(os.Stderr, "gloss source: no external source found; put a gloss-source-<name> executable on PATH or list one under sources: in "+load.ConfigName+" (docs/PLUGINS.md)")
	}
	return 0
}
