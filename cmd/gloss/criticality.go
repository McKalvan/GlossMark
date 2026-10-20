// @gloss implements gmi:CriticalityStamper scope=file

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mckalvan/glossmark/internal/ontoedit"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/textdiff"
)

func init() {
	register(command{"criticality", "gloss criticality suggest: propose a first classification (constraints, published concepts, security-verified concepts → high) as a diff; --write applies it", cmdCriticality})
}

func cmdCriticality(args []string) int {
	fs, root := newFlags("criticality")
	write := fs.Bool("write", false, "write the suggestions into the ontology files instead of printing a diff")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || fs.Arg(0) != "suggest" {
		fmt.Fprintln(os.Stderr, "usage: gloss criticality suggest [--write]")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	sugg := rollup.SuggestCriticality(p.Graph)
	if len(sugg) == 0 {
		fmt.Fprintln(os.Stderr, "gloss criticality: nothing to suggest; every constraint, published concept and security-verified concept is classified")
		return 0
	}
	// Apply to scratch copies (or the files with --write) and show the diff.
	type file struct{ path, before string }
	files := map[string]*file{}
	tmp, err := os.MkdirTemp("", "gloss-crit-")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(tmp)
	for _, s := range sugg {
		ns, name, _ := splitCurie(s.Concept)
		path, err := ontologyPath(p, ns)
		if err != nil {
			continue
		}
		f, ok := files[path]
		if !ok {
			b, err := os.ReadFile(path)
			if err != nil {
				return fail(err)
			}
			f = &file{path: path, before: string(b)}
			files[path] = f
			if !*write {
				if err := os.WriteFile(filepath.Join(tmp, ns+".yaml"), b, 0o644); err != nil {
					return fail(err)
				}
			}
		}
		target := path
		if !*write {
			target = filepath.Join(tmp, ns+".yaml")
		}
		if err := ontoedit.SetField(target, name, "criticality", s.Level); err != nil {
			fmt.Fprintf(os.Stderr, "gloss criticality: %s: %v\n", s.Concept, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "  %s → %s  (%s)\n", s.Concept, s.Level, s.Reason)
	}
	if *write {
		fmt.Fprintf(os.Stderr, "gloss criticality: %s classified; review the diff, then gloss build\n", plural(len(sugg), "concept"))
		return 0
	}
	for path, f := range files {
		ns := strings.TrimSuffix(filepath.Base(path), ".gloss.yaml")
		after, _ := os.ReadFile(filepath.Join(tmp, ns+".yaml"))
		fmt.Print(textdiff.Unified(relOrAbs(p.WS.Root, path), f.before, string(after)))
	}
	fmt.Fprintf(os.Stderr, "gloss criticality: %s; apply with git apply or gloss criticality suggest --write\n", plural(len(sugg), "suggestion"))
	return 0
}
