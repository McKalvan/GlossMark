package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// workspace copies an example repository into a temp dir so a CLI test can
// write to it.
func workspace(t *testing.T, example string) string {
	t.Helper()
	src := filepath.Join("..", "..", "examples", example)
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

// gloss runs one CLI invocation and returns its exit code and stdout.
func gloss(t *testing.T, args ...string) (int, string) {
	t.Helper()
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	code := run(args)
	w.Close()
	os.Stdout = old
	return code, <-done
}

func TestParseFlagsInterspersed(t *testing.T) {
	cases := []struct {
		args []string
		pos  []string
		ns   string
		dry  bool
	}{
		{[]string{"--from", "md", "a.md", "--namespace", "lib"}, []string{"a.md"}, "lib", false},
		{[]string{"a.md", "--namespace", "lib", "--dry-run"}, []string{"a.md"}, "lib", true},
		{[]string{"--namespace=lib", "a.md", "b.md", "--dry-run"}, []string{"a.md", "b.md"}, "lib", true},
		{[]string{"a.md", "--", "--namespace", "x"}, []string{"a.md", "--namespace", "x"}, "", false},
		{nil, nil, "", false},
	}
	for _, c := range cases {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.String("from", "", "")
		ns := fs.String("namespace", "", "")
		dry := fs.Bool("dry-run", false, "")
		if err := parseFlags(fs, c.args); err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		if got := fs.Args(); !(len(got) == 0 && len(c.pos) == 0) && !reflect.DeepEqual(got, c.pos) {
			t.Errorf("%v: positionals %v, want %v", c.args, got, c.pos)
		}
		if *ns != c.ns || *dry != c.dry {
			t.Errorf("%v: namespace=%q dry=%v, want %q %v", c.args, *ns, *dry, c.ns, c.dry)
		}
	}
}

// Issue #122: the QUICKSTART order, flags after the file, parses.
func TestImportFlagsAfterFile(t *testing.T) {
	root := workspace(t, "horse-factory")
	md := filepath.Join(root, "glossary.md")
	os.WriteFile(md, []byte("# Glossary\n\n## Paddock\n\n**Def.** A fenced field where horses graze.\n"), 0o644)
	code, out := gloss(t, "import", "--from", "markdown-glossary", md, "--namespace", "zoo", "--dry-run", "--root", root)
	if code != 0 || !strings.Contains(out, "1 added") {
		t.Fatalf("import exit %d, out %q", code, out)
	}
}

// Issue #29: --version and -v are aliases of the version subcommand.
func TestVersionFlags(t *testing.T) {
	_, want := gloss(t, "version")
	for _, a := range []string{"--version", "-v"} {
		code, out := gloss(t, a)
		if code != 0 || out != want || out == "" {
			t.Errorf("gloss %s: exit %d, out %q, want %q", a, code, out, want)
		}
	}
}
