// @gloss verifies gm:Concept scope=file

package e2e

import (
	"slices"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/gate"
)

// TestSeeAlso pins ADR 0050: see-also entries reach the graph node; check
// rejects an entry that is neither an http(s) URL nor a repository path
// (GM309) and warns on a missing file or Markdown anchor (GM313); a changed
// entry is gated as a doc change, a PATCH.
func TestSeeAlso(t *testing.T) {
	root, write := tempRepo(t)
	cfg := "owns:\n  z: { iri: https://x/z/, tier: core }\n  zi: { iri: https://x/zi/, tier: implementation }\nprefixes:\n  o: https://x/o/\npolicy:\n  hard-paths: [nothing/**]\n"
	write("glossmark.yaml", cfg)
	write(".gloss.yaml", "boundary: o:root\nlevel: system\n")
	write("docs/holds.md", "# Holds\n\n## Queue rules\n\n```\n# Not a heading\n```\n\n## Queue rules\n")
	onto := func(version, see string) {
		write("ontology/z.gloss.yaml", "version: "+version+"\nconcepts:\n  Hold:\n    kind: entity\n    doc: \"A place in line.\"\n    see-also: ["+see+"]\n")
	}
	impl := func(version, see string) {
		write("ontology/zi.gloss.yaml", "version: "+version+"\nconcepts:\n  HoldQueue:\n    kind: resource\n    realizes: z:Hold\n    doc: \"The queue.\"\n    see-also: ["+see+"]\n")
	}
	onto("1.0.0", "https://wiki.x/holds, docs/holds.md#queue-rules, docs/holds.md#queue-rules-1")
	impl("1.0.0", "docs/holds.md")

	_, g, _, _ := build(t, root)
	n := g.Node("https://x/z/Hold")
	if n == nil || !slices.Equal(n.SeeAlso, []string{"https://wiki.x/holds", "docs/holds.md#queue-rules", "docs/holds.md#queue-rules-1"}) {
		t.Fatalf("graph node see-also = %+v", n)
	}
	for _, e := range g.Out(n.ID, "") {
		if e.P == "see-also" {
			t.Errorf("see-also became an edge: %+v", e)
		}
	}
	codes := func() map[string][]string {
		ws, g, _, _ := build(t, root)
		out := map[string][]string{}
		for _, d := range check.Run(ws, g) {
			out[d.Code] = append(out[d.Code], d.Msg)
		}
		return out
	}
	if got := codes(); len(got["GM309"])+len(got["GM313"])+len(got["GM200"]) != 0 {
		t.Errorf("valid entries: %v", got)
	}
	onto("1.0.0", "ftp://x/holds, /abs/holds.md, ../outside.md, docs/missing.md, docs/holds.md#not-there, docs/holds.md#not-a-heading")
	got := codes()
	if len(got["GM309"]) != 3 || !strings.Contains(strings.Join(got["GM309"], "\n"), "ftp://x/holds") {
		t.Errorf("GM309 = %v, want ftp, absolute and ../ entries", got["GM309"])
	}
	if len(got["GM313"]) != 3 || !strings.Contains(strings.Join(got["GM313"], "\n"), "docs/missing.md") {
		t.Errorf("GM313 = %v, want the missing file and the two missing anchors", got["GM313"])
	}

	// A changed entry is a doc change: HARD on a core concept, SOFT on an
	// implementation one, and a PATCH bump either way.
	onto("1.0.0", "https://wiki.x/holds")
	classify := func(head func()) *gate.Impact {
		_, bg, _, _ := build(t, root)
		head()
		ws, hg, _, _ := build(t, root)
		return gate.Classify(bg, hg, ws, gate.Options{Base: "base", Policy: ws.Config.EffectivePolicy()})
	}
	im := classify(func() { onto("1.0.0", "https://wiki.x/holds-v2") })
	if chs := changesOf(im, "concept-changed"); len(chs) != 1 || chs[0].Gate != gate.HARD {
		t.Errorf("core see-also change: %+v, want one HARD concept-changed", chs)
	}
	if v := versionOf(im, "z"); v == nil || v.Required != "PATCH" || v.OK {
		t.Errorf("core see-also version: %+v, want PATCH required and missing", v)
	}
	im = classify(func() { impl("1.0.1", "docs/holds.md, https://wiki.x/queue") })
	if chs := changesOf(im, "concept-changed"); len(chs) != 1 || chs[0].Gate != gate.SOFT {
		t.Errorf("impl see-also change: %+v, want one SOFT concept-changed", chs)
	}
	if v := versionOf(im, "zi"); v == nil || v.Required != "PATCH" || !v.OK {
		t.Errorf("impl see-also version: %+v, want PATCH satisfied", v)
	}
}
