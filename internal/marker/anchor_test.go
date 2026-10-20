// @gloss verifies gmi:DeclarationAnchor scope=file

package marker

import (
	"reflect"
	"strings"
	"testing"
)

// TestDecoratorStackFingerprint: the decorator stack between a marker and its
// declaration is part of the anchor fingerprint (ADR 0014), so deleting a
// guard stales the link like a signature change; the older signature-only
// fingerprint is kept as Legacy so an accepted ledger entry migrates, and an
// undecorated declaration fingerprints as before.
func TestDecoratorStackFingerprint(t *testing.T) {
	style, _ := StyleFor("h.py", nil)
	anchor := func(src string) Anchor {
		t.Helper()
		found, _, lines := Scan(strings.NewReader(src), style)
		if len(found) != 1 {
			t.Fatalf("%d markers in %q", len(found), src)
		}
		return Resolve("h.py", found[0], lines, style)
	}
	const marker = "# @gloss implements .Approval\n"
	const sig = "def approve(claim_id):\n    pass\n"
	const route, role, limit = "@bp.post(\"/approve\")\n", "@requires_role(\n    \"approver\",\n)\n", "@rate_limit(\"write\")\n"
	guarded := anchor(marker + route + role + limit + sig)
	if want := []string{`@bp.post("/approve")`, `@requires_role( "approver", )`, `@rate_limit("write")`}; !reflect.DeepEqual(guarded.Decorators, want) {
		t.Errorf("decorators = %q, want %q", guarded.Decorators, want)
	}
	if guarded.Symbol != "approve" || guarded.StartLine != 7 {
		t.Errorf("anchor %s at %d, want approve at 7", guarded.Symbol, guarded.StartLine)
	}
	plain := anchor(marker + sig)
	cases := []struct {
		name string
		src  string
		same bool
	}{
		{"guard deleted", marker + route + limit + sig, false},
		{"guard added", marker + route + role + limit + "@audit\n" + sig, false},
		{"guards reordered", marker + route + limit + role + sig, false},
		{"guard argument changed", marker + route + strings.Replace(role, "approver", "finance", 1) + limit + sig, false},
		{"guard reflowed", marker + route + "@requires_role(  \"approver\",\n)\n" + limit + sig, true},
		{"blank line in the stack", marker + route + role + "\n" + limit + sig, true},
		{"body edited", marker + route + role + limit + "def approve(claim_id):\n    return 1\n", true},
	}
	for _, c := range cases {
		if got := anchor(c.src).Fingerprint == guarded.Fingerprint; got != c.same {
			t.Errorf("%s: same fingerprint = %v, want %v", c.name, got, c.same)
		}
	}
	if guarded.Legacy != plain.Fingerprint || plain.Legacy != "" {
		t.Errorf("legacy = %q, want the undecorated fingerprint %q (and none without decorators, got %q)", guarded.Legacy, plain.Fingerprint, plain.Legacy)
	}
	if plain.Fingerprint != fp("def approve(claim_id):") {
		t.Errorf("an undecorated declaration's fingerprint changed: %s", plain.Fingerprint)
	}
	// suggest's detector records the fingerprint the anchor would.
	_, _, lines := Scan(strings.NewReader(route+role+limit+sig), style)
	if ds := Declarations("h.py", lines, style); len(ds) != 1 || ds[0].Fingerprint != guarded.Fingerprint {
		t.Errorf("Declarations = %+v, want fingerprint %s", ds, guarded.Fingerprint)
	}
}
