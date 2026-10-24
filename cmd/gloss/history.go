// @gloss implements gmi:Gate scope=file

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

func init() {
	register(command{"history", "the gate folded over commits: when each concept, boundary and link was added, changed or removed (ADR 0040)", cmdHistory})
}

// historyStep is one step of the history: the gate of a commit against its
// parent, or of a version chapter against the commit before it.
type historyStep struct {
	Commit string `json:"commit"`
	Parent string `json:"parent,omitempty"`
	// From is the first commit of a folded step (step version).
	From   string `json:"from,omitempty"`
	Date   string `json:"date"`
	Author string `json:"author"`
	Title  string `json:"title"`
	// Status is built (classified from the committed graphs), verified
	// (rebuilt from the trees), unbuilt (ontology files changed and
	// graph.jsonl did not; still classified) or skipped (Reason says why).
	Status string       `json:"status"`
	Reason string       `json:"reason,omitempty"`
	Impact *gate.Impact `json:"impact,omitempty"`
}

type historyResult struct {
	Repo   string   `json:"repo"`
	Range  string   `json:"range"`
	Step   string   `json:"step"`
	Follow []string `json:"follow,omitempty"`
	// Lineage is each replaced-by edge among the followed subjects, "old → new".
	Lineage []string      `json:"lineage,omitempty"`
	Steps   []historyStep `json:"steps"`
	// Hidden counts the AUTO-only steps left out without --all.
	Hidden    int  `json:"hidden"`
	Truncated bool `json:"truncated,omitempty"`
}

type historyCommit struct {
	hash, parent, date, author, title string
	files                             []string
}

func cmdHistory(args []string) int {
	fs, root := newFlags("history")
	rng := fs.String("range", "HEAD", "git revision range to walk (<a>..<b>); the default is all of HEAD's history")
	step := fs.String("step", "", "commit | first-parent | version (default: history.step in glossmark.yaml, else commit)")
	all := fs.Bool("all", false, "also show steps whose changes are all AUTO")
	verify := fs.Bool("verify", false, "rebuild each step's graphs from their trees instead of reading the committed graph.jsonl")
	format := fs.String("format", "text", "text | json")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	r := *root
	if r == "" {
		cwd, _ := os.Getwd()
		found, err := load.FindRoot(cwd)
		if err != nil {
			return fail(err)
		}
		r = found
	}
	ws, err := load.Load(r)
	if err != nil {
		return fail(err)
	}
	mode := *step
	if mode == "" && ws.Config.History != nil {
		mode = ws.Config.History.Step
	}
	if mode == "" {
		mode = "commit"
	}
	if mode != "commit" && mode != "first-parent" && mode != "version" {
		fmt.Fprintf(os.Stderr, "gloss history: unknown step %q; use commit, first-parent or version\n", mode)
		return 2
	}
	if strings.HasPrefix(*rng, "-") {
		fmt.Fprintf(os.Stderr, "gloss history: range %q is not a revision range\n", *rng)
		return 2
	}
	h, err := runHistory(ws, *rng, mode, fs.Args(), *all, *verify)
	if err != nil {
		return fail(err)
	}
	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(h)
		return 0
	}
	os.Stdout.WriteString(renderHistory(h))
	return 0
}

func runHistory(ws *load.Workspace, rng, mode string, follow []string, all, verify bool) (*historyResult, error) {
	top, err := gitOut(ws.Root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a git repository: %v", ws.Root, err)
	}
	prefix, _ := gitOut(ws.Root, "rev-parse", "--show-prefix")
	s := &snapshots{ws: ws, top: strings.TrimSpace(top), prefix: strings.TrimSpace(prefix), verify: verify, cache: map[string]*snapshot{},
		sem: make(chan struct{}, runtime.GOMAXPROCS(0))}
	commits, err := historyCommits(ws.Root, rng, mode)
	if err != nil {
		return nil, err
	}
	h := &historyResult{Repo: filepath.Base(ws.Root), Range: rng, Step: mode, Follow: follow}
	if out, err := gitOut(ws.Root, "rev-parse", "--is-shallow-repository"); err == nil && strings.TrimSpace(out) == "true" {
		h.Truncated = true
	}
	var steps []*historyStep
	replaced := map[string]string{}
	if mode == "version" {
		steps = s.chapters(commits, replaced)
	} else {
		for _, c := range commits {
			if st := s.step(c, c.parent, c.files, replaced); st != nil {
				steps = append(steps, st)
			}
		}
	}
	s.wg.Wait()
	keep := followSet(follow, replaced)
	for k := range replaced {
		if keep == nil || keep[k] || keep[replaced[k]] {
			h.Lineage = append(h.Lineage, k+" → "+replaced[k])
		}
	}
	sort.Strings(h.Lineage)
	// Newest first, as git log prints.
	for i := len(steps) - 1; i >= 0; i-- {
		st := *steps[i]
		if st.Impact != nil {
			filterImpact(st.Impact, keep)
			if keep != nil && len(st.Impact.Changes) == 0 && len(st.Impact.Realizations) == 0 && len(st.Impact.Versions) == 0 {
				continue
			}
			if !all && st.Impact.Gate == gate.AUTO {
				h.Hidden++
				continue
			}
		}
		h.Steps = append(h.Steps, st)
	}
	return h, nil
}

// historyCommits lists the commits of rng that touch the workspace, oldest
// first, with the files each changed relative to the workspace root. Step
// commit leaves merges out (their changes are reported on the commits they
// merge); first-parent and version walk the main line, merges included.
func historyCommits(root, rng, mode string) ([]historyCommit, error) {
	args := []string{"log", "--reverse", "--no-renames", "--relative", "--name-only", "--format=%x1e%H%x1f%P%x1f%aI%x1f%an%x1f%s"}
	if mode == "commit" {
		args = append(args, "--no-merges", "--full-history")
	} else {
		args = append(args, "--first-parent", "-m")
	}
	out, err := gitOut(root, append(args, rng, "--", ".")...)
	if err != nil {
		return nil, err
	}
	var commits []historyCommit
	for _, rec := range strings.Split(out, "\x1e") {
		lines := strings.Split(strings.TrimSpace(rec), "\n")
		f := strings.Split(lines[0], "\x1f")
		if len(f) < 5 {
			continue
		}
		c := historyCommit{hash: f[0], date: f[2], author: f[3], title: f[4]}
		if ps := strings.Fields(f[1]); len(ps) > 0 {
			c.parent = ps[0]
		}
		for _, l := range lines[1:] {
			if l = strings.TrimSpace(l); l != "" {
				c.files = append(c.files, l)
			}
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// snapshot is the workspace, graph and ledger committed at one commit.
type snapshot struct {
	ws     *load.Workspace
	g      *graph.Graph
	ledger map[string]*model.LedgerEntry
	// none is set before the first commit and where glossmark.yaml is absent.
	none bool
	err  error
}

type snapshots struct {
	ws          *load.Workspace
	top, prefix string
	verify      bool
	cache       map[string]*snapshot // keyed by the blobs a snapshot reads
	order       []string
	// Snapshots load in order, so consecutive steps share them; the gate of
	// each step runs on its own goroutine, at most cap(sem) at a time.
	sem chan struct{}
	wg  sync.WaitGroup
}

// relevant reports whether a commit's change touches what history reads:
// the graph or ledger (built), or ontology, manifest, config or views files
// without the graph (unbuilt).
func (s *snapshots) relevant(files []string) (bool, bool) {
	graphPath := load.StateDir + "/graph.jsonl"
	ledgerPath := load.StateDir + "/" + ledger.FileName
	built, onto := false, false
	for _, f := range files {
		switch {
		case f == graphPath || f == ledgerPath:
			built = true
		case s.ontologyFile(f):
			onto = true
		}
	}
	return built || onto, onto && !slices.Contains(files, graphPath)
}

func (s *snapshots) ontologyFile(f string) bool {
	if f == load.ConfigName || f == s.ws.Config.Views || strings.HasSuffix(path.Base(f), ".gloss.yaml") {
		return true
	}
	for _, d := range s.ws.Config.Owns {
		if f == d.File {
			return true
		}
	}
	return false
}

// step classifies commit c against parent; nil when c changes nothing
// history reads.
func (s *snapshots) step(c historyCommit, parent string, files []string, replaced map[string]string) *historyStep {
	ok, unbuilt := s.relevant(files)
	if !ok {
		return nil
	}
	st := &historyStep{Commit: c.hash, Parent: parent, Date: c.date, Author: c.author, Title: c.title, Status: "built"}
	if s.verify {
		st.Status = "verified"
	} else if unbuilt {
		st.Status = "unbuilt"
		st.Reason = "ontology files changed and graph.jsonl did not: the graph was not rebuilt, or the edit changes nothing in it"
	}
	s.classify(st, parent, c.hash, files, replaced)
	return st
}

func (s *snapshots) classify(st *historyStep, base, head string, files []string, replaced map[string]string) {
	b, hd := s.load(base), s.load(head)
	for _, sn := range []*snapshot{b, hd} {
		if sn.err != nil {
			st.Status, st.Reason, st.Impact = "skipped", sn.err.Error(), nil
			return
		}
	}
	if hd.none {
		st.Status, st.Reason = "skipped", "no "+load.ConfigName+" at this commit"
		return
	}
	lineage(hd.g, replaced)
	opt := gate.Options{Base: short(base), Policy: hd.ws.Config.EffectivePolicy(), HeadLedger: hd.ledger, BaseLedger: b.ledger, ChangedFiles: files}
	if !b.none {
		opt.BaseViews, opt.BaseHardPaths = b.ws.Views, b.ws.Config.EffectivePolicy().HardPaths
	}
	if b.g.Stats()["nodes"] == 0 {
		opt.Adoption = true
		opt.Base += " (no graph)"
	}
	s.sem <- struct{}{}
	s.wg.Add(1)
	go func() {
		defer func() { <-s.sem; s.wg.Done() }()
		im := gate.Classify(b.g, hd.g, hd.ws, opt)
		// A link stale at the base as well is not news at this step.
		kept := im.Changes[:0]
		for _, ch := range im.Changes {
			if ch.Kind == "stale" {
				if le := b.ledger[ch.MarkerID]; le != nil && le.Status == model.StatusStale {
					continue
				}
			}
			kept = append(kept, ch)
		}
		im.Changes = kept
		st.Impact = im
	}()
}

// chapters folds the main line into one step per namespace version bump; the
// commits after the last bump form an open chapter.
func (s *snapshots) chapters(commits []historyCommit, replaced map[string]string) []*historyStep {
	var steps []*historyStep
	var start, from string
	var startVersions string
	var files []string
	var last historyCommit
	open := false
	for _, c := range commits {
		if ok, _ := s.relevant(c.files); !ok {
			continue
		}
		if !open {
			start, from, files, open = c.parent, c.hash, nil, true
			startVersions = s.versions(c.parent)
		}
		files = append(files, c.files...)
		last = c
		if v := s.versions(c.hash); v != startVersions {
			steps = append(steps, s.chapter(last, start, from, files, replaced))
			open = false
		}
	}
	if open {
		steps = append(steps, s.chapter(last, start, from, files, replaced))
	}
	return steps
}

func (s *snapshots) chapter(last historyCommit, start, from string, files []string, replaced map[string]string) *historyStep {
	st := &historyStep{Commit: last.hash, Parent: start, Date: last.date, Author: last.author, Title: last.title, Status: "built"}
	if from != last.hash {
		st.From = from
	}
	if s.verify {
		st.Status = "verified"
	}
	s.classify(st, start, last.hash, files, replaced)
	return st
}

// versions is the owned namespaces' versions at commit, as one string.
func (s *snapshots) versions(commit string) string {
	sn := s.load(commit)
	if sn.err != nil || sn.none {
		return ""
	}
	var vs []string
	for p := range sn.ws.Owned {
		vs = append(vs, p+"@"+sn.ws.Ontologies[p].Version)
	}
	sort.Strings(vs)
	return strings.Join(vs, " ")
}

// load reads the snapshot at commit: the workspace from its ontology,
// manifest, config, views, lock and state files only (no code is scanned),
// and the graph.jsonl it committed, or with verify the graph rebuilt from
// its tree. "" (before the first commit) and a commit without glossmark.yaml
// are empty.
func (s *snapshots) load(commit string) *snapshot {
	empty := &snapshot{g: graph.New(s.ws.Prefixes), ledger: map[string]*model.LedgerEntry{}, none: true}
	if commit == "" {
		return empty
	}
	if _, err := gitOut(s.ws.Root, "cat-file", "-e", commit+":"+s.prefix+load.ConfigName); err != nil {
		return empty
	}
	paths, key, err := s.wanted(commit)
	if err != nil {
		return &snapshot{err: err}
	}
	if sn, ok := s.cache[key]; ok && !s.verify {
		return sn
	}
	sn := s.read(commit, paths)
	if !s.verify {
		s.cache[key] = sn
		s.order = append(s.order, key)
		if len(s.order) > 8 {
			delete(s.cache, s.order[0])
			s.order = s.order[1:]
		}
	}
	return sn
}

// wanted lists the files a snapshot reads at commit (full-tree paths) and a
// key naming their blobs, so two commits with the same files share one load.
func (s *snapshots) wanted(commit string) ([]string, string, error) {
	cfgText, err := gitOut(s.ws.Root, "show", commit+":"+s.prefix+load.ConfigName)
	if err != nil {
		return nil, "", err
	}
	var cfg model.Config
	yaml.Unmarshal([]byte(cfgText), &cfg)
	exact := map[string]bool{load.ConfigName: true, "glossmark.lock": true, "ontology/views.yaml": true,
		load.StateDir + "/graph.jsonl": true, load.StateDir + "/" + ledger.FileName: true, load.StateDir + "/plan.jsonl": true}
	if cfg.Views != "" {
		exact[cfg.Views] = true
	}
	var dirs []string
	for _, d := range cfg.Owns {
		exact[d.File] = true
	}
	for p, im := range cfg.Imports {
		exact["ontology/"+p+".gloss.yaml"] = true
		if src := path.Clean(im.Source); im.Source != "" && !strings.Contains(src, ":") && !strings.HasPrefix(src, "..") && !path.IsAbs(src) {
			exact[src] = true
			dirs = append(dirs, src+"/")
		}
	}
	dirs = append(dirs, load.StateDir+"/imports/")
	out, err := gitOut(s.ws.Root, "ls-tree", "-r", "--full-tree", commit)
	if err != nil {
		return nil, "", err
	}
	var paths []string
	var key strings.Builder
	for _, l := range strings.Split(out, "\n") {
		meta, full, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		rel, ok := strings.CutPrefix(full, s.prefix)
		if !ok {
			continue
		}
		want := exact[rel] || strings.HasSuffix(path.Base(rel), ".gloss.yaml")
		for _, d := range dirs {
			want = want || strings.HasPrefix(rel, d)
		}
		if want {
			paths = append(paths, full)
			f := strings.Fields(meta)
			key.WriteString(rel + "=" + f[len(f)-1] + "\n")
		}
	}
	return paths, key.String(), nil
}

func (s *snapshots) read(commit string, paths []string) *snapshot {
	tmp, err := os.MkdirTemp("", "gloss-history-")
	if err != nil {
		return &snapshot{err: err}
	}
	defer os.RemoveAll(tmp)
	archive := exec.Command("git", append([]string{"-C", s.top, "archive", "--format=tar", commit, "--"}, paths...)...)
	var tar, stderr bytes.Buffer
	archive.Stdout, archive.Stderr = &tar, &stderr
	if err := archive.Run(); err != nil {
		return &snapshot{err: fmt.Errorf("git archive %s: %v: %s", commit, err, strings.TrimSpace(stderr.String()))}
	}
	untar := exec.Command("tar", "-x", "-C", tmp)
	untar.Stdin = &tar
	if err := untar.Run(); err != nil {
		return &snapshot{err: fmt.Errorf("extracting %s: %v", commit, err)}
	}
	bws, err := load.Load(filepath.Join(tmp, filepath.FromSlash(s.prefix)))
	if err != nil {
		return &snapshot{err: fmt.Errorf("loading %s: %v", short(commit), err)}
	}
	sn := &snapshot{ws: bws}
	if lg, err := ledger.Open(bws.StatePath(ledger.FileName)); err == nil {
		sn.ledger = lg.Entries
	} else {
		sn.ledger = map[string]*model.LedgerEntry{}
	}
	if s.verify {
		sn.g, sn.err = loadBaseGraph(s.ws, commit)
		return sn
	}
	f, err := os.Open(bws.StatePath("graph.jsonl"))
	if err != nil {
		sn.g = graph.New(bws.Prefixes)
		return sn
	}
	defer f.Close()
	if sn.g, err = graph.ReadJSONL(f); err != nil {
		return &snapshot{err: fmt.Errorf("%s: %v", short(commit), err)}
	}
	return sn
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// lineage records each replaced-by in g, by CURIE.
func lineage(g *graph.Graph, replaced map[string]string) {
	for _, n := range g.Nodes() {
		if n.ReplacedBy == "" || n.CURIE == "" {
			continue
		}
		to := n.ReplacedBy
		if strings.Contains(to, "://") {
			to = g.Curie(to)
		}
		replaced[n.CURIE] = to
	}
}

// followSet is the subjects to follow and every predecessor and successor
// replaced-by links them to; nil follows everything.
func followSet(follow []string, replaced map[string]string) map[string]bool {
	if len(follow) == 0 {
		return nil
	}
	keep := map[string]bool{}
	for _, f := range follow {
		keep[f] = true
	}
	for grew := true; grew; {
		grew = false
		for from, to := range replaced {
			if keep[from] != keep[to] {
				keep[from], keep[to], grew = true, true, true
			}
		}
	}
	return keep
}

// filterImpact keeps the changes, realizations and version bumps that name a
// followed subject, and recomputes the gate from what is left.
func filterImpact(im *gate.Impact, keep map[string]bool) {
	if keep == nil {
		return
	}
	names := func(xs ...string) bool {
		for _, x := range xs {
			for _, p := range strings.Split(x, ", ") {
				if keep[strings.TrimSuffix(p, "?")] {
					return true
				}
			}
		}
		return false
	}
	var changes []gate.Change
	g := gate.AUTO
	for _, ch := range im.Changes {
		if names(ch.Subject, ch.Object, ch.Boundary) {
			changes = append(changes, ch)
			g = gate.Max(g, ch.Gate)
		}
	}
	var rs []gate.RealizationChange
	for _, r := range im.Realizations {
		if names(append([]string{r.Concept}, append(r.Before, r.After...)...)...) {
			rs = append(rs, r)
		}
	}
	var vs []gate.VersionChange
	for _, v := range im.Versions {
		for k := range keep {
			if strings.HasPrefix(k, v.Namespace+":") {
				vs = append(vs, v)
				break
			}
		}
	}
	im.Changes, im.Realizations, im.Versions, im.Gate = changes, rs, vs, g
}

func renderHistory(h *historyResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Ontology history — %s   range %s   step %s   %d steps", h.Repo, h.Range, h.Step, len(h.Steps))
	if h.Hidden > 0 {
		fmt.Fprintf(&sb, " (%d AUTO-only hidden; --all shows them)", h.Hidden)
	}
	sb.WriteString("\n")
	if h.Truncated {
		sb.WriteString("  shallow clone: history starts at the shallow boundary; fetch with --unshallow or fetch-depth: 0\n")
	}
	if len(h.Lineage) > 0 {
		sb.WriteString("Lineage: " + strings.Join(h.Lineage, "; ") + "\n")
	}
	for _, st := range h.Steps {
		sb.WriteString("\n")
		commit := short(st.Commit)
		if st.From != "" {
			commit = short(st.From) + ".." + commit
		}
		date, _, _ := strings.Cut(st.Date, "T")
		fmt.Fprintf(&sb, "%s  %s  %s  %s", date, commit, st.Author, st.Title)
		if st.Impact != nil {
			fmt.Fprintf(&sb, "   gate %s", st.Impact.Gate)
		}
		sb.WriteString("\n")
		if st.Status == "unbuilt" || st.Status == "skipped" {
			fmt.Fprintf(&sb, "  (%s: %s)\n", st.Status, st.Reason)
		}
		if st.Impact == nil {
			continue
		}
		for _, v := range st.Impact.Versions {
			bump := v.Actual
			if !v.OK {
				bump += ", " + v.Required + " required"
			}
			fmt.Fprintf(&sb, "version %s %s → %s (%s)\n", v.Namespace, v.From, v.To, bump)
		}
		sb.WriteString(gate.RenderRows(st.Impact.Changes))
		if len(st.Impact.Realizations) > 0 {
			sb.WriteString("Realization changes:\n")
			for _, r := range st.Impact.Realizations {
				sb.WriteString("  " + r.String() + "\n")
			}
		}
	}
	return sb.String()
}
