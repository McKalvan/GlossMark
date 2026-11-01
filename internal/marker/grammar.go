// @gloss implements gmi:MarkerGrammar scope=file
// @gloss represents gm:Link, gm:Confidence scope=file

// Package marker implements the `@gloss` comment grammar: parsing one marker
// line, the per-extension comment table, SPDX-style header sugar, and the
// lightweight declaration detector used for anchoring in pass 1.
package marker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mckalvan/glossmark/internal/model"
)

// ReferenceRegex is the regex frozen by the design, verbatim. It is exported
// for conformance tooling in other languages; Parse uses the extended form
// below (NEW: infix, * in targets, quoted values) and reports the difference
// as a diagnostic so the two never silently disagree.
const ReferenceRegex = `@gloss(\?)?\s+([a-z][a-z0-9-]*)\s+((?:[A-Za-z][\w-]*:|\.)[\w.-]+(?:\s*,\s*(?:[A-Za-z][\w-]*:|\.)[\w.-]+)*)((?:\s+[a-z]+=[^\s]+)*)`

// target: prefix:Name | prefix:NEW:Name | .Local | prefix:* (globs for may-modify)
const targetPat = `(?:[A-Za-z][\w-]*:(?:NEW:)?|\.)[\w.*-]+`

// Grammar is the extended regex used by the parser.
var Grammar = regexp.MustCompile(
	`@gloss(\?)?\s+([a-z][a-z0-9-]*)\s+(` + targetPat + `(?:\s*,\s*` + targetPat + `)*)((?:\s+[a-z]+=(?:"[^"]*"|[^\s]+))*)`)

// keyPat splits the key=value tail.
var keyPat = regexp.MustCompile(`([a-z]+)=("([^"]*)"|[^\s]+)`)

// loosePat finds anything that looks like an attempted marker, for diagnostics.
var loosePat = regexp.MustCompile(`@gloss\b`)

// spdxPat matches the SPDX-style header sugar: `Gloss-Implements: impl:X`,
// at the start of the comment (after block-comment stars), so prose that
// mentions the form mid-sentence is not a link.
var spdxPat = regexp.MustCompile(`^[\s*]*Gloss-([A-Z][A-Za-z]*(?:-[A-Z][A-Za-z]*)*):\s*(` + targetPat + `(?:\s*,\s*` + targetPat + `)*)\s*$`)

// Marker is one parsed `@gloss` line.
type Marker struct {
	Proposed bool              `json:"proposed,omitempty"`
	Relation string            `json:"relation"`
	Targets  []string          `json:"targets"`
	Keys     map[string]string `json:"keys,omitempty"`
	Line     int               `json:"line"` // 1-based physical line
	Col      int               `json:"col"`  // 1-based physical column of `@gloss` (or `Gloss-`)
	Raw      string            `json:"raw,omitempty"`
	// Sugar is true when the marker came from an SPDX-style header.
	Sugar bool `json:"sugar,omitempty"`
}

// Conf returns the conf= key as a float, or -1 when absent.
func (m *Marker) Conf() float64 {
	if v, ok := m.Keys["conf"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
	}
	return -1
}

// Scope returns the explicit scope= key, or "".
func (m *Marker) Scope() string { return m.Keys["scope"] }

// Diagnostic is a build warning or error at file:line:col. A hint is a
// pointer at a switch (GM255), never a defect: gloss check --strict ignores it.
type Diagnostic struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Col      int    `json:"col,omitempty"`
	Severity string `json:"severity"` // hint | warning | error
	Code     string `json:"code"`
	Msg      string `json:"msg"`
}

func (d Diagnostic) String() string {
	if d.Col > 0 {
		return fmt.Sprintf("%s:%d:%d: %s: %s [%s]", d.Path, d.Line, d.Col, d.Severity, d.Msg, d.Code)
	}
	return fmt.Sprintf("%s:%d: %s: %s [%s]", d.Path, d.Line, d.Severity, d.Msg, d.Code)
}

// ParseLine parses the marker in one comment line. text is the comment body
// (everything after the comment token). col0 is the physical column where
// text starts (1-based). It returns nil, nil when the line holds no marker,
// and a diagnostic when it holds a malformed one.
func ParseLine(text string, lineNo, col0 int) (*Marker, []Diagnostic) {
	var diags []Diagnostic
	if m := spdxPat.FindStringSubmatchIndex(text); m != nil {
		rel := strings.ToLower(text[m[2]:m[3]])
		mk := &Marker{
			Relation: rel,
			Targets:  splitTargets(text[m[4]:m[5]]),
			Keys:     map[string]string{"scope": "file"},
			Line:     lineNo,
			Col:      col0 + m[0],
			Raw:      strings.TrimSpace(text[m[0]:m[1]]),
			Sugar:    true,
		}
		return mk, nil
	}
	loc := loosePat.FindStringIndex(text)
	if loc == nil {
		return nil, nil
	}
	g := Grammar.FindStringSubmatchIndex(text)
	if g == nil || g[0] != loc[0] {
		diags = append(diags, Diagnostic{Line: lineNo, Col: col0 + loc[0], Severity: "warning", Code: "GM001",
			Msg: "malformed @gloss marker: expected '@gloss[?] <relation> <target>[, <target>...] [key=value ...]'"})
		return nil, diags
	}
	mk := &Marker{
		Proposed: g[2] >= 0,
		Relation: text[g[4]:g[5]],
		Targets:  splitTargets(text[g[6]:g[7]]),
		Keys:     map[string]string{},
		Line:     lineNo,
		Col:      col0 + g[0],
		Raw:      strings.TrimSpace(text[g[0]:g[1]]),
	}
	for _, kv := range keyPat.FindAllStringSubmatch(text[g[8]:g[9]], -1) {
		k := kv[1]
		v := kv[2]
		if strings.HasPrefix(v, `"`) {
			v = kv[3]
		}
		if !isAllowedKey(k) {
			diags = append(diags, Diagnostic{Line: lineNo, Col: col0 + g[0], Severity: "warning", Code: "GM002",
				Msg: fmt.Sprintf("unknown marker key %q; allowed: %s (provenance lives in the ledger)", k, strings.Join(model.MarkerKeys, ", "))})
			continue
		}
		if _, dup := mk.Keys[k]; dup {
			diags = append(diags, Diagnostic{Line: lineNo, Col: col0 + g[0], Severity: "warning", Code: "GM003", Msg: fmt.Sprintf("duplicate marker key %q", k)})
		}
		mk.Keys[k] = v
	}
	// Trailing text after the match must be whitespace or a comment closer.
	rest := strings.TrimSpace(text[g[1]:])
	rest = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(rest, "-->"), "*/"), "*)")
	rest = strings.TrimSpace(rest)
	if rest != "" && !strings.HasPrefix(rest, "<-") && !strings.HasPrefix(rest, "#") && !strings.HasPrefix(rest, "//") && !strings.HasPrefix(rest, "--") {
		diags = append(diags, Diagnostic{Line: lineNo, Col: col0 + g[1], Severity: "warning", Code: "GM004",
			Msg: fmt.Sprintf("unexpected text after marker: %q (keys are key=value; quote values with spaces)", rest)})
	}
	if s, ok := mk.Keys["scope"]; ok && !isScope(s) {
		diags = append(diags, Diagnostic{Line: lineNo, Col: mk.Col, Severity: "warning", Code: "GM005",
			Msg: fmt.Sprintf("invalid scope %q; allowed: %s", s, strings.Join(model.Scopes, ", "))})
		delete(mk.Keys, "scope")
	}
	if c, ok := mk.Keys["conf"]; ok {
		f, err := strconv.ParseFloat(c, 64)
		if err != nil || !(f >= 0 && f <= 1) { // NaN fails both
			diags = append(diags, Diagnostic{Line: lineNo, Col: mk.Col, Severity: "warning", Code: "GM006",
				Msg: fmt.Sprintf("conf must be a number in [0,1], got %q", c)})
			delete(mk.Keys, "conf")
		}
	}
	for _, t := range mk.Targets {
		if strings.Contains(t, "*") && mk.Relation != "may-modify" {
			diags = append(diags, Diagnostic{Line: lineNo, Col: mk.Col, Severity: "warning", Code: "GM007",
				Msg: fmt.Sprintf("glob target %q is only allowed on may-modify", t)})
		}
	}
	if len(mk.Keys) == 0 {
		mk.Keys = nil
	}
	return mk, diags
}

func splitTargets(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isAllowedKey(k string) bool {
	for _, a := range model.MarkerKeys {
		if a == k {
			return true
		}
	}
	return false
}

func isScope(s string) bool {
	for _, a := range model.Scopes {
		if a == s {
			return true
		}
	}
	return false
}

// IsNewTarget reports whether a target proposes a new concept (prefix:NEW:Name).
func IsNewTarget(t string) bool {
	i := strings.Index(t, ":")
	return i > 0 && strings.HasPrefix(t[i+1:], "NEW:")
}

// StripNew returns prefix:Name for a prefix:NEW:Name target.
func StripNew(t string) string {
	return strings.Replace(t, ":NEW:", ":", 1)
}
