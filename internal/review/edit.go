// @gloss implements gmi:ReviewEngine scope=file

package review

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/ontoedit"
)

// RenderMarker renders a marker line in the file's comment style, or "" when
// the file type has no comments (its links go in a .gloss sidecar).
func RenderMarker(ws *load.Workspace, path string, d Decision) string {
	style, ok := marker.StyleFor(path, ws.Config.CommentStyles)
	if !ok {
		return ""
	}
	at := "@gloss?"
	if d.Accepted {
		at = "@gloss"
	}
	line := at + " " + d.Relation + " " + d.Target
	if d.Scope != "" {
		line += " scope=" + d.Scope
	}
	if d.Conf > 0 {
		line += fmt.Sprintf(" conf=%.2f", d.Conf)
	}
	if d.Note != "" {
		line += " note=" + strconv.Quote(d.Note)
	}
	switch {
	case len(style.Line) > 0 && style.Line[0] != "":
		return style.Line[0] + " " + line
	case len(style.Block) > 0:
		return style.Block[0][0] + " " + line + " " + style.Block[0][1]
	}
	return ""
}

// scanned reports whether path is a file the build scans for markers, named
// as the walk names it: a link may go into nothing else. That keeps an agent
// out of glossmark.yaml, manifests and ontology files (./glossmark.yaml and
// a/../ontology/x.gloss.yaml included) and out of anything outside the root.
func scanned(ws *load.Workspace, path string) bool {
	i := sort.SearchStrings(ws.Files, path)
	if i == len(ws.Files) || ws.Files[i] != path {
		return false
	}
	_, err := ws.Within(path)
	return err == nil
}

// plainFields refuses decision fields that would not stay one marker on one
// line: a newline in a target or scope writes a second, unreviewed line.
func plainFields(d Decision) error {
	for _, f := range []struct{ name, v string }{{"relation", d.Relation}, {"target", d.Target}, {"scope", d.Scope}} {
		if strings.ContainsFunc(f.v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return fmt.Errorf("write-link: %s %q holds whitespace", f.name, f.v)
		}
	}
	if d.Scope != "" && !slices.Contains(model.Scopes, d.Scope) {
		return fmt.Errorf("write-link: invalid scope %q; allowed: %s", d.Scope, strings.Join(model.Scopes, ", "))
	}
	return nil
}

// writeLinkAction classifies a link to write the way gloss diff would. An
// agent writes proposals only; an accepted marker is a person's decision.
func writeLinkAction(st *State, d Decision, actor string) (*Action, error) {
	ws := st.WS
	if r := ws.Vocab.Get(d.Relation); r == nil || !r.Inline {
		return nil, fmt.Errorf("write-link: relation %q cannot be used inline", d.Relation)
	}
	if !scanned(ws, d.Path) {
		return nil, fmt.Errorf("write-link: %s is outside the workspace or excluded", d.Path)
	}
	if err := plainFields(d); err != nil {
		return nil, err
	}
	target := d.Target
	if strings.HasPrefix(target, ".") {
		if m := ws.ManifestFor(d.Path); m != nil && m.DefaultNamespace != "" {
			target = m.DefaultNamespace + ":" + target[1:]
		}
	}
	oiri := "urn:gloss:new:" + target
	if !marker.IsNewTarget(target) {
		var ok bool
		if oiri, ok = st.Graph.Expand(target); !ok || st.Graph.Node(oiri) == nil {
			return nil, fmt.Errorf("write-link: no concept %s; write %s to propose one", target, strings.Replace(target, ":", ":NEW:", 1))
		}
	}
	e := &graph.Edge{S: graph.ArtifactPrefix + d.Path, P: d.Relation, O: oiri, Source: graph.SourceInline,
		Prov: &graph.Prov{Path: d.Path, Proposed: !d.Accepted, Actor: actor, ID: extract.MarkerID(d.Path, d.Relation, []string{target}, "", "file")}}
	if d.Conf > 0 {
		c := d.Conf
		e.Prov.Conf = &c
	}
	cls, why := gate.ClassifyLink(st.Graph, ws, e, ws.Config.EffectivePolicy())
	a := action(ActWriteLink, cls, why)
	a.AgentMay = !d.Accepted
	return &a, nil
}

// writeLink inserts the marker before line d.Line (1-based; 0 or 1 is the top
// of the file, after a #! line), indented like the line it precedes.
func writeLink(st *State, d Decision) (string, error) {
	text := RenderMarker(st.WS, d.Path, d)
	if text == "" {
		return "", fmt.Errorf("write-link: %s has no comment syntax; write the link in %s.gloss", d.Path, d.Path)
	}
	if !scanned(st.WS, d.Path) {
		return "", fmt.Errorf("write-link: %s is outside the workspace or excluded", d.Path)
	}
	path := st.WS.Abs(d.Path)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(b), "\n")
	at := d.Line - 1
	if at <= 0 {
		at = 0
		if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
			at = 1
		}
	}
	if at > len(lines) {
		at = len(lines)
	}
	indent := ""
	if at < len(lines) {
		indent = lines[at][:len(lines[at])-len(strings.TrimLeft(lines[at], " \t"))]
	}
	lines = append(lines[:at], append([]string{indent + text}, lines[at:]...)...)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %s:%d: %s", d.Path, at+1, text), nil
}

// editDocAction: a doc is a concept's definition, so editing it is HARD.
func editDocAction(st *State, d Decision) (*Action, error) {
	if _, _, err := ownedConcept(st, d.Curie); err != nil {
		return nil, err
	}
	a := action(ActEditDoc, gate.HARD, []string{"a concept's definition is a human decision"}, "curie", "doc")
	return &a, nil
}

func editDoc(st *State, d Decision) (string, error) {
	path, name, err := ownedConcept(st, d.Curie)
	if err != nil {
		return "", err
	}
	doc := strings.Join(strings.Fields(d.Doc), " ")
	if err := ontoedit.SetField(path, name, "doc", ontoedit.Quote(doc)); err != nil {
		return "", err
	}
	return fmt.Sprintf("set the doc of %s in %s", d.Curie, rel(st.WS, path)), nil
}

// ownedConcept returns the ontology file and name of a concept this
// workspace owns.
func ownedConcept(st *State, curie string) (string, string, error) {
	ns, name, ok := strings.Cut(curie, ":")
	if !ok || name == "" {
		return "", "", fmt.Errorf("%q is not a CURIE (ns:Name)", curie)
	}
	decl := st.WS.Config.Owns[ns]
	if decl == nil {
		return "", "", fmt.Errorf("namespace %s: is not owned by this workspace; its owner edits it", ns)
	}
	if st.Graph.NodeByCurie(curie) == nil {
		return "", "", fmt.Errorf("no concept %s", curie)
	}
	return st.WS.Abs(decl.File), name, nil
}

var (
	topPolicy  = regexp.MustCompile(`^policy:\s*(#.*)?$`)
	childKey   = regexp.MustCompile(`^(\s+)waivers:\s*(#.*)?$`)
	flowPolicy = regexp.MustCompile(`^policy:\s*\{`)
)

// addWaiver appends a policy.waivers entry to glossmark.yaml as text, so the
// rest of the file keeps its formatting, and checks that the result parses
// and holds the waiver before writing it.
func addWaiver(ws *load.Workspace, finding string, d Decision) (string, error) {
	path := filepath.Join(ws.Root, load.ConfigName)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	src := strings.TrimRight(string(b), "\n")
	lines := strings.Split(src, "\n")
	entry := fmt.Sprintf("- { finding: %s, owner: %s, expires: %s", strconv.Quote(finding), strconv.Quote(d.Owner), d.Expires)
	if d.Reason != "" {
		entry += ", reason: " + strconv.Quote(d.Reason)
	}
	entry += " }"
	pol := -1
	for i, l := range lines {
		if flowPolicy.MatchString(l) {
			return "", fmt.Errorf("%s:%d: policy is a flow mapping; add the waiver by hand: %s", load.ConfigName, i+1, entry)
		}
		if topPolicy.MatchString(l) {
			pol = i
		}
	}
	var out []string
	switch {
	case pol < 0:
		out = append(lines, "policy:", "  waivers:", "    "+entry)
	default:
		end := len(lines) // the policy block ends at the next top-level line
		for i := pol + 1; i < len(lines); i++ {
			if l := lines[i]; l != "" && !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "\t") && !strings.HasPrefix(l, "#") {
				end = i
				break
			}
		}
		wv, childIndent := -1, "  "
		for i := pol + 1; i < end; i++ {
			if t := strings.TrimLeft(lines[i], " "); t != "" && !strings.HasPrefix(t, "#") && childIndent == "  " {
				childIndent = lines[i][:len(lines[i])-len(t)]
			}
			if m := childKey.FindStringSubmatch(lines[i]); m != nil && m[1] == childIndent {
				wv = i
			}
		}
		if wv < 0 {
			for i := pol + 1; i < end; i++ {
				if strings.HasPrefix(strings.TrimLeft(lines[i], " "), "waivers:") {
					return "", fmt.Errorf("%s:%d: waivers is not a block list; add the waiver by hand: %s", load.ConfigName, i+1, entry)
				}
			}
			out = append(append(append([]string{}, lines[:pol+1]...), childIndent+"waivers:", childIndent+"  "+entry), lines[pol+1:]...)
			break
		}
		// Items keep the indentation of the first existing item.
		itemIndent := childIndent + "  "
		for i := wv + 1; i < end; i++ {
			if t := strings.TrimLeft(lines[i], " "); t != "" && !strings.HasPrefix(t, "#") {
				if strings.HasPrefix(t, "-") {
					itemIndent = lines[i][:len(lines[i])-len(t)]
				}
				break
			}
		}
		last := wv
		for i := wv + 1; i < end; i++ {
			t := strings.TrimLeft(lines[i], " ")
			ind := len(lines[i]) - len(t)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			if ind < len(itemIndent) || (ind == len(itemIndent) && !strings.HasPrefix(t, "-")) {
				break
			}
			last = i
		}
		out = append(append(append([]string{}, lines[:last+1]...), itemIndent+entry), lines[last+1:]...)
	}
	text := strings.Join(out, "\n") + "\n"
	var cfg model.Config
	if err := yaml.Unmarshal([]byte(text), &cfg); err != nil {
		return "", fmt.Errorf("adding the waiver would break %s (%v); add it by hand: %s", load.ConfigName, err, entry)
	}
	found := false
	if cfg.Policy != nil {
		for _, w := range cfg.Policy.Waivers {
			found = found || (w.Finding == finding && w.Expires == d.Expires)
		}
	}
	if !found {
		return "", fmt.Errorf("could not place the waiver in %s; add it by hand under policy.waivers: %s", load.ConfigName, entry)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("waived %s until %s (owner %s) in %s", finding, d.Expires, d.Owner, load.ConfigName), nil
}
