// @gloss implements gmi:Scaffolder scope=file

package main

import (
	"fmt"
	"os"

	"github.com/mckalvan/glossmark/internal/scaffold"
)

func init() {
	register(command{"init", "scaffold a repository: glossmark.yaml, tier files, manifests, views, .gitignore, task target, CI job, hook (--dry-run prints the plan)", cmdInit})
}

func cmdInit(args []string) int {
	fs, root := newFlags("init")
	dry := fs.Bool("dry-run", false, "print the plan and write nothing")
	name := fs.String("name", "", "repository name (default: the directory name)")
	core := fs.String("core", "", "core namespace prefix (default: guessed from the name)")
	impl := fs.String("impl", "", "implementation namespace prefix (default: core + i)")
	iri := fs.String("iri", "", "base IRI for the namespaces (default https://onto.example.com/<name>/)")
	noHook := fs.Bool("no-hook", false, "do not run gloss hook install")
	actor := fs.String("actor", "agent:claude-code", "GLOSS_ACTOR for the installed hook and MCP server")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := *root
	if dir == "" {
		dir, _ = os.Getwd()
	}
	plan, err := scaffold.Make(dir, scaffold.Options{Name: *name, Core: *core, Impl: *impl, IRIBase: *iri})
	if err != nil {
		return fail(err)
	}
	fmt.Print(plan.String())
	if *dry {
		fmt.Println("gloss init: dry run; nothing written")
		return 0
	}
	if err := plan.Apply(); err != nil {
		return fail(err)
	}
	if !*noHook {
		if err := installHooks(plan.Root, *actor, true, true); err != nil {
			return fail(err)
		}
	}
	if _, err := buildGraph(plan.Root, true); err != nil {
		return fail(err)
	}
	fmt.Println("gloss init: done. Next: name the core concepts in ontology/" + plan.Core + ".gloss.yaml, mark the code (docs/MARKING.md), then gloss check && gloss build && gloss drift")
	return 0
}
