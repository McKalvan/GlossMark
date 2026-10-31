// @gloss implements gmi:AgentBenchmark scope=file

// glossbench scores what a coding agent did to a showcase repository against
// the task it was given (issue #271). bench/run.sh runs the agents and calls
// it; docs/BENCHMARK.md describes the method.
//
//	glossbench score  --task bench/tasks/<id>.yaml --base <pristine repo> --head <agent's tree> [--condition c] [--run n] [--agent a] [--format text|json] [-o file]
//	glossbench report <results dir>          # per-task and per-condition tables from the JSON scores
//	glossbench tasks  [--dir bench/tasks]    # validate the task files and list them
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mckalvan/glossmark/internal/bench"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "score":
		return cmdScore(args[1:])
	case "report":
		return cmdReport(args[1:])
	case "tasks":
		return cmdTasks(args[1:])
	case "help", "-h", "--help":
		usage()
		return 0
	}
	fmt.Fprintf(os.Stderr, "glossbench: unknown command %q\n", args[0])
	usage()
	return 2
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: glossbench <command> [flags]

  score   score an agent's tree against the task it was given (--task, --base, --head)
  report  per-task and per-condition tables from a directory of JSON scores
  tasks   validate and list the task files (--dir, default bench/tasks)

See docs/BENCHMARK.md.`)
}

func cmdScore(args []string) int {
	fs := flag.NewFlagSet("score", flag.ContinueOnError)
	task := fs.String("task", "", "task file (bench/tasks/<id>.yaml)")
	base := fs.String("base", "", "the repository as the agent found it")
	head := fs.String("head", "", "the repository as the agent left it")
	condition := fs.String("condition", "", "the condition the run was made under (bare, glossmark)")
	runN := fs.Int("run", 0, "the run number, when a task is run more than once")
	agent := fs.String("agent", "", "the agent that made the run (claude, cursor-agent, ...)")
	format := fs.String("format", "text", "text | json")
	out := fs.String("o", "", "also write the JSON score to this file (what glossbench report reads)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *task == "" || *base == "" || *head == "" {
		fmt.Fprintln(os.Stderr, "glossbench score: --task, --base and --head are required")
		return 2
	}
	t, err := bench.ReadTask(*task)
	if err != nil {
		return fail(err)
	}
	s, err := bench.ScoreDirs(t, *base, *head, bench.Options{Condition: *condition, Run: *runN, Agent: *agent})
	if err != nil {
		return fail(err)
	}
	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(s)
	} else {
		fmt.Print(bench.RenderText(s))
	}
	if *out != "" {
		b, _ := json.MarshalIndent(s, "", "  ")
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
			return fail(err)
		}
	}
	return 0
}

func cmdReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := "bench/results"
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	scores, err := bench.ReadScores(dir)
	if err != nil {
		return fail(err)
	}
	fmt.Print(bench.RenderMarkdown(scores))
	return 0
}

func cmdTasks(args []string) int {
	fs := flag.NewFlagSet("tasks", flag.ContinueOnError)
	dir := fs.String("dir", "bench/tasks", "directory of task files")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ts, err := bench.ReadTasks(*dir)
	if err != nil {
		return fail(err)
	}
	for _, t := range ts {
		fmt.Printf("%-40s %-28s %d pitfalls, %d assertions   %s\n", t.ID, t.Repo, len(t.Pitfalls), len(t.Effective()), t.Title)
	}
	return 0
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "glossbench:", err)
	return 1
}
