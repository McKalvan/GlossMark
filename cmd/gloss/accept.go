// @gloss implements gmi:ReviewCommands scope=file
// @gloss publishes gm:LinkAccepted, gm:LinkRejected scope=file

package main

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/model"
)

func init() {
	register(command{"accept", "accept proposed links: the one-character diff @gloss? → @gloss plus the ledger entry", cmdAccept})
	register(command{"reject", "reject a proposed link with a reason (optionally remove the marker line)", cmdReject})
	register(command{"status", "list proposed, stale and rejected links from the ledger, each proposal with its gate and the reason gloss diff gives", cmdStatus})
}

func reviewer(root string) string {
	if a := os.Getenv("GLOSS_ACTOR"); a != "" {
		return a
	}
	return ledger.DefaultActor(root)
}

// selectMarkers resolves selectors (marker ids or path:line) to markers.
func selectMarkers(p *pipeline, sel []string) ([]extract.FoundMarker, error) {
	var out []extract.FoundMarker
	for _, s := range sel {
		found := false
		for _, fm := range p.Res.Markers {
			if fm.ID == s || strings.TrimPrefix(fm.ID, "m-") == s {
				out = append(out, fm)
				found = true
				break
			}
			if i := strings.LastIndex(s, ":"); i > 0 {
				if ln, err := strconv.Atoi(s[i+1:]); err == nil && fm.Path == s[:i] && fm.Marker.Line == ln {
					out = append(out, fm)
					found = true
					break
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("no marker matches %q (use a marker id from gloss status, or path:line)", s)
		}
	}
	return out, nil
}

// flipMarker rewrites `@gloss?` to `@gloss` on the marker's line.
func flipMarker(p *pipeline, fm extract.FoundMarker) error {
	path := p.WS.Abs(fm.Path)
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	if fm.Marker.Line-1 >= len(lines) {
		return fmt.Errorf("%s:%d: line out of range", fm.Path, fm.Marker.Line)
	}
	l := lines[fm.Marker.Line-1]
	col := fm.Marker.Col - 1
	if col < 0 || col >= len(l) || !strings.HasPrefix(l[col:], "@gloss?") {
		idx := strings.Index(l, "@gloss?")
		if idx < 0 {
			return fmt.Errorf("%s:%d: no @gloss? on this line", fm.Path, fm.Marker.Line)
		}
		col = idx
	}
	lines[fm.Marker.Line-1] = l[:col] + "@gloss" + l[col+len("@gloss?"):]
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func cmdAccept(args []string) int {
	fs, root := newFlags("accept")
	all := fs.Bool("all", false, "accept every proposed link")
	byPolicy := fs.Bool("policy", false, "accept every proposed link the policy classifies SOFT")
	dry := fs.Bool("n", false, "dry run: show what would be accepted")
	concept := fs.Bool("concept", false, "the arguments are proposed concept CURIEs (proposed: true in an ontology file)")
	value := fs.Bool("value", false, "the arguments are concept CURIEs whose drifted published value is confirmed as is")
	force := fs.Bool("force", false, "with --value: confirm even when the bound line does not hold the published value")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() == 0 && !*all && !*byPolicy {
		fmt.Fprintln(os.Stderr, "usage: gloss accept <marker-id|path:line>... | --all | --policy | --concept <ns:Name>... | --value <ns:Name>...")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if *value {
		rev, now, n := reviewer(p.WS.Root), time.Now().UTC(), 0
		for _, c := range fs.Args() {
			node := p.Graph.NodeByCurie(c)
			e := p.Ledger.Entry("value:" + c)
			if node == nil || e == nil || node.ValueFP == "" {
				return fail(fmt.Errorf("%s has no value-from binding", c))
			}
			// Confirming says the line holds the published value. When the
			// value's literal is nowhere in it, the code and the ontology
			// disagree, and confirming would hide that until the next edit.
			if !valueInLine(node.Value, node.LiveValue) {
				msg := fmt.Sprintf("%s: the bound code %s (%s) does not hold the published value %q; update value: in the ontology to match the code, or change the code", c, node.ValueFrom, node.LiveValue, fmt.Sprint(node.Value))
				if !*force {
					return fail(fmt.Errorf("%s; --force confirms the line as is", msg))
				}
				fmt.Fprintf(os.Stderr, "gloss accept: %s; confirming anyway (--force)\n", msg)
			}
			fmt.Printf("%s value of %s: %s is now the baseline for %v\n", map[bool]string{true: "would confirm", false: "confirm"}[*dry], c, node.LiveValue, node.Value)
			if *dry {
				continue
			}
			p.Ledger.Accept("value:"+c, rev, node.ValueFP, now)
			e.Live = node.LiveValue
			n++
		}
		if !*dry {
			if err := p.Ledger.Save(); err != nil {
				return fail(err)
			}
			fmt.Printf("gloss accept: %s confirmed by %s\n", plural(n, "published value"), rev)
		}
		return 0
	}
	if *concept {
		n, err := acceptConcepts(p, fs.Args(), reviewer(p.WS.Root), *dry)
		if err != nil {
			return fail(err)
		}
		if !*dry {
			if err := p.Ledger.Save(); err != nil {
				return fail(err)
			}
			fmt.Printf("gloss accept: %s accepted by %s\n", plural(n, "concept"), reviewer(p.WS.Root))
		}
		return 0
	}
	pol := p.WS.Config.EffectivePolicy()
	var targets []extract.FoundMarker
	if fs.NArg() > 0 {
		targets, err = selectMarkers(p, fs.Args())
		if err != nil {
			return fail(err)
		}
	} else {
		for _, fm := range p.Res.Markers {
			if !fm.Marker.Proposed {
				continue
			}
			if *byPolicy {
				soft := true
				for _, e := range fm.Edges {
					if g, _ := gate.ClassifyLink(p.Graph, p.WS, e, pol); g != gate.SOFT {
						soft = false
					}
				}
				if !soft {
					continue
				}
			}
			targets = append(targets, fm)
		}
	}
	rev := reviewer(p.WS.Root)
	now := time.Now().UTC()
	n := 0
	done := map[string]bool{}
	for _, fm := range targets {
		if done[fm.ID] {
			continue
		}
		done[fm.ID] = true
		// Every `@gloss?` line carrying the id is accepted, not just the first
		// (two identical markers on one anchor share an id; GM132).
		var lines []extract.FoundMarker
		for _, o := range p.Res.Markers {
			if o.ID == fm.ID && o.Marker.Proposed {
				lines = append(lines, o)
			}
		}
		if len(lines) > 0 {
			fm = lines[0]
		}
		if !fm.Marker.Proposed {
			// A stale link is re-confirmed: the current anchor becomes the baseline.
			if e := p.Ledger.Entry(fm.ID); e != nil && e.Status == model.StatusStale {
				fmt.Printf("%s %s:%d %s %s (stale)\n", map[bool]string{true: "would re-confirm", false: "re-confirm"}[*dry], fm.Path, fm.Marker.Line, fm.Marker.Relation, join(fm.Marker.Targets))
				if !*dry {
					p.Ledger.Accept(fm.ID, rev, fm.Anchor.Fingerprint, now)
					n++
				}
				continue
			}
			fmt.Fprintf(os.Stderr, "%s:%d: %s is not proposed; nothing to accept\n", fm.Path, fm.Marker.Line, fm.ID)
			continue
		}
		g := gate.SOFT
		var why []string
		for _, e := range fm.Edges {
			cg, r := gate.ClassifyLink(p.Graph, p.WS, e, pol)
			g = gate.Max(g, cg)
			why = append(why, r...)
		}
		fmt.Printf("%s %s:%d %s %s [%s]\n", map[bool]string{true: "would accept", false: "accept"}[*dry], fm.Path, fm.Marker.Line, fm.Marker.Relation, join(fm.Marker.Targets), g)
		if *dry {
			continue
		}
		for _, l := range lines {
			if err := flipMarker(p, l); err != nil {
				return fail(err)
			}
		}
		p.Ledger.Accept(fm.ID, rev, fm.Anchor.Fingerprint, now)
		if e := p.Ledger.Entry(fm.ID); e != nil {
			e.Gate = string(g)
		}
		n++
	}
	if !*dry {
		if err := p.Ledger.Save(); err != nil {
			return fail(err)
		}
		fmt.Printf("gloss accept: %s accepted by %s\n", plural(n, "link"), rev)
	}
	return 0
}

// numLiteral matches a number as code or prose writes it: a sign, digit
// group separators (1_000, or 1,000 in thousands), a fraction, an exponent.
var numLiteral = regexp.MustCompile(`[-+]?(?:\d{1,3}(?:,\d{3})+(?:\.\d+)?|\d[\d_]*(?:\.\d+)?|\.\d+)(?:[eE][-+]?\d+)?`)

// valueInLine reports whether a published value appears in the bound code:
// some number in the value equals a number in the line (a percentage also
// as a fraction, so 4% matches 0.04; a suffix such as 60L or 30d is not part
// of the number), or, for a value without numbers, the value's text appears
// in the line, unquoted and case-insensitive. A value with nothing to
// compare (none, or a list or map) is not judged.
func valueInLine(value any, line string) bool {
	text := strings.TrimSpace(fmt.Sprint(value))
	if value == nil || text == "" {
		return true
	}
	switch value.(type) {
	case []any, map[string]any:
		return true
	}
	nums := func(s string) []float64 {
		var out []float64
		for _, m := range numLiteral.FindAllStringIndex(s, -1) {
			if m[0] > 0 && isIdentByte(s[m[0]-1]) {
				continue // the digits of a name: MAX_2, v1
			}
			f, err := strconv.ParseFloat(strings.NewReplacer("_", "", ",", "").Replace(s[m[0]:m[1]]), 64)
			if err != nil {
				continue
			}
			out = append(out, f)
			if m[1] < len(s) && s[m[1]] == '%' {
				out = append(out, f/100)
			}
		}
		return out
	}
	want := nums(text)
	if len(want) == 0 {
		return strings.Contains(strings.ToLower(line), strings.ToLower(strings.Trim(text, `"'`)))
	}
	have := nums(line)
	for _, w := range want {
		for _, h := range have {
			if w == h || math.Abs(w-h) <= 1e-9*math.Max(math.Abs(w), math.Abs(h)) {
				return true
			}
		}
	}
	return false
}

func isIdentByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func cmdReject(args []string) int {
	fs, root := newFlags("reject")
	reason := fs.String("reason", "", "why the link is wrong (recorded in the ledger)")
	remove := fs.Bool("remove", false, "also delete the marker line from the source file (or the concept entry with --concept)")
	concept := fs.Bool("concept", false, "the arguments are proposed concept CURIEs")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: gloss reject <marker-id|path:line>... --reason '...' [--remove]\n       gloss reject --concept <ns:Name>... --reason '...' [--remove]")
		return 2
	}
	if *reason == "" {
		fmt.Fprintln(os.Stderr, "gloss reject: --reason is required; rejected(reason) is a review state")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if *concept {
		for _, c := range fs.Args() {
			if err := rejectConcept(p, c, reviewer(p.WS.Root), *reason, *remove); err != nil {
				return fail(err)
			}
			fmt.Printf("reject concept %s — %s\n", c, *reason)
		}
		if *remove {
			// The markers that linked to a removed concept went back to
			// ns:NEW:Name; their entries follow them (ADR 0027).
			if err := carryMarkerIDs(p); err != nil {
				return fail(err)
			}
		}
		if err := p.Ledger.Save(); err != nil {
			return fail(err)
		}
		return 0
	}
	targets, err := selectMarkers(p, fs.Args())
	if err != nil {
		return fail(err)
	}
	rev := reviewer(p.WS.Root)
	now := time.Now().UTC()
	for _, fm := range targets {
		p.Ledger.Reject(fm.ID, rev, *reason, now)
		fmt.Printf("reject %s:%d %s %s — %s\n", fm.Path, fm.Marker.Line, fm.Marker.Relation, join(fm.Marker.Targets), *reason)
		if *remove {
			if err := removeMarkerLine(p, fm); err != nil {
				return fail(err)
			}
		}
	}
	if err := p.Ledger.Save(); err != nil {
		return fail(err)
	}
	return 0
}

func removeMarkerLine(p *pipeline, fm extract.FoundMarker) error {
	path := p.WS.Abs(fm.Path)
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	i := fm.Marker.Line - 1
	if i >= len(lines) {
		return nil
	}
	l := lines[i]
	// Only remove when the line holds nothing but the marker comment.
	stripped := strings.TrimSpace(l)
	if !strings.Contains(stripped, fm.Marker.Raw) || strings.Index(stripped, fm.Marker.Raw) > 8 {
		fmt.Fprintf(os.Stderr, "%s:%d: line holds more than the marker; not removed\n", fm.Path, fm.Marker.Line)
		return nil
	}
	lines = append(lines[:i], lines[i+1:]...)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// proposalGate renders the gate of a proposed marker the way gloss diff does:
// "[SOFT: auto-accept eligible]", or "[HARD: <reasons>]" so a reader can tell
// a confidence below the threshold from a target outside the allowed set
// without running diff. It is "" for an id with no marker in the tree (a
// proposed concept).
func proposalGate(p *pipeline, pol *model.Policy, id string) string {
	for _, fm := range p.Res.Markers {
		if fm.ID != id {
			continue
		}
		cls := gate.SOFT
		var why []string
		seen := map[string]bool{}
		for _, ed := range fm.Edges {
			c, r := gate.ClassifyLink(p.Graph, p.WS, ed, pol)
			cls = gate.Max(cls, c)
			if c != gate.HARD {
				continue
			}
			for _, x := range r {
				if !seen[x] {
					seen[x] = true
					why = append(why, x)
				}
			}
		}
		if cls == gate.HARD {
			return "  [HARD: " + strings.Join(why, "; ") + "]"
		}
		return "  [" + string(cls) + ": auto-accept eligible]"
	}
	return ""
}

func cmdStatus(args []string) int {
	fs, root := newFlags("status")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	pol := p.WS.Config.EffectivePolicy()
	for _, st := range []string{model.StatusProposed, model.StatusStale, model.StatusDrifted, model.StatusRejected} {
		es := p.Ledger.ByStatus(st)
		if len(es) == 0 {
			continue
		}
		fmt.Printf("%s (%d)\n", st, len(es))
		for _, e := range es {
			g := ""
			if st == model.StatusProposed {
				// Only a proposal waits on a gate: a stale link was accepted
				// and a rejected one decided, so neither gets one (issue #158).
				g = proposalGate(p, pol, e.MarkerID)
			}
			conf := ""
			if e.Conf > 0 {
				conf = fmt.Sprintf(" conf=%.2f", e.Conf)
			}
			fmt.Printf("  %s  %s  %s %s%s  by %s%s\n", e.MarkerID, e.Path, e.Relation, join(e.Targets), conf, e.Actor, g)
		}
	}
	return 0
}
