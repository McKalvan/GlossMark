// @gloss verifies gmi:Diagnostics scope=file

package diag

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// repoRoot is the repository root, two levels above this package.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// sourceLine is one line of non-test Go source under internal/ or cmd/,
// outside this package (which names every code by construction).
type sourceLine struct {
	pos  string
	text string
}

func goSource(t *testing.T) []sourceLine {
	t.Helper()
	root := repoRoot(t)
	self := filepath.Join(root, "internal", "diag")
	var out []sourceLine
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path == self {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			for i, l := range strings.Split(string(b), "\n") {
				out = append(out, sourceLine{pos: filepath.ToSlash(rel) + ":" + strconv.Itoa(i+1), text: l})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(out) == 0 {
		t.Fatal("no Go source found under internal/ and cmd/")
	}
	return out
}

var (
	// anyGM is any short string literal that starts with GM: a whole code,
	// or a fragment such as "GM12" or "GM%03d" that builds one.
	anyGM = regexp.MustCompile(`"GM[^"\s]{0,6}"`)
	whole = regexp.MustCompile(`^"GM[0-9]{3}"$`)
	// emitted is a code passed as an argument or set as Code: — an emission,
	// not a comparison such as d.Code == "GM107".
	emitted  = regexp.MustCompile(`(?:Code:\s*|,\s*)"(GM[0-9]{3})"`)
	severity = regexp.MustCompile(`"(error|warning|info)"`)
)

// TestEveryEmittedCodeHasARow fails when Go source names a GM code that the
// table does not document, or builds a code from a fragment this scan cannot
// follow.
func TestEveryEmittedCodeHasARow(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range goSource(t) {
		for _, lit := range anyGM.FindAllString(l.text, -1) {
			if !whole.MatchString(lit) {
				t.Errorf("%s: %s builds a diagnostic code from a fragment; write every code as a full literal such as \"GM305\" so this test and gloss explain can see it", l.pos, lit)
				continue
			}
			code := strings.Trim(lit, `"`)
			if _, ok := Lookup(code); !ok && !seen[code] {
				t.Errorf("%s: %s has no row in internal/diag; add one (severity, meaning, fix, doc page), then run %s", l.pos, code, RegenerateCommand)
			}
			seen[code] = true
		}
	}
}

// TestEveryRowIsEmitted fails on a stale row, and on a row whose severity
// disagrees with what the emitting line passes.
func TestEveryRowIsEmitted(t *testing.T) {
	sevs := map[string]map[string]bool{}
	for _, l := range goSource(t) {
		for _, m := range emitted.FindAllStringSubmatch(l.text, -1) {
			code := m[1]
			if sevs[code] == nil {
				sevs[code] = map[string]bool{}
			}
			// A severity held in a variable (GM320, GM502) is not on the line.
			for _, s := range severity.FindAllStringSubmatch(l.text, -1) {
				sevs[code][s[1]] = true
			}
		}
	}
	for _, c := range Codes {
		got, ok := sevs[c.Code]
		if !ok {
			t.Errorf("%s has a row but nothing under internal/ or cmd/ emits it; remove the row and run %s", c.Code, RegenerateCommand)
			continue
		}
		for s := range got {
			if !strings.Contains(c.Severity, s) {
				t.Errorf("%s is emitted as %s but its row says %q", c.Code, s, c.Severity)
			}
		}
	}
}

// TestRows keeps the table well formed: ordered, unique, in a range, one line
// per field, and pointing at a page and heading that exist.
func TestRows(t *testing.T) {
	root := repoRoot(t)
	codeShape := regexp.MustCompile(`^GM[0-9]{3}$`)
	anchors := map[string]map[string]bool{}
	for i, c := range Codes {
		if !codeShape.MatchString(c.Code) {
			t.Errorf("row %d: code %q is not GMnnn", i, c.Code)
		}
		if i > 0 && Codes[i-1].Code >= c.Code {
			t.Errorf("%s: rows must be in code order without duplicates (follows %s)", c.Code, Codes[i-1].Code)
		}
		if RangeOf(c.Code).Prefix == "" {
			t.Errorf("%s: no entry in Ranges covers it", c.Code)
		}
		switch c.Severity {
		case "error", "warning", "info", "error or warning":
		default:
			t.Errorf("%s: severity %q", c.Code, c.Severity)
		}
		for name, v := range map[string]string{"meaning": c.Meaning, "fix": c.Fix, "doc": c.Doc} {
			if strings.TrimSpace(v) == "" || strings.Contains(v, "\n") {
				t.Errorf("%s: %s must be one non-empty line", c.Code, name)
			}
		}
		page, anchor, _ := strings.Cut(c.Doc, "#")
		if _, ok := anchors[page]; !ok {
			b, err := os.ReadFile(filepath.Join(root, "docs", page))
			if err != nil {
				t.Errorf("%s: doc page docs/%s: %v", c.Code, page, err)
				continue
			}
			anchors[page] = headingAnchors(string(b))
		}
		if anchor != "" && !anchors[page][anchor] {
			t.Errorf("%s: docs/%s has no heading with anchor #%s", c.Code, page, anchor)
		}
	}
	var prefixes []string
	for _, r := range Ranges {
		prefixes = append(prefixes, r.Prefix)
	}
	if !sort.StringsAreSorted(prefixes) {
		t.Errorf("Ranges must be in code order: %v", prefixes)
	}
}

// headingAnchors returns the GitHub anchors of a Markdown page's headings,
// outside fenced code blocks.
func headingAnchors(md string) map[string]bool {
	out := map[string]bool{}
	fenced := false
	for _, l := range strings.Split(md, "\n") {
		if strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || !strings.HasPrefix(l, "#") {
			continue
		}
		h := strings.TrimSpace(strings.TrimLeft(l, "#"))
		var sb strings.Builder
		for _, r := range strings.ToLower(h) {
			switch {
			case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
				sb.WriteRune(r)
			case r == ' ':
				sb.WriteRune('-')
			}
		}
		out[sb.String()] = true
	}
	return out
}

func TestLookupAndFamily(t *testing.T) {
	for _, in := range []string{"GM305", "gm305", "305", " GM305 "} {
		if c, ok := Lookup(in); !ok || c.Code != "GM305" {
			t.Errorf("Lookup(%q) = %v, %v", in, c.Code, ok)
		}
	}
	if _, ok := Lookup("GM999"); ok {
		t.Error("Lookup(GM999) found a row")
	}
	var got []string
	for _, c := range Family("GM12x") {
		got = append(got, c.Code)
	}
	if strings.Join(got, ",") != "GM120,GM121,GM122" {
		t.Errorf("Family(GM12x) = %v", got)
	}
	if len(Family("GM3xx")) == 0 || Family("GM305") != nil || Family("GMxxx") != nil {
		t.Error("Family accepts only GMnx and GMnnx patterns")
	}
	txt := Text([]Code{Codes[0]})
	for _, want := range []string{Codes[0].Code, "Meaning:", "Fix:", "Docs:    docs/"} {
		if !strings.Contains(txt, want) {
			t.Errorf("Text output lacks %q:\n%s", want, txt)
		}
	}
}

// TestDiagnosticsDocIsGenerated is the golden test for docs/DIAGNOSTICS.md.
func TestDiagnosticsDocIsGenerated(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docs", "DIAGNOSTICS.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; generate it from the repository root with: %s", err, RegenerateCommand)
	}
	if string(b) != Markdown() {
		t.Fatalf("docs/DIAGNOSTICS.md is out of date with the table in internal/diag; never edit it by hand. Regenerate it from the repository root with:\n\n  %s", RegenerateCommand)
	}
}
