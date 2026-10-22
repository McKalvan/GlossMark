// @gloss implements gmi:ReviewEngine scope=file

package review

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Detail is an item with what a reviewer reads before deciding it.
type Detail struct {
	Item
	// Context is the marker (or concept entry) and the lines after it.
	Context []string `json:"context,omitempty"`
	// History is what changed in the file since Since: the commits, then
	// the uncommitted diff. Capped at HistoryLines lines.
	History  string        `json:"history,omitempty"`
	Concepts []ConceptInfo `json:"concepts,omitempty"`
}

// ConceptInfo is a target concept's definition.
type ConceptInfo struct {
	Curie  string `json:"curie"`
	Exists bool   `json:"exists"`
	Kind   string `json:"kind,omitempty"`
	Tier   string `json:"tier,omitempty"`
	Doc    string `json:"doc,omitempty"`
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
}

// HistoryLines caps Detail.History.
const HistoryLines = 400

// contextLines is how much of the file Detail.Context shows.
const contextLines = 12

// Detail returns the item with its context, history and concepts.
func (e *Engine) Detail(id string) (*Detail, error) {
	it, err := e.Find(id)
	if err != nil {
		return nil, err
	}
	st, err := e.State()
	if err != nil {
		return nil, err
	}
	d := &Detail{Item: *it}
	if it.Path != "" && it.Line > 0 {
		d.Context = fileLines(st.WS.Abs(it.Path), it.Line, contextLines)
	}
	if it.Kind == KindStaleLink && it.Path != "" {
		d.History = history(e.Root, it.Path, it.Since)
	}
	curies := it.Targets
	if it.Concept != "" {
		curies = []string{it.Concept}
	}
	for _, c := range curies {
		ci := ConceptInfo{Curie: c}
		if n := st.Graph.NodeByCurie(resolveLocal(st, it.Path, c)); n != nil {
			ci = ConceptInfo{Curie: n.CURIE, Exists: true, Kind: n.Kind, Tier: n.Tier, Doc: n.Doc, Path: n.File, Line: n.Line}
		}
		d.Concepts = append(d.Concepts, ci)
	}
	return d, nil
}

// resolveLocal expands a .Local target with the default namespace of the
// file's manifest.
func resolveLocal(st *State, path, c string) string {
	if !strings.HasPrefix(c, ".") {
		return c
	}
	if m := st.WS.ManifestFor(path); m != nil && m.DefaultNamespace != "" {
		return m.DefaultNamespace + ":" + c[1:]
	}
	return c
}

// fileLines returns n numbered lines of a file from line from (1-based).
func fileLines(path string, from, n int) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(b), "\n")
	var out []string
	for i := from - 1; i >= 0 && i < len(lines) && i < from-1+n; i++ {
		out = append(out, fmt.Sprintf("%4d  %s", i+1, lines[i]))
	}
	return out
}

// history is the file's commits since a time, with patches, then its
// uncommitted diff; empty outside git.
func history(root, path, since string) string {
	var buf bytes.Buffer
	if since != "" {
		out, _ := exec.Command("git", "-C", root, "log", "-p", "--format=commit %h %ad %an%n    %s", "--date=short", "--since="+since, "--", path).Output()
		buf.Write(out)
	}
	if out, _ := exec.Command("git", "-C", root, "diff", "HEAD", "--", path).Output(); len(out) > 0 {
		buf.WriteString("uncommitted:\n")
		buf.Write(out)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) > HistoryLines {
		lines = append(lines[:HistoryLines], fmt.Sprintf("... %d more lines (git log -p --since=%s -- %s)", len(lines)-HistoryLines, since, path))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
