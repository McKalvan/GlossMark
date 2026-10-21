// @gloss implements gmi:EvidenceHarvester scope=file

package harvest

import (
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
