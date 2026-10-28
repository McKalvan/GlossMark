// @gloss verifies gmi:GlossCli scope=file

package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestHelpCoversCommands: every registered command is listed in exactly one
// group of 'gloss -h' and has a help entry, and the groups name no command
// that does not exist (#231).
func TestHelpCoversCommands(t *testing.T) {
	registered := map[string]bool{}
	for _, c := range commands {
		registered[c.name] = true
	}
	seen := map[string]string{}
	for _, g := range helpGroups {
		for _, name := range g.commands {
			if prev, ok := seen[name]; ok {
				t.Errorf("%s is listed under both %s and %s", name, prev, g.name)
			}
			seen[name] = g.name
			if !registered[name] {
				t.Errorf("%s is listed under %s but not registered", name, g.name)
			}
		}
	}
	for name := range registered {
		if _, ok := seen[name]; !ok {
			t.Errorf("%s is registered but not listed in any group of gloss -h", name)
		}
		if help[name].line == "" {
			t.Errorf("%s has no help entry", name)
		}
	}
	for name := range help {
		if !registered[name] {
			t.Errorf("help has an entry for %s, which is not registered", name)
		}
	}
}

// TestTopLevelHelp: 'gloss -h' fits 80 columns and lists each command once.
func TestTopLevelHelp(t *testing.T) {
	out, code := capture(t, "-h")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, l := range strings.Split(out, "\n") {
		if n := utf8.RuneCountInString(l); n > 80 {
			t.Errorf("line is %d columns: %q", n, l)
		}
	}
	for _, c := range commands {
		if n := strings.Count(out, "\n  "+c.name+" "); n != 1 {
			t.Errorf("%s listed %d times", c.name, n)
		}
	}
}

// TestCommandHelp: 'gloss <command> -h' exits 0 with a synopsis, the
// description, a docs link and, when the command has flags, the flags (#232).
func TestCommandHelp(t *testing.T) {
	for _, c := range commands {
		out, code := capture(t, c.name, "-h")
		if code != 0 {
			t.Errorf("gloss %s -h: exit %d", c.name, code)
		}
		if !strings.Contains(out, "gloss "+c.name) || !strings.Contains(out, "usage:") {
			t.Errorf("gloss %s -h: no synopsis:\n%s", c.name, out)
		}
		if !strings.Contains(out, "Docs: docs/") {
			t.Errorf("gloss %s -h: no docs link:\n%s", c.name, out)
		}
		if !strings.Contains(out, sentence(c.short)[:20]) {
			t.Errorf("gloss %s -h: no description:\n%s", c.name, out)
		}
		if !help[c.name].noFlags && !strings.Contains(out, "-root") {
			t.Errorf("gloss %s -h: no flags:\n%s", c.name, out)
		}
		head, _, _ := strings.Cut(out, "\nDocs: ")
		for _, l := range strings.Split(head, "\n") {
			if n := utf8.RuneCountInString(l); n > 80 {
				t.Errorf("gloss %s -h: line is %d columns: %q", c.name, n, l)
			}
		}
		if strings.Contains(out, "Usage of") {
			t.Errorf("gloss %s -h: Go's default usage:\n%s", c.name, out)
		}
	}
	if out, code := capture(t, "help", "drift"); code != 0 || !strings.Contains(out, "usage: gloss drift") {
		t.Errorf("gloss help drift: exit %d\n%s", code, out)
	}
}

// TestUnknownCommand: a typo gets a suggestion and a pointer to the list,
// not the whole help (#233).
func TestUnknownCommand(t *testing.T) {
	out, code := capture(t, "chek")
	want := "gloss: unknown command \"chek\"; did you mean \"check\"?\nRun 'gloss -h' for the list.\n"
	if code != 2 || out != want {
		t.Errorf("gloss chek: exit %d, output %q; want 2, %q", code, out, want)
	}
	out, _ = capture(t, "frobnicate")
	if strings.Contains(out, "did you mean") || strings.Contains(out, "Daily:") {
		t.Errorf("gloss frobnicate: %q", out)
	}
}
