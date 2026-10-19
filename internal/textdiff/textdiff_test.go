package textdiff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnifiedApplies(t *testing.T) {
	var a []string
	for i := 0; i < 40; i++ {
		a = append(a, "line "+string(rune('A'+i%26)))
	}
	b := append([]string{}, a...)
	b[2] = "changed 2"
	b = append(b[:20], append([]string{"inserted"}, b[20:]...)...)
	b = b[:len(b)-1]
	before, after := strings.Join(a, "\n")+"\n", strings.Join(b, "\n")+"\n"
	d := Unified("f.txt", before, after)
	if Unified("f.txt", before, before) != "" {
		t.Error("equal texts give an empty diff")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte(before), 0o644)
	os.WriteFile(filepath.Join(dir, "p.diff"), []byte(d), 0o644)
	cmd := exec.Command("git", "apply", "p.diff")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git apply: %v %s\n%s", err, out, d)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "f.txt"))
	if string(got) != after {
		t.Errorf("applied text differs:\n%s", d)
	}
}
