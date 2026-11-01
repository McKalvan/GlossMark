// @gloss verifies gmi:LanguageServer scope=file

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/lsp"
)

// lspClient drives a Server over an in-memory stdio pair the way an editor
// would: framed requests in, responses matched by id, notifications kept.
type lspClient struct {
	t      *testing.T
	root   string
	w      io.WriteCloser
	r      *bufio.Reader
	mu     sync.Mutex
	next   int
	resps  map[int]chan json.RawMessage
	notifs chan map[string]any
	done   chan error
}

func newLSPClient(t *testing.T, root string) *lspClient {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := lsp.New(lspBackend{}, inR, outW, "", "test", lsp.Options{Debounce: 20 * time.Millisecond})
	c := &lspClient{t: t, root: root, w: inW, r: bufio.NewReader(outR), resps: map[int]chan json.RawMessage{}, notifs: make(chan map[string]any, 256), done: make(chan error, 1)}
	go func() { c.done <- srv.Serve(); outW.Close() }()
	go c.readLoop()
	t.Cleanup(func() { inW.Close() })
	return c
}

func (c *lspClient) readLoop() {
	for {
		var length int
		for {
			line, err := c.r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			fmt.Sscanf(line, "Content-Length: %d", &length)
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(c.r, body); err != nil {
			return
		}
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			c.t.Errorf("bad message %s: %v", body, err)
			return
		}
		if method, ok := m["method"].(string); ok {
			if _, isReq := m["id"]; isReq && method != "" {
				continue // a server-to-client request (registerCapability); unanswered on purpose
			}
			c.notifs <- m
			continue
		}
		id := int(m["id"].(float64))
		c.mu.Lock()
		ch := c.resps[id]
		delete(c.resps, id)
		c.mu.Unlock()
		if ch != nil {
			ch <- body
		}
	}
}

func (c *lspClient) send(m map[string]any) {
	m["jsonrpc"] = "2.0"
	b, _ := json.Marshal(m)
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n%s", len(b), b)
}

// call sends a request and decodes its result into out; it returns the
// error object, if any.
func (c *lspClient) call(method string, params any, out any) map[string]any {
	c.t.Helper()
	c.mu.Lock()
	c.next++
	id := c.next
	ch := make(chan json.RawMessage, 1)
	c.resps[id] = ch
	c.mu.Unlock()
	c.send(map[string]any{"id": id, "method": method, "params": params})
	select {
	case body := <-ch:
		var env struct {
			Result json.RawMessage `json:"result"`
			Error  map[string]any  `json:"error"`
		}
		json.Unmarshal(body, &env)
		if env.Error != nil {
			return env.Error
		}
		if out != nil && string(env.Result) != "null" {
			if err := json.Unmarshal(env.Result, out); err != nil {
				c.t.Fatalf("%s: decode %s: %v", method, env.Result, err)
			}
		}
		return nil
	case <-time.After(10 * time.Second):
		c.t.Fatalf("%s: no response", method)
		return nil
	}
}

func (c *lspClient) notify(method string, params any) {
	c.send(map[string]any{"method": method, "params": params})
}

func (c *lspClient) uri(rel string) string {
	return "file://" + filepath.ToSlash(filepath.Join(c.root, rel))
}

// diagnostics waits for the next publishDiagnostics for rel and returns them.
func (c *lspClient) diagnostics(rel string) []lsp.Diagnostic {
	c.t.Helper()
	want := c.uri(rel)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case n := <-c.notifs:
			if n["method"] != "textDocument/publishDiagnostics" {
				continue
			}
			b, _ := json.Marshal(n["params"])
			var p lsp.PublishDiagnosticsParams
			json.Unmarshal(b, &p)
			if p.URI == want {
				return p.Diagnostics
			}
		case <-deadline:
			c.t.Fatalf("no diagnostics for %s", rel)
		}
	}
}

// built waits for the next rebuild to finish.
func (c *lspClient) built() {
	c.t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case n := <-c.notifs:
			if n["method"] == "glossmark/built" {
				return
			}
		case <-deadline:
			c.t.Fatal("no rebuild")
		}
	}
}

func (c *lspClient) open(rel, text string) {
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": c.uri(rel), "languageId": "go", "version": 1, "text": text}})
}

func (c *lspClient) change(rel, text string) {
	c.notify("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": c.uri(rel), "version": 2}, "contentChanges": []map[string]any{{"text": text}}})
}

// clinicBooking copies the showcase clinic-booking repository into a temp
// directory and returns it.
func clinicBooking(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "internal", "suggest", "testdata", "calibration", "glossmark-examples", "clinic-booking")
	dst := t.TempDir()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

const cancellationGo = "services/booking-api/internal/booking/cancellation.go"

func readRel(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLSPInitializeAndDiagnostics(t *testing.T) {
	root := clinicBooking(t)
	c := newLSPClient(t, root)
	var init struct {
		Capabilities map[string]any `json:"capabilities"`
	}
	if e := c.call("initialize", map[string]any{"rootUri": c.uri(""), "capabilities": map[string]any{}}, &init); e != nil {
		t.Fatal(e)
	}
	for _, cap := range []string{"completionProvider", "hoverProvider", "definitionProvider", "codeActionProvider", "executeCommandProvider", "textDocumentSync"} {
		if _, ok := init.Capabilities[cap]; !ok {
			t.Errorf("initialize: no %s capability", cap)
		}
	}
	c.notify("initialized", map[string]any{})

	orig := readRel(t, root, cancellationGo)
	c.open(cancellationGo, orig)
	// A clean file publishes nothing for itself; the first publish we see
	// for it comes once the typed marker breaks. Type a dangling target and
	// one outside the allowed set, as you would, without saving.
	bad := strings.Replace(orig, "// @gloss implements .CancellationPolicy conf=0.95", "// @gloss implements clinic:NoShowRate conf=0.95\n// @gloss implements clinic:Nope", 1)
	c.change(cancellationGo, bad)
	ds := c.diagnostics(cancellationGo)
	codes := map[string]lsp.Diagnostic{}
	for _, d := range ds {
		codes[d.Code] = d
	}
	if d, ok := codes["GM215"]; !ok || d.Severity != lsp.SeverityError || d.Range.Start.Line != 15 {
		t.Errorf("GM215 (dangling) as an error on line 15, got %+v", ds)
	}
	if d, ok := codes["GM503"]; !ok || d.Severity != lsp.SeverityWarning || !strings.Contains(d.Message, "allowed set") {
		t.Errorf("GM503 (allowed set) as a warning, got %+v", ds)
	}
	if string(readRel(t, root, cancellationGo)) != orig {
		t.Fatal("the unsaved buffer was written to disk")
	}
	// Fixing the buffer clears them.
	c.change(cancellationGo, orig)
	if ds := c.diagnostics(cancellationGo); len(ds) != 0 {
		t.Errorf("after the fix: %+v", ds)
	}

	if e := c.call("shutdown", nil, nil); e != nil {
		t.Fatal(e)
	}
	c.notify("exit", nil)
	select {
	case err := <-c.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("exit did not end the session")
	}
}

func TestLSPCompletion(t *testing.T) {
	root := clinicBooking(t)
	c := newLSPClient(t, root)
	c.call("initialize", map[string]any{"rootUri": c.uri("")}, nil)
	orig := readRel(t, root, cancellationGo)
	c.open(cancellationGo, orig)
	complete := func(line string) map[string]lsp.CompletionItem {
		t.Helper()
		text := strings.Replace(orig, "// @gloss implements .CancellationPolicy conf=0.95", line, 1)
		c.change(cancellationGo, text)
		var list lsp.CompletionList
		if e := c.call("textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": c.uri(cancellationGo)}, "position": map[string]any{"line": 14, "character": len(line)}}, &list); e != nil {
			t.Fatal(e)
		}
		out := map[string]lsp.CompletionItem{}
		for _, it := range list.Items {
			out[it.Label] = it
		}
		return out
	}
	// Relations first, under the repository's profile.
	rels := complete("// @gloss ")
	if _, ok := rels["implements"]; !ok || len(rels) < 5 {
		t.Errorf("relations after @gloss: %v", keys(rels))
	}
	if _, ok := rels["clinic:Booking"]; ok {
		t.Error("concepts offered before the relation")
	}
	if rels["implements"].TextEdit == nil || rels["implements"].TextEdit.NewText != "implements " {
		t.Errorf("relation edit: %+v", rels["implements"].TextEdit)
	}
	// Then concepts, the boundary's allowed set first; a proposal completes too.
	cs := complete("// @gloss? implements ")
	in, out := cs["lark:CancellationPolicy"], cs["clinic:NoShowRate"]
	if in.Label == "" || out.Label == "" {
		t.Fatalf("concepts after the relation: %v", keys(cs))
	}
	if !strings.HasPrefix(in.SortText, "0") || !strings.HasPrefix(out.SortText, "1") {
		t.Errorf("allowed set ranks first: %q vs %q", in.SortText, out.SortText)
	}
	if !strings.Contains(out.Detail, "HARD") || strings.Contains(in.Detail, "HARD") {
		t.Errorf("outside-the-set detail: in %q, out %q", in.Detail, out.Detail)
	}
	if in.Documentation == nil || in.Documentation.Value == "" {
		t.Error("concept doc missing")
	}
	if _, ok := cs["implements"]; ok {
		t.Error("relations offered where a concept goes")
	}
	// A local target offers the default namespace's concepts as .Name.
	loc := complete("// @gloss implements .")
	if _, ok := loc[".CancellationPolicy"]; !ok {
		t.Errorf("local form: %v", keys(loc))
	}
	if _, ok := loc["clinic:Booking"]; ok {
		t.Error("core concepts offered in local form")
	}
	if ed := loc[".CancellationPolicy"].TextEdit; ed == nil || ed.Range.Start.Character != len("// @gloss implements ") {
		t.Errorf("local edit range: %+v", ed)
	}
	// After a comma the next target completes; then the key=value tail.
	second := complete("// @gloss implements .CancellationPolicy, cli")
	if ed := second["clinic:Booking"].TextEdit; ed == nil || ed.Range.Start.Character != len("// @gloss implements .CancellationPolicy, ") {
		t.Errorf("second target edit range: %+v", ed)
	}
	tail := complete("// @gloss implements .CancellationPolicy ")
	if _, ok := tail["scope="]; !ok {
		t.Errorf("tail keys: %v", keys(tail))
	}
	if _, ok := complete("// a plain comment ")["implements"]; ok {
		t.Error("completion outside a marker")
	}
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestLSPHoverAndDefinition(t *testing.T) {
	root := clinicBooking(t)
	c := newLSPClient(t, root)
	c.call("initialize", map[string]any{"rootUri": c.uri("")}, nil)
	test := "services/booking-api/internal/booking/cancellation_test.go"
	c.open(test, readRel(t, root, test))
	c.open(cancellationGo, readRel(t, root, cancellationGo))
	pos := func(rel string, line, col int) map[string]any {
		return map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}, "position": map[string]any{"line": line, "character": col}}
	}
	// Line 11 of the test file holds the verifies marker on clinic:Cancellation.
	var h lsp.Hover
	if e := c.call("textDocument/hover", pos(test, 10, 22), &h); e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"**clinic:Cancellation**", "process", "Verified by", "cancellation_test.go:11", "Ontology: `ontology/clinic.gloss.yaml:"} {
		if !strings.Contains(h.Contents.Value, want) {
			t.Errorf("hover lacks %q:\n%s", want, h.Contents.Value)
		}
	}
	if h.Range == nil || h.Range.Start.Character != 19 {
		t.Errorf("hover range: %+v", h.Range)
	}
	// A .Local target resolves through the manifest's default namespace.
	if e := c.call("textDocument/hover", pos(cancellationGo, 14, 25), &h); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(h.Contents.Value, "**lark:CancellationPolicy**") || !strings.Contains(h.Contents.Value, "Implemented in") {
		t.Errorf("local hover:\n%s", h.Contents.Value)
	}
	// Away from a concept, on a marker line, the marker and its ledger row.
	h = lsp.Hover{}
	if e := c.call("textDocument/hover", pos(cancellationGo, 14, 4), &h); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(h.Contents.Value, "**Link**: implements .CancellationPolicy") || !strings.Contains(h.Contents.Value, "Ledger: accepted") {
		t.Errorf("marker hover:\n%s", h.Contents.Value)
	}
	// Off a marker: nothing.
	var raw json.RawMessage
	c.call("textDocument/hover", pos(cancellationGo, 0, 3), &raw)
	if len(raw) != 0 {
		t.Errorf("hover on plain code: %s", raw)
	}
	var locs []lsp.Location
	if e := c.call("textDocument/definition", pos(test, 10, 22), &locs); e != nil {
		t.Fatal(e)
	}
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "/ontology/clinic.gloss.yaml") || locs[0].Range.Start.Line == 0 {
		t.Errorf("definition: %+v", locs)
	}
	var info lsp.FileInfo
	if e := c.call("glossmark/fileInfo", map[string]any{"uri": c.uri(cancellationGo)}, &info); e != nil {
		t.Fatal(e)
	}
	if info.Boundary != "clinicb:booking-api" || info.DefaultNamespace != "lark" || len(info.Links) != 2 || info.Criticality != "high" {
		t.Errorf("fileInfo: %+v", info)
	}
}

func TestLSPCodeActions(t *testing.T) {
	root := clinicBooking(t)
	t.Setenv("GLOSS_ACTOR", "human:reviewer")
	// Make the policy marker a proposal on disk, as an agent would leave it.
	orig := readRel(t, root, cancellationGo)
	proposed := strings.Replace(orig, "// @gloss implements .CancellationPolicy conf=0.95", "// @gloss? implements .CancellationPolicy conf=0.95", 1)
	os.WriteFile(filepath.Join(root, cancellationGo), []byte(proposed), 0o644)

	c := newLSPClient(t, root)
	c.call("initialize", map[string]any{"rootUri": c.uri("")}, nil)
	c.open(cancellationGo, proposed)
	actions := func(line int) []lsp.CodeAction {
		t.Helper()
		var as []lsp.CodeAction
		p := map[string]any{"textDocument": map[string]any{"uri": c.uri(cancellationGo)}, "range": map[string]any{"start": map[string]any{"line": line, "character": 0}, "end": map[string]any{"line": line, "character": 0}}, "context": map[string]any{"diagnostics": []any{}}}
		if e := c.call("textDocument/codeAction", p, &as); e != nil {
			t.Fatal(e)
		}
		return as
	}
	byTitle := func(as []lsp.CodeAction, prefix string) *lsp.CodeAction {
		for i := range as {
			if strings.HasPrefix(as[i].Title, prefix) {
				return &as[i]
			}
		}
		return nil
	}
	as := actions(14)
	accept := byTitle(as, "Accept proposal: implements .CancellationPolicy")
	goTo := byTitle(as, "Go to ontology entry: lark:CancellationPolicy")
	if accept == nil || accept.Command == nil || accept.Command.Command != lsp.CommandAccept {
		t.Fatalf("accept action: %+v", as)
	}
	if goTo == nil || goTo.Command == nil || goTo.Command.Command != lsp.CommandOpenLocation || !strings.HasSuffix(goTo.Command.Arguments[0].(string), "/ontology/lark.gloss.yaml") {
		t.Errorf("go-to action: %+v", as)
	}
	if byTitle(as, "Propose new concept") != nil {
		t.Error("propose offered on a marker line")
	}

	// Accepting an unsaved buffer is refused; the saved file is accepted
	// through gloss accept: the file flips and the ledger records it.
	c.change(cancellationGo, proposed+"\n// edited\n")
	c.built() // the rebuild for the edit, before the command
	exec := map[string]any{"command": accept.Command.Command, "arguments": accept.Command.Arguments}
	if e := c.call("workspace/executeCommand", exec, nil); e == nil || !strings.Contains(e["message"].(string), "unsaved") {
		t.Fatalf("accept with unsaved changes: %v", e)
	}
	c.change(cancellationGo, proposed)
	var msg string
	if e := c.call("workspace/executeCommand", exec, &msg); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(msg, "1 link accepted by human:reviewer") {
		t.Errorf("accept message: %q", msg)
	}
	if got := readRel(t, root, cancellationGo); got != orig {
		t.Errorf("file after accept:\n%s", got)
	}
	ledger := readRel(t, root, ".glossmark/ledger.jsonl")
	if !strings.Contains(ledger, `"reviewed_by":"human:reviewer"`) || strings.Contains(ledger, `"status":"proposed"`) {
		t.Errorf("ledger after accept:\n%s", ledger)
	}
	// The editor reloads the file; accepting again finds nothing to accept.
	c.change(cancellationGo, orig)
	if e := c.call("workspace/executeCommand", exec, nil); e == nil || !strings.Contains(e["message"].(string), "not proposed") {
		t.Errorf("second accept: %v", e)
	}

	// Propose new concept above a declaration: the file's comment style, the
	// boundary's default namespace, the declaration's name.
	c.change(cancellationGo, orig)
	as = actions(20) // func (s *Service) Cancel(...)
	propose := byTitle(as, "Propose new concept lark:NEW:Cancel")
	if propose == nil || propose.Edit == nil {
		t.Fatalf("propose action: %+v", as)
	}
	edits := propose.Edit.Changes[c.uri(cancellationGo)]
	if len(edits) != 1 || edits[0].NewText != "// @gloss? implements lark:NEW:Cancel\n" || edits[0].Range.Start.Line != 20 {
		t.Errorf("propose edit: %+v", edits)
	}
	// In a test file the proposal verifies.
	test := "services/booking-api/internal/booking/cancellation_test.go"
	c.open(test, readRel(t, root, test))
	var as2 []lsp.CodeAction
	p := map[string]any{"textDocument": map[string]any{"uri": c.uri(test)}, "range": map[string]any{"start": map[string]any{"line": 11, "character": 0}, "end": map[string]any{"line": 11, "character": 0}}, "context": map[string]any{"diagnostics": []any{}}}
	c.call("textDocument/codeAction", p, &as2)
	if a := byTitle(as2, "Propose new concept lark:NEW:TestCancelOutsideWindowIsNotLate"); a == nil || !strings.HasPrefix(a.Edit.Changes[c.uri(test)][0].NewText, "// @gloss? verifies ") {
		t.Errorf("propose in a test: %+v", as2)
	}
}
