// @gloss implements gmi:HtmlViewer scope=file

// Package viewer renders the static HTML viewer. By default the page and its
// data are two files, viewer.html and views.js (plus views.json for tools),
// so the page stays small and the data can be regenerated alone; --inline
// embeds the data in one self-contained page. No server, no external assets.
package viewer

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/views"
)

//go:embed assets/viewer.html
var assets embed.FS

// Data is what the page receives.
type Data struct {
	Generated string              `json:"generated"`
	Repo      string              `json:"repo"`
	Views     []*views.Projection `json:"views"`
	// Diff is the optional impact report from gloss diff --format json.
	Diff json.RawMessage `json:"diff,omitempty"`
	// Concepts holds what the concept pane shows, keyed by CURIE.
	Concepts map[string]*ConceptDetail `json:"concepts,omitempty"`
	// Findings are the drift findings, for the findings panel.
	Findings []rollup.Finding `json:"findings,omitempty"`
	// SourceURL is the source-url template from glossmark.yaml
	// ("https://github.com/o/r/blob/main/{path}#L{line}"); paths link to it.
	SourceURL string `json:"sourceUrl,omitempty"`
	// Classified counts concepts with a criticality, for the empty state.
	Classified int `json:"classified"`
	// Slices is every concept's and boundary's feature slice, for "Slice on this".
	Slices *SliceIndex `json:"slices,omitempty"`
}

// ConceptDetail is the concept pane: what the concept stands for, the code
// that links to it, the tests that verify it, and its drift findings.
type ConceptDetail struct {
	Realizes  []string `json:"realizes,omitempty"`
	Artifacts []Link   `json:"artifacts,omitempty"`
	Verifiers []Link   `json:"verifiers,omitempty"`
	Findings  []string `json:"findings,omitempty"`
	More      int      `json:"more,omitempty"` // links left out by the cap
	// Deprecated and ReplacedBy say the concept is being retired and what
	// takes over; Replaces lists the deprecated concepts it succeeds.
	Deprecated bool     `json:"deprecated,omitempty"`
	ReplacedBy string   `json:"replacedBy,omitempty"`
	Replaces   []string `json:"replaces,omitempty"`
}

// Link is one artifact linked to a concept.
type Link struct {
	Relation string `json:"relation"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
	Proposed bool   `json:"proposed,omitempty"`
	Via      string `json:"via,omitempty"` // the concept linked when not this one
}

const maxLinks = 40

// Details builds the concept pane data for every concept in the graph. A
// concept's artifacts include those linked to the concepts that realize it,
// so a core concept in the business view shows the code behind it.
func Details(g *graph.Graph, rep *rollup.Report) map[string]*ConceptDetail {
	out := map[string]*ConceptDetail{}
	realizers := map[string][]string{} // core IRI → concepts realizing it (transitively)
	for _, n := range g.NodesOf(model.NodeConcept) {
		for _, c := range rollup.RealizesStar(g, n.ID) {
			realizers[c] = append(realizers[c], n.ID)
		}
	}
	for _, n := range g.NodesOf(model.NodeConcept) {
		d := &ConceptDetail{Deprecated: n.Deprecated, ReplacedBy: n.ReplacedBy}
		for _, e := range g.In(n.ID, "replaced-by") {
			d.Replaces = append(d.Replaces, g.Curie(e.S))
		}
		sort.Strings(d.Replaces)
		for _, c := range rollup.RealizesStar(g, n.ID) {
			d.Realizes = append(d.Realizes, g.Curie(c))
		}
		for _, id := range append([]string{n.ID}, realizers[n.ID]...) {
			via := ""
			if id != n.ID {
				via = g.Curie(id)
			}
			for _, e := range g.In(id, "") {
				if e.Source != graph.SourceInline && e.Source != graph.SourceInferred || e.Prov == nil {
					continue
				}
				if a := g.Node(e.S); a == nil || a.NodeType != model.NodeArtifact {
					continue
				}
				l := Link{Relation: e.P, Path: e.Prov.Path, Line: e.Prov.Line, Symbol: e.Prov.Symbol, Proposed: e.Prov.Proposed, Via: via}
				if e.P == "verifies" || e.P == "tests" {
					if len(d.Verifiers) < maxLinks {
						d.Verifiers = append(d.Verifiers, l)
					} else {
						d.More++
					}
				} else if len(d.Artifacts) < maxLinks {
					d.Artifacts = append(d.Artifacts, l)
				} else {
					d.More++
				}
			}
		}
		sortLinks(d.Artifacts)
		sortLinks(d.Verifiers)
		out[n.CURIE] = d
	}
	if rep != nil {
		for _, f := range rep.Findings {
			if d := out[f.Concept]; d != nil && f.Kind != rollup.KindConsistent {
				d.Findings = append(d.Findings, f.Kind+": "+f.Msg)
			}
		}
	}
	for k, d := range out {
		if len(d.Realizes)+len(d.Artifacts)+len(d.Verifiers)+len(d.Findings)+len(d.Replaces) == 0 && !d.Deprecated {
			delete(out, k)
		}
	}
	return out
}

func sortLinks(ls []Link) {
	sort.SliceStable(ls, func(i, j int) bool {
		if ls[i].Path != ls[j].Path {
			return ls[i].Path < ls[j].Path
		}
		return ls[i].Line < ls[j].Line
	})
}

const placeholder = "__GLOSS_DATA__"
const scriptSlot = "<!--GLOSS_DATA_SCRIPT-->"

// DataFile is the name of the data script the split page loads.
const DataFile = "views.js"

func template() ([]byte, error) {
	tpl, err := assets.ReadFile("assets/viewer.html")
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(tpl, []byte(placeholder)) || !bytes.Contains(tpl, []byte(scriptSlot)) {
		return nil, fmt.Errorf("viewer template lacks its data placeholders")
	}
	return tpl, nil
}

func safeJSON(d Data) ([]byte, error) {
	b, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	// Keep the JSON safe inside a <script> element.
	b = bytes.ReplaceAll(b, []byte("</"), []byte(`<\/`))
	b = bytes.ReplaceAll(b, []byte("<!--"), []byte(`<\!--`))
	return b, nil
}

// Render produces one self-contained HTML page with the data embedded.
func Render(d Data) ([]byte, error) {
	tpl, err := template()
	if err != nil {
		return nil, err
	}
	b, err := safeJSON(d)
	if err != nil {
		return nil, err
	}
	tpl = bytes.Replace(tpl, []byte(scriptSlot), nil, 1)
	return bytes.Replace(tpl, []byte(placeholder), b, 1), nil
}

// RenderSplit produces the page and its data script (views.js). The page
// loads the script with a relative <script src>, which works from file://.
func RenderSplit(d Data) (page, script []byte, err error) {
	tpl, err := template()
	if err != nil {
		return nil, nil, err
	}
	b, err := safeJSON(d)
	if err != nil {
		return nil, nil, err
	}
	tpl = bytes.Replace(tpl, []byte(scriptSlot), []byte(`<script src="`+DataFile+`"></script>`), 1)
	page = bytes.Replace(tpl, []byte(placeholder), []byte("null"), 1)
	script = append(append([]byte("window.GLOSS_DATA="), b...), []byte(";\n")...)
	return page, script, nil
}
