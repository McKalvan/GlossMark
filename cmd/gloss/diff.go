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

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
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
	before := rollup.Compare(baseGraph, head.WS, rollup.Options{Policy: pol, Now: time.Now()})
	label := *base
	adoption := *baseline
	if baseGraph.Stats()["nodes"] == 0 {
		// The base never ran gloss: this PR is the adoption.
		adoption = true
		label += " (no graph)"
	}
	im := gate.Classify(baseGraph, head.Graph, head.WS, gate.Options{Base: label, Policy: pol, HeadLedger: head.Ledger.Entries, ChangedFiles: changed, BeforeReport: before, AfterReport: after, Adoption: adoption})
	var buf bytes.Buffer
	switch *format {
	case "json":
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		enc.Encode(im)
	case "markdown":
		buf.WriteString(gate.RenderMarkdown(im))
	default:
		buf.WriteString(gate.RenderText(im))
	}
	os.Stdout.Write(buf.Bytes())
	if *out != "" {
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err == nil {
			os.WriteFile(*out, buf.Bytes(), 0o644)
		}
	}
	switch strings.ToUpper(*failOn) {
	case "HARD":
		if im.Gate == gate.HARD {
			fmt.Fprintln(os.Stderr, "gloss diff: HARD changes need a human decision")
			return 1
		}
	case "SOFT":
		if im.Gate == gate.HARD || im.Gate == gate.SOFT {
			return 1
		}
	}
	return 0
}

// loadBaseGraph obtains the graph at a git ref: the committed graph.jsonl when
// present, otherwise an extraction of the ref's tree in a temp directory.
func loadBaseGraph(ws *load.Workspace, ref string) (*graph.Graph, error) {
	prefix, err := gitOut(ws.Root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a git repository: %v", ws.Root, err)
	}
	prefix = strings.TrimSpace(prefix)
	if ref == "" {
		return graph.New(ws.Prefixes), nil
	}
	if out, err := gitOut(ws.Root, "show", ref+":"+prefix+load.StateDir+"/graph.jsonl"); err == nil {
		g, err := graph.ReadJSONL(strings.NewReader(out))
		if err == nil {
			return g, nil
		}
	}
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
	}
	return res.Graph, nil
}

func changedFiles(root, ref string) ([]string, error) {
	out, err := gitOut(root, "diff", "--name-only", "--relative", ref)
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
