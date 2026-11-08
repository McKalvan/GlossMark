// @gloss implements gmi:LanguageServer scope=file

package lsp

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// Commands the server executes (workspace/executeCommand) and the one it
// asks the client to run.
const (
	// CommandAccept runs gloss accept on the marker at [path, line].
	CommandAccept = "gloss.accept"
	// CommandOpenLocation is a client-side command: show [uri, line]. The
	// VS Code extension registers it; a client without it still has
	// textDocument/definition for the same jump.
	CommandOpenLocation = "glossmark.openLocation"
)

// curiePat finds concept references on a line: ns:Name, ns:NEW:Name, a
// .Local target, the forms the marker grammar accepts.
var curiePat = regexp.MustCompile(`(?:[A-Za-z][\w-]*:(?:NEW:)?|\.)[\w.-]+`)

// lineAt returns line n (0-based) of text, "" past the end.
func lineAt(text string, n int) string {
	lines := strings.Split(text, "\n")
	if n < 0 || n >= len(lines) {
		return ""
	}
	return strings.TrimSuffix(lines[n], "\r")
}

// markerPrefix returns, for the text before the cursor on a line, the part
// after the last marker keyword (with or without ?) and true when the cursor is inside a
// marker; false when the line has no marker before the cursor.
func markerPrefix(before string) (string, bool) {
	i := strings.LastIndex(before, "@gloss")
	if i < 0 {
		return "", false
	}
	rest := before[i+len("@gloss"):]
	rest = strings.TrimPrefix(rest, "?")
	if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	return rest, true
}

// completion offers relations after the marker keyword, then concepts for the relation,
// ranked by the boundary's allowed set, then the key=value tail.
func (s *Server) completion(p TextDocumentPositionParams) *CompletionList {
	rel, text, ok := s.doc(p.TextDocument.URI)
	snap := s.current()
	if !ok || snap == nil {
		return nil
	}
	line := lineAt(text, p.Position.Line)
	col := min(p.Position.Character, len(line))
	before := line[:col]
	rest, in := markerPrefix(before)
	if !in {
		return nil
	}
	words := strings.Fields(rest)
	current := ""
	if n := len(rest); n > 0 && rest[n-1] != ' ' && rest[n-1] != '\t' && len(words) > 0 {
		current = words[len(words)-1]
		words = words[:len(words)-1]
	}
	// The edit replaces the word being typed (after a comma in a target list).
	start := col - len(current)
	if i := strings.LastIndex(current, ","); i >= 0 {
		start += i + 1
		current = current[i+1:]
	}
	rng := Range{Position{p.Position.Line, start}, Position{p.Position.Line, col}}
	// Targets continue while each word ends with a comma: "a:X, b:Y".
	inTargets := len(words) >= 1
	for _, w := range words[min(1, len(words)):] {
		if !strings.HasSuffix(w, ",") {
			inTargets = false
		}
	}
	var items []CompletionItem
	switch {
	case len(words) == 0:
		items = s.relationItems(snap, rng)
	case inTargets:
		items = s.conceptItems(snap, rel, words[0], current, rng)
	default:
		items = keyItems(current, rng)
	}
	return &CompletionList{Items: items}
}

func (s *Server) relationItems(snap *Snapshot, rng Range) []CompletionItem {
	ws := snap.WS
	var items []CompletionItem
	for i, name := range ws.Vocab.InlineNames(ws.Config.RelationProfile()) {
		r := ws.Vocab.Get(name)
		it := CompletionItem{Label: name, Kind: KindKeyword, SortText: fmt.Sprintf("%03d", i), TextEdit: &TextEdit{rng, name + " "}}
		if r != nil && r.Doc != "" {
			it.Detail = r.Doc
		}
		items = append(items, it)
	}
	return items
}

// conceptItems lists the concepts a marker in rel may link under relation:
// the boundary's allowed set first, then the rest of the vocabulary, those
// of the relation's range kinds first within each; a current word starting
// with "." offers the default namespace's concepts in their local form.
func (s *Server) conceptItems(snap *Snapshot, rel, relation, current string, rng Range) []CompletionItem {
	ws, g := snap.WS, snap.Graph
	m := ws.ManifestFor(rel)
	allowed := rollup.AllowedFor(g, ws, m)
	r := ws.Vocab.Get(relation)
	local := strings.HasPrefix(current, ".")
	defNS := ""
	if m != nil {
		defNS = m.DefaultNamespace
	}
	var items []CompletionItem
	for _, n := range g.NodesOf(model.NodeConcept) {
		if n.Deprecated || (n.Imported && n.Tier == model.TierImpl && n.Published == "") {
			continue
		}
		if local && n.NS != defNS {
			continue
		}
		label := n.CURIE
		if local {
			label = "." + strings.TrimPrefix(n.CURIE, defNS+":")
		}
		rank := "1"
		if allowed.Matches(n.CURIE) {
			rank = "0"
		}
		if r != nil && len(r.RangeKinds) > 0 {
			if containsString(r.RangeKinds, n.Kind) {
				rank += "a"
			} else {
				rank += "b"
			}
		}
		it := CompletionItem{Label: label, Kind: KindClass, Detail: conceptDetail(n), SortText: rank + " " + n.CURIE, FilterText: label, TextEdit: &TextEdit{rng, label}}
		if n.Doc != "" {
			it.Documentation = &MarkupContent{"markdown", n.Doc}
		}
		if rank[0] == '1' && len(allowed.Patterns) > 0 {
			it.Detail += " · outside this boundary's allowed set (HARD)"
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].SortText < items[j].SortText })
	return items
}

func conceptDetail(n *graph.Node) string {
	d := n.Kind
	if d == "" {
		d = "concept"
	}
	if n.Criticality != "" {
		d += ", " + n.Criticality
	}
	if n.Proposed {
		d += ", proposed"
	}
	if n.Planned {
		d += ", planned"
	}
	return d
}

// keyItems completes the key=value tail of a marker.
func keyItems(current string, rng Range) []CompletionItem {
	if strings.HasPrefix(current, "scope=") {
		var items []CompletionItem
		for i, sc := range []string{"symbol", "file", "block", "line", "range"} {
			items = append(items, CompletionItem{Label: "scope=" + sc, Kind: KindValue, SortText: fmt.Sprint(i), TextEdit: &TextEdit{rng, "scope=" + sc}})
		}
		return items
	}
	return []CompletionItem{
		{Label: "scope=", Kind: KindProperty, Detail: "symbol (default) | file | block | line | range", SortText: "0", TextEdit: &TextEdit{rng, "scope="}},
		{Label: "conf=", Kind: KindProperty, Detail: "confidence 0..1 of a proposal", SortText: "1", TextEdit: &TextEdit{rng, "conf="}},
		{Label: "note=", Kind: KindProperty, Detail: "why the link was written (quoted)", SortText: "2", TextEdit: &TextEdit{rng, `note=""`}},
	}
}

// reference is a concept reference under the cursor.
type reference struct {
	text  string // as written
	curie string // resolved (a .Local through the default namespace)
	rng   Range
}

// referenceAt finds the concept reference the cursor is on, resolving a
// .Local target through the file's manifest.
func referenceAt(snap *Snapshot, rel, line string, pos Position) *reference {
	for _, loc := range curiePat.FindAllStringIndex(line, -1) {
		if pos.Character < loc[0] || pos.Character > loc[1] {
			continue
		}
		text := line[loc[0]:loc[1]]
		curie := text
		if strings.HasPrefix(text, ".") {
			m := snap.WS.ManifestFor(rel)
			if m == nil || m.DefaultNamespace == "" || !strings.Contains(line, "@gloss") {
				return nil
			}
			curie = m.DefaultNamespace + ":" + text[1:]
		}
		curie = strings.TrimRight(curie, ".")
		return &reference{text: text, curie: curie, rng: Range{Position{pos.Line, loc[0]}, Position{pos.Line, loc[1]}}}
	}
	return nil
}

// hover describes the concept under the cursor: doc, kind, criticality,
// owner, where it is implemented and verified, its drift findings and the
// ontology entry; on a marker line away from a concept, the marker itself.
func (s *Server) hover(p TextDocumentPositionParams) *Hover {
	rel, text, ok := s.doc(p.TextDocument.URI)
	snap := s.current()
	if !ok || snap == nil {
		return nil
	}
	line := lineAt(text, p.Position.Line)
	if ref := referenceAt(snap, rel, line, p.Position); ref != nil {
		if marker.IsNewTarget(ref.curie) {
			return &Hover{Contents: MarkupContent{"markdown", fmt.Sprintf("**%s** proposes a concept that has no ontology entry yet (a rogue until it does). `gloss propose --from-new` lifts it into the owning ontology file as a proposed concept.", ref.curie)}, Range: &ref.rng}
		}
		if n := snap.Graph.NodeByCurie(ref.curie); n != nil {
			return &Hover{Contents: MarkupContent{"markdown", describeConcept(snap, n)}, Range: &ref.rng}
		}
		return nil
	}
	if fm := markerAt(snap, rel, p.Position.Line+1); fm != nil {
		return &Hover{Contents: MarkupContent{"markdown", describeMarker(snap, fm)}}
	}
	return nil
}

// markerAt returns the inline marker written at path:line, if any.
func markerAt(snap *Snapshot, rel string, line int) *extract.FoundMarker {
	for i := range snap.Markers {
		fm := &snap.Markers[i]
		if !fm.Mapped() && fm.Path == rel && fm.Marker.Line == line {
			return fm
		}
	}
	return nil
}

func describeConcept(snap *Snapshot, n *graph.Node) string {
	g, ws := snap.Graph, snap.WS
	var sb strings.Builder
	head := fmt.Sprintf("**%s**", n.CURIE)
	if n.Label != "" {
		head += " (" + n.Label + ")"
	}
	parts := []string{conceptDetail(n)}
	if n.Tier != "" {
		parts = append(parts, n.Tier+" tier")
	}
	if n.Owner != "" {
		parts = append(parts, "owner "+n.Owner)
	} else if o, ok := ws.Config.Owns[n.NS]; ok && o.Owner != "" {
		parts = append(parts, "owner "+o.Owner)
	}
	if n.Published != "" {
		parts = append(parts, "published as "+n.Published)
	}
	if n.Deprecated {
		d := "deprecated"
		if n.ReplacedBy != "" {
			d += ", replaced by " + n.ReplacedBy
		}
		parts = append(parts, d)
	}
	fmt.Fprintf(&sb, "%s · %s\n\n", head, strings.Join(parts, " · "))
	if n.Doc != "" {
		sb.WriteString(n.Doc + "\n\n")
	}
	if n.Value != nil {
		fmt.Fprintf(&sb, "Value: `%v`", n.Value)
		if n.ValueFrom != "" {
			fmt.Fprintf(&sb, " bound to `%s`", n.ValueFrom)
		}
		sb.WriteString("\n\n")
	}
	// Declared relations, then where code links to it, by relation.
	var decl []string
	for _, e := range g.Out(n.ID, "") {
		if e.Source == graph.SourceDeclared {
			decl = append(decl, e.P+" "+g.Curie(e.O))
		}
	}
	if len(decl) > 0 {
		fmt.Fprintf(&sb, "Declares: %s\n\n", strings.Join(decl, "; "))
	}
	by := map[string][]string{}
	var order []string
	for _, e := range g.In(n.ID, "") {
		a := g.Node(e.S)
		if a == nil || a.NodeType != model.NodeArtifact || e.Prov == nil || !graph.LinkSource(e.Source) {
			continue
		}
		where := fmt.Sprintf("`%s:%d`", e.Prov.Path, e.Prov.Line)
		if e.Prov.Symbol != "" {
			where += " " + e.Prov.Symbol
		}
		if e.Prov.Proposed {
			where += " (proposed)"
		}
		if _, ok := by[e.P]; !ok {
			order = append(order, e.P)
		}
		by[e.P] = append(by[e.P], where)
	}
	sort.Strings(order)
	for _, p := range order {
		label := p
		switch p {
		case "implements":
			label = "Implemented in"
		case "verifies":
			label = "Verified by"
		case "represents":
			label = "Represented in"
		case "documents":
			label = "Documented in"
		}
		xs := by[p]
		more := ""
		if len(xs) > 6 {
			more = fmt.Sprintf(" and %d more", len(xs)-6)
			xs = xs[:6]
		}
		fmt.Fprintf(&sb, "%s: %s%s\n\n", label, strings.Join(xs, ", "), more)
	}
	if len(order) == 0 && n.NodeType == model.NodeConcept {
		sb.WriteString("No code links to it yet.\n\n")
	}
	var bounds []string
	for _, e := range g.In(n.ID, "") {
		if b := g.Node(e.S); b != nil && b.NodeType == model.NodeBoundary && e.Source == graph.SourceDeclared {
			bounds = append(bounds, b.CURIE+" "+e.P)
		}
	}
	if len(bounds) > 0 {
		sort.Strings(bounds)
		fmt.Fprintf(&sb, "Declared by: %s\n\n", strings.Join(uniq(bounds), "; "))
	}
	if snap.Report != nil {
		var fs []string
		for _, f := range snap.Report.Findings {
			if f.Concept == n.CURIE && !f.Waived {
				fs = append(fs, fmt.Sprintf("- %s: %s", f.Kind, f.Msg))
			}
		}
		if len(fs) > 0 {
			fmt.Fprintf(&sb, "Drift:\n%s\n\n", strings.Join(fs, "\n"))
		}
	}
	if n.File != "" {
		fmt.Fprintf(&sb, "Ontology: `%s:%d`", n.File, max(n.Line, 1))
	}
	return strings.TrimSpace(sb.String())
}

func describeMarker(snap *Snapshot, fm *extract.FoundMarker) string {
	var sb strings.Builder
	mk := fm.Marker
	kind := "Link"
	if mk.Proposed {
		kind = "Proposed link"
	}
	fmt.Fprintf(&sb, "**%s**: %s %s", kind, mk.Relation, strings.Join(mk.Targets, ", "))
	if r := snap.WS.Vocab.Get(mk.Relation); r != nil && r.Doc != "" {
		fmt.Fprintf(&sb, "\n\n%s", r.Doc)
	}
	if fm.Anchor.Symbol != "" {
		fmt.Fprintf(&sb, "\n\nAnchored to `%s` (scope %s)", fm.Anchor.Symbol, fm.Anchor.Scope)
	} else if fm.Anchor.Scope != "" {
		fmt.Fprintf(&sb, "\n\nScope %s", fm.Anchor.Scope)
	}
	if e := snap.Ledger[fm.ID]; e != nil {
		fmt.Fprintf(&sb, "\n\nLedger: %s", e.Status)
		if e.Actor != "" {
			fmt.Fprintf(&sb, ", written by %s", e.Actor)
		}
		if e.ReviewedBy != "" {
			fmt.Fprintf(&sb, ", reviewed by %s", e.ReviewedBy)
		}
		if e.Gate != "" {
			fmt.Fprintf(&sb, " [%s]", e.Gate)
		}
		fmt.Fprintf(&sb, " (`%s`)", fm.ID)
	}
	return sb.String()
}

// definition jumps from a concept reference to its ontology entry.
func (s *Server) definition(p TextDocumentPositionParams) []Location {
	rel, text, ok := s.doc(p.TextDocument.URI)
	snap := s.current()
	if !ok || snap == nil {
		return nil
	}
	ref := referenceAt(snap, rel, lineAt(text, p.Position.Line), p.Position)
	if ref == nil {
		return nil
	}
	if loc := ontologyLocation(snap, ref.curie); loc != nil {
		return []Location{*loc}
	}
	return nil
}

// ontologyLocation is the file and line of a concept's ontology entry.
func ontologyLocation(snap *Snapshot, curie string) *Location {
	n := snap.Graph.NodeByCurie(curie)
	if n == nil || n.File == "" {
		return nil
	}
	path := n.File
	if !filepath.IsAbs(path) {
		path = filepath.Join(snap.WS.Root, filepath.FromSlash(path))
	}
	line := max(n.Line-1, 0)
	return &Location{URI: pathToURI(path), Range: Range{Position{line, 0}, Position{line, 0}}}
}

// codeActions offers, on the selected lines: accepting each proposed marker,
// proposing a new concept above a declaration, and jumping to the ontology
// entry of each concept referenced.
func (s *Server) codeActions(p CodeActionParams) []CodeAction {
	rel, text, ok := s.doc(p.TextDocument.URI)
	snap := s.current()
	if !ok || snap == nil {
		return nil
	}
	var out []CodeAction
	seen := map[string]bool{}
	for ln := p.Range.Start.Line; ln <= p.Range.End.Line; ln++ {
		line := lineAt(text, ln)
		if fm := markerAt(snap, rel, ln+1); fm != nil && fm.Marker.Proposed {
			out = append(out, CodeAction{
				Title:   fmt.Sprintf("Accept proposal: %s %s", fm.Marker.Relation, strings.Join(fm.Marker.Targets, ", ")),
				Kind:    "quickfix",
				Command: &Command{Title: "Accept", Command: CommandAccept, Arguments: []any{rel, fm.Marker.Line}},
			})
		}
		for _, loc := range curiePat.FindAllStringIndex(line, -1) {
			ref := referenceAt(snap, rel, line, Position{ln, loc[0]})
			if ref == nil || seen[ref.curie] {
				continue
			}
			seen[ref.curie] = true
			if l := ontologyLocation(snap, ref.curie); l != nil {
				out = append(out, CodeAction{Title: "Go to ontology entry: " + ref.curie, Kind: "refactor",
					Command: &Command{Title: "Open", Command: CommandOpenLocation, Arguments: []any{l.URI, l.Range.Start.Line}}})
			}
		}
	}
	if a := s.proposeAction(snap, rel, text, p.TextDocument.URI, p.Range.Start.Line); a != nil {
		out = append(out, *a)
	}
	return out
}

// proposeAction inserts a proposed `<relation> ns:NEW:Name` marker above the
// declaration on the line (or the line itself), in the file's comment style
// and the boundary's default namespace, for the author to name.
func (s *Server) proposeAction(snap *Snapshot, rel, text, uri string, line0 int) *CodeAction {
	ws := snap.WS
	style, ok := marker.StyleFor(rel, ws.Config.CommentStyles)
	if !ok || strings.Contains(lineAt(text, line0), "@gloss") {
		return nil
	}
	lines := strings.Split(text, "\n")
	insert, indent, name := line0, "", "Name"
	if line0 < len(lines) {
		indent = lines[line0][:len(lines[line0])-len(strings.TrimLeft(lines[line0], " \t"))]
	}
	for _, d := range marker.Declarations(rel, lines, style) {
		if d.Line == line0+1 || d.Insert == line0+1 {
			// A member's symbol is Class.method; the concept takes the method.
			insert, indent, name = d.Insert-1, d.Indent, d.Symbol[strings.LastIndex(d.Symbol, ".")+1:]
			break
		}
	}
	ns := ""
	if m := ws.ManifestFor(rel); m != nil {
		ns = m.DefaultNamespace
	}
	if ns == "" {
		for _, p := range sortedOwned(ws) {
			if ws.Config.Owns[p].Tier == model.TierImpl || ns == "" {
				ns = p
			}
		}
	}
	if ns == "" {
		return nil
	}
	relation := "implements"
	if ws.IsTestPath(rel) {
		relation = "verifies"
	}
	body := fmt.Sprintf("@gloss? %s %s:NEW:%s", relation, ns, name)
	var comment string
	switch {
	case len(style.Line) > 0 && style.Line[0] != "":
		comment = style.Line[0] + " " + body
	case len(style.Block) > 0:
		comment = style.Block[0][0] + " " + body + " " + style.Block[0][1]
	default:
		comment = body
	}
	edit := TextEdit{Range: Range{Position{insert, 0}, Position{insert, 0}}, NewText: indent + comment + "\n"}
	return &CodeAction{Title: "Propose new concept " + ns + ":NEW:" + name, Kind: "refactor",
		Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {edit}}}}
}

func sortedOwned(ws *load.Workspace) []string {
	var out []string
	for p := range ws.Config.Owns {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// executeCommand runs gloss.accept: the same path as gloss accept, on the
// file as saved, so the editor reloads the one-character diff and the ledger
// row is written once. An unsaved buffer is refused rather than raced.
func (s *Server) executeCommand(p ExecuteCommandParams) (any, *rpcError) {
	if p.Command != CommandAccept {
		return nil, &rpcError{codeMethodNotFound, "unknown command " + p.Command}
	}
	if len(p.Arguments) != 2 {
		return nil, &rpcError{codeInvalidParams, CommandAccept + " takes [path, line]"}
	}
	rel, _ := p.Arguments[0].(string)
	lineF, _ := p.Arguments[1].(float64)
	line := int(lineF)
	s.mu.Lock()
	d := s.docs[rel]
	root := s.root
	s.mu.Unlock()
	if d != nil {
		disk, err := (&load.Workspace{Root: root}).ReadFile(rel)
		if err != nil || string(disk) != d.text {
			return nil, &rpcError{codeRequestFailed, fmt.Sprintf("%s has unsaved changes; save it, then accept", rel)}
		}
	}
	msg, err := s.backend.Accept(root, rel, line)
	if err != nil {
		return nil, &rpcError{codeRequestFailed, err.Error()}
	}
	s.conn.notify("window/showMessage", map[string]any{"type": 3, "message": strings.TrimSpace(msg)})
	s.schedule(0)
	return strings.TrimSpace(msg), nil
}

func containsString(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func uniq(xs []string) []string {
	m := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !m[x] {
			m[x] = true
			out = append(out, x)
		}
	}
	return out
}
