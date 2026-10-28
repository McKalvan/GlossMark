// @gloss implements gmi:Gate scope=file

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func init() {
	register(command{"diff", "impact of the working tree against --base <git ref>: AUTO/SOFT/HARD per change, views changed, PR comment", cmdDiff})
}

func cmdDiff(args []string) int {
	fs, root := newFlags("diff")
	base := fs.String("base", "origin/main", "git ref to compare against")
	format := fs.String("format", "text", "text | markdown | json")
	failOn := fs.String("fail-on", "HARD", "gate class that fails the command: HARD | SOFT | none")
	out := fs.String("o", "", "also write the output to this file")
	actor := fs.String("actor", "", "who made the change according to CI (agent:<name> or human:<name>); new links are gated as this actor too, whatever the committed ledger claims")
	history := fs.Int("history", 3, "markdown: under the impact, the last N SOFT and HARD history steps on --base of each concept and boundary the change touches (0 leaves them out)")
	baseline := fs.Bool("baseline", false, "report the whole tree as an adoption: counts per boundary and per view instead of per link (automatic when the base has no graph)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	head, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	baseGraph, err := loadBaseGraph(head.WS, *base)
	if err != nil {
		return fail(err)
	}
	changed, _ := changedFiles(head.WS.Root, *base)
	pol := head.WS.Config.EffectivePolicy()
	after := rollup.Compare(head.Graph, head.WS, rollup.Options{Ledger: head.Ledger.Entries, Policy: pol, Now: time.Now()})
	before := baseReport(head.WS, *base, baseGraph, pol)
	label := *base
	adoption := *baseline
	if baseGraph.Stats()["nodes"] == 0 {
		// The base never ran gloss: this PR is the adoption.
		adoption = true
		label += " (no graph)"
	}
	im := gate.Classify(baseGraph, head.Graph, head.WS, gate.Options{Base: label, Policy: pol, HeadLedger: head.Ledger.Entries, ChangedFiles: changed, BeforeReport: before, AfterReport: after, Adoption: adoption, BaseViews: baseViews(head.WS, *base),
		Actor: *actor, BaseLedger: baseLedger(head.WS, *base), BaseHardPaths: baseHardPaths(head.WS, *base)})
	// The adoption level caps what fails: the report keeps every gate.
	gate.Enforce(im, head.WS.Config.AdoptionLevel())
	var buf bytes.Buffer
	switch *format {
	case "json":
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		enc.Encode(im)
	case "markdown":
		buf.WriteString(gate.RenderMarkdown(im))
		if *history > 0 && im.Adoption == nil {
			hist, err := renderHistoryMarkdown(head.WS, *base, touchedSubjects(im, baseGraph), *history)
			if err != nil {
				fmt.Fprintln(os.Stderr, "gloss diff: recent history left out:", err)
			}
			buf.WriteString(hist)
		}
	default:
		buf.WriteString(gate.RenderText(im))
	}
	os.Stdout.Write(buf.Bytes())
	if *out != "" {
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err == nil {
			os.WriteFile(*out, buf.Bytes(), 0o644)
		}
	}
	if im.Advisory != "" {
		fmt.Fprintln(os.Stderr, "gloss diff: "+im.Advisory)
	}
	switch strings.ToUpper(*failOn) {
	case "HARD":
		if im.Enforced == gate.HARD {
			fmt.Fprintln(os.Stderr, "gloss diff: HARD changes need a human decision")
			return 1
		}
	case "SOFT":
		if im.Enforced == gate.HARD || im.Enforced == gate.SOFT {
			return 1
		}
	}
	return 0
}

// loadBaseGraph obtains the graph at a git ref by extracting the ref's tree
// in a temp directory.
func loadBaseGraph(ws *load.Workspace, ref string) (*graph.Graph, error) {
	prefix, err := gitOut(ws.Root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a git repository: %v", ws.Root, err)
	}
	prefix = strings.TrimSpace(prefix)
	if ref == "" {
		return graph.New(ws.Prefixes), nil
	}
	if err := verifyBase(ws.Root, ref); err != nil {
		return nil, err
	}
	// The base is always extracted from its tree, never read from the
	// graph.jsonl it committed: a committed graph is repository content, and
	// one that already held a link the tree did not would make the PR that
	// adds the code look like no change. Extracting with this release also
	// keeps an upgrade from reading as a change to every link (ADR 0006).
	// An empty tree (an orphan or --allow-empty base) has nothing to extract.
	if ls, err := gitOut(ws.Root, "ls-tree", "-r", "--name-only", ref); err == nil && strings.TrimSpace(ls) == "" {
		return graph.New(ws.Prefixes), nil
	}
	// Extract the base tree.
	tmp, err := os.MkdirTemp("", "gloss-base-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	archive := exec.Command("git", "-C", ws.Root, "archive", "--format=tar", ref, "./")
	var tar bytes.Buffer
	archive.Stdout = &tar
	var stderr bytes.Buffer
	archive.Stderr = &stderr
	if err := archive.Run(); err != nil {
		return nil, fmt.Errorf("git archive %s: %v: %s", ref, err, strings.TrimSpace(stderr.String()))
	}
	untar := exec.Command("tar", "-x", "-C", tmp)
	untar.Stdin = &tar
	if err := untar.Run(); err != nil {
		return nil, fmt.Errorf("extracting base tree: %v", err)
	}
	baseRoot := tmp
	if _, err := os.Stat(filepath.Join(tmp, load.ConfigName)); err != nil {
		// Archive of a subdirectory keeps the subdirectory name.
		baseRoot = filepath.Join(tmp, filepath.Base(ws.Root))
		if _, err := os.Stat(filepath.Join(baseRoot, load.ConfigName)); err != nil {
			// No GlossMark at the base: empty graph (everything is new).
			return graph.New(ws.Prefixes), nil
		}
	}
	bws, err := load.Load(baseRoot)
	if err != nil {
		return nil, fmt.Errorf("loading base: %w", err)
	}
	res, err := extract.Build(bws)
	if err != nil {
		return nil, err
	}
	rollup.Derive(res.Graph, bws.Vocab)
	// Attribute actors from the base ledger when committed.
	if lg, err := ledger.Open(bws.StatePath(ledger.FileName)); err == nil {
		lg.Sync(res.Graph, res.Markers, "human:unknown", time.Now())
		if lg.DropRejected(res.Graph) > 0 {
			rollup.Derive(res.Graph, bws.Vocab)
		}
	}
	return res.Graph, nil
}

// baseViews reads the views file committed at ref, so a view the change
// deletes is reported; nil when the base has none or git cannot say.
func baseViews(ws *load.Workspace, ref string) *model.Views {
	if ref == "" {
		return nil
	}
	prefix, err := gitOut(ws.Root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil
	}
	prefix = strings.TrimSpace(prefix)
	// The base's own views: path, so a change that moves the views file
	// (views: in glossmark.yaml) is still compared with what the base held.
	path := "ontology/views.yaml"
	if cfg, err := gitOut(ws.Root, "show", ref+":"+prefix+load.ConfigName); err == nil {
		var c struct {
			Views string `yaml:"views"`
		}
		if yaml.Unmarshal([]byte(cfg), &c) == nil && c.Views != "" {
			path = c.Views
		}
	}
	out, err := gitOut(ws.Root, "show", ref+":"+prefix+path)
	if err != nil {
		return nil
	}
	v, _, err := model.ParseViews(path, []byte(out))
	if err != nil {
		return nil
	}
	return v
}

// baseHardPaths reads the hard-paths of the policy committed at ref (the
// defaults when it sets none); nil when the base has no glossmark.yaml.
func baseHardPaths(ws *load.Workspace, ref string) []string {
	if ref == "" {
		return nil
	}
	prefix, err := gitOut(ws.Root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil
	}
	out, err := gitOut(ws.Root, "show", ref+":"+strings.TrimSpace(prefix)+load.ConfigName)
	if err != nil {
		return nil
	}
	var cfg model.Config
	if yaml.Unmarshal([]byte(out), &cfg) != nil {
		return nil
	}
	return cfg.EffectivePolicy().HardPaths
}

// baseLedger reads the ledger committed at ref; nil when the base has none
// or git cannot say.
func baseLedger(ws *load.Workspace, ref string) map[string]*model.LedgerEntry {
	if ref == "" {
		return nil
	}
	rel, err := filepath.Rel(ws.Root, ws.StatePath(ledger.FileName))
	if err != nil {
		return nil
	}
	prefix, err := gitOut(ws.Root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil
	}
	out, err := gitOut(ws.Root, "show", ref+":"+strings.TrimSpace(prefix)+filepath.ToSlash(rel))
	if err != nil {
		return nil
	}
	tmp, err := os.CreateTemp("", "gloss-base-ledger-")
	if err != nil {
		return nil
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString(out)
	tmp.Close()
	lg, err := ledger.Open(tmp.Name())
	if err != nil {
		return nil
	}
	return lg.Entries
}

// baseReport compares the base graph over the base tree's own file set, as
// gloss drift on a checkout of the base would; the head's files would count
// files added since the base against the base's coverage.
func baseReport(ws *load.Workspace, ref string, bg *graph.Graph, pol *model.Policy) *rollup.Report {
	return rollup.Compare(bg, ws, rollup.Options{Policy: pol, Now: time.Now(), Files: baseFiles(ws, ref, bg)})
}

// baseFiles lists the in-scope files of the tree at ref (none when the base
// has no graph); nil, meaning the workspace's files, when git cannot say.
func baseFiles(ws *load.Workspace, ref string, bg *graph.Graph) []string {
	if ref == "" || bg.Stats()["nodes"] == 0 {
		return []string{}
	}
	prefix, err := gitOut(ws.Root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil
	}
	out, err := gitOut(ws.Root, "ls-tree", "-r", "--name-only", "--full-tree", ref)
	if err != nil {
		return nil
	}
	files := []string{}
	for _, l := range strings.Split(out, "\n") {
		rel, ok := strings.CutPrefix(strings.TrimSpace(l), strings.TrimSpace(prefix))
		if ok && rel != "" && ws.InScope(rel) {
			files = append(files, rel)
		}
	}
	return files
}

// verifyBase fails with a message naming --base when ref is not a commit
// here: in CI, usually a shallow clone that never fetched origin/main.
func verifyBase(root, ref string) error {
	// A ref reaches git as an argument: one that starts with "-" would be read
	// as an option (--output=<file> writes anywhere), and the MCP impact_of
	// tool takes it from an agent.
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("base %q is not a ref", ref)
	}
	if _, err := gitOut(root, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return fmt.Errorf("base %s is not a commit in this repository; fetch it (git fetch origin main, or a deeper clone in CI) or name another ref with --base <ref>", ref)
	}
	return nil
}

// baseHasWorkspace reports whether the base commit has glossmark.yaml where
// this workspace has it; the commit that adopts GlossMark has none.
func baseHasWorkspace(ws *load.Workspace, ref string) (bool, string) {
	prefix, err := gitOut(ws.Root, "rev-parse", "--show-prefix")
	if err != nil {
		return false, "."
	}
	prefix = strings.TrimSpace(prefix)
	_, err = gitOut(ws.Root, "cat-file", "-e", ref+":"+prefix+load.ConfigName)
	return err == nil, firstNonEmpty(strings.TrimSuffix(prefix, "/"), ".")
}

func changedFiles(root, ref string) ([]string, error) {
	if err := verifyBase(root, ref); err != nil {
		return nil, err
	}
	out, err := gitOut(root, "diff", "--name-only", "--no-renames", "--relative", ref, "--")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	// Untracked files count as changed too.
	if out, err := gitOut(root, "ls-files", "--others", "--exclude-standard"); err == nil {
		for _, l := range strings.Split(out, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				files = append(files, l)
			}
		}
	}
	return files, nil
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
