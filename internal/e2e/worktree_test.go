// @gloss verifies gmi:WorkspaceLoader scope=file

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mckalvan/glossmark/internal/model"
)

// TestLinkedWorktreesAreSkipped: a linked git worktree inside the repository
// (an agent harness's .claude/worktrees/<name>, or git worktree add into a
// subdirectory) is a second checkout of the same files and is not walked; a
// submodule, whose .git file points into .git/modules, is walked as before.
func TestLinkedWorktreesAreSkipped(t *testing.T) {
	root := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "examples", "horse-factory"), root)
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	marker := "# @gloss implements impl:FoalingPipeline\ndef foal():\n    pass\n"
	write(".claude/worktrees/agent-x/.git", "gitdir: /src/repo/.git/worktrees/agent-x\n")
	write(".claude/worktrees/agent-x/copy.py", marker)
	write("vendored/.git", "gitdir: ../.git/modules/vendored\n")
	write("vendored/sub.py", marker)

	_, g, _, _ := build(t, root)
	paths := map[string]bool{}
	for _, a := range g.NodesOf(model.NodeArtifact) {
		paths[a.Path] = true
	}
	if paths[".claude/worktrees/agent-x/copy.py"] {
		t.Errorf("a file inside a linked worktree was walked")
	}
	if !paths["vendored/sub.py"] {
		t.Errorf("a file inside a submodule was not walked")
	}
}
