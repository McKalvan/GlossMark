// @gloss implements gmi:LandscapeAssembler scope=file

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// importRow is one importer × namespace line of the landscape's Imports
// section: what the importer asks for, has locked and resolves, and what the
// source offers now.
type importRow struct {
	Repo      string `json:"repo"`
	Namespace string `json:"namespace"`
	Range     string `json:"range,omitempty"`
	Locked    string `json:"locked,omitempty"`
	Resolved  string `json:"resolved,omitempty"`
	Available string `json:"available,omitempty"` // what the source resolves to now, when it differs
	// Status: ok, not locked, lock behind, out of range, content changed without bump.
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// repoEntry is one repository in the landscape's JSON: its namespaces with
// their versions and its imports.
type repoEntry struct {
	Path    string            `json:"path"`
	Name    string            `json:"name"`
	Owns    map[string]string `json:"owns,omitempty"` // prefix → version
	Imports []importRow       `json:"imports,omitempty"`
}

func orNoneV(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

// importState classifies one resolved import of a workspace.
func importState(ws *load.Workspace, name string, r *load.Resolved) importRow {
	row := importRow{Repo: name, Namespace: r.Prefix, Range: r.Range, Resolved: r.Version, Status: "ok"}
	up := r
	if r.Pinned && r.Upstream != nil {
		up = r.Upstream
	}
	if up.Version != r.Version || up.Hash != r.Hash {
		row.Available = orNoneV(up.Version)
	}
	var le *load.LockEntry
	if ws.Lock != nil {
		le = ws.Lock.Imports[r.Prefix]
	}
	if le != nil {
		row.Locked = orNoneV(le.Version)
	}
	outside := func(v string) bool {
		code, _ := load.RangeProblem(&load.Resolved{Prefix: r.Prefix, Range: r.Range, Version: v})
		return code == "GM106"
	}
	switch {
	case outside(r.Version):
		row.Status = "out of range"
		row.Detail = fmt.Sprintf("resolves %s, requires %s", orNoneV(r.Version), r.Range)
	case le == nil:
		row.Status = "not locked"
	case up.Version == le.Version && up.Hash != le.Hash:
		row.Status = "content changed without bump"
		row.Detail = fmt.Sprintf("%s changed since it was locked; the owner should bump", orNoneV(le.Version))
	case up.Version != le.Version:
		row.Status = "lock behind"
		in := "inside"
		if outside(up.Version) {
			in = "outside"
		}
		row.Detail = fmt.Sprintf("%s available, %s %s", orNoneV(up.Version), in, orNoneV(r.Range))
	}
	return row
}

// landscapeVersions builds the per-repository version state and the
// out-of-range findings for the landscape.
func landscapeVersions(repos []string, wss []*load.Workspace) ([]repoEntry, []importRow, []rollup.Finding) {
	var entries []repoEntry
	var rows []importRow
	var finds []rollup.Finding
	for i, ws := range wss {
		name := filepath.Base(ws.Root)
		e := repoEntry{Path: repos[i], Name: name, Owns: map[string]string{}}
		for p := range ws.Owned {
			if o := ws.Ontologies[p]; o != nil {
				e.Owns[p] = o.Version
			}
		}
		prefixes := make([]string, 0, len(ws.Resolved))
		for p := range ws.Resolved {
			prefixes = append(prefixes, p)
		}
		sort.Strings(prefixes)
		for _, p := range prefixes {
			row := importState(ws, name, ws.Resolved[p])
			e.Imports = append(e.Imports, row)
			rows = append(rows, row)
			if row.Status == "out of range" {
				finds = append(finds, rollup.Finding{ID: rollup.KindOutOfRange + ":" + name + ":" + p, Kind: rollup.KindOutOfRange, Severity: "error", Concept: p + ":", Path: name + "/" + load.ConfigName, Line: 1,
					Msg: fmt.Sprintf("%s imports %s: %s but resolves %s", name, p, row.Range, orNoneV(row.Resolved))})
			}
		}
		entries = append(entries, e)
	}
	return entries, rows, finds
}

// renderImports renders the Imports section of the text landscape.
func renderImports(rows []importRow) string {
	if len(rows) == 0 {
		return ""
	}
	head := []string{"importer", "namespace", "range", "locked", "resolved", "status"}
	cells := [][]string{head}
	for _, r := range rows {
		st := r.Status
		if r.Detail != "" {
			st += " (" + r.Detail + ")"
		}
		cells = append(cells, []string{r.Repo, r.Namespace, orNoneV(r.Range), orNoneV(r.Locked), orNoneV(r.Resolved), st})
	}
	w := make([]int, len(head))
	for _, c := range cells {
		for i, s := range c {
			w[i] = max(w[i], len(s))
		}
	}
	var sb strings.Builder
	sb.WriteString("\nImports\n")
	for _, c := range cells {
		sb.WriteString(" ")
		for i, s := range c {
			if i == len(c)-1 {
				sb.WriteString(" " + s)
			} else {
				fmt.Fprintf(&sb, " %-*s", w[i], s)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
