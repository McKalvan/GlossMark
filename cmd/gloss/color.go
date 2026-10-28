// @gloss implements gmi:GlossCli scope=file

package main

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// colorOn reports whether output to f may carry ANSI colour: f is a
// terminal, NO_COLOR is unset (https://no-color.org) and TERM is not dumb.
// Piped output and logs stay plain.
func colorOn(f *os.File) bool {
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" && term.IsTerminal(int(f.Fd()))
}

// severityColor colours the severity word of a diagnostic line ("path:1:
// error: ..."): errors red, warnings amber, hints cyan.
func severityColor(line, sev string) string {
	style := "\x1b[1;33m"
	switch sev {
	case "error":
		style = "\x1b[1;31m"
	case "hint":
		style = "\x1b[1;36m"
	}
	return strings.Replace(line, ": "+sev+": ", ": "+style+sev+"\x1b[0m: ", 1)
}
