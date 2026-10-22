// @gloss verifies gmi:ReviewEngine scope=file

package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/load"
)

func TestAddWaiverLayouts(t *testing.T) {
	d := Decision{Owner: "alice", Expires: "2999-01-01", Reason: `says "why"`}
	entry := `- { finding: "f1", owner: "alice", expires: 2999-01-01, reason: "says \"why\"" }`
	cases := []struct{ name, in, want, err string }{
		{"no policy", "owns: {}\n",
			"owns: {}\npolicy:\n  waivers:\n    " + entry + "\n", ""},
		{"policy without waivers", "owns: {}\npolicy:\n  soft-conf-threshold: 0.7\nexclude: []\n",
			"owns: {}\npolicy:\n  waivers:\n    " + entry + "\n  soft-conf-threshold: 0.7\nexclude: []\n", ""},
		{"waivers indented", "policy:\n  waivers:\n    - { finding: a, owner: b, expires: 2999-01-01 }\n  hard-paths: [x]\n",
			"policy:\n  waivers:\n    - { finding: a, owner: b, expires: 2999-01-01 }\n    " + entry + "\n  hard-paths: [x]\n", ""},
		{"waivers at key indent, block items", "policy:\n  waivers:\n  - finding: a\n    owner: b\n    expires: 2999-01-01\n# trailing\n",
			"policy:\n  waivers:\n  - finding: a\n    owner: b\n    expires: 2999-01-01\n  " + entry + "\n# trailing\n", ""},
		{"flow policy", "policy: { waivers: [] }\n", "", "flow mapping"},
		{"flow waivers", "policy:\n  waivers: []\n", "", "not a block list"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, load.ConfigName)
			os.WriteFile(path, []byte(c.in), 0o644)
			_, err := addWaiver(&load.Workspace{Root: root}, "f1", d)
			got, _ := os.ReadFile(path)
			if c.err != "" {
				if err == nil || !strings.Contains(err.Error(), c.err) || string(got) != c.in {
					t.Fatalf("want error %q and the file untouched, got %v\n%s", c.err, err, got)
				}
				return
			}
			if err != nil || string(got) != c.want {
				t.Fatalf("err %v\ngot:\n%s\nwant:\n%s", err, got, c.want)
			}
		})
	}
}
