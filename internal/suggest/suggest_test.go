// @gloss verifies gmi:MarkerSuggester scope=file

package suggest

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func copyStripped(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			if info.Name() == ".glossmark" {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !strings.Contains(rel, "ontology") && !strings.HasSuffix(rel, ".gloss.yaml") && !strings.HasSuffix(rel, ".md") && !strings.HasSuffix(rel, ".gloss") {
			var keep []string
			for _, l := range strings.SplitAfter(string(b), "\n") {
				if !strings.Contains(l, "@gloss") {
					keep = append(keep, l)
				}
			}
			b = []byte(strings.Join(keep, ""))
		}
		return os.WriteFile(target, b, info.Mode())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func build(t *testing.T, root string) (*load.Workspace, *graph.Graph) {
	t.Helper()
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := extract.Build(ws)
	if err != nil {
		t.Fatal(err)
	}
	rollup.Derive(res.Graph, ws.Vocab)
	return ws, res.Graph
}

// TestSuggestRecoversMarkers strips every code marker from horse-factory and
// checks that suggest proposes reviewable `@gloss?` markers that pass check
// with no kind warnings, inserts above decorators, and stops once marked.
func TestSuggestRecoversMarkers(t *testing.T) {
	root := t.TempDir()
	copyStripped(t, filepath.Join("..", "..", "examples", "horse-factory"), root)
	os.WriteFile(filepath.Join(root, "services/horse-factory/breeding/api.py"), []byte("import app\n\n\n@app.route(\n    \"/hooves\",\n)\ndef hoof_assembler_endpoint():\n    pass\n\n\ndef _private_helper():\n    pass\n"), 0o644)
	ws, g := build(t, root)
	ps, err := Run(ws, g, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Proposal{}
	for _, p := range ps {
		got[p.Path+"#"+p.Symbol] = p
		if !strings.Contains(p.Marker, "@gloss? ") || p.Conf <= 0 || p.Conf > 0.95 {
			t.Errorf("not a reviewable proposal: %+v", p)
		}
	}
	if len(ps) < 8 {
		t.Errorf("only %d proposals: %+v", len(ps), ps)
	}
	if p, ok := got["services/horse-factory/models/mare.ts#Mare"]; !ok || p.Relation != "represents" || p.Target != "zoo:Mare" {
		t.Errorf("Mare: %+v", p)
	}
	if p, ok := got["services/horse-factory/breeding/api.py#hoof_assembler_endpoint"]; !ok || p.Line != 4 {
		t.Errorf("decorated function should get a marker above the decorator (line 4): %+v", p)
	}
	if _, ok := got["services/horse-factory/breeding/api.py#_private_helper"]; ok {
		t.Error("private helpers get no proposal")
	}
	patch, err := Patch(root, ps)
	if err != nil || !strings.Contains(patch, "+-- @gloss? represents zoo:Foal") {
		t.Errorf("patch:\n%s", patch)
	}
	if err := Write(root, ps); err != nil {
		t.Fatal(err)
	}
	ws, g = build(t, root)
	for _, d := range check.Run(ws, g) {
		if d.Severity == "error" || d.Code == "GM221" {
			t.Errorf("after write: %s", d)
		}
	}
	if again, _ := Run(ws, g, nil, Options{}); len(again) != 0 {
		t.Errorf("a second run should propose nothing, got %+v", again)
	}
}

// TestMinScoreIsTheFloor: no proposal scores below MinScore, docstring
// matches included, so --min-score 60 emits nothing under conf 0.55.
func TestMinScoreIsTheFloor(t *testing.T) {
	root := t.TempDir()
	copyStripped(t, filepath.Join("..", "..", "examples", "horse-factory"), root)
	ws, g := build(t, root)
	prev := -1
	for _, min := range []int{20, 40, 60, 80, 100} {
		ps, err := Run(ws, g, nil, Options{MinScore: min})
		if err != nil {
			t.Fatal(err)
		}
		floor := math.Round(float64(min)/110*100) / 100
		for _, p := range ps {
			if p.Conf < floor {
				t.Errorf("min-score %d: %s %s conf=%.2f is below %.2f", min, p.Symbol, p.Target, p.Conf, floor)
			}
		}
		if prev >= 0 && len(ps) > prev {
			t.Errorf("min-score %d proposes more (%d) than a lower floor (%d)", min, len(ps), prev)
		}
		prev = len(ps)
	}
}
