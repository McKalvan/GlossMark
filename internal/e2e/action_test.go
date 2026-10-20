// @gloss verifies gmi:GlossCli scope=file

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestActionCommentPerRoot runs the Action's comment script under node with
// a fake GitHub client: a matrix over two roots keeps two comments, and a
// re-run updates its own in place (#46).
func TestActionCommentPerRoot(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var action struct {
		Runs struct {
			Steps []struct {
				Name string
				With map[string]string
			}
		}
	}
	if err := yaml.Unmarshal(b, &action); err != nil {
		t.Fatal(err)
	}
	script := ""
	for _, s := range action.Runs.Steps {
		if s.Name == "Comment on PR" {
			script = s.With["script"]
		}
	}
	if script == "" {
		t.Fatal("no Comment on PR step in action.yml")
	}
	dir := t.TempDir()
	impact := filepath.Join(dir, "impact.md")
	os.WriteFile(impact, []byte("<!-- glossmark-impact -->\n## Ontology impact — gate **SOFT**\n\nbody\n"), 0o644)
	harness := `
const comments = [{ id: 1, body: 'unrelated' }];
let next = 2;
const github = {
  paginate: async () => comments.map(c => ({ ...c })),
  rest: { issues: {
    listComments: null,
    updateComment: async ({ comment_id, body }) => { comments.find(c => c.id === comment_id).body = body; },
    createComment: async ({ body }) => { comments.push({ id: next++, body }); },
  } },
};
const context = { repo: { owner: 'o', repo: 'r' }, payload: { pull_request: { number: 7 } } };
const run = new Function('require', 'github', 'context', 'process', 'return (async () => {' + require('fs').readFileSync(process.argv[2], 'utf8') + '})()');
(async () => {
  for (const root of ['showcase/a', './showcase/b/', 'showcase/a']) {
    await run(require, github, context, { env: { IMPACT: process.argv[3], ROOT: root } });
  }
  console.log(JSON.stringify(comments));
})();
`
	os.WriteFile(filepath.Join(dir, "script.js"), []byte(script), 0o644)
	os.WriteFile(filepath.Join(dir, "harness.js"), []byte(harness), 0o644)
	out, err := exec.Command(node, filepath.Join(dir, "harness.js"), filepath.Join(dir, "script.js"), impact).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	got := string(out)
	if n := strings.Count(got, `"id":`); n != 3 {
		t.Errorf("want the unrelated comment plus one per root, got %d comments: %s", n, got)
	}
	for _, want := range []string{"<!-- glossmark-impact:showcase/a -->", "<!-- glossmark-impact:showcase/b -->", "· `showcase/b`"} {
		if strings.Count(got, want) != 1 {
			t.Errorf("want one %q: %s", want, got)
		}
	}
}
