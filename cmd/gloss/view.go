package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mckalvan/glossmark/internal/viewer"
	"github.com/mckalvan/glossmark/internal/views"
)

func init() {
	register(command{"view", "render the static HTML viewer (or --json) with every declared view precomputed", cmdView})
}

func cmdView(args []string) int {
	fs, root := newFlags("view")
	out := fs.String("o", "", "output path (default .glossmark/viewer.html)")
	asJSON := fs.Bool("json", false, "write the projections as JSON instead of HTML")
	diffPath := fs.String("diff", "", "optional impact JSON from 'gloss diff --format json' to overlay")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	data := viewer.Data{Generated: time.Now().UTC().Format(time.RFC3339), Repo: filepath.Base(p.WS.Root), Views: views.ProjectAll(p.Graph, p.WS.Views)}
	if *diffPath != "" {
		b, err := os.ReadFile(*diffPath)
		if err != nil {
			return fail(err)
		}
		var raw json.RawMessage = b
		data.Diff = raw
	}
	path := *out
	if path == "" {
		if *asJSON {
			path = p.WS.StatePath("views.json")
		} else {
			path = p.WS.StatePath("viewer.html")
		}
	}
	var content []byte
	if *asJSON {
		content, err = json.MarshalIndent(data, "", "  ")
	} else {
		content, err = viewer.Render(data)
	}
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fail(err)
	}
	fmt.Printf("gloss view: %d views → %s\n", len(data.Views), relOrAbs(p.WS.Root, path))
	return 0
}
