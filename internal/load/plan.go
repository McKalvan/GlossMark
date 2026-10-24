// @gloss represents gmi:PlanRecord scope=file

package load

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"

	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/version"
)

// PlanName is the plan record in the state directory (ADR 0034 section 4):
// what each ontology-first increment decided and why, kept beside the
// ledger. The curate-increment skill writes it; gloss reads and validates
// it, and the skill reads it first on a later increment.
const PlanName = "plan.jsonl"

// PlanLine is one record of the plan: a brief per increment, a decision
// per entry written or changed, a deviation per departure found at review.
type PlanLine struct {
	Type      string `json:"type"`
	Increment string `json:"increment,omitempty"` // the ticket URI the increment is for
	Date      string `json:"date,omitempty"`
	Actor     string `json:"actor,omitempty"`
	Owner     string `json:"owner,omitempty"`
	// Answers are a brief's interview answers, by step.
	Answers map[string]string `json:"answers,omitempty"`
	// Subject is what a decision or deviation is about: a CURIE, or a
	// declaration such as "arch:load planned realizes gmi:PlanRecord".
	Subject     string `json:"subject,omitempty"`
	Decision    string `json:"decision,omitempty"`
	Disposition string `json:"disposition,omitempty"`
	Reason      string `json:"reason,omitempty"`
	// Line is the line the record was read from.
	Line int `json:"-"`
}

// PlanDecisions are the values a decision line takes.
var PlanDecisions = []string{"added", "widened", "changed", "published", "criticality", "declined", "removed"}

// PlanDispositions are the values a deviation line takes.
var PlanDispositions = []string{"absorbed", "reopened", "reverted"}

var planDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// readPlan loads the plan record when the repository has one. A line that
// is not a valid record is an error (GM124) at its own line; the valid
// lines still load.
func (ws *Workspace) readPlan() {
	path := ws.StatePath(PlanName)
	f, err := os.Open(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			ws.Diags = append(ws.Diags, planDiag(1, err.Error()))
		}
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var l PlanLine
		dec := json.NewDecoder(strings.NewReader(text))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&l); err != nil {
			// The header carries format and generator, which a record does not.
			var h struct {
				Type      string `json:"type"`
				Format    int    `json:"format"`
				Generator string `json:"generator"`
			}
			if json.Unmarshal([]byte(text), &h) == nil && h.Type == "plan" {
				if h.Format != version.PlanFormat {
					ws.Diags = append(ws.Diags, planDiag(n, fmt.Sprintf("plan format %d is not supported by gloss %s (supported: %d)", h.Format, version.String(), version.PlanFormat)))
				}
				continue
			}
			ws.Diags = append(ws.Diags, planDiag(n, "not a plan record: "+err.Error()))
			continue
		}
		l.Line = n
		if msg := l.check(); msg != "" {
			ws.Diags = append(ws.Diags, planDiag(n, msg))
			continue
		}
		ws.Plan = append(ws.Plan, l)
	}
	if err := sc.Err(); err != nil {
		ws.Diags = append(ws.Diags, planDiag(n, err.Error()))
	}
}

// check returns what is wrong with a record, or "".
func (l PlanLine) check() string {
	switch l.Type {
	case "plan":
		return ""
	case "brief", "decision", "deviation":
	default:
		return fmt.Sprintf("type %q: use brief, decision or deviation", l.Type)
	}
	if strings.TrimSpace(l.Increment) == "" {
		return l.Type + " without increment (the ticket URI)"
	}
	if l.Actor == "" {
		return l.Type + " without actor"
	}
	if l.Date != "" && !planDate.MatchString(l.Date) {
		return fmt.Sprintf("date %q: use YYYY-MM-DD", l.Date)
	}
	switch l.Type {
	case "brief":
		if l.Date == "" {
			return "brief without date"
		}
		if len(l.Answers) == 0 {
			return "brief without answers"
		}
	case "decision":
		if l.Subject == "" || l.Reason == "" {
			return "decision needs a subject and a reason"
		}
		if !contains(PlanDecisions, l.Decision) {
			return fmt.Sprintf("decision %q: use one of %s", l.Decision, strings.Join(PlanDecisions, ", "))
		}
	case "deviation":
		if l.Subject == "" || l.Reason == "" {
			return "deviation needs a subject and a reason"
		}
		if !contains(PlanDispositions, l.Disposition) {
			return fmt.Sprintf("disposition %q: use one of %s", l.Disposition, strings.Join(PlanDispositions, ", "))
		}
	}
	return ""
}

func planDiag(line int, msg string) marker.Diagnostic {
	return marker.Diagnostic{Path: StateDir + "/" + PlanName, Line: line, Severity: "error", Code: "GM124", Msg: "plan record: " + msg}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
