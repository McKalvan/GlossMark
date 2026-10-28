// @gloss verifies gmi:SurfaceRules, gmi:Linter scope=file

package e2e

import (
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
)

// surfaceCase is one case of the ambiguous-surfaces lab (glossmark-examples,
// lab/ambiguous-surfaces), reduced to the lines the surface rules read:
// the ontology, the manifests, and the surface file with its marker line
// swapped per variant. want maps each variant to the GM25x hint it gets,
// "" for the recommended marking.
type surfaceCase struct {
	name  string
	base  map[string]string            // files every variant shares
	file  string                       // the surface file whose marker varies
	body  string                       // its content, with MARK where the marker goes
	marks map[string]string            // variant: marker line(s)
	extra map[string]map[string]string // variant: other files that differ
	want  map[string]string            // variant: hint code and a fragment of its rewrite, "code|fragment"
}

var surfaceCases = []surfaceCase{
	{
		name: "server-page",
		base: map[string]string{
			"glossmark.yaml":          "owns:\n  hp: { iri: https://x/hp/, tier: core }\n  hpx: { iri: https://x/hpx/, tier: implementation }\n",
			"ontology/hp.gloss.yaml":  "concepts:\n  Pricing: { kind: process, criticality: high }\n  VolumeDiscount: { kind: constraint, criticality: high, constrains: Pricing }\n",
			"ontology/hpx.gloss.yaml": "concepts:\n  QuotePage: { kind: process, presents: hp:Pricing }\n",
			"storefront/.gloss.yaml":  "boundary: hpb:storefront\nlevel: container\ndefault-namespace: hpx\n",
		},
		file: "storefront/quotes/views.py",
		body: "from django.shortcuts import render\n\n\nMARK\ndef quote_view(request, sku):\n    subtotal = 10\n    discount = 0\n    # @gloss implements hp:VolumeDiscount scope=block\n    if subtotal >= 50:\n        discount = 1\n    return render(request, \"quotes/quote.html\", {\"total\": subtotal - discount})\n",
		marks: map[string]string{
			"a-implements":   "# @gloss implements hp:Pricing",
			"b-presents":     "# @gloss presents hp:Pricing",
			"c-both":         "# @gloss presents hp:Pricing\n# @gloss implements hp:Pricing",
			"d-page-concept": "# @gloss implements .QuotePage",
			"e-split-block":  "# @gloss presents hp:Pricing",
		},
		want: map[string]string{"a-implements": "", "b-presents": "GM250|`implements hp:Pricing`", "c-both": "", "d-page-concept": "GM250|`implements hp:Pricing`", "e-split-block": "GM250|`implements hp:Pricing`"},
	},
	{
		name: "cli",
		base: map[string]string{
			"glossmark.yaml":           "owns:\n  hp: { iri: https://x/hp/, tier: core }\n  hpx: { iri: https://x/hpx/, tier: implementation }\n",
			"ontology/hp.gloss.yaml":   "concepts:\n  PeriodClose: { kind: process, criticality: critical }\n",
			"ontology/hpx.gloss.yaml":  "concepts:\n  Finance: { kind: system-type, internal: true }\n",
			"tools/hpctl/.gloss.yaml":  "boundary: hpb:hpctl\nlevel: container\ndefault-namespace: hpx\n",
			"tools/hpctl/hpctl/cli.py": "import click\n\nfrom hpctl.close_period import close_period\n\n\n@click.group()\ndef main():\n    pass\n\n\nmain.add_command(close_period)\n",
		},
		file: "tools/hpctl/hpctl/close_period.py",
		body: "import click\n\n\nMARK\n@click.command(\"close-period\")\ndef close_period(month):\n    pass\n",
		marks: map[string]string{
			"a-implements":  "# @gloss implements hp:PeriodClose",
			"b-presents":    "# @gloss presents hp:PeriodClose",
			"c-both":        "# @gloss presents hp:PeriodClose\n# @gloss implements hp:PeriodClose",
			"d-hosts-group": "# @gloss implements hp:PeriodClose",
			"e-hosts-only":  "# @gloss hosts hp:PeriodClose",
		},
		extra: map[string]map[string]string{
			"d-hosts-group": {"tools/hpctl/hpctl/cli.py": "import click\n\nfrom hpctl.close_period import close_period\n\n\n# @gloss hosts hp:PeriodClose\n@click.group()\ndef main():\n    pass\n\n\nmain.add_command(close_period)\n"},
		},
		want: map[string]string{"a-implements": "", "b-presents": "GM250|`implements hp:PeriodClose`", "c-both": "", "d-hosts-group": "GM251|system-type", "e-hosts-only": "GM251|`implements hp:PeriodClose`"},
	},
	{
		name: "bff",
		base: map[string]string{
			"glossmark.yaml":                      "owns:\n  hp: { iri: https://x/hp/, tier: core }\n  hpx: { iri: https://x/hpx/, tier: implementation }\n",
			"ontology/hp.gloss.yaml":              "concepts:\n  OrderPlacement: { kind: process, criticality: high }\n  CreditLimit: { kind: constraint, criticality: critical }\n",
			"ontology/hpx.gloss.yaml":             "concepts:\n  OrderIntake: { kind: process, realizes: hp:OrderPlacement }\n  CreditCheck: { kind: constraint, realizes: hp:CreditLimit }\n  MobileBff: { kind: system-type, realizes: hp:OrderPlacement }\n",
			"services/orders/.gloss.yaml":         "boundary: hpb:orders\nlevel: container\ndefault-namespace: hpx\n",
			"services/orders/orders/placement.py": "# @gloss implements .CreditCheck\ndef check_credit(acct, total):\n    pass\n\n\n# @gloss implements .OrderIntake\ndef place_order(acct, lines):\n    pass\n",
			"bff/.gloss.yaml":                     "boundary: hpb:mobile-bff\nlevel: container\ndefault-namespace: hpx\n",
		},
		file: "bff/src/routes/reorder.ts",
		body: "import { Router } from \"express\";\n\nHELPER\nexport function canReorder(open: number, limit: number, total: number) {\n  return open + total <= limit;\n}\n\nexport const reorderRouter = Router();\n\nMARK\nreorderRouter.get(\"/mobile/v1/reorder-card\", async (req, res) => {\n  res.json({ ok: canReorder(1, 2, 3) });\n});\n",
		marks: map[string]string{
			"a-implements": "// @gloss implements hp:OrderPlacement",
			"b-presents":   "// @gloss presents hp:OrderPlacement",
			"c-hosts":      "// @gloss hosts hp:OrderPlacement",
			"d-split":      "// @gloss presents hp:OrderPlacement",
			"e-impl-bff":   "// @gloss implements .MobileBff",
		},
		want: map[string]string{"a-implements": "GM252|`presents hp:OrderPlacement`", "b-presents": "", "c-hosts": "GM251|`implements hp:OrderPlacement`", "d-split": "", "e-impl-bff": "GM252|presents: hp:OrderPlacement"},
	},
	{
		name: "graphql",
		base: map[string]string{
			"glossmark.yaml":                      "owns:\n  hp: { iri: https://x/hp/, tier: core }\n  hpx: { iri: https://x/hpx/, tier: implementation }\n",
			"ontology/hp.gloss.yaml":              "concepts:\n  OrderPlacement: { kind: process, criticality: high }\n  Order: { kind: entity }\n",
			"ontology/hpx.gloss.yaml":             "concepts:\n  OrderIntake: { kind: process, realizes: hp:OrderPlacement }\n",
			"services/orders/.gloss.yaml":         "boundary: hpb:orders\nlevel: container\nholds: [hp:Order]\ndefault-namespace: hpx\n",
			"services/orders/orders/placement.py": "# @gloss implements .OrderIntake\ndef place_order(acct, lines):\n    pass\n",
			"services/orders/orders/models.py":    "# @gloss represents hp:Order\nclass Order:\n    pass\n",
			"gateway/.gloss.yaml":                 "boundary: hpb:gateway\nlevel: container\ndefault-namespace: hpx\n",
			"gateway/src/server.ts":               "import { ApolloServer } from \"@apollo/server\";\nimport { resolvers } from \"./resolvers\";\n\nSERVER\nexport async function start() {\n  const server = new ApolloServer({ resolvers });\n  return server;\n}\n",
			"gateway/schema.graphql":              "SCHEMA\ntype Order {\n  id: ID!\n}\n",
		},
		file: "gateway/src/resolvers.ts",
		body: "import { GraphQLError } from \"graphql\";\n\nMARK\nexport const resolvers = {\n  Query: { order: () => null },\n};\n",
		marks: map[string]string{
			"a-implements":        "// @gloss implements hp:OrderPlacement",
			"b-presents":          "// @gloss presents hp:OrderPlacement",
			"c-hosts-server":      "// @gloss presents hp:OrderPlacement",
			"d-schema-represents": "// @gloss presents hp:OrderPlacement",
		},
		extra: map[string]map[string]string{
			"c-hosts-server":      {"gateway/src/server.ts": "import { ApolloServer } from \"@apollo/server\";\nimport { resolvers } from \"./resolvers\";\n\n// @gloss hosts hp:OrderPlacement\nexport async function start() {\n  const server = new ApolloServer({ resolvers });\n  return server;\n}\n"},
			"d-schema-represents": {"gateway/schema.graphql": "# @gloss represents hp:Order scope=block\ntype Order {\n  id: ID!\n}\n"},
		},
		want: map[string]string{"a-implements": "GM252|`presents hp:OrderPlacement`", "b-presents": "", "c-hosts-server": "GM251|system-type", "d-schema-represents": ""},
	},
	{
		name: "admin-limit",
		base: map[string]string{
			"glossmark.yaml":              "owns:\n  hp: { iri: https://x/hp/, tier: core }\n  hpx: { iri: https://x/hpx/, tier: implementation }\n",
			"ontology/hp.gloss.yaml":      "concepts:\n  CreditLimit: { kind: constraint, criticality: critical }\n",
			"ontology/hpx.gloss.yaml":     "concepts:\n  CreditCheck: { kind: constraint, realizes: hp:CreditLimit }\n  CreditLimitEditor: { kind: process, presents: hp:CreditLimit }\n",
			"services/orders/.gloss.yaml": "boundary: hpb:orders\nlevel: container\ndefault-namespace: hpx\n",
			"services/orders/credit.go":   "package orders\n\n// @gloss implements .CreditCheck\nfunc Check(open, limit int) bool { return open <= limit }\n",
			"apps/admin/.gloss.yaml":      "boundary: hpb:admin\nlevel: container\ndefault-namespace: hpx\n",
		},
		file: "apps/admin/src/pages/CreditLimitPage.tsx",
		body: "import { useState } from \"react\";\n\nMARK\nexport default function CreditLimitPage() {\n  const [draft, setDraft] = useState(\"\");\n  return <form onSubmit={() => setDraft(\"\")} />;\n}\n",
		marks: map[string]string{
			"a-presents":              "// @gloss presents hp:CreditLimit",
			"b-implements":            "// @gloss implements hp:CreditLimit",
			"c-configures":            "// @gloss configures hp:CreditLimit",
			"d-page-concept":          "// @gloss implements .CreditLimitEditor",
			"e-page-concept-critical": "// @gloss implements .CreditLimitEditor",
		},
		extra: map[string]map[string]string{
			"e-page-concept-critical": {"ontology/hpx.gloss.yaml": "concepts:\n  CreditCheck: { kind: constraint, realizes: hp:CreditLimit }\n  CreditLimitEditor: { kind: process, presents: hp:CreditLimit, criticality: critical }\n"},
		},
		want: map[string]string{"a-presents": "GM253|`configures hp:CreditLimit`", "b-implements": "GM253|`configures hp:CreditLimit`", "c-configures": "", "d-page-concept": "GM253|criticality:", "e-page-concept-critical": ""},
	},
	{
		name: "pdf-invoice",
		base: map[string]string{
			"glossmark.yaml":                        "owns:\n  hp: { iri: https://x/hp/, tier: core }\n  hpx: { iri: https://x/hpx/, tier: implementation }\n",
			"ontology/hp.gloss.yaml":                "concepts:\n  Invoice: { kind: entity, criticality: critical }\n  Invoicing: { kind: process, criticality: critical }\n",
			"ontology/hpx.gloss.yaml":               "concepts:\n  InvoiceRun: { kind: process, realizes: hp:Invoicing }\n  InvoiceDocument: { kind: process, realizes: hp:Invoicing }\n",
			"services/billing/.gloss.yaml":          "boundary: hpb:billing\nlevel: container\nholds: [hp:Invoice]\ndefault-namespace: hpx\n",
			"services/billing/billing/models.py":    "# @gloss represents hp:Invoice\nclass Invoice:\n    pass\n",
			"services/billing/billing/invoicing.py": "# @gloss implements .InvoiceRun\ndef invoice_customer(customer):\n    pass\n",
			"services/documents/.gloss.yaml":        "boundary: hpb:documents\nlevel: container\ndefault-namespace: hpx\n",
		},
		file: "services/documents/documents/invoice_pdf.py",
		body: "from reportlab.pdfgen import canvas\n\n\nMARK\ndef render_invoice_pdf(invoice) -> bytes:\n    return b\"\"\n",
		marks: map[string]string{
			"a-presents-entity":  "# @gloss presents hp:Invoice",
			"b-presents-process": "# @gloss presents hp:Invoicing",
			"c-implements":       "# @gloss implements .InvoiceDocument",
			"d-produces":         "# @gloss produces hp:Invoice",
			"e-represents":       "# @gloss represents hp:Invoice",
		},
		want: map[string]string{"a-presents-entity": "GM254|`produces hp:Invoice`", "b-presents-process": "GM254|`produces hp:Invoice`", "c-implements": "GM254|`produces hp:Invoice`", "d-produces": "", "e-represents": "GM254|`produces hp:Invoice`"},
	},
}

// TestSurfaceRuleHints: every variant of the ambiguous-surfaces lab that
// departs from its surface's rule gets exactly one hint (GM250 to GM254)
// naming the rule and the rewrite, and the recommended variant gets none;
// hints never raise a warning or an error (#262, ADR 0041).
func TestSurfaceRuleHints(t *testing.T) {
	for _, sc := range surfaceCases {
		for variant, mark := range sc.marks {
			t.Run(sc.name+"/"+variant, func(t *testing.T) {
				root, write := workspace(t)
				for p, c := range sc.base {
					write(p, strings.NewReplacer("SERVER\n", "", "SCHEMA\n", "").Replace(c))
				}
				for p, c := range sc.extra[variant] {
					write(p, c)
				}
				body := strings.NewReplacer("MARK", mark, "HELPER\n", "").Replace(sc.body)
				if sc.name == "bff" && variant == "d-split" {
					body = strings.Replace(sc.body, "HELPER", "// @gloss implements hp:CreditLimit", 1)
					body = strings.Replace(body, "MARK", mark, 1)
				}
				write(sc.file, body)
				ws, g, _, _ := build(t, root)
				var hints []string
				for _, d := range check.Run(ws, g) {
					if strings.HasPrefix(d.Code, "GM25") {
						hints = append(hints, d.String())
						if d.Severity != "hint" {
							t.Errorf("%s is %s, want hint", d.Code, d.Severity)
						}
					} else if d.Code == "GM223" || d.Code == "GM224" {
						t.Errorf("general %s beside a surface rule: %s", d.Code, d)
					} else if d.Severity == "error" {
						t.Errorf("error: %s", d)
					}
				}
				want := sc.want[variant]
				if want == "" {
					if len(hints) != 0 {
						t.Errorf("recommended variant got %v", hints)
					}
					return
				}
				code, frag, _ := strings.Cut(want, "|")
				if len(hints) != 1 {
					t.Fatalf("hints = %v, want one %s", hints, code)
				}
				if !strings.Contains(hints[0], "["+code+"]") || !strings.Contains(hints[0], frag) || !strings.Contains(hints[0], "[surface rule: "+sc.name+"]") || !strings.HasPrefix(hints[0], sc.file) && !strings.HasPrefix(hints[0], "tools/hpctl/hpctl/cli.py") && !strings.HasPrefix(hints[0], "gateway/src/server.ts") {
					t.Errorf("hint = %s, want %s with %q naming the %s rule on the surface file", hints[0], code, frag, sc.name)
				}
			})
		}
	}
}
