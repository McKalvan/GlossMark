// @gloss verifies gmi:Bootstrapper scope=file

package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/model"
)

func TestConceptName(t *testing.T) {
	cases := map[string]string{
		"appointments":        "Appointment",
		"late cancellations":  "LateCancellation",
		"No-shows":            "NoShow",
		"Practitioners":       "Practitioner",
		"policies":            "Policy",
		"CirculationService":  "Circulation",
		"PatientID":           "Patient",
		"status":              "",
		"Service":             "",
		"Repository":          "",
		"join the waitlist":   "",
		"booking and payment": "",
		"when it expires":     "",
		"SMS":                 "",
		"EMAIL":               "",
		"booking api":         "Booking",
		"catch rate":          "CatchRate",
		"x":                   "",
	}
	for in, want := range cases {
		if got := conceptName(in); got != want {
			t.Errorf("conceptName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSingular(t *testing.T) {
	for in, want := range map[string]string{"loans": "loan", "policies": "policy", "buses": "bus", "status": "status", "class": "class", "analysis": "analysis", "Shows": "Show", "ids": "ids", "boxes": "box"} {
		if got := singular(in); got != want {
			t.Errorf("singular(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestK8sWorkloads(t *testing.T) {
	lines := strings.Split(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: claim-intake
  labels: { app: x }
spec:
  template:
    metadata:
      name: ignored
---
apiVersion: v1
kind: Service
metadata:
  name: claim-intake
---
kind: CronJob
metadata:
  name: "noshow-report"
`, "\n")
	ws := k8sWorkloads(lines)
	if len(ws) != 2 || ws[0].Name != "claim-intake" || ws[0].Kind != "Deployment" || ws[1].Name != "noshow-report" || ws[1].Kind != "CronJob" {
		t.Errorf("workloads: %+v", ws)
	}
}

func TestComposeServices(t *testing.T) {
	lines := strings.Split(`services:
  kafka:
    image: apache/kafka:3.8.0
  intake:
    build: ./services/intake
    depends_on: [kafka]
  notifier:
    build:
      context: ./services/notifier
volumes:
  data: {}
`, "\n")
	got := composeServices(lines)
	if strings.Join(got, ",") != "intake,notifier" {
		t.Errorf("compose services: %v", got)
	}
}

func TestResourceAndSystemTypeNames(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"aws_sqs_queue", "reminder_events"}: "ReminderEventsQueue",
		{"aws_db_instance", "booking"}:       "BookingDatabase",
		{"aws_s3_bucket", "notices_bucket"}:  "NoticesBucket",
		{"aws_ecs_cluster", "circ"}:          "CircCluster",
	} {
		if got := resourceName(in[0], in[1]); got != want {
			t.Errorf("resourceName(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
	for in, want := range map[string]string{"claim-intake": "ClaimIntake", "reminders-worker": "RemindersWorker", "web": "WebService", "booking-api": "BookingApi", "assessment": "AssessmentService"} {
		if got := systemTypeName(in); got != want {
			t.Errorf("systemTypeName(%q) = %q, want %q", in, got, want)
		}
	}
	if !tfSupport.MatchString("aws_iam_role") || !tfSupport.MatchString("aws_ecs_task_definition") || tfSupport.MatchString("aws_sqs_queue") {
		t.Error("tfSupport")
	}
}

func TestMatchDir(t *testing.T) {
	dirs := []string{"apps/web", "deploy", "services/booking-api", "services/reminders"}
	for name, want := range map[string]string{"reminders-worker": "services/reminders", "booking-api": "services/booking-api", "web": "apps/web", "noshow-report": ""} {
		if got := matchDir(name, dirs); got != want {
			t.Errorf("matchDir(%q) = %q, want %q", name, got, want)
		}
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// tree is a small repository with no GlossMark files: a Python service with
// a model and a test, a Kubernetes deployment for it, a Terraform queue, a
// glossary and a README that uses the vocabulary.
var tree = map[string]string{
	"README.md":                                "# Invoicing\n\nCustomers receive an invoice for each order. An overdue invoice blocks new orders.\n\n## Invoice\n\nWhat a customer owes for one order.\n\n## Order\n\nA customer's purchase.\n",
	"docs/glossary.md":                         "# Glossary\n\n## Customer\n\nDef. A person or company that places orders.\n\n## Dunning\n\nDef. Chasing an overdue invoice.\nKind: process\n",
	"services/billing/app/models.py":           "class Invoice:\n    \"\"\"What a customer owes for one order.\"\"\"\n    invoice_id: str\n    order_id: str\n    customer_id: str\n\n\nclass Order:\n    \"\"\"A customer's purchase.\"\"\"\n    order_id: str\n    customer_id: str\n\n\nclass Customer:\n    \"\"\"A person or company that places orders.\"\"\"\n    customer_id: str\n",
	"services/billing/app/invoicing.py":        "from .models import Invoice, Order\n\n\ndef issue_invoice(order: Order) -> Invoice:\n    \"\"\"Issue the invoice for an order.\"\"\"\n    return Invoice()\n",
	"services/billing/tests/test_invoicing.py": "from app.invoicing import issue_invoice\n\n\ndef test_issue_invoice_for_order():\n    assert issue_invoice(None)\n",
	"services/billing/pyproject.toml":          "[project]\nname = \"billing\"\n",
	"db/migrations/001_invoices.sql":           "CREATE TABLE invoices (invoice_id text primary key, order_id text, customer_id text);\nCREATE TABLE orders (order_id text primary key, customer_id text);\n",
	"deploy/k8s/billing.yaml":                  "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: billing\nspec:\n  replicas: 1\n",
	"docker-compose.yml":                       "services:\n  postgres:\n    image: postgres:16\n  billing:\n    build: ./services/billing\n",
	"infra/queue.tf":                           "resource \"aws_sqs_queue\" \"invoice_events\" {\n  name = \"invoice-events\"\n}\n\nresource \"aws_iam_role\" \"billing\" {\n  name = \"billing\"\n}\n",
}

// TestRun: on a repository with no GlossMark files, Run scaffolds it,
// imports the glossary, drafts concepts from the harvest and a boundary
// map from the deployment and the Terraform, all as proposals; a second
// run adds nothing.
func TestRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "invoicing")
	writeTree(t, root, tree)
	res, err := Run(root, Options{Core: "inv", By: "agent:test"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Scaffolded || res.Core != "inv" || res.Impl != "invi" || res.Boundary != "invb" {
		t.Errorf("scaffold: %+v", res)
	}
	if len(res.Vocabulary) != 1 || !strings.HasPrefix(res.Vocabulary[0], "docs/glossary.md (2 added") {
		t.Errorf("vocabulary: %v", res.Vocabulary)
	}
	core, err := model.LoadOntology(res.CoreFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Customer", "Dunning", "Invoice", "Order"} {
		c := core.Concepts[want]
		if c == nil {
			t.Errorf("core lacks %s; has %v", want, res.CoreConcepts)
			continue
		}
		if !c.Proposed || c.ProposedBy != "agent:test" {
			t.Errorf("%s is not proposed by agent:test: %+v", want, c)
		}
	}
	if core.Concepts["Dunning"].Kind != "process" || core.Concepts["Invoice"].Kind != "entity" {
		t.Errorf("kinds: Dunning %s, Invoice %s", core.Concepts["Dunning"].Kind, core.Concepts["Invoice"].Kind)
	}
	impl, err := model.LoadOntology(res.ImplFile)
	if err != nil {
		t.Fatal(err)
	}
	for name, kind := range map[string]string{"BillingService": "system-type", "InvoiceEventsQueue": "resource"} {
		c := impl.Concepts[name]
		if c == nil || c.Kind != kind || !c.Proposed {
			t.Errorf("implementation %s: %+v (have %v)", name, c, res.ImplConcepts)
		}
	}
	if _, ok := impl.Concepts["BillingRole"]; ok {
		t.Error("an IAM role became a resource")
	}
	for _, c := range impl.Concepts {
		if !c.Proposed {
			t.Errorf("implementation concept not proposed: %+v", c)
		}
	}
	if len(res.Workloads) != 1 || res.Workloads[0].Name != "billing" || res.Workloads[0].Dir != "services/billing" || res.Workloads[0].Concept != "invi:BillingService" {
		t.Errorf("workloads: %+v", res.Workloads)
	}
	for rel, want := range map[string]string{"services/billing/.gloss.yaml": "realizes: [invi:BillingService]", "deploy/.gloss.yaml": "realizes: [invi:BillingService]", "infra/.gloss.yaml": "realizes: [invi:InvoiceEventsQueue]"} {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || !strings.Contains(string(b), want) {
			t.Errorf("%s: %v\n%s", rel, err, b)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(root, "db", ".gloss.yaml")); strings.Contains(string(b), "\nrealizes:") {
		t.Errorf("db/ (no program) realizes something:\n%s", b)
	}
	// Idempotent: the second run drafts nothing new.
	again, err := Run(root, Options{By: "agent:test"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Scaffolded || len(again.CoreConcepts)+len(again.ImplConcepts) != 0 || again.SkippedExisting == 0 {
		t.Errorf("second run: %+v", again)
	}
	core2, _ := model.LoadOntology(res.CoreFile)
	if len(core2.Concepts) != len(core.Concepts) {
		t.Errorf("second run changed the core tier: %d → %d", len(core.Concepts), len(core2.Concepts))
	}
	b, _ := os.ReadFile(filepath.Join(root, "deploy", ".gloss.yaml"))
	if strings.Count(string(b), "\nrealizes:") != 1 {
		t.Errorf("second run duplicated realizes:\n%s", b)
	}
}
