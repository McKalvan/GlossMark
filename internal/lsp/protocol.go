// @gloss implements gmi:LanguageServer scope=file

package lsp

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// The slice of the Language Server Protocol (3.17) gloss lsp speaks, as
// plain structs. Positions are zero-based lines and UTF-16 columns; the
// marker grammar is ASCII, so byte and UTF-16 offsets agree on marker lines
// and the server does not convert.

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type DidOpenParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeParams struct {
	TextDocument   TextDocumentIdentifier `json:"textDocument"`
	ContentChanges []struct {
		Range *Range `json:"range,omitempty"`
		Text  string `json:"text"`
	} `json:"contentChanges"`
}

type DidCloseParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type DidSaveParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

type DidChangeWatchedFilesParams struct {
	Changes []struct {
		URI  string `json:"uri"`
		Type int    `json:"type"`
	} `json:"changes"`
}

// Diagnostic severities.
const (
	SeverityError       = 1
	SeverityWarning     = 2
	SeverityInformation = 3
	SeverityHint        = 4
)

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Code     string `json:"code,omitempty"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Completion item kinds.
const (
	KindKeyword  = 14
	KindClass    = 7
	KindProperty = 10
	KindValue    = 12
)

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type CompletionItem struct {
	Label         string         `json:"label"`
	Kind          int            `json:"kind,omitempty"`
	Detail        string         `json:"detail,omitempty"`
	Documentation *MarkupContent `json:"documentation,omitempty"`
	SortText      string         `json:"sortText,omitempty"`
	FilterText    string         `json:"filterText,omitempty"`
	TextEdit      *TextEdit      `json:"textEdit,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type Command struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes"`
}

type CodeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
	Command     *Command       `json:"command,omitempty"`
}

type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      struct {
		Diagnostics []Diagnostic `json:"diagnostics"`
		Only        []string     `json:"only,omitempty"`
	} `json:"context"`
}

type ExecuteCommandParams struct {
	Command   string `json:"command"`
	Arguments []any  `json:"arguments"`
}

type InitializeParams struct {
	RootURI          string `json:"rootUri"`
	RootPath         string `json:"rootPath"`
	WorkspaceFolders []struct {
		URI  string `json:"uri"`
		Name string `json:"name"`
	} `json:"workspaceFolders"`
	Capabilities struct {
		Workspace struct {
			DidChangeWatchedFiles struct {
				DynamicRegistration bool `json:"dynamicRegistration"`
			} `json:"didChangeWatchedFiles"`
		} `json:"workspace"`
	} `json:"capabilities"`
}

// FileInfo answers the glossmark/fileInfo request an editor makes for its
// status bar: the boundary a file sits in and the links it carries.
type FileInfo struct {
	Path             string   `json:"path"`
	Boundary         string   `json:"boundary,omitempty"`
	Level            string   `json:"level,omitempty"`
	Manifest         string   `json:"manifest,omitempty"`
	DefaultNamespace string   `json:"defaultNamespace,omitempty"`
	Criticality      string   `json:"criticality,omitempty"`
	Links            []string `json:"links"`
	Proposed         int      `json:"proposed"`
}

// uriToPath turns a file: URI into a local path.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, "/")
	}
	return filepath.FromSlash(p)
}

// pathToURI turns a local path into a file: URI.
func pathToURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
