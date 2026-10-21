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

// TestAnchorLanguages: declarations the anchorer recognizes, with the line
// range it gives them (issue #156): Kotlin functions with modifiers and
// backtick names, Kotlin classes with a multi-line primary constructor,
// Perl subs and packages, SQL functions with a dollar-quoted body, and
// Prisma models.
func TestAnchorLanguages(t *testing.T) {
	cases := []struct {
		name, path, src string
		symbol, kind    string
		start, end      int
	}{
		{"kotlin suspend fun", "a.kt", "// @gloss implements .Claim\nsuspend fun fileClaim(req: Req): Resp {\n    return x\n}\n", "fileClaim", "function", 2, 4},
		{"kotlin inline generic extension", "a.kt", "// @gloss implements .Claim\ninline fun <reified T> List<T>.only(): T {\n    return first()\n}\n", "only", "function", 2, 4},
		{"kotlin operator expression body", "a.kt", "// @gloss implements .Money\noperator fun Money.plus(o: Money) = Money(a + o.a)\n\nfun other() {}\n", "plus", "function", 2, 2},
		{"kotlin member modifiers", "a.kt", "// @gloss implements .Claim\n    private override suspend fun handle(e: Event) {\n    }\n", "handle", "function", 2, 3},
		{"kotlin backtick test name", "a.kt", "// @gloss verifies .Fraud\n@Test\nfun `early claim plus prior claims scores high`() {\n    assert(x)\n}\n", "early claim plus prior claims scores high", "function", 3, 5},
		{"kotlin data class constructor", "a.kt", "// @gloss represents .MotorClaim\ndata class MotorClaim(\n    val id: String,\n    val amount: Long,\n)\n\nclass Next {\n}\n", "MotorClaim", "type", 2, 5},
		{"kotlin class constructor and body", "a.kt", "// @gloss implements .Withdrawal\nclass WithdrawalService(\n    private val repo: Repo,\n    private val clock: Clock,\n) : Service {\n    fun run() {\n    }\n}\n", "WithdrawalService", "class", 2, 8},
		{"kotlin one-line class", "a.kt", "// @gloss represents .Id\ndata class Id(val v: String)\n\nclass Next {\n}\n", "Id", "type", 2, 2},
		{"java brace on next line", "A.java", "// @gloss implements .A\npublic void run(int a,\n    int b)\n{\n    go();\n}\n", "run", "method", 2, 6},
		{"perl sub", "batch.pl", "# @gloss implements .Lost\nsub mark_lost {\n    my ($id) = @_;\n    return 1;\n}\n", "mark_lost", "function", 2, 5},
		{"perl package", "Overdue.pm", "# @gloss implements .Overdue\npackage Library::Overdue;\nuse strict;\n\nsub run {\n}\n\npackage Library::Other;\n", "Library::Overdue", "package", 2, 6},
		{"perl package block", "Overdue.pm", "# @gloss implements .Overdue\npackage Library::Overdue {\n    sub run {}\n}\n1;\n", "Library::Overdue", "package", 2, 4},
		{"sql function", "f.sql", "-- @gloss implements .HoldExpiry\nCREATE OR REPLACE FUNCTION sp_expire_holds() RETURNS void AS $$\nDECLARE\n    n int;\nBEGIN\n    UPDATE holds SET state = 'expired; really';\n    -- done; next\nEND;\n$$ LANGUAGE plpgsql;\n\nCREATE TABLE t (id int);\n", "function.sp_expire_holds", "sql", 2, 9},
		{"sql procedure tagged quote", "f.sql", "-- @gloss implements .Fines\nCREATE PROCEDURE sp_assess_fines()\nLANGUAGE plpgsql\nAS $body$\nBEGIN\n    PERFORM 1;\nEND;\n$body$;\n", "procedure.sp_assess_fines", "sql", 2, 8},
		{"sql table", "f.sql", "-- @gloss represents .Loan\nCREATE TABLE loans (\n  id uuid -- the key; unique\n);\n", "table.loans", "sql", 2, 4},
		{"prisma model", "schema.prisma", "// @gloss represents .Driver\nmodel Driver {\n  id String @id\n  @@map(\"drivers\")\n}\n", "Driver", "model", 2, 5},
		{"prisma enum", "schema.prisma", "// @gloss represents .Level\nenum ServiceLevel {\n  STANDARD\n}\n", "ServiceLevel", "type", 2, 4},
	}
	for _, c := range cases {
		style, ok := StyleFor(c.path, nil)
		if !ok {
			t.Fatalf("%s: no comment style for %s", c.name, c.path)
		}
		found, _, lines := Scan(strings.NewReader(c.src), style)
		if len(found) != 1 {
			t.Fatalf("%s: %d markers", c.name, len(found))
		}
		a := Resolve(c.path, found[0], lines, style)
		if a.Scope != "symbol" || a.Symbol != c.symbol || a.Kind != c.kind || a.StartLine != c.start || a.EndLine != c.end {
			t.Errorf("%s: got %s %q (%s) lines %d-%d, want symbol %q (%s) lines %d-%d", c.name, a.Scope, a.Symbol, a.Kind, a.StartLine, a.EndLine, c.symbol, c.kind, c.start, c.end)
		}
	}
}
