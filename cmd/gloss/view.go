// @gloss implements gmi:ViewProjector scope=file
// @gloss produces gm:View scope=file

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/viewer"
	"github.com/mckalvan/glossmark/internal/views"
)

func init() {
	register(command{"view", "render the static HTML viewer (or --json) with every declared view precomputed", cmdView})
}

func cmdView(args []string) int {
	fs, root := newFlags("view")
	out := fs.String("o", "", "output path (default .glossmark/viewer.html)")
	asJSON := fs.Bool("json", false, "write only the data as JSON (views.json)")
	inline := fs.Bool("inline", false, "embed the data in one self-contained viewer.html instead of viewer.html + views.js")
	diffPath := fs.String("diff", "", "optional impact JSON from 'gloss diff --format json' to overlay")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	rep := rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: p.WS.Config.EffectivePolicy(), Now: time.Now()})
	var findings []rollup.Finding
	for _, f := range rep.Findings {
		if f.Kind != rollup.KindConsistent && len(findings) < 500 {
			findings = append(findings, f)
		}
	}
	data := viewer.Data{Generated: time.Now().UTC().Format(time.RFC3339), Repo: filepath.Base(p.WS.Root), Views: views.ProjectAll(p.Graph, p.WS.Views),
		Concepts: viewer.Details(p.Graph, rep), Findings: findings, SourceURL: p.WS.Config.SourceURL, Slices: viewer.Slices(p.Graph, viewer.SliceBudget)}
	for _, n := range p.Graph.NodesOf(model.NodeConcept) {
		if n.Criticality != "" && !n.Imported {
			data.Classified++
		}
	}
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail(err)
	}
	write := func(path string, b []byte) bool {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			fail(err)
			return false
		}
		return true
	}
	switch {
	case *asJSON:
		content, err := json.MarshalIndent(data, "", "  ")
		if err != nil || !write(path, content) {
			return 1
		}
		fmt.Printf("gloss view: %d views → %s\n", len(data.Views), relOrAbs(p.WS.Root, path))
	case *inline:
		content, err := viewer.Render(data)
		if err != nil {
			return fail(err)
		}
		if !write(path, content) {
			return 1
		}
		fmt.Printf("gloss view: %d views → %s (%d KB, data embedded)\n", len(data.Views), relOrAbs(p.WS.Root, path), len(content)/1024)
	default:
		page, script, err := viewer.RenderSplit(data)
		if err != nil {
			return fail(err)
		}
		js := filepath.Join(filepath.Dir(path), viewer.DataFile)
		jsonPath := filepath.Join(filepath.Dir(path), "views.json")
		raw, _ := json.Marshal(data)
		if !write(path, page) || !write(js, script) || !write(jsonPath, raw) {
			return 1
		}
		fmt.Printf("gloss view: %d views → %s (%d KB) + %s (%d KB data) + views.json\n", len(data.Views), relOrAbs(p.WS.Root, path), len(page)/1024, relOrAbs(p.WS.Root, js), len(script)/1024)
	}
	return 0
}
