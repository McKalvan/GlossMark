// Command gloss is the GlossMark CLI: build the canonical graph from markers,
// manifests and ontology files, then check, drift, diff, accept, view and serve it.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

type command struct {
	name  string
	short string
	run   func(args []string) int
}

var commands []command

func register(c command) { commands = append(commands, c) }

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage()
		return 0
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].name < commands[j].name })
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(args[1:])
		}
	}
	fmt.Fprintf(os.Stderr, "gloss: unknown command %q\n\n", args[0])
	usage()
	return 2
}

func usage() {
	sort.Slice(commands, func(i, j int) bool { return commands[i].name < commands[j].name })
	fmt.Fprintln(os.Stderr, "gloss — GlossMark: comment-native code ontology")
	fmt.Fprintln(os.Stderr, "\nUsage: gloss <command> [flags]\n\nCommands:")
	w := 0
	for _, c := range commands {
		if len(c.name) > w {
			w = len(c.name)
		}
	}
	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "  %-*s  %s\n", w, c.name, c.short)
	}
	fmt.Fprintln(os.Stderr, "\nRun 'gloss <command> -h' for flags. Set GLOSS_ACTOR (human:alice, agent:claude-code@model) for ledger attribution.")
}

// newFlags returns a FlagSet with the shared --root flag.
func newFlags(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet("gloss "+name, flag.ContinueOnError)
	root := fs.String("root", "", "repository root (default: nearest glossmark.yaml above the working directory)")
	return fs, root
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "gloss:", err)
	return 1
}

func plural(n int, s string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, s)
	}
	return fmt.Sprintf("%d %ss", n, s)
}

func join(xs []string) string { return strings.Join(xs, ", ") }
