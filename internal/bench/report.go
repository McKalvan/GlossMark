// @gloss implements gmi:AgentBenchmark scope=file

package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/gate"
)

// ReadScores reads every *.json score under dir (bench/run.sh writes one
// per task, condition and run).
func ReadScores(dir string) ([]*Score, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []*Score
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var s Score
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("%s: %v", p, err)
		}
		if s.Task == "" {
			continue
		}
		out = append(out, &s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no score files under %s", dir)
	}
	return out, nil
}

// Cell aggregates the runs of one task under one condition.
type Cell struct {
	Task, Condition string
	Runs            int
	Mistakes        Mistakes // summed over runs
	Hard            int      // runs the gate classified HARD
	Passed, Total   int
}

func (c Cell) mean(n int) float64 {
	if c.Runs == 0 {
		return 0
	}
	return float64(n) / float64(c.Runs)
}

// Aggregate groups scores by task and condition, in task then condition order.
func Aggregate(scores []*Score) []Cell {
	cells := map[string]*Cell{}
	for _, s := range scores {
		k := s.Task + "\x00" + s.Condition
		c := cells[k]
		if c == nil {
			c = &Cell{Task: s.Task, Condition: s.Condition}
			cells[k] = c
		}
		c.Runs++
		c.Mistakes.Rule += s.Mistakes.Rule
		c.Mistakes.Concept += s.Mistakes.Concept
		c.Mistakes.Naming += s.Mistakes.Naming
		c.Mistakes.Gate += s.Mistakes.Gate
		c.Mistakes.Total += s.Mistakes.Total
		if s.Gate == gate.HARD {
			c.Hard++
		}
		c.Passed += s.Passed
		c.Total += s.Total
	}
	out := make([]Cell, 0, len(cells))
	for _, c := range cells {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Task != out[j].Task {
			return out[i].Task < out[j].Task
		}
		return out[i].Condition < out[j].Condition
	})
	return out
}

// RenderMarkdown renders the per-task table and the per-condition totals,
// the shape docs/BENCHMARK.md quotes. Mistakes are means per run.
func RenderMarkdown(scores []*Score) string {
	cells := Aggregate(scores)
	var sb strings.Builder
	sb.WriteString("| Task | Condition | Runs | Mistakes/run | Rule | Concept | Naming | Gate expectation missed | HARD | Assertions passed |\n")
	sb.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, c := range cells {
		fmt.Fprintf(&sb, "| %s | %s | %d | %.2f | %.2f | %.2f | %.2f | %.2f | %d/%d | %d/%d |\n",
			c.Task, c.Condition, c.Runs, c.mean(c.Mistakes.Total), c.mean(c.Mistakes.Rule), c.mean(c.Mistakes.Concept),
			c.mean(c.Mistakes.Naming), c.mean(c.Mistakes.Gate), c.Hard, c.Runs, c.Passed, c.Total)
	}
	// Totals per condition.
	totals := map[string]*Cell{}
	var conds []string
	for _, c := range cells {
		t := totals[c.Condition]
		if t == nil {
			t = &Cell{Condition: c.Condition}
			totals[c.Condition] = t
			conds = append(conds, c.Condition)
		}
		t.Runs += c.Runs
		t.Mistakes.Rule += c.Mistakes.Rule
		t.Mistakes.Concept += c.Mistakes.Concept
		t.Mistakes.Naming += c.Mistakes.Naming
		t.Mistakes.Gate += c.Mistakes.Gate
		t.Mistakes.Total += c.Mistakes.Total
		t.Hard += c.Hard
		t.Passed += c.Passed
		t.Total += c.Total
	}
	sort.Strings(conds)
	sb.WriteString("\n| Condition | Runs | Mistakes/run | Rule | Concept | Naming | Gate expectation missed | HARD | Assertions passed |\n")
	sb.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, k := range conds {
		t := totals[k]
		fmt.Fprintf(&sb, "| %s | %d | %.2f | %.2f | %.2f | %.2f | %.2f | %d/%d | %d/%d |\n",
			t.Condition, t.Runs, t.mean(t.Mistakes.Total), t.mean(t.Mistakes.Rule), t.mean(t.Mistakes.Concept),
			t.mean(t.Mistakes.Naming), t.mean(t.Mistakes.Gate), t.Hard, t.Runs, t.Passed, t.Total)
	}
	return sb.String()
}

// RenderText renders one score for a terminal.
func RenderText(s *Score) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s", s.Task)
	if s.Condition != "" {
		fmt.Fprintf(&sb, " [%s]", s.Condition)
	}
	fmt.Fprintf(&sb, "   gate %s   mistakes %d (rule %d, concept %d, naming %d)   assertions %d/%d\n",
		s.Gate, s.Mistakes.Total, s.Mistakes.Rule, s.Mistakes.Concept, s.Mistakes.Naming, s.Passed, s.Total)
	for _, r := range s.Results {
		mark := "✓"
		if !r.OK {
			mark = "✗"
		}
		fmt.Fprintf(&sb, "  %s %-22s %s\n", mark, r.Kind, r.Msg)
		if !r.OK && r.Why != "" {
			fmt.Fprintf(&sb, "    → %s\n", r.Why)
		}
	}
	if len(s.NewConcepts) > 0 {
		sb.WriteString("New concepts:\n")
		for _, c := range s.NewConcepts {
			fmt.Fprintf(&sb, "  %s (%s", c.Curie, c.State)
			if c.DuplicateOf != "" {
				fmt.Fprintf(&sb, ", duplicates %s", c.DuplicateOf)
			}
			sb.WriteString(")\n")
		}
	}
	if len(s.NewLinks) > 0 {
		sb.WriteString("New markers:\n")
		for _, l := range s.NewLinks {
			fmt.Fprintf(&sb, "  %s\n", l)
		}
	}
	if len(s.HardChanges) > 0 {
		sb.WriteString("HARD changes:\n")
		for _, h := range s.HardChanges {
			fmt.Fprintf(&sb, "  %s\n", h)
		}
	}
	fmt.Fprintf(&sb, "Changed files: %d\n", len(s.ChangedFiles))
	return sb.String()
}
