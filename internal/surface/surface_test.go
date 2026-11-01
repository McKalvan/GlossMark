// @gloss verifies gmi:SurfaceRules scope=file

package surface

import (
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		path, text string
		want       Kind
		shell      bool
	}{
		{"gateway/schema.graphql", "type Order { id: ID! }", Schema, false},
		{"gateway/api.gql", "", Schema, false},
		{"services/documents/documents/invoice_pdf.py", "import io\n", Document, false},
		{"services/documents/render.py", "from reportlab.pdfgen import canvas\n", Document, false},
		{"lib/statement.ts", "import PDFDocument from \"pdfkit\";\n", Document, false},
		{"lib/export.ts", "const buf = await page.pdf({ format: \"A4\" });\n", Document, false},
		{"tools/hpctl/hpctl/close_period.py", "import click\n\n@click.command(\"close-period\")\ndef close_period(): pass\n", CLI, false},
		{"tools/hpctl/hpctl/cli.py", "import click\n\n@click.group()\ndef main(): pass\n\nmain.add_command(close_period)\n", CLI, true},
		{"cmd/root.go", "var rootCmd = &cobra.Command{Use: \"hp\"}\n", CLI, true},
		{"scripts/cli.py", "", CLI, false},
		{"batch/assess_fines.rb", "OptionParser.new do |o|\nend\n", CLI, false},
		{"gateway/src/resolvers.ts", "import { GraphQLError } from \"graphql\";\n", GraphQL, false},
		{"gateway/src/server.ts", "import { ApolloServer } from \"@apollo/server\";\nconst s = new ApolloServer({});\n", GraphQL, true},
		{"api/schema.py", "import graphene\n", GraphQL, false},
		{"bff/src/routes/reorder.ts", "import { Router } from \"express\";\n", BFF, false},
		{"services/mobile-bff/app.ts", "", BFF, false},
		{"apps/admin/src/pages/CreditLimitPage.tsx", "export default function CreditLimitPage() {}\n", AdminPage, false},
		{"apps/web/app/bookings/page.tsx", "export default function BookingsPage() {}\n", Page, false},
		{"apps/web/app/bookings/layout.tsx", "export default function Layout() {}\n", Page, true},
		{"apps/web/pages/api/orders.ts", "export default function handler() {}\n", None, false},
		{"storefront/quotes/views.py", "from django.shortcuts import render\n\ndef quote_view(request):\n    return render(request, \"q.html\", {})\n", Page, false},
		{"storefront/quotes/templates/quotes/quote.html", "<h1>Quote</h1>\n", Page, false},
		{"app/controllers/quotes_controller.rb", "class QuotesController < ApplicationController\nend\n", Page, false},
		{"internal/views/views.go", "package views\n", None, false},
		{"services/orders/orders/placement.py", "def place_order(): pass\n", None, false},
		{"services/orders/credit.go", "package orders\n", None, false},
		// A file that names a library in prose or a pattern is not a
		// renderer; only an import line is a signal.
		{"internal/surface/surface.go", "// reportlab fpdf weasyprint\nvar re = `jspdf|\\.pdf\\(`\n", None, false},
	}
	for _, c := range cases {
		lines := strings.Split(c.text, "\n")
		if got := Detect(c.path, lines); got != c.want {
			t.Errorf("Detect(%s) = %q, want %q", c.path, got, c.want)
		} else if got != None && Shell(got, c.path, lines) != c.shell {
			t.Errorf("Shell(%s) = %v, want %v", c.path, !c.shell, c.shell)
		}
	}
}

func TestRules(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rules() {
		if r.Text == "" || r.Surface == "" || r.Lab == "" {
			t.Errorf("rule %s is incomplete: %+v", r.Kind, r)
		}
		if seen[r.Lab] {
			t.Errorf("two rules for lab case %s", r.Lab)
		}
		seen[r.Lab] = true
	}
	if len(seen) != 6 {
		t.Errorf("%d rules, want one per lab case", len(seen))
	}
	if _, ok := RuleFor(None); ok {
		t.Error("RuleFor(None) should not exist")
	}
}
