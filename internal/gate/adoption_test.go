// @gloss verifies gmi:AdoptionLadder scope=file

package gate

import (
	"strings"
	"testing"
)

// TestEnforce: the adoption level caps the gate that fails gloss diff and
// leaves the classified gate as it is. Below level 2 nothing is enforced; at
// level 2 only changes on high or critical concepts; at level 3 the gate
// itself.
func TestEnforce(t *testing.T) {
	changes := []Change{
		{Gate: HARD, Kind: "concept-added", Criticality: ""},
		{Gate: SOFT, Kind: "link-added", Criticality: "high"},
		{Gate: HARD, Kind: "link-added", Criticality: "medium"},
	}
	cases := []struct {
		level    int
		enforced Class
		advisory string
	}{
		{0, AUTO, "level 0 (vocabulary): gate HARD is advisory"},
		{1, AUTO, "level 1 (manifests): gate HARD is advisory"},
		{2, SOFT, "gate HARD enforced as SOFT"},
		{3, HARD, ""},
	}
	for _, c := range cases {
		im := &Impact{Gate: HARD, Changes: changes}
		Enforce(im, c.level)
		if im.Gate != HARD || im.Enforced != c.enforced || !strings.Contains(im.Advisory, c.advisory) || (c.advisory == "" && im.Advisory != "") {
			t.Errorf("level %d: gate %s enforced %s advisory %q", c.level, im.Gate, im.Enforced, im.Advisory)
		}
		txt, md := RenderText(im), RenderMarkdown(im)
		if (strings.Contains(txt, "adoption level") != (c.advisory != "")) || (strings.Contains(md, "adoption level") != (c.advisory != "")) {
			t.Errorf("level %d: advisory rendered: text %v markdown %v", c.level, strings.Contains(txt, "adoption level"), strings.Contains(md, "adoption level"))
		}
	}
	// A critical HARD change is enforced at level 2.
	im := &Impact{Gate: HARD, Changes: []Change{{Gate: HARD, Criticality: "critical"}}}
	Enforce(im, 2)
	if im.Enforced != HARD || im.Advisory != "" {
		t.Errorf("critical at level 2: enforced %s advisory %q", im.Enforced, im.Advisory)
	}
}
