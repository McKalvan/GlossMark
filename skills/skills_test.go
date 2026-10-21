// @gloss verifies gmi:AdoptionSkill scope=file

package skills

import (
	"strings"
	"testing"
)

// TestInstallableDropsSelfHostMarkers: the installed skill carries no marker
// naming GlossMark's own namespaces, and keeps its text (issue #118).
func TestInstallableDropsSelfHostMarkers(t *testing.T) {
	for _, name := range Names {
		raw, _ := FS.ReadFile(name + "/SKILL.md")
		b, err := Installable(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "<!-- @gloss") {
				t.Errorf("%s:%d: installed skill keeps a marker: %s", name, i+1, l)
			}
		}
		if !strings.Contains(string(b), "\nname: "+name+"\n") || len(b) < len(raw)-80 {
			t.Errorf("%s: installed skill lost its text", name)
		}
	}
}
