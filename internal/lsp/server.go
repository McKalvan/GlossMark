// @gloss implements gmi:LanguageServer scope=file
// @gloss publishes gm:LinkAccepted scope=file

// Package lsp serves markers to an editor over the Language Server Protocol
// (JSON-RPC 2.0 on stdio): gloss check diagnostics as you type, completion
// of relations and concepts in a marker, hover and go-to-definition on a
// concept, and the accept, propose and go-to-ontology code actions. It
// reuses the loader, extractor, linter, comparator and ledger through a
// Backend the command supplies; no rule lives here (ADR 0051).
package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// Snapshot is one build of the workspace: what every feature reads.
type Snapshot struct {
	WS      *load.Workspace
	Graph   *graph.Graph
	Markers []extract.FoundMarker
	// Diags are the extractor's and the linter's diagnostics: gloss check.
	Diags  []marker.Diagnostic
	Ledger map[string]*model.LedgerEntry
	// Report is the drift comparison, for the findings hover shows.
	Report *rollup.Report
}

// Backend runs the pipeline on a workspace root. Build extracts the tree
// with the editor's unsaved buffers laid over it; Accept runs gloss accept
// on the marker at path:line and returns what it printed.
type Backend interface {
	Build(root string, overlay map[string][]byte) (*Snapshot, error)
	Accept(root, path string, line int) (string, error)
}

// Options tune the server.
type Options struct {
	// Debounce is how long after the last edit the tree is rebuilt.
	Debounce time.Duration
	// Log receives one line per event; nil discards.
	Log io.Writer
}

// Server is one LSP session over a reader/writer.
type Server struct {
	backend Backend
	conn    *conn
	opt     Options
	version string

	mu        sync.Mutex
	root      string
	docs      map[string]*document // by repo-relative path
	snap      *Snapshot
	published map[string][]Diagnostic // what each URI was last sent
	timer     *time.Timer
	building  sync.Mutex
	shutdown  bool
	// built is signalled after every publish, for tests.
	built chan struct{}
}

type document struct {
	uri     string
	text    string
	version int
}

// New returns a server. The backend's workspace root is root; "" lets the
// initialize request choose it from the editor's workspace folder.
func New(b Backend, in io.Reader, out io.Writer, root, version string, opt Options) *Server {
	if opt.Debounce == 0 {
		opt.Debounce = 300 * time.Millisecond
	}
	return &Server{backend: b, conn: newConn(in, out), opt: opt, root: root, version: version, docs: map[string]*document{}, published: map[string][]Diagnostic{}, built: make(chan struct{}, 1)}
}

// Serve runs until the client closes the stream or sends exit.
func (s *Server) Serve() error {
	for {
		m, err := s.conn.read()
		if err == io.EOF {
			return nil
		}
		if rerr, ok := err.(*rpcError); ok {
			s.conn.reply(nil, nil, rerr)
			continue
		}
		if err != nil {
			return err
		}
		if m.Method == "" {
			continue // a response to one of our requests
		}
		if m.Method == "exit" {
			return nil
		}
		if m.ID == nil {
			s.notification(m.Method, m.Params)
			continue
		}
		res, rerr := s.request(m.Method, m.Params)
		s.conn.reply(m.ID, res, rerr)
	}
}

func (s *Server) logf(format string, args ...any) {
	if s.opt.Log != nil {
		fmt.Fprintf(s.opt.Log, format+"\n", args...)
	}
}

func (s *Server) request(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		return s.initialize(params)
	case "shutdown":
		s.mu.Lock()
		s.shutdown = true
		if s.timer != nil {
			s.timer.Stop()
		}
		s.mu.Unlock()
		return nil, nil
	case "textDocument/completion":
		var p TextDocumentPositionParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
		return s.completion(p), nil
	case "textDocument/hover":
		var p TextDocumentPositionParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
		return s.hover(p), nil
	case "textDocument/definition":
		var p TextDocumentPositionParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
		return s.definition(p), nil
	case "textDocument/codeAction":
		var p CodeActionParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
		return s.codeActions(p), nil
	case "workspace/executeCommand":
		var p ExecuteCommandParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
		return s.executeCommand(p)
	case "glossmark/fileInfo":
		var p TextDocumentIdentifier
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
		return s.fileInfo(p.URI), nil
	}
	return nil, &rpcError{codeMethodNotFound, "method not found: " + method}
}

func (s *Server) notification(method string, params json.RawMessage) {
	switch method {
	case "initialized":
		s.registerWatchers()
		s.schedule(0)
	case "textDocument/didOpen":
		var p DidOpenParams
		if json.Unmarshal(params, &p) == nil {
			s.setDoc(p.TextDocument.URI, p.TextDocument.Text, p.TextDocument.Version)
			s.schedule(0)
		}
	case "textDocument/didChange":
		var p DidChangeParams
		if json.Unmarshal(params, &p) == nil && len(p.ContentChanges) > 0 {
			// Full sync: the last change is the whole document.
			s.setDoc(p.TextDocument.URI, p.ContentChanges[len(p.ContentChanges)-1].Text, 0)
			s.schedule(s.opt.Debounce)
		}
	case "textDocument/didSave":
		var p DidSaveParams
		if json.Unmarshal(params, &p) == nil {
			if p.Text != nil {
				s.setDoc(p.TextDocument.URI, *p.Text, 0)
			}
			s.schedule(0)
		}
	case "textDocument/didClose":
		var p DidCloseParams
		if json.Unmarshal(params, &p) == nil {
			s.mu.Lock()
			delete(s.docs, s.relOf(p.TextDocument.URI))
			s.mu.Unlock()
			s.schedule(0)
		}
	case "workspace/didChangeWatchedFiles":
		// glossmark.yaml, a manifest or an ontology file changed on disk:
		// the vocabulary, the boundaries or the allowed sets did.
		s.schedule(s.opt.Debounce)
	}
}

func (s *Server) initialize(params json.RawMessage) (any, *rpcError) {
	var p InitializeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == "" {
		start := ""
		switch {
		case len(p.WorkspaceFolders) > 0:
			start = uriToPath(p.WorkspaceFolders[0].URI)
		case p.RootURI != "":
			start = uriToPath(p.RootURI)
		case p.RootPath != "":
			start = p.RootPath
		}
		if start == "" {
			start, _ = os.Getwd()
		}
		// The nearest glossmark.yaml above the folder, else the folder
		// itself: a workspace opened on a sub-directory still finds its root.
		if r, err := load.FindRoot(start); err == nil {
			s.root = r
		} else {
			s.root = start
			s.logf("no %s at or above %s; diagnostics are off until one exists", load.ConfigName, start)
		}
	}
	s.logf("root %s", s.root)
	return map[string]any{
		"capabilities": map[string]any{
			"textDocumentSync":       map[string]any{"openClose": true, "change": 1, "save": map[string]any{"includeText": true}},
			"completionProvider":     map[string]any{"triggerCharacters": []string{" ", ":", ".", "?", ","}},
			"hoverProvider":          true,
			"definitionProvider":     true,
			"codeActionProvider":     map[string]any{"codeActionKinds": []string{"quickfix", "refactor"}},
			"executeCommandProvider": map[string]any{"commands": []string{CommandAccept}},
		},
		"serverInfo": map[string]any{"name": "gloss", "version": s.version},
	}, nil
}

// registerWatchers asks a client that supports it to report changes to the
// files the graph is built from besides the open documents. The VS Code
// extension configures the same watchers statically.
func (s *Server) registerWatchers() {
	s.conn.request("client/registerCapability", map[string]any{"registrations": []map[string]any{{
		"id": "gloss-watchers", "method": "workspace/didChangeWatchedFiles",
		"registerOptions": map[string]any{"watchers": []map[string]any{
			{"globPattern": "**/" + load.ConfigName},
			{"globPattern": "**/" + load.ManifestName},
			{"globPattern": "**/*.gloss.yaml"},
			{"globPattern": "**/ontology/**"},
			{"globPattern": "**/.glossmark/ledger.jsonl"},
		}},
	}}})
}

// relOf maps a document URI to its repo-relative path; "" when it lies
// outside the root.
func (s *Server) relOf(uri string) string {
	p := uriToPath(uri)
	if p == "" || s.root == "" {
		return ""
	}
	rel, err := filepath.Rel(s.root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

func (s *Server) setDoc(uri, text string, version int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rel := s.relOf(uri)
	if rel == "" {
		return
	}
	s.docs[rel] = &document{uri: uri, text: text, version: version}
}

// doc returns an open document's text and repo-relative path.
func (s *Server) doc(uri string) (string, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rel := s.relOf(uri)
	d, ok := s.docs[rel]
	if !ok {
		return rel, "", false
	}
	return rel, d.text, true
}

// schedule rebuilds the tree after the delay; a new call restarts the clock.
func (s *Server) schedule(after time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shutdown {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(after, s.rebuild)
}

// rebuild runs the pipeline over the tree with the open buffers laid over it
// and publishes gloss check's diagnostics for every file that has any.
func (s *Server) rebuild() {
	s.building.Lock()
	defer s.building.Unlock()
	s.mu.Lock()
	root := s.root
	overlay := make(map[string][]byte, len(s.docs))
	for rel, d := range s.docs {
		overlay[rel] = []byte(d.text)
	}
	s.mu.Unlock()
	if _, err := os.Stat(filepath.Join(root, load.ConfigName)); err != nil {
		s.signalBuilt()
		return // not a GlossMark workspace (yet); initialize said so
	}
	started := time.Now()
	snap, err := s.backend.Build(root, overlay)
	if err != nil {
		s.logf("build: %v", err)
		s.conn.notify("window/showMessage", map[string]any{"type": 1, "message": "gloss: " + err.Error()})
		s.signalBuilt()
		return
	}
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
	s.publish(snap)
	took := time.Since(started).Round(time.Millisecond)
	s.logf("rebuilt in %s: %d markers, %d diagnostics", took, len(snap.Markers), len(snap.Diags))
	// glossmark/built tells an editor the graph it asks about is current
	// (the VS Code extension refreshes its status bar on it).
	s.conn.notify("glossmark/built", map[string]any{"markers": len(snap.Markers), "diagnostics": len(snap.Diags), "ms": took.Milliseconds()})
	s.signalBuilt()
}

func (s *Server) signalBuilt() {
	select {
	case s.built <- struct{}{}:
	default:
	}
}

// current returns the latest snapshot, building one when none exists yet.
func (s *Server) current() *Snapshot {
	s.mu.Lock()
	snap := s.snap
	s.mu.Unlock()
	if snap == nil {
		s.rebuild()
		s.mu.Lock()
		snap = s.snap
		s.mu.Unlock()
	}
	return snap
}

// publish sends every diagnostic grouped by file, clears the files that had
// some last time and have none now, and says nothing about a file whose
// diagnostics did not change (two rebuilds can see the same buffer).
func (s *Server) publish(snap *Snapshot) {
	byURI := map[string][]Diagnostic{}
	lines := map[string][]string{}
	for _, d := range snap.Diags {
		rel := d.Path
		if rel == "" {
			rel = load.ConfigName
		}
		if strings.HasPrefix(rel, "../") {
			continue
		}
		uri := pathToURI(filepath.Join(snap.WS.Root, filepath.FromSlash(rel)))
		if _, ok := lines[rel]; !ok {
			b, _ := snap.WS.ReadFile(rel)
			lines[rel] = strings.Split(string(b), "\n")
		}
		byURI[uri] = append(byURI[uri], toDiagnostic(d, lines[rel]))
	}
	s.mu.Lock()
	for uri := range s.published {
		if _, ok := byURI[uri]; !ok {
			byURI[uri] = []Diagnostic{}
		}
	}
	var uris []string
	for u, ds := range byURI {
		if !sameDiagnostics(s.published[u], ds) {
			uris = append(uris, u)
		}
		if len(ds) > 0 {
			s.published[u] = ds
		} else {
			delete(s.published, u)
		}
	}
	s.mu.Unlock()
	sort.Strings(uris)
	for _, u := range uris {
		s.conn.notify("textDocument/publishDiagnostics", PublishDiagnosticsParams{URI: u, Diagnostics: byURI[u]})
	}
}

func sameDiagnostics(a, b []Diagnostic) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// toDiagnostic maps a gloss diagnostic onto the protocol's: the GM code is
// the code, error and warning keep their severity, a hint (GM250, the
// starter relation profile) is a Hint, which editors render faded rather
// than as a squiggle.
func toDiagnostic(d marker.Diagnostic, lines []string) Diagnostic {
	sev := SeverityWarning
	switch d.Severity {
	case "error":
		sev = SeverityError
	case "hint":
		sev = SeverityHint
	case "info":
		sev = SeverityInformation
	}
	line := d.Line - 1
	if line < 0 {
		line = 0
	}
	start, end := 0, 0
	if line < len(lines) {
		l := lines[line]
		end = len(l)
		if d.Col > 0 && d.Col-1 <= len(l) {
			start = d.Col - 1
		} else {
			start = len(l) - len(strings.TrimLeft(l, " \t"))
		}
	}
	return Diagnostic{Range: Range{Position{line, start}, Position{line, end}}, Severity: sev, Code: d.Code, Source: "gloss", Message: d.Msg}
}

// fileInfo answers glossmark/fileInfo for an editor's status bar.
func (s *Server) fileInfo(uri string) *FileInfo {
	snap := s.current()
	rel, _, _ := s.doc(uri)
	if snap == nil || rel == "" {
		return nil
	}
	info := &FileInfo{Path: rel, Links: []string{}}
	if m := snap.WS.ManifestFor(rel); m != nil {
		info.Boundary, info.Level, info.Manifest, info.DefaultNamespace = m.Boundary, m.Level, m.Path, m.DefaultNamespace
		if bn := snap.Graph.NodeByCurie(m.Boundary); bn != nil {
			info.Criticality = bn.Criticality
		}
	}
	for _, fm := range snap.Markers {
		if fm.Path != rel {
			continue
		}
		info.Links = append(info.Links, fm.Marker.Relation+" "+strings.Join(fm.Marker.Targets, ", "))
		if fm.Marker.Proposed {
			info.Proposed++
		}
	}
	return info
}
