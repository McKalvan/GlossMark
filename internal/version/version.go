// @gloss represents gmi:FormatVersions scope=file

// Package version holds the three version numbers GlossMark exposes: the tool
// release (semver, injected at build time), the grammar version a repository
// opts into with the `grammar:` directive, and the on-disk format versions of
// the files the tool maintains.
package version

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
)

// pseudo matches Go pseudo-versions (vX.Y.Z-0.yyyymmddhhmmss-abcdefabcdef) and dirty builds.
var pseudo = regexp.MustCompile(`-\d{14}-[0-9a-f]{12}|\+dirty$`)

// Version is the tool release. Builds from a tagged module get it from the Go
// build info; release builds inject it with
// -ldflags "-X github.com/mckalvan/glossmark/internal/version.Version=v0.2.0".
var Version = ""

// Grammar is the newest `@gloss` grammar version this tool understands. A
// repository pins one with `grammar: N` in glossmark.yaml; markers that parse
// under grammar N keep parsing under every later tool release that lists N.
const Grammar = 1

// SupportedGrammars lists the grammar versions this release accepts.
var SupportedGrammars = []int{1}

// GraphFormat is the .glossmark/graph.jsonl record format written by this release.
const GraphFormat = 1

// LedgerFormat is the .glossmark/ledger.jsonl format written by this release.
const LedgerFormat = 1

// LockFormat is the glossmark.lock format.
const LockFormat = 1

// String returns the effective tool version: the injected one, else the Go
// module version (go install ...@v0.2.0), else a development marker.
func String() string {
	if Version != "" {
		return strings.TrimPrefix(Version, "v")
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" && !pseudo.MatchString(bi.Main.Version) {
		// A real module version (go install ...@vX.Y.Z); pseudo-versions from a
		// plain checkout build are treated as development builds so generated
		// files do not churn with every commit.
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return "0.0.0-dev"
}

// Generator is the string stamped into generated files.
func Generator() string { return "gloss " + String() }

// SupportsGrammar reports whether this release accepts a grammar version.
func SupportsGrammar(n int) bool {
	for _, g := range SupportedGrammars {
		if g == n {
			return true
		}
	}
	return false
}

// Report renders every version for `gloss version`.
func Report() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "gloss %s\n", String())
	fmt.Fprintf(&sb, "grammar %d (supported: %s)\n", Grammar, join(SupportedGrammars))
	fmt.Fprintf(&sb, "graph format %d, ledger format %d, lock format %d\n", GraphFormat, LedgerFormat, LockFormat)
	if bi, ok := debug.ReadBuildInfo(); ok {
		fmt.Fprintf(&sb, "built with %s\n", bi.GoVersion)
	}
	return sb.String()
}

func join(xs []int) string {
	var s []string
	for _, x := range xs {
		s = append(s, fmt.Sprint(x))
	}
	return strings.Join(s, ", ")
}
