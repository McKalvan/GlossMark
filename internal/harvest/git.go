// @gloss implements gmi:EvidenceHarvester scope=file

package harvest

import (
	"bufio"
	"os/exec"
	"path"
	"strings"

	"github.com/mckalvan/glossmark/internal/model"
)

// gitRenames reads the history for files whose name changed (not just
// their directory): the old name is a retired surface of the new one.
// Outside a git repository it returns nothing.
func gitRenames(root string, unitFor func(string) string, exclude []string) []Evidence {
	out, err := exec.Command("git", "-C", root, "log", "--diff-filter=R", "-M", "--name-status", "--format=commit %h").Output()
	if err != nil {
		return nil
	}
	var ev []Evidence
	seen := map[string]bool{}
	rev := ""
	for _, ln := range strings.Split(string(out), "\n") {
		if r, ok := strings.CutPrefix(ln, "commit "); ok {
			rev = r
			continue
		}
		f := strings.Split(ln, "\t")
		if len(f) != 3 || !strings.HasPrefix(f[0], "R") {
			continue
		}
		from, to := f[1], f[2]
		if model.MatchAnyGlob(exclude, to) || model.MatchAnyGlob(exclude, from) {
			continue
		}
		old, cur := stemOf(from), stemOf(to)
		if old == cur || genericModules[strings.ToLower(old)] || seen[old+"\x00"+cur] {
			continue
		}
		seen[old+"\x00"+cur] = true
		e := Evidence{Unit: unitFor(to), Signal: "rename", Surface: old, Source: Source{Family: FamilyHistory, Path: to, Rev: rev}, Context: []string{"renamed-to:" + cur, "from:" + from}}
		e.ID = idFor(e)
		ev = append(ev, e)
	}
	return ev
}

func stemOf(p string) string {
	b := path.Base(p)
	return strings.TrimSuffix(b, path.Ext(b))
}

// maxSeenLine bounds the diff lines gitSeen tokenizes: a minified bundle or
// a data dump names nothing a person wrote.
const maxSeenLine = 400

// gitSeen dates every surface from the history (ADR 0025, section 4): the
// first commit whose diff adds a line naming it, and, for an observation of
// the current tree, the HEAD commit; for one from history (a rename), the
// last commit whose diff names it, so a retired term reads as retired. A
// surface is named by a line holding its stems as a contiguous run, as
// --cluster counts mentions. Outside a git repository nothing changes.
func gitSeen(root string, ev []Evidence, exclude []string) {
	head, err := exec.Command("git", "-C", root, "log", "-1", "--format=%cs").Output()
	if err != nil {
		return
	}
	headDate := strings.TrimSpace(string(head))
	type span struct{ first, last string }
	seqs := map[string][]string{}
	first := map[string][]string{} // first stem → surfaces starting with it
	for _, e := range ev {
		if _, ok := seqs[e.Surface]; ok {
			continue
		}
		toks := Tokens(e.Surface)
		if len(toks) == 0 || len(toks) == 1 && len(toks[0]) < 3 {
			seqs[e.Surface] = nil
			continue
		}
		seqs[e.Surface] = toks
		first[toks[0]] = append(first[toks[0]], e.Surface)
	}
	cmd := exec.Command("git", "-C", root, "log", "--reverse", "--no-renames", "--no-color", "--no-ext-diff", "-p", "--unified=0", "--format=%x01%cs")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if cmd.Start() != nil {
		return
	}
	seen := map[string]*span{}
	date, skip := "", false
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		ln := sc.Text()
		switch {
		case strings.HasPrefix(ln, "\x01"):
			date = strings.TrimSpace(ln[1:])
			continue
		case strings.HasPrefix(ln, "diff --git "):
			f := strings.Fields(ln)
			p := strings.TrimPrefix(f[len(f)-1], "b/")
			skip = model.MatchAnyGlob(exclude, p)
			continue
		case strings.HasPrefix(ln, "+++ "), strings.HasPrefix(ln, "--- "):
			continue
		}
		if skip || len(ln) < 2 || len(ln) > maxSeenLine || ln[0] != '+' && ln[0] != '-' {
			continue
		}
		added := ln[0] == '+'
		toks := Tokens(ln[1:])
		for i := range toks {
			for _, s := range first[toks[i]] {
				if !runAt(toks, i, seqs[s]) {
					continue
				}
				sp := seen[s]
				if sp == nil {
					if !added {
						continue // removed before it was ever added: older than the history
					}
					sp = &span{first: date}
					seen[s] = sp
				}
				sp.last = date
			}
		}
	}
	cmd.Wait()
	for i := range ev {
		sp := seen[ev[i].Surface]
		if sp == nil {
			continue
		}
		ev[i].FirstSeen = sp.first
		if ev[i].Source.Family == FamilyHistory {
			ev[i].LastSeen = sp.last
		} else {
			ev[i].LastSeen = headDate
		}
	}
}
