// @gloss implements gmi:ReviewCommands scope=file

package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/mckalvan/glossmark/internal/extract"
)

// A mapped link (a links: entry in a manifest or glossmark.yaml, ADR 0045)
// is reviewed like a marker: accepting it drops proposed: true from its
// entry, the way accepting a marker drops the ?, and reject --remove
// deletes the entry.

var (
	proposedLine = regexp.MustCompile(`^\s*proposed:\s*true\s*(#.*)?$`)
	proposedTail = regexp.MustCompile(`,\s*proposed:\s*true\b`)
	proposedHead = regexp.MustCompile(`\bproposed:\s*true\s*,\s*`)
)

// where names the place a link is written, path:line, for messages.
func where(fm extract.FoundMarker) string {
	p, l := fm.Stated()
	return fmt.Sprintf("%s:%d", p, l)
}

// linkSpan returns the lines of the file that holds a mapped link's entry
// and the entry's 0-based inclusive range in them.
func linkSpan(p *pipeline, fm extract.FoundMarker) ([]string, int, int, error) {
	b, err := os.ReadFile(p.WS.Abs(fm.From))
	if err != nil {
		return nil, 0, 0, err
	}
	lines := strings.Split(string(b), "\n")
	start, end := fm.FromLine-1, fm.FromEnd-1
	if start < 0 || start >= len(lines) {
		return nil, 0, 0, fmt.Errorf("%s: the links: entry is not at line %d any more; rebuild and retry", fm.From, fm.FromLine)
	}
	if end < start {
		end = start
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}
	return lines, start, end, nil
}

// unproposeLink drops proposed: true from the links: entry that maps fm.
func unproposeLink(p *pipeline, fm extract.FoundMarker) error {
	lines, start, end, err := linkSpan(p, fm)
	if err != nil {
		return err
	}
	for i := start; i <= end; i++ {
		l := lines[i]
		switch {
		case proposedLine.MatchString(l):
			lines = append(lines[:i], lines[i+1:]...)
		case proposedTail.MatchString(l):
			lines[i] = proposedTail.ReplaceAllString(l, "")
		case proposedHead.MatchString(l):
			lines[i] = proposedHead.ReplaceAllString(l, "")
		default:
			continue
		}
		return os.WriteFile(p.WS.Abs(fm.From), []byte(strings.Join(lines, "\n")), 0o644)
	}
	return fmt.Errorf("%s: no proposed: true to drop in the links: entry at line %d (a proposed: on the entry's first line is left to you)", fm.From, fm.FromLine)
}

// removeLinkEntry deletes the links: entry that maps fm.
func removeLinkEntry(p *pipeline, fm extract.FoundMarker) error {
	lines, start, end, err := linkSpan(p, fm)
	if err != nil {
		return err
	}
	lines = append(lines[:start], lines[end+1:]...)
	return os.WriteFile(p.WS.Abs(fm.From), []byte(strings.Join(lines, "\n")), 0o644)
}
