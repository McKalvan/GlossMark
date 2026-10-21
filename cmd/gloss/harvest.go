// @gloss implements gmi:EvidenceHarvester scope=file

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/harvest"
	"github.com/mckalvan/glossmark/internal/version"
)

func init() {
	register(command{"harvest", "collect evidence of the domain vocabulary from code, schemas, tests, UI, docs and git, before an ontology exists; --cluster groups it into candidate concepts (ADR 0025)", cmdHarvest})
}

func cmdHarvest(args []string) int {
	fs, root := newFlags("harvest")
	var exclude, units, ingest []string
	fs.Func("exclude", "glob to leave out (repeatable), e.g. to hide a vocabulary the derivation is evaluated against", func(s string) error {
		exclude = append(exclude, s)
		return nil
	})
	fs.Func("unit", "application directory (repeatable; default: the deployables gloss init would plan)", func(s string) error {
		units = append(units, s)
		return nil
	})
	fs.Func("ingest", "evidence JSONL produced elsewhere (a connector or tracker reader) to validate and merge (repeatable)", func(s string) error {
		ingest = append(ingest, s)
		return nil
	})
	in := fs.String("in", "", "read evidence from this file instead of harvesting")
	noGit := fs.Bool("no-git", false, "skip rename evidence from the git history")
	cluster := fs.Bool("cluster", false, "print candidate concepts instead of evidence")
	format := fs.String("format", "jsonl", "jsonl | text (text: a summary of --cluster)")
	top := fs.Int("top", 60, "text: how many candidates to list")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if *format != "jsonl" && *format != "text" {
		return fail(fmt.Errorf("--format %q: use jsonl or text", *format))
	}
	dir := *root
	if dir == "" {
		dir, _ = os.Getwd()
	}
	var ev []harvest.Evidence
	var err error
	if *in != "" {
		ev, err = readEvidence(*in)
	} else {
		ev, err = harvest.Harvest(dir, harvest.Options{Exclude: exclude, Units: units, Git: !*noGit})
	}
	if err != nil {
		return fail(err)
	}
	for _, f := range ingest {
		more, err := readEvidence(f)
		if err != nil {
			return fail(err)
		}
		ev = append(ev, more...)
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	enc := json.NewEncoder(w)
	if !*cluster {
		if *format == "text" {
			return fail(fmt.Errorf("--format text needs --cluster"))
		}
		_ = enc.Encode(map[string]any{"type": "evidence", "format": version.EvidenceFormat, "generator": version.Generator()})
		for _, e := range ev {
			_ = enc.Encode(e)
		}
		return 0
	}
	cs := harvest.Clusters(ev)
	if *format == "text" {
		printClusters(w, ev, cs, *top)
		return 0
	}
	for _, c := range cs {
		_ = enc.Encode(c)
	}
	return 0
}

// readEvidence reads an evidence file, validating every record; the header
// line is skipped.
func readEvidence(path string) ([]harvest.Evidence, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []harvest.Evidence
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.Contains(line, `"type":"evidence"`) {
			continue
		}
		var e harvest.Evidence
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		if err := e.Validate(); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func printClusters(w io.Writer, ev []harvest.Evidence, cs []harvest.Cluster, top int) {
	signals := map[string]int{}
	for _, e := range ev {
		signals[e.Signal]++
	}
	var names []string
	for s := range signals {
		names = append(names, s)
	}
	sort.Strings(names)
	fmt.Fprintf(w, "%s from %d signals:", plural(len(ev), "observation"), len(names))
	for _, s := range names {
		fmt.Fprintf(w, " %s %d", s, signals[s])
	}
	cats := map[string]int{}
	for _, c := range cs {
		cats[c.Category]++
	}
	fmt.Fprintf(w, "\n%s: %d corroborated, %d docs-only, %d code-only visible, %d code-only internal\n\n", plural(len(cs), "candidate concept"),
		cats[harvest.Corroborated], cats[harvest.DocsOnly], cats[harvest.CodeOnlyVisible], cats[harvest.CodeOnlyInternal])
	for i, c := range cs {
		if i == top {
			fmt.Fprintf(w, "... %d more (--top, or --format jsonl for all)\n", len(cs)-top)
			break
		}
		var fams []string
		for f, n := range c.Families {
			fams = append(fams, fmt.Sprintf("%s %d", f, n))
		}
		sort.Strings(fams)
		fmt.Fprintf(w, "%-32s %-18s named by %-10s %s\n", c.Name, c.Category, c.NamedBy, strings.Join(fams, ", "))
	}
}
