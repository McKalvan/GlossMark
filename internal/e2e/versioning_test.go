// @gloss verifies gmi:Gate scope=file

package e2e

import (
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/model"
)

func changesOf(im *gate.Impact, kind string) []gate.Change {
	var out []gate.Change
	for _, ch := range im.Changes {
		if ch.Kind == kind {
			out = append(out, ch)
		}
	}
	return out
}

func versionOf(im *gate.Impact, ns string) *gate.VersionChange {
	for i := range im.Versions {
		if im.Versions[i].Namespace == ns {
			return &im.Versions[i]
		}
	}
	return nil
}

// TestVersioningGate pins ADR 0018: a core concept's definition is HARD and
// changes the business view (#97); a missing or undersized bump is HARD
// (#106); a deprecation requires MINOR (#113); removing a concept that the
// base already deprecated names its replacement instead of advising a
// deprecation (#112).
func TestVersioningGate(t *testing.T) {
	root, write := tempRepo(t)
	cfg := "owns:\n  z: { iri: https://x/z/, tier: core }\n  zi: { iri: https://x/zi/, tier: implementation }\nprefixes:\n  o: https://x/o/\n"
	write("glossmark.yaml", cfg)
	onto := func(version, invDoc, old string) {
		write("ontology/z.gloss.yaml", "version: "+version+"\nconcepts:\n  Invoicing: { kind: process, doc: \""+invDoc+"\" }\n  New: { kind: process }\n"+old)
	}
	write("ontology/zi.gloss.yaml", "version: 1.0.0\nconcepts:\n  Runner: { kind: process, realizes: z:Invoicing, doc: \"runs\" }\n")
	write(".gloss.yaml", "boundary: o:root\nlevel: system\n")
	write("a.py", "# @gloss implements z:Invoicing\ndef a(): pass\n")
	onto("1.0.0", "Invoiced on return.", "  Old: { kind: process }\n")
	classify := func(base, head func()) *gate.Impact {
		base()
		_, bg, _, _ := build(t, root)
		head()
		ws, hg, _, _ := build(t, root)
		return gate.Classify(bg, hg, ws, gate.Options{Base: "base", Policy: ws.Config.EffectivePolicy()})
	}
	noop := func() {}

	// #97 + #106: a rewritten core definition with the version left alone.
	im := classify(noop, func() { onto("1.0.0", "Invoiced weekly in advance.", "  Old: { kind: process }\n") })
	if chs := changesOf(im, "concept-changed"); len(chs) != 1 || chs[0].Gate != gate.HARD {
		t.Errorf("core doc change: %+v, want one HARD concept-changed", chs)
	}
	if chs := changesOf(im, "version-bump"); len(chs) != 1 || chs[0].Gate != gate.HARD {
		t.Errorf("missing bump: %+v, want one HARD version-bump", chs)
	}
	if !has(im.ViewsChanged, "business") || im.Gate != gate.HARD {
		t.Errorf("gate %s, views changed %v; want HARD with business", im.Gate, im.ViewsChanged)
	}

	// An implementation concept's doc stays SOFT when its file is not under hard-paths.
	write("glossmark.yaml", cfg+"policy:\n  hard-paths: [nothing/**]\n")
	im = classify(noop, func() {
		write("ontology/zi.gloss.yaml", "version: 1.0.1\nconcepts:\n  Runner: { kind: process, realizes: z:Invoicing, doc: \"runs weekly\" }\n")
	})
	if chs := changesOf(im, "concept-changed"); len(chs) != 1 || chs[0].Gate != gate.SOFT {
		t.Errorf("impl doc change outside hard-paths: %+v, want SOFT", chs)
	}
	// Under the default hard-paths the same edit is HARD: the path rule is not lowered.
	write("glossmark.yaml", cfg)
	im = classify(noop, func() {
		write("ontology/zi.gloss.yaml", "version: 1.0.2\nconcepts:\n  Runner: { kind: process, realizes: z:Invoicing, doc: \"runs daily\" }\n")
	})
	if chs := changesOf(im, "concept-changed"); len(chs) != 1 || chs[0].Gate != gate.HARD || !strings.Contains(strings.Join(chs[0].Reasons, ";"), "hard-paths") {
		t.Errorf("impl doc change under ontology/: %+v, want HARD by hard-paths", chs)
	}

	// #113: a deprecation needs MINOR; a PATCH is undersized and HARD.
	im = classify(noop, func() {
		onto("1.0.1", "Invoiced weekly in advance.", "  Old: { kind: process, deprecated: true, replaced-by: z:New }\n")
	})
	if v := versionOf(im, "z"); v == nil || v.Required != "MINOR" || v.OK {
		t.Errorf("deprecation: version %+v, want required MINOR, not OK", v)
	}
	if chs := changesOf(im, "version-bump"); len(chs) != 1 || chs[0].Gate != gate.HARD || !strings.Contains(chs[0].Msg, "MINOR") {
		t.Errorf("undersized bump: %+v", chs)
	}

	// #112: removing it after the deprecation cycle names the replacement.
	im = classify(noop, func() { onto("2.0.0", "Invoiced weekly in advance.", "") })
	chs := changesOf(im, "concept-removed")
	if len(chs) != 1 || chs[0].Msg != "concept z:Old removed (deprecated since base, replaced-by z:New)" {
		t.Errorf("removal after deprecation: %+v", chs)
	}
	if v := versionOf(im, "z"); v == nil || v.Required != "MAJOR" || !v.OK {
		t.Errorf("removal: version %+v", v)
	}
	// A removal out of nowhere keeps the advice.
	im = classify(func() { onto("2.0.0", "Invoiced weekly in advance.", "  Gone: { kind: process }\n") }, func() { onto("3.0.0", "Invoiced weekly in advance.", "") })
	if chs := changesOf(im, "concept-removed"); len(chs) != 1 || !strings.Contains(chs[0].Msg, "prefer deprecated: true") {
		t.Errorf("plain removal: %+v", chs)
	}
}

// TestRealizationChanges: gloss diff says what moved (#98): one line per
// concept whose realizing boundaries changed (holds is declared, so a change
// to it is a manifest change, ADR 0037), a change for the
// boundary that lost its last implementation, HARD when nothing realizes
// the concept any more, and a deleted view listed as changed.
func TestRealizationChanges(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\nprefixes:\n  o: https://x/o/\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Invoicing: { kind: process }\n  Invoice: { kind: entity }\n")
	write(".gloss.yaml", "boundary: o:root\nlevel: system\n")
	write("old/.gloss.yaml", "boundary: o:old\nlevel: container\n")
	write("new/.gloss.yaml", "boundary: o:new\nlevel: container\n")
	write("old/gen.py", "# @gloss implements z:Invoicing\ndef gen(): pass\n")
	write("old/models.py", "# @gloss represents z:Invoice\nclass Invoice: pass\n")
	_, before, _, _ := build(t, root)
	write("new/run.py", "# @gloss implements z:Invoicing\ndef run(): pass\n")
	write("new/table.sql", "-- @gloss represents z:Invoice\nCREATE TABLE invoices (id int);\n")
	ws, during, _, _ := build(t, root)
	im := gate.Classify(before, during, ws, gate.Options{Base: "before", Policy: ws.Config.EffectivePolicy()})
	var lines []string
	for _, r := range im.Realizations {
		lines = append(lines, r.String())
	}
	if want := []string{"z:Invoicing realized by {o:old} → {o:new, o:old}"}; strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("realization changes %v, want %v", lines, want)
	}
	if !strings.Contains(gate.RenderText(im), "Realization changes:\n  z:Invoicing realized by") {
		t.Errorf("text render lacks the block:\n%s", gate.RenderText(im))
	}

	// Cutover: the old side goes, and the migration view with it.
	write("old/gen.py", "def gen(): pass\n")
	write("old/models.py", "class Invoice: pass\n")
	ws, after, _, _ := build(t, root)
	im = gate.Classify(during, after, ws, gate.Options{Base: "during", Policy: ws.Config.EffectivePolicy(),
		BaseViews: &model.Views{Views: map[string]*model.View{"business": {}, "invoicing-migration": {}}}})
	rm := changesOf(im, "realization-removed")
	if len(rm) != 1 || rm[0].Msg == "" || !strings.Contains(rm[0].Msg, "o:old no longer") || !strings.Contains(rm[0].Msg, "now o:new") || rm[0].Gate != gate.SOFT {
		t.Errorf("cutover: %+v", rm)
	}
	if !has(im.ViewsChanged, "invoicing-migration (removed)") {
		t.Errorf("views changed %v, want the removed migration view", im.ViewsChanged)
	}
	// Removing the last implementation anywhere is HARD.
	write("new/run.py", "def run(): pass\n")
	ws, gone, _, _ := build(t, root)
	im = gate.Classify(after, gone, ws, gate.Options{Base: "after", Policy: ws.Config.EffectivePolicy()})
	if rm := changesOf(im, "realization-removed"); len(rm) != 1 || rm[0].Gate != gate.HARD || !strings.Contains(rm[0].Msg, "nothing does now") {
		t.Errorf("last implementation: %+v", rm)
	}
}
