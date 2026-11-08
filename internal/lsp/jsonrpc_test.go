// @gloss verifies gmi:LanguageServer scope=file

package lsp

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/marker"
)

// A message round-trips through the Content-Length framing; headers are
// case-insensitive and may carry a Content-Type.
func TestFramingRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	c := newConn(strings.NewReader(""), &buf)
	c.notify("textDocument/publishDiagnostics", map[string]any{"uri": "file:///a", "diagnostics": []any{}})
	if !strings.HasPrefix(buf.String(), "Content-Length: ") || !strings.Contains(buf.String(), "\r\n\r\n{") {
		t.Fatalf("frame: %q", buf.String())
	}
	in := "content-type: application/vscode-jsonrpc; charset=utf-8\r\n" + buf.String()
	m, err := newConn(strings.NewReader(in), io.Discard).read()
	if err != nil || m.Method != "textDocument/publishDiagnostics" || m.ID != nil {
		t.Fatalf("read: %+v %v", m, err)
	}
	if _, err := newConn(strings.NewReader(in), io.Discard).read(); err != nil {
		t.Fatal(err)
	}
}

func TestFramingErrors(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "EOF"},
		{"Content-Length: 5\r\n\r\n{\"a\"", "EOF"},
		{"\r\n{}", "without Content-Length"},
		{"Content-Length: x\r\n\r\n", "bad Content-Length"},
		{"Content-Length: 2\r\n\r\n{]", "parse error"},
		{"nonsense\r\n\r\n", "malformed header"},
	}
	for _, tc := range cases {
		_, err := newConn(strings.NewReader(tc.in), io.Discard).read()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: %v, want %s", tc.in, err, tc.want)
		}
	}
}

// A nil result is sent as null, as the protocol requires; an error carries
// no result.
func TestReply(t *testing.T) {
	var buf bytes.Buffer
	c := newConn(strings.NewReader(""), &buf)
	c.reply(json.RawMessage("1"), nil, nil)
	c.reply(json.RawMessage("2"), nil, &rpcError{codeMethodNotFound, "no"})
	out := buf.String()
	if !strings.Contains(out, `"id":1,"result":null}`) || !strings.Contains(out, `"id":2,"error":{"code":-32601,"message":"no"}}`) {
		t.Fatalf("replies: %s", out)
	}
}

// Diagnostics map onto the protocol: GM code, severities, a column when the
// scanner gave one and the line's indentation otherwise.
func TestToDiagnostic(t *testing.T) {
	lines := []string{"  # @gloss implements x:Y", ""}
	d := toDiagnostic(diag("warning", "GM503", 1, 0), lines)
	if d.Severity != SeverityWarning || d.Code != "GM503" || d.Range.Start.Character != 2 || d.Range.End.Character != len(lines[0]) || d.Source != "gloss" {
		t.Errorf("warning: %+v", d)
	}
	if d := toDiagnostic(diag("hint", "GM255", 1, 5), lines); d.Severity != SeverityHint || d.Range.Start.Character != 4 {
		t.Errorf("hint: %+v", d)
	}
	if d := toDiagnostic(diag("error", "GM215", 9, 0), lines); d.Severity != SeverityError || d.Range.Start.Line != 8 {
		t.Errorf("past the end: %+v", d)
	}
}

func TestMarkerPrefix(t *testing.T) {
	cases := []struct {
		before string
		rest   string
		in     bool
	}{
		{"// @gloss ", " ", true},
		{"# @gloss? implements ", " implements ", true},
		{"// @glossary ", "", false},
		{"// plain", "", false},
		{"// @gloss", "", false},
	}
	for _, c := range cases {
		rest, in := markerPrefix(c.before)
		if rest != c.rest || in != c.in {
			t.Errorf("%q: %q %v", c.before, rest, in)
		}
	}
}

func diag(sev, code string, line, col int) marker.Diagnostic {
	return marker.Diagnostic{Path: "a.py", Line: line, Col: col, Severity: sev, Code: code, Msg: "m"}
}
