// @gloss implements gmi:GlossCli scope=file

package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// helpGroups are the sections of 'gloss -h' in order: the core loop first,
// the rarely used commands last (#231). Every registered command is in
// exactly one (TestHelpCoversCommands).
var helpGroups = []struct {
	name     string
	commands []string
}{
	{"Start", []string{"init", "doctor", "strip"}},
	{"Daily", []string{"check", "build", "drift", "coverage", "review", "view"}},
	{"Pull requests", []string{"diff", "impact", "accept", "reject", "status"}},
	{"Agents", []string{"hook", "context", "mcp", "agents"}},
	{"Ontology", []string{"propose", "suggest", "harvest", "import", "criticality"}},
	{"Advanced", []string{"export", "history", "landscape", "lock", "publish", "slice", "version"}},
}

// helpEntry is what 'gloss -h' and 'gloss <command> -h' say about a command
// beyond its registered description, which is the -h paragraph (#232).
type helpEntry struct {
	line     string   // the 'gloss -h' line; at most 64 characters so it fits 80 columns
	usage    []string // synopsis lines, without the leading "gloss "
	more     string   // optional second paragraph
	examples []string // "command  # comment"
	docs     string   // relative path under the repository
	noFlags  bool     // the command takes no flags (-h is answered before it runs)
}

// help is keyed by command name, in the order of helpGroups.
var help = map[string]helpEntry{
	// Start
	"init": {line: "scaffold a repository: config, manifests, CI, hook, first graph",
		usage: []string{"init [--dry-run] [--name n] [--core prefix] [--import dir]..."},
		examples: []string{
			"gloss init --dry-run  # print the plan, write nothing",
			"gloss init --core lib  # name the core namespace yourself",
			"gloss init --import ../platform  # import a sibling's core, not own one",
		},
		docs: "docs/QUICKSTART.md#0-scaffold"},
	"doctor": {line: "check the setup end to end; one line per check with its fix",
		usage:    []string{"doctor [--format text|json]"},
		examples: []string{"gloss doctor", "gloss doctor --format json"},
		docs:     "docs/QUICKSTART.md#6-run-the-tooling"},
	"strip": {line: "leave: export the graph, then remove every marker and file",
		usage: []string{"strip [--keep-ontology] [--dry-run] [--export dir|-]"},
		more:  "The reverse of gloss init. It writes glossmark-export/ (the graph as JSON Lines, JSON-LD and SQLite, the ledger as it is and as PROV-O, the ontology, the manifests), then removes every @gloss and @gloss? marker in every comment style the scanner reads, the JSON sidecars, every .gloss.yaml manifest, .glossmark/, glossmark.lock and, unless --keep-ontology, ontology/ and glossmark.yaml. exclude: and generated: are honoured. Afterwards git diff shows only marker lines and GlossMark files gone; the hook, MCP, CI and Makefile entries it cannot edit are listed to remove by hand.",
		examples: []string{
			"gloss strip --dry-run  # what would go, and what stays for you",
			"gloss strip  # export, then remove; review with git diff",
			"gloss strip --keep-ontology  # keep the vocabulary, drop the markers",
		},
		docs: "docs/LEAVING.md"},

	// Daily
	"check": {line: "lint markers, manifests and ontologies",
		usage: []string{"check [--strict]", "check --explain [--format text|json]"},
		more:  "Errors fail the command; warnings fail it only with --strict. Run it before gloss build; the pre-commit hook runs both.",
		examples: []string{
			"gloss check  # before a commit",
			"gloss check --strict  # in CI: warnings fail too",
			"gloss check --explain  # why each manifest allows what it does",
		},
		docs: "docs/QUICKSTART.md#6-run-the-tooling"},
	"build": {line: "write .glossmark/graph.jsonl and the ledger",
		usage: []string{"build [-q] [-o path] [--no-ledger]", "build --check"},
		examples: []string{
			"gloss build  # after editing markers; commit .glossmark/",
			"gloss build --check  # in CI: fail when the committed graph is stale",
		},
		docs: "docs/GRAPH.md"},
	"drift": {line: "compare declared boundaries with what the code does",
		usage: []string{"drift [--format text|json|markdown] [--fail-on kinds] [--ids]"},
		more:  "Violations (code outside what a manifest declares) and gaps (declared, never reached) fail by default; orphans, rogues and stale links are reported. Waive a finding under policy.waivers by its id (--ids).",
		examples: []string{
			"gloss drift  # the report; non-zero exit on a failing kind",
			"gloss drift --fail-on violation  # fail only on violations",
			"gloss drift --ids  # show each finding's id, to waive it",
		},
		docs: "docs/ADVANCED.md#finding-kinds-gloss-drift"},
	"coverage": {line: "share of concepts with code and a test; --fail-under N",
		usage: []string{"coverage [--fail-under N|current] [--changed-only] [--base ref]"},
		examples: []string{
			"gloss coverage --fail-under 80",
			"gloss coverage --fail-under current --changed-only  # ratchet against the base",
		},
		docs: "docs/QUICKSTART.md#6-run-the-tooling"},
	"review": {line: "work the queue of proposed links, concepts and findings",
		examples: []string{
			"gloss review  # interactive on a terminal",
			"gloss review --format json  # the queue, for an agent or a script",
			"gloss review show m-1a2b  # one item with its context and history",
		},
		docs: "docs/adr/0033-review-sessions.md"},
	"view": {line: "render the static HTML viewer of every declared view",
		usage: []string{"view [-o path] [--inline] [--json] [--diff impact.json] [--history]"},
		examples: []string{
			"gloss view  # .glossmark/viewer.html, no server needed",
			"gloss view --diff d.json  # overlay gloss diff --format json -o d.json",
			"gloss view --history  # step through every commit",
		},
		docs: "docs/QUICKSTART.md#6-run-the-tooling"},

	// Pull requests
	"diff": {line: "gate a change against a base: AUTO/SOFT/HARD per change",
		usage: []string{"diff [--base ref] [--format text|markdown|json] [--fail-on class]"},
		more:  "AUTO changes need no review, SOFT ones a reviewer's glance and HARD ones a decision; HARD fails the command by default. --format markdown is the PR comment.",
		examples: []string{
			"gloss diff  # against origin/main",
			"gloss diff --base main --format markdown -o comment.md",
			"gloss diff --baseline  # first adoption: counts per boundary",
		},
		docs: "docs/ADVANCED.md#gates-gloss-diff"},
	"impact": {line: "what changed files touch: concepts, seam, tests, blast radius",
		usage: []string{"impact [--base ref] [--format text|json] [path]..."},
		examples: []string{
			"gloss impact --base origin/main",
			"gloss impact services/circulation/loans/renew.py",
		},
		docs: "docs/QUICKSTART.md#agent-experience"},
	"accept": {line: "accept proposed links (@gloss? becomes @gloss) or concepts",
		usage: []string{
			"accept <marker-id|path:line>... | --all | --policy",
			"accept --stale [path]...",
			"accept --concept|--planned|--value <ns:Name>...",
		},
		examples: []string{
			"gloss accept --policy  # every proposal the policy classifies SOFT",
			"gloss accept app/loans.py:12",
			"gloss accept --concept circ:RenewalHandler",
		},
		docs: "docs/QUICKSTART.md#review-workflow"},
	"reject": {line: "reject a proposed link or concept with a reason",
		usage: []string{
			"reject <marker-id|path:line>... --reason '...' [--remove]",
			"reject --concept <ns:Name>... --reason '...' [--remove]",
		},
		examples: []string{
			"gloss reject app/loans.py:12 --reason \"wrong concept\" --remove",
			"gloss reject --concept circ:RenewalHandler --reason \"duplicate\"",
		},
		docs: "docs/QUICKSTART.md#review-workflow"},
	"status": {line: "list proposed, stale and rejected links with their gate",
		usage:    []string{"status"},
		examples: []string{"gloss status"},
		docs:     "docs/QUICKSTART.md#review-workflow"},

	// Agents
	"hook": {line: "install the agent SessionStart hook and MCP server",
		usage: []string{"hook install [--skill] [--cursor] [--actor agent:name]"},
		examples: []string{
			"gloss hook install  # Claude Code: .claude/settings.json, .mcp.json",
			"gloss hook install --skill --cursor",
		},
		docs: "docs/QUICKSTART.md#agent-experience"},
	"context": {line: "session-start briefing for an agent in a directory or feature",
		usage: []string{"context [--dir dir | --concept curie...] [--changed] [--budget n]"},
		examples: []string{
			"gloss context --dir services/circulation",
			"gloss context --changed --budget 2000",
		},
		docs: "docs/QUICKSTART.md#agent-experience"},
	"mcp": {line: "serve the graph and the review tools to agents over MCP",
		usage:    []string{"mcp"},
		examples: []string{"gloss mcp  # stdio; gloss hook install registers it"},
		docs:     "docs/QUICKSTART.md#agent-experience"},
	"agents": {line: "print the AGENTS.md vocabulary snippet",
		usage: []string{"agents [--dir dir] [--write file]"},
		examples: []string{
			"gloss agents --dir services/circulation",
			"gloss agents --write AGENTS.md  # between the gloss:begin/end markers",
		},
		docs: "docs/QUICKSTART.md#agent-experience"},

	// Ontology
	"propose": {line: "add a proposed concept, or lift ns:NEW:Name markers",
		usage: []string{
			"propose concept <ns:Name> --kind <kind> [--realizes ns:X] [flags]",
			"propose --from-new",
		},
		examples: []string{
			"gloss propose concept circ:Renewer --kind process --realizes lib:Renewal",
			"gloss propose --from-new",
		},
		docs: "docs/QUICKSTART.md#agent-experience"},
	"suggest": {line: "draft @gloss? markers for unmarked declarations",
		examples: []string{
			"gloss suggest services/  # a patch to review",
			"gloss suggest --write  # insert the proposals",
			"gloss suggest --concepts  # propose new concepts from clusters",
		},
		docs: "docs/MARKING.md"},
	"harvest": {line: "collect domain-vocabulary evidence before an ontology exists",
		usage: []string{"harvest [--cluster] [--format jsonl|text] [--about term]"},
		examples: []string{
			"gloss harvest --cluster --format text  # candidate concepts",
			"gloss harvest --about loan --format text",
		},
		docs: "docs/adr/0025-deriving-the-ontology.md"},
	"import": {line: "import a glossary or enums into an owned namespace",
		usage: []string{"import --from markdown-glossary|enums <file> --namespace <ns>"},
		examples: []string{
			"gloss import --from markdown-glossary GLOSSARY.md --namespace lib --dry-run",
			"gloss import --from enums app/models.py --namespace lib --values",
		},
		docs: "docs/FILE-FORMATS.md"},
	"criticality": {line: "suggest a first criticality classification",
		usage:    []string{"criticality suggest [--write]"},
		examples: []string{"gloss criticality suggest  # a diff to review", "gloss criticality suggest --write"},
		docs:     "docs/GOVERNANCE.md"},

	// Advanced
	"export": {line: "export the graph as SQLite, PROV-O or per-concept status",
		usage: []string{"export --sqlite file | --prov file | --status [-o file]"},
		examples: []string{
			"gloss export --sqlite graph.db",
			"gloss export --status -o status.json",
		},
		docs: "docs/GRAPH.md#querying"},
	"history": {line: "when each concept, boundary and link changed, by gate",
		usage: []string{"history [curie|path]... [--range a..b] [--step mode] [--all]"},
		examples: []string{
			"gloss history gm:Increment",
			"gloss history --range v0.2.0..HEAD --step version",
		},
		docs: "docs/ADVANCED.md#history-gloss-history"},
	"landscape": {line: "union several repositories' graphs; cross-repo drift",
		usage: []string{"landscape [--format text|json|markdown] [--view file] <root>..."},
		examples: []string{
			"gloss landscape ../repo-a ../repo-b",
			"gloss landscape --view landscape.html ../repo-a ../repo-b",
		},
		docs: "docs/QUICKSTART.md#6-run-the-tooling"},
	"lock": {line: "pin every import in glossmark.lock and vendor it",
		usage:    []string{"lock [--update] [--allow-out-of-range]"},
		examples: []string{"gloss lock", "gloss lock --update  # move to what upstream has now"},
		docs:     "docs/adr/0002-lock-pins-imports.md"},
	"publish": {line: "write per-namespace ontology.jsonld for importers",
		usage:    []string{"publish [-o dir]"},
		examples: []string{"gloss publish  # dist/glossmark/<ns>/"},
		docs:     "docs/VERSIONING.md#namespaces"},
	"slice": {line: "everything relevant to one feature, concept or path",
		usage: []string{"slice <curie|path>... [--follow rels] [--format text|json|md]"},
		examples: []string{
			"gloss slice lib:Checkout",
			"gloss slice services/circulation --format md",
		},
		docs: "docs/ADVANCED.md#feature-slices-gloss-slice"},
	"version": {line: "print the tool, grammar and file-format versions",
		usage:    []string{"version"},
		examples: []string{"gloss version", "gloss --version"},
		docs:     "docs/VERSIONING.md", noFlags: true},
}

// usage prints 'gloss -h': the commands grouped, one short line each.
func usage(w io.Writer) {
	fmt.Fprintln(w, "gloss — GlossMark: comment-native code ontology")
	fmt.Fprintln(w, "\nUsage: gloss <command> [flags]")
	width := 0
	for _, c := range commands {
		width = max(width, len(c.name))
	}
	for _, g := range helpGroups {
		fmt.Fprintf(w, "\n%s:\n", g.name)
		for _, name := range g.commands {
			fmt.Fprintf(w, "  %-*s  %s\n", width, name, help[name].line)
		}
	}
	fmt.Fprintln(w, "\nRun 'gloss <command> -h' for details and examples. Docs: docs/QUICKSTART.md")
	fmt.Fprintln(w, "Set GLOSS_ACTOR (human:alice, agent:claude-code) for ledger attribution.")
}

// commandHelp prints 'gloss <name> -h' above the flags: synopsis, the
// registered description, examples and a docs link. own is a usage the
// command set itself, which then supplies the synopsis and the flags.
func commandHelp(w io.Writer, fs *flag.FlagSet, name string, own func()) {
	h := help[name]
	if own == nil {
		for i, u := range h.usage {
			prefix := "usage: "
			if i > 0 {
				prefix = "       "
			}
			fmt.Fprintf(w, "%sgloss %s\n", prefix, u)
		}
		if len(h.usage) == 0 {
			fmt.Fprintf(w, "usage: gloss %s [flags]\n", name)
		}
		fmt.Fprintln(w)
	}
	for _, c := range commands {
		if c.name == name {
			fmt.Fprintln(w, wrap(sentence(c.short), 78))
		}
	}
	if h.more != "" {
		fmt.Fprintln(w, "\n"+wrap(h.more, 78))
	}
	if len(h.examples) > 0 {
		// Line the comments up, unless a command is too long to.
		col := 0
		for _, e := range h.examples {
			if cmd, _, ok := strings.Cut(e, "# "); ok && len(strings.TrimSpace(cmd)) <= 40 {
				col = max(col, len(strings.TrimSpace(cmd)))
			}
		}
		fmt.Fprintln(w, "\nExamples:")
		for _, e := range h.examples {
			cmd, comment, ok := strings.Cut(e, "# ")
			if !ok {
				fmt.Fprintln(w, "  "+e)
				continue
			}
			fmt.Fprintf(w, "  %-*s  # %s\n", col, strings.TrimSpace(cmd), comment)
		}
	}
	if h.docs != "" {
		fmt.Fprintln(w, "\nDocs: "+h.docs)
	}
	if own != nil {
		fmt.Fprintln(w)
		own()
		return
	}
	if fs != nil {
		fmt.Fprintln(w, "\nFlags:")
		fs.PrintDefaults()
	}
}

// sentence capitalizes a registered description and ends it with a period.
func sentence(s string) string {
	if s == "" || strings.HasPrefix(s, "gloss ") {
		return strings.TrimSuffix(s, ".") + "."
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return strings.TrimSuffix(string(r), ".") + "."
}

// wrap breaks s into lines of at most width runes at spaces.
func wrap(s string, width int) string {
	var b strings.Builder
	n := 0
	for _, word := range strings.Fields(s) {
		l := len([]rune(word))
		if n > 0 && n+1+l > width {
			b.WriteByte('\n')
			n = 0
		} else if n > 0 {
			b.WriteByte(' ')
			n++
		}
		b.WriteString(word)
		n += l
	}
	return b.String()
}
