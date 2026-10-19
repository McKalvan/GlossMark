// @gloss implements gmi:Scaffolder scope=file

package main

import (
	"fmt"
	"os"

	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/scaffold"
)

func init() {
	register(command{"init", "scaffold a repository: glossmark.yaml, tier files, manifests, views, .gitignore, task target, CI job, AGENTS.md rules block, hook (--dry-run prints the plan); --role <name> --may-modify <targets> writes only a role agent, .claude/agents/<name>.md", cmdInit})
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
	role := fs.String("role", "", "write only a role agent, .claude/agents/<role>.md, a Claude Code subagent that runs as agent:<role> (lower-case words and hyphens)")
	mayModify := fs.String("may-modify", "", "with --role: the role's remit, concept CURIEs and globs, comma-separated (\"pay:Refund, pa:*\")")
	instructs := fs.String("instructs", "", "with --role: the concept the role is about (pay:Refunding)")
	description := fs.String("description", "", "with --role: when Claude Code should use the subagent (default: derived from --may-modify)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *role != "" {
		return initRole(*root, *dry, scaffold.Role{Name: *role, MayModify: *mayModify, Instructs: *instructs, Description: *description})
	}
	if *mayModify != "" || *instructs != "" || *description != "" {
		fmt.Fprintln(os.Stderr, "gloss init: --may-modify, --instructs and --description need --role <name>")
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

// initRole writes one role agent file at the repository root (the nearest
// glossmark.yaml above the working directory, else the working directory).
func initRole(root string, dry bool, r scaffold.Role) int {
	dir := root
	if dir == "" {
		wd, _ := os.Getwd()
		dir = wd
		if found, err := load.FindRoot(wd); err == nil {
			dir = found
		}
	}
	plan, err := scaffold.RolePlan(dir, r)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("role agent %s: actor agent:%s\n", r.Name, r.Name)
	fmt.Print(plan.Files[0].String())
	if dry {
		fmt.Println("gloss init: dry run; nothing written")
		return 0
	}
	if err := plan.Apply(); err != nil {
		return fail(err)
	}
	if plan.Files[0].Action == scaffold.Skip {
		return 0
	}
	fmt.Printf("gloss init: next: gloss check, then describe the role's job in %s. Inside the subagent the guard judges edits against its remit; run it on its own with GLOSS_ACTOR=agent:%s\n", plan.Files[0].Path, r.Name)
	return 0
}
