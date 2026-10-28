// @gloss implements gmi:Bootstrapper scope=file
// @gloss produces gm:ProposedLink scope=file

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/mckalvan/glossmark/internal/bootstrap"
	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/suggest"
)

func init() {
	register(command{"onboard", "draft everything from an unmodified repository in one run: scaffold (gloss init), import a glossary or enums, draft core and implementation concepts from the harvested vocabulary, a boundary map from deploy manifests and Terraform, @gloss? markers, and the viewer; every concept and link is a proposal for gloss review", cmdOnboard})
}

func cmdOnboard(args []string) int {
	fs, root := newFlags("onboard")
	name := fs.String("name", "", "repository name (default: the directory name)")
	core := fs.String("namespace", "", "core namespace prefix (default: guessed from the name); implementation and boundary prefixes follow it (ns+i, ns+b)")
	fs.StringVar(core, "core", "", "same as --namespace")
	impl := fs.String("impl", "", "implementation namespace prefix (default: core + i)")
	bprefix := fs.String("boundary-prefix", "", "boundary prefix (default: core + b)")
	top := fs.Int("top", bootstrap.DefaultTop, "how many core concepts to draft from the harvested candidates, best first")
	noGit := fs.Bool("no-git", false, "skip the git history when harvesting")
	noSuggest := fs.Bool("no-suggest", false, "draft no @gloss? markers; only the ontology and the boundary map")
	open := fs.Bool("open", false, "open the viewer in the browser when done")
	by := fs.String("by", "", "proposer written on every draft (default GLOSS_ACTOR or git user)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	dir := *root
	if dir == "" {
		dir, _ = os.Getwd()
	}
	actor := *by
	if actor == "" {
		actor = reviewer(dir)
	}
	res, err := bootstrap.Run(dir, bootstrap.Options{Name: *name, Core: *core, Impl: *impl, BoundaryPrefix: *bprefix, Top: *top, By: actor, Git: !*noGit})
	if err != nil {
		return fail(err)
	}
	if res.Scaffolded {
		fmt.Printf("gloss onboard: scaffolded %s: core %s:, implementation %s:, boundaries %s: (%s; gloss init --dry-run shows the plan)\n", res.Plan.Name, res.Core, res.Impl, res.Boundary, plural(len(res.Plan.Dirs), "container manifest"))
		if res.Plan.Guessed {
			fmt.Printf("  prefixes guessed from the name %q; choose your own with --namespace <ns> before anything carries them\n", res.Plan.Name)
		}
	} else {
		fmt.Printf("gloss onboard: %s already set up (core %s:, implementation %s:); adding drafts only where nothing exists\n", res.Core+"/"+res.Impl, res.Core, res.Impl)
	}
	for _, v := range res.Vocabulary {
		fmt.Printf("  imported %s into %s: (proposed)\n", v, res.Core)
	}
	fmt.Printf("  harvested %s into %s; drafted %s into %s: and %s into %s:",
		plural(res.Observations, "observation"), plural(res.Candidates, "candidate"), plural(len(res.CoreConcepts), "core concept"), res.Core, plural(len(res.ImplConcepts), "implementation concept"), res.Impl)
	if res.SkippedExisting > 0 {
		fmt.Printf(" (%d already there)", res.SkippedExisting)
	}
	fmt.Println()
	if len(res.CoreConcepts) > 0 {
		fmt.Printf("    core: %s\n", preview(res.CoreConcepts, 12))
	}
	for _, w := range res.Workloads {
		where := ""
		if w.Dir != "" {
			where = ", built from " + w.Dir + "/"
		}
		fmt.Printf("    %s: %s %s (%s%s)\n", w.Concept, w.Kind, w.Name, w.Path, where)
	}
	for _, x := range res.Resources {
		fmt.Printf("    %s: %s %s (%s)\n", x.Concept, x.Type, x.Name, x.Path)
	}
	if len(res.Manifests) > 0 {
		fmt.Printf("  boundary map: draft realizes: in %s\n", strings.Join(res.Manifests, ", "))
	}
	// Markers: suggest with proposed concepts as targets, since every
	// concept is one; the gate classes each such link HARD (ADR 0030).
	p, err := buildGraph(res.Root, true)
	if err != nil {
		return fail(err)
	}
	if !*noSuggest {
		ps, _, err := suggest.Run(p.WS, p.Graph, nil, suggest.Options{MinConf: p.WS.Config.SuggestMinConf(), Ledger: p.Ledger.Entries, Proposed: true})
		if err != nil {
			return fail(err)
		}
		if err := suggest.Write(p.WS.Root, ps); err != nil {
			return fail(err)
		}
		files := map[string]bool{}
		for _, s := range ps {
			files[s.Path] = true
		}
		fmt.Printf("  markers: %s written as @gloss? proposals in %s\n", plural(len(ps), "link"), plural(len(files), "file"))
		if p, err = buildGraph(res.Root, true); err != nil {
			return fail(err)
		}
	}
	if err := writeGraph(p.Graph, p.WS.StatePath("graph.jsonl")); err != nil {
		return fail(err)
	}
	// Nothing drafted may fail gloss check; a warning is the review backlog
	// (an incomplete proposed concept, a link outside an allowed set).
	diags := append([]marker.Diagnostic{}, p.Diags...)
	diags = append(diags, check.Run(p.WS, p.Graph)...)
	errs, warns := 0, 0
	for _, d := range diags {
		if d.Severity == "error" {
			errs++
		} else {
			warns++
		}
	}
	if errs > 0 {
		printDiags(diags)
		return fail(fmt.Errorf("the draft fails gloss check with %s; this is a bug in gloss onboard, please report it with the output above", plural(errs, "error")))
	}
	fmt.Printf("  gloss check: 0 errors, %s; graph built (.glossmark/graph.jsonl)\n", plural(warns, "warning"))
	if rc := cmdView([]string{"--root", res.Root}); rc != 0 {
		return rc
	}
	viewer := p.WS.StatePath("viewer.html")
	if *open {
		if err := openInBrowser(viewer); err != nil {
			fmt.Fprintf(os.Stderr, "gloss onboard: could not open the viewer: %v\n", err)
		}
	}
	fmt.Printf("gloss onboard: done; everything written is a proposal (proposed: true, @gloss?), nothing is enforced until accepted.\n")
	fmt.Printf("Next: open %s to see the draft, then run gloss review to accept, correct or reject each concept and link.\n", relOrAbs(res.Root, viewer))
	return 0
}

func preview(xs []string, n int) string {
	if len(xs) <= n {
		return strings.Join(xs, ", ")
	}
	return strings.Join(xs[:n], ", ") + fmt.Sprintf(", ... %d more", len(xs)-n)
}

// openInBrowser hands a file to the desktop's opener.
func openInBrowser(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
