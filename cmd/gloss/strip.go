// @gloss implements gmi:Stripper scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/store"
)

func init() {
	register(command{"strip", "leave GlossMark: write a final export of the graph, ledger and ontology, then remove every @gloss marker, manifest and tool file from the tree (--keep-ontology keeps ontology/ and glossmark.yaml; --dry-run prints the plan)", cmdStrip})
}

// exportDirName is where gloss strip keeps the final export, under the root.
const exportDirName = "glossmark-export"

// agentsBegin and agentsEnd bracket the block gloss agents --write maintains.
const agentsBegin, agentsEnd = "<!-- gloss:begin -->", "<!-- gloss:end -->"

// stripPlan is what gloss strip will do, computed before anything is written
// so --dry-run prints exactly what a run would change.
type stripPlan struct {
	ws      *load.Workspace
	edits   map[string][]byte // repo-relative path → stripped content
	markers map[string]int    // repo-relative path → markers removed
	blocks  []string          // files whose gloss:begin/end block goes
	remove  []string          // repo-relative files and directories deleted
	keep    []string          // GlossMark traces the command leaves, with the fix
}

func cmdStrip(args []string) int {
	fs, root := newFlags("strip")
	keepOnt := fs.Bool("keep-ontology", false, "keep ontology/ and glossmark.yaml (the vocabulary stays readable; only markers, manifests and .glossmark/ go)")
	dry := fs.Bool("dry-run", false, "print what would be removed and exported; write nothing")
	exportDir := fs.String("export", "", "where to write the final export (default "+exportDirName+"/ under the root; - for none)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if n := countErrors(p.Diags); n > 0 {
		fmt.Fprintf(os.Stderr, "gloss strip: warning: gloss check reports %s; the export carries the graph as it builds now\n", plural(n, "error"))
	}
	plan, err := makeStripPlan(p.WS, *keepOnt)
	if err != nil {
		return fail(err)
	}
	dir := ""
	if *exportDir != "-" {
		dir = *exportDir
		if dir == "" {
			dir = filepath.Join(p.WS.Root, exportDirName)
		} else if !filepath.IsAbs(dir) {
			dir = filepath.Join(p.WS.Root, dir)
		}
	}
	printStripPlan(plan, dir)
	if *dry {
		fmt.Println("gloss strip: dry run; nothing written")
		return 0
	}
	if dirty(p.WS.Root) {
		fmt.Fprintln(os.Stderr, "gloss strip: warning: the working tree has uncommitted changes, so git diff will mix them with the strip; commit or stash first to review the strip alone")
	}
	if dir != "" {
		if err := writeStripExport(p, dir); err != nil {
			return fail(fmt.Errorf("export: %w (nothing removed)", err))
		}
	}
	if err := applyStripPlan(plan); err != nil {
		return fail(err)
	}
	files := len(plan.markers)
	total := 0
	for _, n := range plan.markers {
		total += n
	}
	fmt.Printf("gloss strip: %s removed from %s, %s deleted", plural(total, "marker"), plural(files, "file"), plural(len(plan.remove), "GlossMark file"))
	if dir != "" {
		fmt.Printf("; export in %s/", relOrAbs(p.WS.Root, dir))
	}
	fmt.Println()
	fmt.Println("Review with git diff: only marker lines and GlossMark files should be gone. Docs: docs/LEAVING.md")
	return 0
}

// makeStripPlan reads every in-scope file (the walk already honours
// exclude: and generated:) and strips its markers with the scanner's own
// comment table, lists the GlossMark files to delete, and notes what the
// strip leaves for a person to undo.
func makeStripPlan(ws *load.Workspace, keepOntology bool) (*stripPlan, error) {
	plan := &stripPlan{ws: ws, edits: map[string][]byte{}, markers: map[string]int{}}
	removeSet := map[string]bool{}
	remove := func(rel string) {
		if rel != "" && !removeSet[rel] {
			removeSet[rel] = true
			plan.remove = append(plan.remove, rel)
		}
	}
	for _, rel := range ws.Files {
		if marker.IsSidecar(rel) {
			remove(rel) // a sidecar is GlossMark's file, markers or not
			continue
		}
		b, err := os.ReadFile(ws.Abs(rel))
		if err != nil {
			return nil, err
		}
		style, _ := marker.StyleFor(rel, ws.Config.CommentStyles)
		out, n := marker.Strip(b, style)
		if n > 0 {
			plan.edits[rel] = out
			plan.markers[rel] = n
			b = out
		}
		if style.Fences && hasAgentsBlock(b) {
			plan.edits[rel] = dropAgentsBlock(b)
			plan.blocks = append(plan.blocks, rel)
		}
	}
	for _, m := range ws.Manifests {
		remove(m.Path)
	}
	for _, rel := range []string{load.StateDir, load.LockName} {
		if _, err := os.Stat(ws.Abs(rel)); err == nil {
			remove(rel)
		}
	}
	if !keepOntology {
		remove(load.ConfigName)
		for _, o := range ws.Ontologies {
			if o.Path != "" && !strings.HasPrefix(ws.Rel(o.Path), "../") {
				if _, err := os.Stat(o.Path); err == nil {
					remove(ws.Rel(o.Path))
				}
			}
		}
		if ws.Config.Views != "" {
			if _, err := os.Stat(ws.Abs(ws.Config.Views)); err == nil {
				remove(filepath.ToSlash(ws.Config.Views))
			}
		}
		// Vendored copies of imports and anything else gloss wrote under ontology/.
		if entries, err := os.ReadDir(ws.Abs("ontology")); err == nil {
			for _, e := range entries {
				if !e.IsDir() && (strings.HasSuffix(e.Name(), ".gloss.yaml") || e.Name() == "views.yaml") {
					remove("ontology/" + e.Name())
				}
			}
		}
	}
	sort.Strings(plan.remove)
	plan.keep = leftovers(ws)
	return plan, nil
}

// leftovers names the GlossMark wiring outside the tree's own files that
// strip does not touch, because each sits inside a file that is not GlossMark's.
func leftovers(ws *load.Workspace) []string {
	var out []string
	has := func(rel string) bool { _, err := os.Stat(ws.Abs(rel)); return err == nil }
	contains := func(rel, s string) bool {
		b, err := os.ReadFile(ws.Abs(rel))
		return err == nil && strings.Contains(string(b), s)
	}
	if contains(".claude/settings.json", "gloss") || contains(".claude/settings.local.json", "gloss") {
		out = append(out, ".claude/settings.json: the gloss context SessionStart hook (remove the hook entry; delete .claude/settings.local.json if it only holds the path)")
	}
	if contains(".mcp.json", "gloss") {
		out = append(out, ".mcp.json: the gloss MCP server (remove the \"gloss\" server)")
	}
	if contains(".cursor/mcp.json", "gloss") {
		out = append(out, ".cursor/mcp.json: the gloss MCP server (remove the \"gloss\" server)")
	}
	for _, skill := range []string{"adopt-glossmark", "derive-ontology", "review-glossmark", "curate-increment"} {
		if has(".claude/skills/" + skill) {
			out = append(out, ".claude/skills/"+skill+"/: a skill gloss hook install --skill vendored (delete the directory)")
		}
	}
	if entries, err := os.ReadDir(ws.Abs(".github/workflows")); err == nil {
		for _, e := range entries {
			if contains(".github/workflows/"+e.Name(), "glossmark") {
				out = append(out, ".github/workflows/"+e.Name()+": the GlossMark job (remove the step or the workflow)")
			}
		}
	}
	if contains(".pre-commit-config.yaml", "glossmark") {
		out = append(out, ".pre-commit-config.yaml: the gloss hooks (remove the repo entry)")
	}
	if contains("Makefile", "gloss") {
		out = append(out, "Makefile: the gloss target (remove it)")
	}
	if contains("package.json", "gloss ") {
		out = append(out, "package.json: the gloss script (remove it)")
	}
	if contains(".gitignore", ".glossmark") {
		out = append(out, ".gitignore: the .glossmark/ lines (remove them)")
	}
	if has("dist/glossmark") {
		out = append(out, "dist/glossmark/: gloss publish output (delete it)")
	}
	return out
}

// hasAgentsBlock reports a gloss:begin line, on its own, as gloss agents
// --write writes it; prose that quotes the marker is not a block.
func hasAgentsBlock(b []byte) bool {
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == agentsBegin {
			return true
		}
	}
	return false
}

// dropAgentsBlock removes the lines from gloss:begin to gloss:end, inclusive,
// with the blank line that separates the block from the text above it.
func dropAgentsBlock(b []byte) []byte {
	lines := strings.SplitAfter(string(b), "\n")
	var out []string
	in := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case !in && t == agentsBegin:
			in = true
			if n := len(out); n > 0 && strings.TrimSpace(out[n-1]) == "" {
				out = out[:n-1]
			}
		case in && t == agentsEnd:
			in = false
		case !in:
			out = append(out, l)
		}
	}
	return []byte(strings.Join(out, ""))
}

func printStripPlan(plan *stripPlan, exportDir string) {
	root := plan.ws.Root
	if exportDir != "" {
		fmt.Printf("export %s/: graph.jsonl, context.jsonld, graph.jsonld, graph.db, ledger.jsonl, ledger.prov.jsonld, glossmark.yaml, ontology/, manifests/\n", relOrAbs(root, exportDir))
	}
	paths := make([]string, 0, len(plan.edits))
	for rel := range plan.edits {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		switch n := plan.markers[rel]; {
		case n == 0:
			fmt.Printf("edit   %s: the gloss:begin/end block\n", rel)
		case containsStr(plan.blocks, rel):
			fmt.Printf("edit   %s: %s, the gloss:begin/end block\n", rel, plural(n, "marker"))
		default:
			fmt.Printf("edit   %s: %s\n", rel, plural(n, "marker"))
		}
	}
	for _, rel := range plan.remove {
		if st, err := os.Stat(plan.ws.Abs(rel)); err == nil && st.IsDir() {
			rel += "/"
		}
		fmt.Printf("delete %s\n", rel)
	}
	for _, k := range plan.keep {
		fmt.Printf("keep   %s\n", k)
	}
}

// applyStripPlan writes the edits and deletes the files, edits first so a
// failure leaves the ontology in place to retry against.
func applyStripPlan(plan *stripPlan) error {
	for rel, b := range plan.edits {
		path := plan.ws.Abs(rel)
		st, err := os.Stat(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, b, st.Mode().Perm()); err != nil {
			return err
		}
	}
	for _, rel := range plan.remove {
		if err := os.RemoveAll(plan.ws.Abs(rel)); err != nil {
			return err
		}
	}
	// ontology/ and the parent of a deleted manifest stay; an empty ontology/ goes.
	if entries, err := os.ReadDir(plan.ws.Abs("ontology")); err == nil && len(entries) == 0 {
		os.Remove(plan.ws.Abs("ontology"))
	}
	return nil
}

// writeStripExport keeps everything the tree is about to lose in one
// directory: the graph as JSON Lines with its context, as one JSON-LD
// document and as SQLite, the ledger as it is and as PROV-O, and copies of
// glossmark.yaml, the ontology files and every manifest.
func writeStripExport(p *pipeline, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeGraph(p.Graph, filepath.Join(dir, "graph.jsonl")); err != nil {
		return err
	}
	b, err := graphJSONLD(p.Graph)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "graph.jsonld"), b, 0o644); err != nil {
		return err
	}
	if err := store.Export(p.Graph, filepath.Join(dir, "graph.db"), floorFiles(p)...); err != nil {
		return err
	}
	if b, err := os.ReadFile(p.WS.StatePath(ledger.FileName)); err == nil {
		if err := os.WriteFile(filepath.Join(dir, ledger.FileName), b, 0o644); err != nil {
			return err
		}
	}
	prov, err := p.Ledger.ExportPROV()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "ledger.prov.jsonld"), prov, 0o644); err != nil {
		return err
	}
	copies := map[string]string{load.ConfigName: load.ConfigName}
	for _, o := range p.WS.Ontologies {
		if o.Path != "" {
			copies[o.Path] = filepath.Join("ontology", filepath.Base(o.Path))
		}
	}
	if v := p.WS.Config.Views; v != "" {
		copies[p.WS.Abs(v)] = filepath.Join("ontology", filepath.Base(v))
	}
	for _, m := range p.WS.Manifests {
		copies[m.Path] = filepath.Join("manifests", filepath.FromSlash(m.Path))
	}
	for src, dst := range copies {
		if !filepath.IsAbs(src) {
			src = p.WS.Abs(src)
		}
		b, err := os.ReadFile(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		out := filepath.Join(dir, dst)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// graphJSONLD renders the graph as one JSON-LD document: the context gloss
// build writes beside graph.jsonl, and every node and edge record under
// @graph, so a JSON-LD processor loads it without the JSON Lines framing.
func graphJSONLD(g *graph.Graph) ([]byte, error) {
	var ctx struct {
		Context map[string]any `json:"@context"`
	}
	if err := json.Unmarshal([]byte(graph.JSONLDContext(g)), &ctx); err != nil {
		return nil, err
	}
	records := make([]any, 0, len(g.Nodes())+len(g.Edges()))
	for _, n := range g.Nodes() {
		records = append(records, n)
	}
	for _, e := range g.Edges() {
		records = append(records, e)
	}
	doc := map[string]any{"@context": ctx.Context, "@graph": records}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// dirty reports whether root is a git working tree with uncommitted changes.
func dirty(root string) bool {
	out, err := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func countErrors(diags []marker.Diagnostic) int {
	n := 0
	for _, d := range diags {
		if d.Severity == "error" {
			n++
		}
	}
	return n
}
