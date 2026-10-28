// @gloss verifies gmi:HtmlViewer scope=file

package viewer

import (
	"path/filepath"
	"runtime"
	"testing"
)

// Issue #242: with no source-url, paths open in the editor at the
// repository root, escaped segment by segment.
func TestEditorURL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX paths")
	}
	if got, want := EditorURL("/home/a b/repo"), "vscode://file/home/a%20b/repo/{path}:{line}"; got != want {
		t.Errorf("EditorURL = %q, want %q", got, want)
	}
	if got := EditorURL(filepath.Join("rel", "repo")); got != "" {
		t.Errorf("relative root gave %q, want none", got)
	}
}
