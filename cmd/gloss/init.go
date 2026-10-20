// @gloss implements gmi:Scaffolder scope=file

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mckalvan/glossmark/internal/scaffold"
)

func init() {
	register(command{"init", "scaffold a repository: glossmark.yaml, tier files, manifests, views, .gitignore, task target, CI job, hook, first graph (--dry-run prints the plan)", cmdInit})
}

func cmdInit(args []string) int {
	fs, root := newFlags("init")
	dry := fs.Bool("dry-run", false, "print the plan and write nothing")
	name := fs.String("name", "", "repository name (default: the directory name)")
	core := fs.String("core", "", "core namespace prefix (default: guessed from the name, or the imported core namespace)")
	impl := fs.String("impl", "", "implementation namespace prefix (default: core + i)")
	bprefix := fs.String("boundary-prefix", "", "boundary prefix (default: core + b)")
	iri := fs.String("iri", "", "base IRI shared by sibling repositories: namespaces become <iri><prefix>/, the repository <iri><name>/ (default https://onto.example.com/<name>/)")
	var imports []string
	fs.Func("import", "a sibling workspace whose core namespace this repository imports instead of owning (repeatable); init then runs gloss lock", func(s string) error {
		imports = append(imports, s)
		return nil
	})
	noHook := fs.Bool("no-hook", false, "do not run gloss hook install")
	actor := fs.String("actor", "agent:claude-code", "GLOSS_ACTOR for the installed hook and MCP server")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	dir := *root
	if dir == "" {
		dir, _ = os.Getwd()
	}
	plan, err := scaffold.Make(dir, scaffold.Options{Name: *name, Core: *core, Impl: *impl, IRIBase: *iri, BoundaryPrefix: *bprefix, Imports: imports})
	if err != nil {
		return fail(err)
	}
	fmt.Print(plan.String())
	// What runs after the files are written, so the plan lists every write.
	if len(plan.Imported) > 0 {
		fmt.Printf("  then   gloss lock (pins %s: glossmark.lock, .glossmark/imports/)\n", strings.Join(plan.Imported, ":, "))
	}
	if !*noHook {
		fmt.Println("  then   gloss hook install (.claude/settings.json, .mcp.json; --no-hook to skip)")
	}
	fmt.Println("  then   gloss build (.glossmark/graph.jsonl, .glossmark/ledger.jsonl)")
	if *dry {
		fmt.Println("gloss init: dry run; nothing written")
		return 0
	}
	if err := plan.Apply(); err != nil {
		return fail(err)
	}
	if len(plan.Imported) > 0 {
		if rc := cmdLock([]string{"--root", plan.Root}); rc != 0 {
			return rc
		}
	}
	if !*noHook {
		if err := installHooks(plan.Root, *actor, true, true); err != nil {
			return fail(err)
		}
	}
	p, err := buildGraph(plan.Root, true)
	if err != nil {
		return fail(err)
	}
	if err := writeGraph(p.Graph, p.WS.StatePath("graph.jsonl")); err != nil {
		return fail(err)
	}
	next := "name the core concepts in ontology/" + plan.Core + ".gloss.yaml"
	if len(plan.Imported) > 0 {
		next = "name the mechanisms in ontology/" + plan.Impl + ".gloss.yaml (realizes: " + plan.Core + ":...)"
	}
	if len(plan.Vocabulary) > 0 {
		next = "import the vocabulary (" + strings.Join(plan.Vocabulary, "; ") + "), review the guessed kinds"
	}
	fmt.Println("gloss init: done; graph built. Next: " + next + ", mark the code (docs/MARKING.md), then gloss check && gloss build && gloss drift")
	return 0
}
