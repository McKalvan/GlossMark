// @gloss verifies gmi:DriftComparator scope=file

package main

import (
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/rollup"
)

// TestFailOnKinds: an unknown kind in --fail-on is an error naming the valid
// ones, so a typo cannot disable part of the gate (#45).
func TestFailOnKinds(t *testing.T) {
	for _, tc := range []struct {
		list string
		want []string
		err  bool
	}{
		{"violation,value-drift,gap,dangling", []string{"violation", "value-drift", "gap", "dangling"}, false},
		{"stale, rogue", []string{"stale", "rogue"}, false},
		{"none", nil, false},
		{"", nil, false},
		{"bogus", nil, true},
		{"violation,valuedrift", nil, true},
		{"violations", nil, true},
	} {
		got, err := rollup.ParseFailOn(tc.list)
		if (err != nil) != tc.err || strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("ParseFailOn(%q) = %v, %v", tc.list, got, err)
		}
		if err != nil && !strings.Contains(err.Error(), "valid kinds: violation, gap") {
			t.Errorf("ParseFailOn(%q): error does not list the valid kinds: %v", tc.list, err)
		}
	}
	root := t.TempDir()
	for _, cmd := range []func([]string) int{cmdDrift, cmdLandscape} {
		if code := cmd([]string{"--fail-on", "bogus", root}); code != 2 {
			t.Errorf("--fail-on bogus exited %d, want 2", code)
		}
	}
}
