// @gloss verifies gmi:ValueBinder scope=file

package extract

import (
	"strings"
	"testing"
)

// TestFindKey: value-from symbol: in YAML, TOML and JSON is a dotted key
// path, or a bare key or tail that ends exactly one path (#77).
func TestFindKey(t *testing.T) {
	yaml := "# @gloss configures .RateLimiting scope=file\npolicies:\n  claims-write:\n    # comment\n    per_minute: 30\n    burst: 10\n  \"export\":\n    per_minute: 2\nclaims_write_per_minute: 30\nlist:\n  - name: a\n    size: 3\n"
	toml := "title = \"x\"\n[policies.claims-write]\nper_minute = 30\n[policies.\"export\"]\nper_minute = 2\nlimits.burst = 9\n"
	json := "{\n  \"policies\": {\n    \"claims-write\": { \"per_minute\": 30, \"burst\": 10 },\n    \"export\": {\n      \"per_minute\": 2\n    }\n  },\n  \"top\": 1\n}\n"
	cases := []struct {
		file, src, symbol string
		line              int
		cands             int
	}{
		{"r.yaml", yaml, "policies.claims-write.per_minute", 5, 0},
		{"r.yaml", yaml, "export.per_minute", 8, 0},
		{"r.yaml", yaml, "claims_write_per_minute", 9, 0},
		{"r.yaml", yaml, "burst", 6, 0},
		{"r.yaml", yaml, "list.size", 12, 0},
		{"r.yaml", yaml, "per_minute", 0, 2},
		{"r.yaml", yaml, "nope", 0, 0},
		{"r.yml", "a:\n  b: 1\n---\nb: 2\n", "b", 4, 0},
		{"p.toml", toml, "policies.claims-write.per_minute", 3, 0},
		{"p.toml", toml, "policies.export.per_minute", 5, 0},
		{"p.toml", toml, "policies.export.limits.burst", 6, 0},
		{"p.toml", toml, "title", 1, 0},
		{"p.toml", toml, "per_minute", 0, 2},
		{"c.json", json, "policies.claims-write.per_minute", 3, 0},
		{"c.json", json, "policies.claims-write.burst", 3, 0},
		{"c.json", json, "policies.export.per_minute", 5, 0},
		{"c.json", json, "top", 8, 0},
	}
	for _, c := range cases {
		line, cands := findKey(c.file, strings.Split(c.src, "\n"), c.symbol)
		if line != c.line || len(cands) != c.cands {
			t.Errorf("%s %s: line %d candidates %v, want line %d and %d candidates", c.file, c.symbol, line, cands, c.line, c.cands)
		}
	}
}

// TestValueLiteral: gloss bind reads the value a line assigns as the
// ontology would publish it (#272).
func TestValueLiteral(t *testing.T) {
	cases := []struct{ line, symbol, want string }{
		{"AUTO_APPROVE_LIMIT_EUR = 5_000", "AUTO_APPROVE_LIMIT_EUR", "5000"},
		{"const val REFERRAL_SCORE_THRESHOLD = 0.80", "REFERRAL_SCORE_THRESHOLD", "0.80"},
		{"const AutoApproveLimitEUR = 5000 // EUR", "AutoApproveLimitEUR", "5000"},
		{"    public static final BigDecimal LIMIT = new BigDecimal(\"5000\");", "LIMIT", "5000"},
		{"const val LIMIT = 5_000L", "LIMIT", "5000"},
		{"def plan(years: int, swr: float = 0.04):", "swr", "0.04"},
		{"ADJUSTER_QUEUE = \"property-and-motor\"  # the queue", "ADJUSTER_QUEUE", "property-and-motor"},
		{"export const WINDOW = '24h'; // hours", "WINDOW", "24h"},
		{"var Mode = fast", "Mode", "fast"},
		{"    per_minute: 30", "per_minute", "30"},
		{"per_minute = 30 # toml", "per_minute", "30"},
		{"  \"ceiling\": 12,", "ceiling", "12"},
		{"REQUIRED_EVIDENCE = {", "REQUIRED_EVIDENCE", ""},
		{"other = 1", "LIMIT", ""},
	}
	for _, c := range cases {
		if got := ValueLiteral(c.line, c.symbol); got != c.want {
			t.Errorf("ValueLiteral(%q, %s) = %q, want %q", c.line, c.symbol, got, c.want)
		}
	}
}
