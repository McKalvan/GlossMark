// @gloss implements gmi:AdoptionLadder scope=file

package gate

import (
	"fmt"

	"github.com/mckalvan/glossmark/internal/model"
)

// Enforce applies the adoption level (docs/QUICKSTART.md, issue #258) to a
// classified impact: Gate stays what the changes are, Enforced is what the
// level lets fail the command. Below level 2 nothing is enforced; at level 2
// only changes on a concept or boundary classified high or critical count;
// at level 3 Enforced is Gate. When the level lowered the gate, Advisory
// says so in one line.
func Enforce(im *Impact, level int) {
	im.Enforced = im.Gate
	im.Advisory = ""
	if level >= model.AdoptionFull {
		return
	}
	enforced := AUTO
	if model.AdoptionGateFails(level) {
		min := model.CriticalityRank(model.AdoptionMinCriticality(level))
		for _, ch := range im.Changes {
			if model.CriticalityRank(ch.Criticality) >= min {
				enforced = Max(enforced, ch.Gate)
			}
		}
	}
	im.Enforced = enforced
	if enforced == im.Gate {
		return
	}
	what := fmt.Sprintf("gate %s is advisory, nothing fails", im.Gate)
	if model.AdoptionGateFails(level) {
		what = fmt.Sprintf("gate %s enforced as %s: only changes on %s or critical concepts fail", im.Gate, enforced, model.AdoptionMinCriticality(level))
	}
	im.Advisory = model.AdoptionNote(level, what)
}
