// @gloss implements gmi:ReviewEngine scope=file

// Package review is the engine behind gloss review (ADR 0033): the queue of
// everything that waits on a reviewer, the context each item needs, the
// actions it allows and who may take them, recommendations for decisions an
// agent may not take, and an undo journal for every decision. The headless
// CLI, the MCP tools and the terminal UI are front ends over one Engine.
package review

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// Item kinds.
const (
	KindProposedLink    = "proposed-link"
	KindStaleLink       = "stale-link"
	KindProposedConcept = "proposed-concept"
	KindValueDrift      = "value-drift"
	KindFinding         = "finding"
	// KindRecommendation is a recommendation with no other item to sit on:
	// an edit an agent may not make (a concept's doc), or one whose item no
	// longer waits on review.
	KindRecommendation = "recommendation"
)

// Kinds lists the item kinds in queue order.
var Kinds = []string{KindRecommendation, KindProposedLink, KindStaleLink, KindProposedConcept, KindValueDrift, KindFinding}

// Action names. Item actions act on a queue item; edit actions (write-link,
// edit-doc) name what they change in their own fields.
const (
	ActAccept    = "accept"    // a proposed link or concept
	ActReconfirm = "reconfirm" // a stale link: its current anchor becomes the baseline
	ActReject    = "reject"    // a link or a proposed concept, with a reason
	ActConfirm   = "confirm"   // a drifted published value, as the code has it
	ActWaive     = "waive"     // a drift finding, until a date
	ActPropose   = "propose"   // a rogue ns:NEW: target becomes a proposed concept
	ActWriteLink = "write-link"
	ActEditDoc   = "edit-doc"
	ActApply     = "apply"   // a recommended edit, as recommended
	ActDismiss   = "dismiss" // a recommendation, without deciding its item
	ActUndo      = "undo"
)

// Action is one thing a reviewer can do with an item, the gate that decision
// carries and whether an agent may take it rather than only recommend it.
type Action struct {
	Name     string   `json:"name"`
	Gate     string   `json:"gate"`
	Reasons  []string `json:"reasons,omitempty"`
	AgentMay bool     `json:"agent_may"`
	// Params are the decision fields the action reads (reason, remove, ...).
	Params []string `json:"params,omitempty"`
}

// Item is one entry in the review queue.
type Item struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Finding  string   `json:"finding,omitempty"` // the drift finding kind, for KindFinding
	Severity string   `json:"severity,omitempty"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	Relation string   `json:"relation,omitempty"`
	Targets  []string `json:"targets,omitempty"`
	Concept  string   `json:"concept,omitempty"`
	Boundary string   `json:"boundary,omitempty"`
	Summary  string   `json:"summary"`
	Actor    string   `json:"actor,omitempty"`
	Conf     float64  `json:"conf,omitempty"`
	// Since is when the item was last reviewed, or created: what changed in
	// its file after that is what a stale link's reviewer reads.
	Since          string          `json:"since,omitempty"`
	Gate           string          `json:"gate"` // the gate of its first action
	Actions        []Action        `json:"actions"`
	Recommendation *Recommendation `json:"recommendation,omitempty"`
	// Basis fingerprints what the item is about, so a recommendation made
	// on an earlier version reads as outdated.
	Basis string `json:"-"`
}

// Action returns the named action, or nil.
func (it *Item) Action(name string) *Action {
	for i := range it.Actions {
		if it.Actions[i].Name == name {
			return &it.Actions[i]
		}
	}
	return nil
}

// Decision is one decision or edit, the line format of gloss review apply and
// the arguments of the review_decide MCP tool.
type Decision struct {
	ID     string `json:"id,omitempty"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
	// Remove: reject also deletes the marker line (or the concept entry).
	Remove bool `json:"remove,omitempty"`
	// Force: confirm a value whose line does not hold it; undo over a file
	// that changed since.
	Force bool `json:"force,omitempty"`
	// Waive.
	Owner   string `json:"owner,omitempty"`
	Expires string `json:"expires,omitempty"`
	// Propose and edit-doc.
	Kind  string `json:"kind,omitempty"`
	Doc   string `json:"doc,omitempty"`
	Curie string `json:"curie,omitempty"`
	// Write-link.
	Path     string  `json:"path,omitempty"`
	Line     int     `json:"line,omitempty"`
	Relation string  `json:"relation,omitempty"`
	Target   string  `json:"target,omitempty"`
	Scope    string  `json:"scope,omitempty"`
	Conf     float64 `json:"conf,omitempty"`
	Note     string  `json:"note,omitempty"`
	// Accepted writes an accepted marker, not a proposal: a person's link.
	Accepted bool `json:"accepted,omitempty"`
	// Recommend records the decision for a human instead of taking it.
	Recommend bool `json:"recommend,omitempty"`
}

// Result is the outcome of one Decision.
type Result struct {
	ID          string `json:"id,omitempty"`
	Action      string `json:"action"`
	OK          bool   `json:"ok"`
	Recommended bool   `json:"recommended,omitempty"`
	Message     string `json:"message,omitempty"`
	// Undo names the journal entry that reverts this decision.
	Undo  string `json:"undo,omitempty"`
	Error string `json:"error,omitempty"`
}

// State is what one build of the workspace gives the engine.
type State struct {
	WS      *load.Workspace
	Graph   *graph.Graph
	Ledger  *ledger.Ledger
	Markers []extract.FoundMarker
	Report  *rollup.Report
}

// Engine runs a review session against one workspace.
type Engine struct {
	Root  string
	Actor string
	// Load builds the workspace afresh.
	Load func() (*State, error)
	// Run runs a gloss command in-process and returns its exit code and
	// output; decisions reuse the commands so they cannot disagree.
	Run func(args ...string) (int, string)
	Now func() time.Time

	state *State
}

// ErrNeedsHuman is returned when an agent takes a HARD action.
var ErrNeedsHuman = errors.New("HARD: a human decides this; an agent can only recommend it (recommend: true)")

// IsAgent reports whether an actor is an agent (agent:<name>).
func IsAgent(actor string) bool { return strings.HasPrefix(actor, "agent:") }

// State returns the current build, loading it when needed.
func (e *Engine) State() (*State, error) {
	if e.state == nil {
		st, err := e.Load()
		if err != nil {
			return nil, err
		}
		e.state = st
	}
	return e.state, nil
}

// Invalidate drops the cached build; the next call reloads.
func (e *Engine) Invalidate() { e.state = nil }

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

// Queue lists every item that waits on a reviewer, with recommendations
// attached, in Kinds order.
func (e *Engine) Queue() ([]Item, error) {
	st, err := e.State()
	if err != nil {
		return nil, err
	}
	items := buildQueue(st)
	recs, err := readRecommendations(st.WS)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i := range items {
		seen[items[i].ID] = true
		if r, ok := recs[items[i].ID]; ok {
			r.Outdated = r.Basis != items[i].Basis
			items[i].Recommendation = &r
			items[i].Actions = append(items[i].Actions, action(ActDismiss, gate.AUTO, nil))
		}
	}
	var extra []Item
	for id, r := range recs {
		if !seen[id] {
			extra = append(extra, recommendationItem(st, r))
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].ID < extra[j].ID })
	return append(extra, items...), nil
}

// Find returns the queue item with this id.
func (e *Engine) Find(id string) (*Item, error) {
	items, err := e.Queue()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("no review item %q (gloss review --format json lists them)", id)
}

// Decide applies one decision and returns its result; a refused or failed
// decision is a Result with an Error, not a Go error, so a batch goes on.
func (e *Engine) Decide(d Decision) Result {
	res := Result{ID: d.ID, Action: d.Action}
	msg, undo, rec, err := e.decide(d)
	res.Message, res.Undo, res.Recommended = msg, undo, rec
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	return res
}

func (e *Engine) decide(d Decision) (msg, undo string, recommended bool, err error) {
	if d.Action == ActUndo {
		m, err := e.Undo(d.Force)
		return m, "", false, err
	}
	st, err := e.State()
	if err != nil {
		return "", "", false, err
	}
	var it *Item
	var act *Action
	switch d.Action {
	case ActWriteLink:
		act, err = writeLinkAction(st, d, e.Actor)
	case ActEditDoc:
		act, err = editDocAction(st, d)
	default:
		if it, err = e.Find(d.ID); err == nil {
			if act = it.Action(d.Action); act == nil {
				err = fmt.Errorf("%s: %s takes %s, not %s", d.ID, it.Kind, actionNames(it.Actions), d.Action)
			}
		}
	}
	if err != nil {
		return "", "", false, err
	}
	if err := checkParams(d); err != nil {
		return "", "", false, err
	}
	if d.Recommend {
		if it == nil {
			if d.Action != ActEditDoc {
				return "", "", false, fmt.Errorf("%s: an agent writes a link as a proposal; only a queue item or a doc edit takes a recommendation", d.Action)
			}
			it = &Item{ID: editDocID(d.Curie), Basis: docBasis(st, d.Curie)}
			d.ID = it.ID
		}
		if err := e.recommend(st, it, d); err != nil {
			return "", "", false, err
		}
		return fmt.Sprintf("recommended %s on %s for a human to decide", d.Action, d.ID), "", true, nil
	}
	if IsAgent(e.Actor) && !act.AgentMay {
		return "", "", false, fmt.Errorf("%s %s: %w", d.Action, firstNonEmpty(d.ID, d.Curie, d.Path), ErrNeedsHuman)
	}
	snap, err := takeSnapshot(st, d)
	if err != nil {
		return "", "", false, err
	}
	msg, err = e.execute(st, it, d)
	if err == nil {
		if code, out := e.Run("build", "-q", "--root", e.Root); code != 0 {
			err = fmt.Errorf("gloss build after %s: %s", d.Action, strings.TrimSpace(out))
		}
	}
	e.Invalidate()
	if err == nil && it != nil {
		err = dropRecommendation(st.WS, it.ID)
	}
	desc := strings.TrimSpace(d.Action + " " + firstNonEmpty(d.ID, d.Curie, d.Path))
	id, jerr := snap.journal(e.Root, desc, e.Actor, e.now())
	if err != nil {
		// A command that failed half way is reverted at once, so a
		// refused decision leaves the tree as it was.
		if id != "" {
			_, _ = e.Undo(true)
		}
		return msg, "", false, err
	}
	if jerr != nil {
		return msg, "", false, fmt.Errorf("decision applied, but the undo journal failed: %w", jerr)
	}
	return msg, id, false, nil
}

func (e *Engine) execute(st *State, it *Item, d Decision) (string, error) {
	run := func(args ...string) (string, error) {
		code, out := e.Run(append(args, "--root", e.Root)...)
		out = strings.TrimSpace(out)
		if code != 0 {
			return out, fmt.Errorf("gloss %s: %s", args[0], firstNonEmpty(out, fmt.Sprintf("exit %d", code)))
		}
		return out, nil
	}
	withRemove := func(args []string) []string {
		if d.Remove {
			return append(args, "--remove")
		}
		return args
	}
	switch d.Action {
	case ActWriteLink:
		return writeLink(st, d)
	case ActEditDoc:
		return editDoc(st, d)
	case ActDismiss:
		return "dismissed the recommendation on " + it.ID, dropRecommendation(st.WS, it.ID)
	case ActApply:
		rd := it.Recommendation.Decision
		if rd.Action != ActEditDoc {
			return "", fmt.Errorf("%s: apply takes a recommended edit; take the recommended %s on its item", it.ID, rd.Action)
		}
		return editDoc(st, rd)
	}
	switch it.Kind {
	case KindProposedLink, KindStaleLink:
		switch d.Action {
		case ActAccept, ActReconfirm:
			return run("accept", it.ID)
		case ActReject:
			return run(withRemove([]string{"reject", it.ID, "--reason", d.Reason})...)
		}
	case KindProposedConcept:
		switch d.Action {
		case ActAccept:
			return run("accept", "--concept", it.Concept)
		case ActReject:
			return run(withRemove([]string{"reject", "--concept", it.Concept, "--reason", d.Reason})...)
		}
	case KindValueDrift:
		args := []string{"accept", "--value", it.Concept}
		if d.Force {
			args = append(args, "--force")
		}
		return run(args...)
	case KindFinding:
		switch d.Action {
		case ActAccept:
			if it.Finding == rollup.KindPlannedClosed && it.Concept != "" {
				return run("accept", "--planned", it.Concept)
			}
		case ActWaive:
			return addWaiver(st.WS, strings.TrimPrefix(it.ID, "finding:"), d)
		case ActPropose:
			args := []string{"propose", "concept", rollup.StripNewCurie(it.Concept), "--kind", firstNonEmpty(d.Kind, kindForRelation(it.Relation))}
			if d.Doc != "" {
				args = append(args, "--doc", d.Doc)
			}
			return run(args...)
		}
	}
	return "", fmt.Errorf("%s: no way to %s a %s", it.ID, d.Action, it.Kind)
}

func checkParams(d Decision) error {
	switch d.Action {
	case ActReject:
		if strings.TrimSpace(d.Reason) == "" {
			return errors.New("reject needs a reason")
		}
	case ActWaive:
		if d.Owner == "" || d.Expires == "" {
			return errors.New("waive needs an owner and an expires date (YYYY-MM-DD)")
		}
		if _, err := time.Parse("2006-01-02", d.Expires); err != nil {
			return fmt.Errorf("waive: expires %q is not a YYYY-MM-DD date", d.Expires)
		}
	case ActEditDoc:
		if d.Curie == "" || strings.TrimSpace(d.Doc) == "" {
			return errors.New("edit-doc needs a curie and a doc")
		}
	case ActWriteLink:
		if d.Path == "" || d.Relation == "" || d.Target == "" {
			return errors.New("write-link needs a path, a relation and a target")
		}
	}
	return nil
}

func actionNames(as []Action) string {
	var n []string
	for _, a := range as {
		n = append(n, a.Name)
	}
	return strings.Join(n, ", ")
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// kindForRelation is the concept kind gloss propose --from-new gives a
// ns:NEW: target, by the relation that named it.
func kindForRelation(rel string) string {
	switch rel {
	case "represents":
		return "entity"
	case "provisions", "configures":
		return "resource"
	case "deploys":
		return "system-type"
	case "publishes", "subscribes":
		return "event"
	}
	return "process"
}

// stateFile is a path under .glossmark/.
func stateFile(ws *load.Workspace, name string) string { return ws.StatePath(name) }

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func rel(ws *load.Workspace, path string) string {
	if r, err := filepath.Rel(ws.Root, path); err == nil {
		return filepath.ToSlash(r)
	}
	return path
}
