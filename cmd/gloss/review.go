// @gloss implements gmi:ReviewEngine scope=file

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/review"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func init() {
	register(command{"review", "review session (experimental, ADR 0033): the queue of proposed and stale links, proposed concepts, drifted values and drift findings; interactive on a terminal, headless with --format json, show, apply and undo", cmdReview})
}

const reviewUsage = `usage:
  gloss review                          interactive review on a terminal
  gloss review --format json|text       the queue (--kind proposed-link,stale-link,... --path dir)
  gloss review show <id> [--format text] one item with its context, history and concepts
  gloss review apply [file|-]           apply decisions, one JSON object per line; prints one result per line
  gloss review undo [--force]           revert the last decision
Decisions: {"id":"m-…","action":"accept|reconfirm|reject|confirm|waive|propose","reason":"…","remove":true,"owner":"…","expires":"YYYY-MM-DD","recommend":true}
           {"action":"write-link","path":"…","line":12,"relation":"implements","target":"ns:Name"}
           {"action":"edit-doc","curie":"ns:Name","doc":"…"}
An actor named agent:… may take AUTO and SOFT actions; on HARD ones it records a recommendation (recommend: true) for a human.`

// newEngine builds a review engine for the workspace at root.
func newEngine(root string) (*review.Engine, error) {
	if root == "" {
		cwd, _ := os.Getwd()
		r, err := load.FindRoot(cwd)
		if err != nil {
			return nil, err
		}
		root = r
	}
	return &review.Engine{
		Root:  root,
		Actor: reviewer(root),
		Run:   invoke,
		Load: func() (*review.State, error) {
			p, err := buildGraph(root, false)
			if err != nil {
				return nil, err
			}
			pol := p.WS.Config.EffectivePolicy()
			rep := rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: pol, Now: time.Now()})
			rollup.AddDeprecatedUse(rep, p.Graph, pol, time.Now())
			return &review.State{WS: p.WS, Graph: p.Graph, Ledger: p.Ledger, Markers: p.Res.Markers, Report: rep}, nil
		},
	}, nil
}

var invokeMu sync.Mutex

// invoke runs a gloss command in this process with its output captured, so
// a review decision is exactly the command a person would type. Output goes
// to a temporary file rather than a pipe: a command can write more than a
// pipe buffers before it returns.
func invoke(args ...string) (code int, out string) {
	invokeMu.Lock()
	defer invokeMu.Unlock()
	f, err := os.CreateTemp("", "gloss-review-*")
	if err != nil {
		return 1, err.Error()
	}
	defer os.Remove(f.Name())
	defer f.Close()
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = f, f
	defer func() {
		os.Stdout, os.Stderr = stdout, stderr
		if r := recover(); r != nil {
			code, out = 1, fmt.Sprintf("gloss %s: %v", strings.Join(args, " "), r)
		}
	}()
	code = run(args)
	b, _ := os.ReadFile(f.Name())
	return code, string(b)
}

func cmdReview(args []string) int {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	fs, root := newFlags("review")
	format := fs.String("format", "", "json | text (default: interactive on a terminal, text otherwise)")
	kinds := fs.String("kind", "", "comma-separated item kinds: "+strings.Join(review.Kinds, ", "))
	paths := fs.String("path", "", "comma-separated files or directories the items must be under")
	force := fs.Bool("force", false, "undo: restore even when a file changed since the decision")
	fs.Usage = func() { fmt.Fprintln(fs.Output(), reviewUsage+"\n\nflags:"); fs.PrintDefaults() }
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	e, err := newEngine(*root)
	if err != nil {
		return fail(err)
	}
	switch sub {
	case "":
		if *format == "" {
			if isTerminal() {
				return interactiveReview(e)
			}
			*format = "text"
		}
		items, err := e.Queue()
		if err != nil {
			return fail(err)
		}
		items = review.Filter(items, splitList(*kinds), splitList(*paths))
		if *format == "json" {
			return printJSON(items)
		}
		printQueue(os.Stdout, items)
		return 0
	case "show":
		if fs.NArg() != 1 {
			fmt.Fprintln(os.Stderr, reviewUsage)
			return 2
		}
		d, err := e.Detail(fs.Arg(0))
		if err != nil {
			return fail(err)
		}
		if *format == "text" {
			printDetail(os.Stdout, d)
			return 0
		}
		return printJSON(d)
	case "apply":
		in := io.Reader(os.Stdin)
		if fs.NArg() == 1 && fs.Arg(0) != "-" {
			f, err := os.Open(fs.Arg(0))
			if err != nil {
				return fail(err)
			}
			defer f.Close()
			in = f
		}
		return applyDecisions(e, in, os.Stdout)
	case "undo":
		msg, err := e.Undo(*force)
		if err != nil {
			return fail(err)
		}
		fmt.Println("gloss review:", msg)
		return 0
	}
	fmt.Fprintf(os.Stderr, "gloss review: unknown subcommand %q\n%s\n", sub, reviewUsage)
	return 2
}

// applyDecisions reads one decision per line and writes one result per
// line; it exits 1 when any decision failed or was refused.
func applyDecisions(e *review.Engine, in io.Reader, out io.Writer) int {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(out)
	failed := 0
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var d review.Decision
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			enc.Encode(review.Result{Error: fmt.Sprintf("line %d: %v", n, err)})
			failed++
			continue
		}
		r := e.Decide(d)
		if !r.OK {
			failed++
		}
		enc.Encode(r)
	}
	if err := sc.Err(); err != nil {
		return fail(err)
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func printQueue(w io.Writer, items []review.Item) {
	if len(items) == 0 {
		fmt.Fprintln(w, "gloss review: nothing waits on review")
		return
	}
	kind := ""
	for _, it := range items {
		k := it.Kind
		if it.Finding != "" {
			k += " " + it.Finding
		}
		if k != kind {
			kind = k
			fmt.Fprintf(w, "%s\n", k)
		}
		var acts []string
		for _, a := range it.Actions {
			acts = append(acts, a.Name+"["+a.Gate+"]")
		}
		rec := ""
		if r := it.Recommendation; r != nil {
			rec = fmt.Sprintf("  (%s recommends %s)", r.By, r.Decision.Action)
		}
		fmt.Fprintf(w, "  %s  %s  %s%s\n", it.ID, it.Summary, strings.Join(acts, " "), rec)
	}
}

func printDetail(w io.Writer, d *review.Detail) {
	fmt.Fprintf(w, "%s  %s\n%s\n", d.ID, d.Kind, d.Summary)
	for _, a := range d.Actions {
		why := ""
		if len(a.Reasons) > 0 {
			why = ": " + strings.Join(a.Reasons, "; ")
		}
		fmt.Fprintf(w, "  %s [%s%s]\n", a.Name, a.Gate, why)
	}
	if r := d.Recommendation; r != nil {
		out := ""
		if r.Outdated {
			out = " (outdated: the item changed since)"
		}
		fmt.Fprintf(w, "recommendation: %s by %s at %s%s: %s\n", r.Decision.Action, r.By, r.At, out, r.Decision.Reason)
	}
	for _, c := range d.Concepts {
		if !c.Exists {
			fmt.Fprintf(w, "concept %s: does not exist\n", c.Curie)
			continue
		}
		fmt.Fprintf(w, "concept %s (%s, %s): %s\n", c.Curie, c.Kind, c.Tier, firstNonEmpty(c.Doc, "(no doc)"))
	}
	if len(d.Context) > 0 {
		fmt.Fprintln(w, "\n"+strings.Join(d.Context, "\n"))
	}
	if d.History != "" {
		fmt.Fprintf(w, "\nchanged since %s:\n%s\n", d.Since, d.History)
	}
}

// isTerminal reports whether stdin and stdout are both a terminal.
func isTerminal() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		st, err := f.Stat()
		if err != nil || st.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

// interactiveReview is replaced by the terminal UI; until then a terminal
// gets the text queue.
var interactiveReview = func(e *review.Engine) int {
	items, err := e.Queue()
	if err != nil {
		return fail(err)
	}
	printQueue(os.Stdout, items)
	return 0
}
