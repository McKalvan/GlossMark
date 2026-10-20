// @gloss implements gmi:Doctor scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/doctor"
	"github.com/mckalvan/glossmark/internal/marker"
)

func init() {
	register(command{"doctor", "check the setup end to end: gloss on PATH, GLOSS_ACTOR, hook, committed graph current, ledger in git, lockfile, CI pinned, ontology docs; one line per check with its fix", cmdDoctor})
}

func cmdDoctor(args []string) int {
	fs, root := newFlags("doctor")
	format := fs.String("format", "text", "text | json")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	diags := append([]marker.Diagnostic{}, p.Diags...)
	diags = append(diags, check.Run(p.WS, p.Graph)...)
	checks := doctor.Run(p.WS, p.Graph, diags, doctor.Env{Actor: os.Getenv("GLOSS_ACTOR")})
	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(checks)
	} else {
		fmt.Print(doctor.String(checks))
	}
	if doctor.Failed(checks) {
		return 1
	}
	return 0
}
