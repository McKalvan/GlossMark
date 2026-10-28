// @gloss verifies gmi:Bootstrapper scope=file

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOnboard (issue #257): one command on a repository with no GlossMark
// files writes a draft the lint accepts, every concept and link a
// proposal, renders the viewer and ends with the next step; a second run
// changes nothing.
func TestOnboard(t *testing.T) {
	root := filepath.Join(t.TempDir(), "invoicing")
	writeTree(t, root, map[string]string{
		"README.md":                                "# Invoicing\n\nCustomers receive an invoice for each order.\n\n## Invoice\n\nWhat a customer owes for one order.\n",
		"services/billing/app/models.py":           "class Invoice:\n    \"\"\"What a customer owes for one order.\"\"\"\n    invoice_id: str\n    order_id: str\n\n\nclass Order:\n    \"\"\"A customer's purchase.\"\"\"\n    order_id: str\n",
		"services/billing/app/invoicing.py":        "from .models import Invoice, Order\n\n\ndef issue_invoice(order: Order) -> Invoice:\n    \"\"\"Issue the invoice for an order.\"\"\"\n    return Invoice()\n",
		"services/billing/tests/test_invoicing.py": "from app.invoicing import issue_invoice\n\n\ndef test_issue_invoice_for_order():\n    assert issue_invoice(None)\n",
		"services/billing/pyproject.toml":          "[project]\nname = \"billing\"\n",
		"db/migrations/001_invoices.sql":           "CREATE TABLE invoices (invoice_id text primary key, order_id text);\nCREATE TABLE orders (order_id text primary key);\n",
		"deploy/k8s/billing.yaml":                  "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: billing\nspec:\n  replicas: 1\n",
	})
	out, code := capture(t, "onboard", "--root", root, "--no-git", "--namespace", "inv")
	if code != 0 {
		t.Fatalf("onboard: %d\n%s", code, out)
	}
	for _, want := range []string{"scaffolded invoicing: core inv:, implementation invi:, boundaries invb:", "drafted ", "invi:BillingService: Deployment billing (deploy/k8s/billing.yaml, built from services/billing/)",
		"boundary map: draft realizes: in deploy/.gloss.yaml, services/billing/.gloss.yaml", "written as @gloss? proposals", "gloss check: 0 errors", "gloss view: 2 views", "everything written is a proposal"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "Next: open .glossmark/viewer.html") || !strings.Contains(last, "gloss review") {
		t.Errorf("no single next step at the end:\n%s", out)
	}
	for _, f := range []string{"glossmark.yaml", "ontology/inv.gloss.yaml", "ontology/invi.gloss.yaml", ".glossmark/graph.jsonl", ".glossmark/viewer.html", "deploy/.gloss.yaml"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".mcp.json")); err == nil {
		t.Error("onboard installed the hook")
	}
	// Every concept is proposed, every marker a proposal.
	for _, f := range []string{"ontology/inv.gloss.yaml", "ontology/invi.gloss.yaml"} {
		b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if strings.Count(string(b), "kind:") == 0 || strings.Count(string(b), "kind:") != strings.Count(string(b), "proposed: true") {
			t.Errorf("%s: %d concepts, %d proposed:\n%s", f, strings.Count(string(b), "kind:"), strings.Count(string(b), "proposed: true"), b)
		}
	}
	b, _ := os.ReadFile(filepath.Join(root, "deploy", "k8s", "billing.yaml"))
	if !strings.Contains(string(b), "? deploys .BillingService conf=") {
		t.Errorf("no deploys proposal on the workload:\n%s", b)
	}
	if out, code := capture(t, "check", "--root", root); code != 0 || !strings.Contains(out, "0 errors") {
		t.Errorf("check after onboard: %d\n%s", code, out)
	}
	if out, code := capture(t, "status", "--root", root); code != 0 || !strings.Contains(out, "proposed concept awaiting acceptance") {
		t.Errorf("status after onboard: %d\n%s", code, out)
	}
	// A second run adds nothing and still passes.
	before, _ := os.ReadFile(filepath.Join(root, "ontology", "inv.gloss.yaml"))
	out, code = capture(t, "onboard", "--root", root, "--no-git")
	if code != 0 || !strings.Contains(out, "already set up") || !strings.Contains(out, "drafted 0 core concepts") {
		t.Errorf("second onboard: %d\n%s", code, out)
	}
	if after, _ := os.ReadFile(filepath.Join(root, "ontology", "inv.gloss.yaml")); string(after) != string(before) {
		t.Errorf("second run changed the core tier:\n%s", after)
	}
}
