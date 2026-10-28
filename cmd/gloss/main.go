// @gloss implements gmi:GlossCli scope=file

// Command gloss is the GlossMark CLI: build the canonical graph from markers,
// manifests and ontology files, then check, drift, diff, accept, view and serve it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/check"
)

// command is a subcommand. short is its one-paragraph description, printed
// by 'gloss <name> -h'; the one-line summary 'gloss -h' lists is in help.
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
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || (args[0] == "help" && len(args) == 1) {
		usage(os.Stderr)
		return 0
	}
	if args[0] == "help" {
		args = []string{args[1], "-h"}
	}
	if args[0] == "--version" || args[0] == "-v" {
		args = append([]string{"version"}, args[1:]...)
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].name < commands[j].name })
	for _, c := range commands {
		if c.name == args[0] {
			if help[c.name].noFlags && len(args) > 1 && isHelpFlag(args[1]) {
				commandHelp(os.Stderr, nil, c.name, nil)
				return 0
			}
			helped = false
			code := c.run(args[1:])
			if helped {
				return 0 // -h is not a usage error
			}
			return code
		}
	}
	names := make([]string, len(commands))
	for i, c := range commands {
		names[i] = c.name
	}
	if near := check.Closest(args[0], names); near != "" {
		fmt.Fprintf(os.Stderr, "gloss: unknown command %q; did you mean %q?\n", args[0], near)
	} else {
		fmt.Fprintf(os.Stderr, "gloss: unknown command %q\n", args[0])
	}
	fmt.Fprintln(os.Stderr, "Run 'gloss -h' for the list.")
	return 2
}

func isHelpFlag(s string) bool { return s == "-h" || s == "-help" || s == "--help" }

// helped is set when parseFlags answered -h, so run exits 0 although the
// command returns its usage-error code.
var helped bool

// newFlags returns a FlagSet with the shared --root flag. Its Usage is left
// nil so parseFlags can tell whether the command set its own.
func newFlags(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet("gloss "+name, flag.ContinueOnError)
	fs.Usage = nil
	root := fs.String("root", "", "repository root (default: nearest glossmark.yaml above the working directory)")
	return fs, root
}

// parseFlags parses flags wherever they appear among the positional
// arguments ("gloss import <file> --namespace ns"); the flag package alone
// stops at the first positional. A "--" ends flag parsing as usual. Afterwards
// fs.Args() holds the positionals in their original order. -h prints the
// command's help (commandHelp) above its flags, or above the usage the
// command set itself.
func parseFlags(fs *flag.FlagSet, args []string) error {
	own := fs.Usage
	fs.Usage = func() { commandHelp(fs.Output(), fs, strings.TrimPrefix(fs.Name(), "gloss "), own) }
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			helped = errors.Is(err, flag.ErrHelp)
			return err
		}
		rest := fs.Args()
		if n := len(args) - len(rest); n > 0 && args[n-1] == "--" {
			pos = append(pos, rest...)
			break
		}
		if len(rest) == 0 {
			break
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
	return fs.Parse(append([]string{"--"}, pos...))
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
