// @gloss implements gmi:ImportLock scope=file

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/mapping"
)

func init() {
	register(command{"lock", "pin every import: record its version and hash in glossmark.lock and vendor it under .glossmark/imports/ (--update moves to the upstream)", cmdLock})
}

func cmdLock(args []string) int {
	fs, root := newFlags("lock")
	update := fs.Bool("update", false, "move every pinned import to what its source resolves to now")
	allow := fs.Bool("allow-out-of-range", false, "lock a version that is absent or outside the import's range")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if len(p.WS.Resolved) == 0 {
		fmt.Println("gloss lock: no imports to pin")
		return 0
	}
	// A pinned import stays at its copy unless --update; anything else
	// (new, re-sourced, or locked before copies existed) takes the upstream.
	sel := map[string]*load.Resolved{}
	for prefix, r := range p.WS.Resolved {
		if *update && r.Upstream != nil {
			r = r.Upstream
		}
		sel[prefix] = r
	}
	refused := 0
	for _, prefix := range load.SortedPrefixes(sel) {
		if code, msg := load.RangeProblem(sel[prefix]); code != "" && !*allow {
			if sel[prefix].Pinned {
				msg += "; this is the pinned copy, gloss lock --update takes the upstream"
			}
			fmt.Fprintf(os.Stderr, "gloss lock: %s [%s]\n", msg, code)
			refused++
		}
	}
	if refused > 0 {
		fmt.Fprintf(os.Stderr, "gloss lock: %s left unchanged; fix the range or the upstream, or pass --allow-out-of-range\n", load.LockName)
		return 1
	}
	if err := load.WriteLock(p.WS.Root, sel); err != nil {
		return fail(err)
	}
	for _, prefix := range load.SortedPrefixes(sel) {
		r := sel[prefix]
		note := ""
		if up := r.Upstream; up != nil && up.Hash != r.Hash {
			note = fmt.Sprintf(" (pinned; upstream is %s, gloss lock --update to move)", orNone(up.Version))
		}
		fmt.Printf("gloss lock: %s: %s (%s) %s%s\n", prefix, orNone(r.Version), orNone(r.Range), r.Hash[:19], note)
	}
	fmt.Printf("gloss lock: written %s and %s/%s/; commit them\n", load.LockName, load.StateDir, load.ImportsDir)
	if *update {
		reportMappingsMoved(p, sel)
	}
	return 0
}

// reportMappingsMoved lists the mappings whose imported endpoint changed
// between the copy that was pinned and the one just locked (ADR 0025 §4):
// they go stale on the next build and need a review.
func reportMappingsMoved(before *pipeline, sel map[string]*load.Resolved) {
	moved := map[string]bool{}
	for prefix, r := range before.WS.Resolved {
		if n := sel[prefix]; n != nil && n.Hash != r.Hash {
			moved[prefix] = true
		}
	}
	if len(moved) == 0 || len(before.Res.Mappings) == 0 {
		return
	}
	after, err := buildGraph(before.WS.Root, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gloss lock: could not compare mappings after the update: %v\n", err)
		return
	}
	var lines []string
	for _, fm := range before.Res.Mappings {
		if fm.Negative || !fm.Supported || len(fm.Unresolved) > 0 || !fm.Row.Reviewed() {
			continue
		}
		for _, end := range []string{fm.Subject, fm.Object} {
			c := before.Graph.Curie(end)
			if !moved[nsOf(c)] {
				continue
			}
			was, now := mapping.Facts(before.Graph, end), mapping.Facts(after.Graph, end)
			switch {
			case was != nil && now == nil:
				lines = append(lines, fmt.Sprintf("  %s %s %s: %s no longer exists upstream (dangling)", fm.SubjectCURIE, fm.Predicate.CURIE, fm.ObjectCURIE, c))
			case was != nil:
				if ch := mapping.ChangedFacts(was, now); len(ch) > 0 {
					lines = append(lines, fmt.Sprintf("  %s %s %s: %s %s changed", fm.SubjectCURIE, fm.Predicate.CURIE, fm.ObjectCURIE, c, strings.Join(ch, ", ")))
				}
			}
		}
	}
	if len(lines) == 0 {
		fmt.Println("gloss lock: no accepted mapping's imported endpoint changed")
		return
	}
	fmt.Printf("gloss lock: %s whose imported endpoint changed; they go stale on the next build, review each with gloss map accept or reject:\n", plural(len(lines), "mapping"))
	for _, l := range lines {
		fmt.Println(l)
	}
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
