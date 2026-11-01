// @gloss implements gmi:LanguageServer scope=file
// @gloss publishes gm:LinkAccepted scope=file

package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/lsp"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/version"
)

func init() {
	register(command{"lsp", "serve markers to an editor over the Language Server Protocol on stdio: gloss check diagnostics as you type, completion of relations and concepts after @gloss, hover and go-to-definition on a concept, and the accept, propose and go-to-ontology code actions (editors/vscode is the VS Code extension)", cmdLSP})
}

// lspBackend runs the pipeline for the language server: the same build as
// gloss check, over the editor's unsaved buffers, and the same accept as
// gloss accept.
type lspBackend struct{}

func (lspBackend) Build(root string, overlay map[string][]byte) (*lsp.Snapshot, error) {
	p, err := buildGraphOverlay(root, false, overlay)
	if err != nil {
		return nil, err
	}
	diags := append([]marker.Diagnostic{}, p.Diags...)
	diags = append(diags, check.Run(p.WS, p.Graph)...)
	pol := p.WS.Config.EffectivePolicy()
	rep := rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: pol, Now: time.Now()})
	return &lsp.Snapshot{WS: p.WS, Graph: p.Graph, Markers: p.Res.Markers, Diags: diags, Ledger: p.Ledger.Entries, Report: rep}, nil
}

func (lspBackend) Accept(root, path string, line int) (string, error) {
	p, err := buildGraph(root, false)
	if err != nil {
		return "", err
	}
	targets, err := selectMarkers(p, []string{fmt.Sprintf("%s:%d", path, line)})
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	n, err := acceptLinks(p, targets, p.WS.Config.EffectivePolicy(), false, &out)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", fmt.Errorf("%s", bytes.TrimSpace(out.Bytes()))
	}
	if err := p.Ledger.Save(); err != nil {
		return "", err
	}
	fmt.Fprintf(&out, "gloss accept: %s accepted by %s\n", plural(n, "link"), reviewer(p.WS.Root))
	return out.String(), nil
}

func cmdLSP(args []string) int {
	fs, root := newFlags("lsp")
	logPath := fs.String("log", "", "append one line per event to this file (the editor's output panel shows stderr otherwise)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	r := *root
	if r != "" {
		rr, err := load.FindRoot(r)
		if err != nil {
			return fail(err)
		}
		r = rr
	}
	opt := lsp.Options{Log: os.Stderr}
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		opt.Log = f
	}
	srv := lsp.New(lspBackend{}, os.Stdin, os.Stdout, r, version.String(), opt)
	if err := srv.Serve(); err != nil {
		return fail(err)
	}
	return 0
}
