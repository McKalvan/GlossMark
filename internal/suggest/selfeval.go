// @gloss implements gmi:SuggestCalibration scope=file

package suggest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
	"gopkg.in/yaml.v3"
)

// Measurement is suggest scored on a workspace stripped of its markers.
type Measurement struct {
	WS     *load.Workspace // the stripped copy (gone once SelfEval returns)
	Scored []Scored
	Labels []Label
}

// SelfEval measures suggest on a reviewed workspace, as the calibration
// harness does: it copies the workspace's files to a temporary directory
// with every `@gloss` line removed (ontology files and manifests kept),
// suggests on the copy with every conf kept, and scores the proposals
// against labels, or against the workspace's own accepted links when
// labels is nil. Relative import sources are rewritten to point at the
// original, so imports from a sibling checkout still resolve.
func SelfEval(ws *load.Workspace, g *graph.Graph, labels []Label) (*Measurement, error) {
	if labels == nil {
		labels = LabelsFromGraph(g)
	}
	tmp, err := os.MkdirTemp("", "gloss-calibrate-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := StripCopy(ws, tmp); err != nil {
		return nil, err
	}
	cws, err := load.Load(tmp)
	if err != nil {
		return nil, err
	}
	res, err := extract.Build(cws)
	if err != nil {
		return nil, err
	}
	rollup.Derive(res.Graph, cws.Vocab)
	ps, _, err := Run(cws, res.Graph, nil, Options{MinConf: 0.01})
	if err != nil {
		return nil, err
	}
	return &Measurement{WS: cws, Scored: Score(cws, ps, labels), Labels: labels}, nil
}

// StripCopy writes the workspace's files, its config and its ontology
// files under dst, with every marker line dropped from code and docs.
func StripCopy(ws *load.Workspace, dst string) error {
	files := map[string]bool{load.ConfigName: true}
	for _, f := range ws.Files {
		files[f] = true
	}
	for _, f := range []string{"glossmark.lock"} {
		if _, err := os.Stat(filepath.Join(ws.Root, f)); err == nil {
			files[f] = true
		}
	}
	for p, ns := range ws.Config.Owns {
		f := ns.File
		if f == "" {
			f = "ontology/" + p + ".gloss.yaml"
		}
		files[filepath.ToSlash(f)] = true
	}
	if ws.Config.Views != "" {
		files[filepath.ToSlash(ws.Config.Views)] = true
	}
	for _, m := range ws.Manifests {
		if m.Path != "" {
			files[filepath.ToSlash(m.Path)] = true
		}
	}
	for rel := range files {
		src := filepath.Join(ws.Root, filepath.FromSlash(rel))
		b, err := os.ReadFile(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		switch {
		case rel == load.ConfigName:
			if b, err = rebaseImports(b, ws.Root); err != nil {
				return err
			}
		case !keepMarkers(rel):
			var keep []string
			for _, l := range strings.SplitAfter(string(b), "\n") {
				if !strings.Contains(l, "@gloss") {
					keep = append(keep, l)
				}
			}
			b = []byte(strings.Join(keep, ""))
		}
		out := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// keepMarkers: ontology files and manifests are kept whole.
func keepMarkers(rel string) bool {
	return strings.Contains(rel, "ontology") || strings.HasSuffix(rel, ".gloss.yaml") || strings.HasSuffix(rel, ".gloss")
}

// rebaseImports makes relative local import sources absolute against root.
func rebaseImports(b []byte, root string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", load.ConfigName, err)
	}
	if len(doc.Content) == 0 {
		return b, nil
	}
	imports := mapValue(doc.Content[0], "imports")
	if imports == nil {
		return b, nil
	}
	changed := false
	for i := 1; i < len(imports.Content); i += 2 {
		src := mapValue(imports.Content[i], "source")
		if src == nil || src.Value == "" || filepath.IsAbs(src.Value) || strings.Contains(src.Value, "://") || strings.HasPrefix(src.Value, "git+") {
			continue
		}
		src.Value = filepath.Join(root, filepath.FromSlash(src.Value))
		changed = true
	}
	if !changed {
		return b, nil
	}
	return yaml.Marshal(&doc)
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
