// @gloss implements gmi:ImportLock scope=file

package main

import (
	"fmt"
	"os"

	"github.com/mckalvan/glossmark/internal/load"
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
	return 0
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
