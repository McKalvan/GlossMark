// @gloss implements gmi:ImportLock scope=file

package main

import (
	"fmt"

	"github.com/mckalvan/glossmark/internal/load"
)

func init() {
	register(command{"lock", "pin the resolved version and content hash of every import in glossmark.lock", cmdLock})
}

func cmdLock(args []string) int {
	fs, root := newFlags("lock")
	if err := fs.Parse(args); err != nil {
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
	if err := load.WriteLock(p.WS.Root, p.WS.Resolved); err != nil {
		return fail(err)
	}
	for prefix, r := range p.WS.Resolved {
		fmt.Printf("gloss lock: %s: %s (%s) %s\n", prefix, orNone(r.Version), orNone(r.Range), r.Hash[:19])
	}
	fmt.Printf("gloss lock: written %s; commit it\n", load.LockName)
	return 0
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
