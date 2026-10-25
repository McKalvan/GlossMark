// @gloss verifies gmi:Gate scope=file

package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/mcp"
)

// TestHistory: gloss history reads each commit's committed graph, reports
// the gate of each step newest first, marks an ontology edit committed
// without its graph unbuilt, leaves out commits that touch no ontology,
// follows a subject through replaced-by, and folds steps between version
// bumps (ADR 0040).
func TestHistory(t *testing.T) {
	root, write, _ := historyRepo(t)
	run := func(args ...string) historyResult {
		t.Helper()
		out := captureFunc(t, func() { cmdHistory(append([]string{"--root", root, "--format", "json"}, args...)) })
		var h historyResult
		if err := json.Unmarshal(out, &h); err != nil {
			t.Fatalf("history json: %v\n%s", err, out)
		}
		return h
	}
	titles := func(h historyResult) []string {
		var ts []string
		for _, st := range h.Steps {
			ts = append(ts, st.Title)
		}
		return ts
	}
	kinds := func(st historyStep) []string {
		var ks []string
		for _, ch := range st.Impact.Changes {
			ks = append(ks, ch.Kind+" "+ch.Subject)
		}
		return ks
	}

	h := run("--all")
	for _, st := range h.Steps {
		if st.Status == "skipped" {
			t.Errorf("%s: skipped (%s)", st.Title, st.Reason)
		}
	}
	if got, want := titles(h), []string{"edit without build", "retire Doing", "add Other", "adopt"}; !slices.Equal(got, want) {
		t.Fatalf("steps %v, want %v", got, want)
	}
	if st := h.Steps[0]; st.Status != "unbuilt" || st.Impact == nil {
		t.Errorf("edit without build: status %q impact %v, want unbuilt and classified", st.Status, st.Impact)
	}
	if ks := kinds(h.Steps[2]); !slices.Contains(ks, "concept-added z:Other") {
		t.Errorf("add Other changes %v, want concept-added z:Other", ks)
	}
	if !slices.Contains(h.Lineage, "z:Doing → z:Other") {
		t.Errorf("lineage %v, want z:Doing → z:Other", h.Lineage)
	}

	// Following z:Other reaches z:Doing through replaced-by.
	f := run("--all", "z:Other")
	if got := titles(f); !slices.Contains(got, "retire Doing") || !slices.Contains(got, "add Other") {
		t.Errorf("following z:Other: steps %v, want retire Doing and add Other", got)
	}
	for _, st := range f.Steps {
		for _, ch := range st.Impact.Changes {
			if ch.Subject != "z:Doing" && ch.Subject != "z:Other" && ch.Object != "z:Doing" && ch.Object != "z:Other" && ch.Boundary == "" {
				t.Errorf("following z:Other: %s kept %s %s", st.Title, ch.Kind, ch.Subject)
			}
		}
	}

	// --range bounds the walk; --verify rebuilds the trees and agrees.
	if got, want := titles(run("--all", "--range", "HEAD~3..HEAD")), []string{"edit without build", "retire Doing"}; !slices.Equal(got, want) {
		t.Errorf("range steps %v, want %v", got, want)
	}
	vr := run("--all", "--verify", "--range", "HEAD~3..HEAD")
	if vr.Steps[1].Status != "verified" || !slices.Equal(kinds(vr.Steps[1]), kinds(h.Steps[1])) {
		t.Errorf("verify: %s %v, want verified %v", vr.Steps[1].Status, kinds(vr.Steps[1]), kinds(h.Steps[1]))
	}

	// One step per version bump; the edit after the last bump is an open chapter.
	v := run("--all", "--step", "version")
	if got, want := titles(v), []string{"edit without build", "retire Doing", "add Other", "adopt"}; !slices.Equal(got, want) {
		t.Errorf("version steps %v, want %v", got, want)
	}

	// history.step in glossmark.yaml sets the default; --step overrides it.
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\nhistory: { step: first-parent }\n")
	if got := run().Step; got != "first-parent" {
		t.Errorf("step %q, want first-parent from glossmark.yaml", got)
	}
	if got := run("--step", "commit").Step; got != "commit" {
		t.Errorf("step %q, want commit from --step", got)
	}
}

// historyRepo is a repository of six commits: one before adoption, adopt
// (z 1.0.0), add Other (1.1.0), retire Doing in favour of Other (1.2.0), a
// code-only commit, and an ontology edit committed without rebuilding the
// graph.
func historyRepo(t *testing.T) (string, func(rel, content string), func(args ...string)) {
	t.Helper()
	root, write, git := gitRepo(t)
	commit := func(msg string, build bool) {
		t.Helper()
		if build {
			if code := cmdBuild([]string{"--root", root, "-q"}); code != 0 {
				t.Fatalf("gloss build exited %d", code)
			}
		}
		git("add", ".")
		git("commit", "-q", "-m", msg)
	}
	// Before adoption: a manifest-named file but no glossmark.yaml.
	write("old/.gloss.yaml", "boundary: o:old\nlevel: container\n")
	commit("before adoption", false)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "version: 1.0.0\nconcepts:\n  Doing: { kind: process, doc: Does. }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\nrealizes: [z:Doing]\n")
	write("a.py", "# @gloss implements z:Doing\ndef a(): pass\n")
	commit("adopt", true)
	write("ontology/z.gloss.yaml", "version: 1.1.0\nconcepts:\n  Doing: { kind: process, doc: Does. }\n  Other: { kind: process, doc: Does otherwise. }\n")
	commit("add Other", true)
	write("ontology/z.gloss.yaml", "version: 1.2.0\nconcepts:\n  Doing: { kind: process, doc: Does., deprecated: true, replaced-by: z:Other }\n  Other: { kind: process, doc: Does otherwise. }\n")
	commit("retire Doing", true)
	write("b.py", "def b(): pass\n")
	commit("code only", false)
	write("ontology/z.gloss.yaml", "version: 1.2.0\nconcepts:\n  Doing: { kind: process, doc: Does still., deprecated: true, replaced-by: z:Other }\n  Other: { kind: process, doc: Does otherwise. }\n")
	commit("edit without build", false)
	return root, write, git
}

// TestHistoryOf: MCP history_of starts after the last version bump on the
// default branch and at the merge base on a branch, keeps only SOFT and HARD
// changes, and caps the steps at limit.
func TestHistoryOf(t *testing.T) {
	root, write, git := historyRepo(t)
	call := func(args string) historyOf {
		t.Helper()
		req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"history_of","arguments":` + args + `}}` + "\n"
		var buf strings.Builder
		if err := mcp.New(&mcpBackend{root: root}, strings.NewReader(req), &buf, "test").Serve(); err != nil {
			t.Fatal(err)
		}
		var resp struct {
			Result struct {
				IsError           bool      `json:"isError"`
				StructuredContent historyOf `json:"structuredContent"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(buf.String()), &resp); err != nil || resp.Result.IsError {
			t.Fatalf("history_of %s: %v: %s", args, err, buf.String())
		}
		return resp.Result.StructuredContent
	}
	titles := func(h historyOf) []string {
		var ts []string
		for _, st := range h.Steps {
			ts = append(ts, st.Title)
			for _, ch := range st.Changes {
				if ch.Gate == gate.AUTO {
					t.Errorf("%s: AUTO change %s kept without all", st.Title, ch.Msg)
				}
			}
		}
		return ts
	}

	h := call(`{}`)
	if got, want := titles(h), []string{"edit without build", "retire Doing"}; !slices.Equal(got, want) || !strings.HasPrefix(h.Since, "last version bump") {
		t.Errorf("default since: %q steps %v, want last version bump and %v", h.Since, got, want)
	}
	if h := call(`{"curies":["z:Other"],"since":"HEAD~5","limit":1}`); len(h.Steps) != 1 || h.More == 0 {
		t.Errorf("limit 1: %d steps, more %d", len(h.Steps), h.More)
	}

	git("checkout", "-q", "-b", "feature")
	write("ontology/z.gloss.yaml", "version: 1.3.0\nconcepts:\n  Doing: { kind: process, doc: Does still., deprecated: true, replaced-by: z:Other }\n  Other: { kind: process, doc: Does otherwise. }\n  Third: { kind: process, doc: Does a third thing. }\n")
	if code := cmdBuild([]string{"--root", root, "-q"}); code != 0 {
		t.Fatalf("gloss build exited %d", code)
	}
	git("add", ".")
	git("commit", "-q", "-m", "add Third")
	if h := call(`{}`); !slices.Equal(titles(h), []string{"add Third"}) || !strings.HasPrefix(h.Since, "merge base with") {
		t.Errorf("on a branch: %q steps %v, want merge base and [add Third]", h.Since, titles(h))
	}
}

// TestDiffHistory: the markdown impact ends with each touched concept's last
// N SOFT and HARD steps on the base, lineage followed, the PR's own commits
// left out.
func TestDiffHistory(t *testing.T) {
	root, write, git := historyRepo(t)
	git("checkout", "-q", "-b", "feature")
	write("ontology/z.gloss.yaml", "version: 2.0.0\nconcepts:\n  Doing: { kind: process, doc: Does still., deprecated: true, replaced-by: z:Other }\n  Other: { kind: process, doc: Does it differently. }\n  Third: { kind: process, doc: New here. }\n")
	if code := cmdBuild([]string{"--root", root, "-q"}); code != 0 {
		t.Fatalf("gloss build exited %d", code)
	}
	git("commit", "-qam", "reword Other")
	out, code := capture(t, "diff", "--root", root, "--base", "master", "--format", "markdown", "--fail-on", "none", "--history", "2")
	if code != 0 {
		t.Fatalf("gloss diff exited %d: %s", code, out)
	}
	_, hist, ok := strings.Cut(out, "### Recent history")
	if !ok {
		t.Fatalf("no recent history:\n%s", out)
	}
	for _, want := range []string{"<b>z:Other</b>: last 2 step(s)", "retire Doing", "add Other"} {
		if !strings.Contains(hist, want) {
			t.Errorf("recent history lacks %q:\n%s", want, hist)
		}
	}
	for _, not := range []string{"adopt", "reword Other", "z:Third"} {
		if strings.Contains(hist, not) {
			t.Errorf("recent history has %q:\n%s", not, hist)
		}
	}
	if out, _ := capture(t, "diff", "--root", root, "--base", "master", "--format", "markdown", "--fail-on", "none", "--history", "0"); strings.Contains(out, "Recent history") {
		t.Errorf("--history 0 still renders history")
	}
}
