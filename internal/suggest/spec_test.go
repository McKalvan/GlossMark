// @gloss verifies gmi:SpecInference scope=file

package suggest

import (
	"path/filepath"
	"strings"
	"testing"
)

// The built-in sources' names, as Proposal.Source carries them.
const (
	sourceAsyncAPI = "asyncapi"
	sourceOpenAPI  = "openapi"
)

var claimsEvents = filepath.Join("testdata", "calibration", "glossmark-examples", "claims-events")

// TestAsyncAPIContractYieldsFlows (#260): on claims-events stripped of its
// markers, the AsyncAPI contract alone proposes a documents link per
// channel, on the nearest common ancestor of the channel's messages, and
// the publishes and subscribes flows of the code that names the channels
// (through a config alias, a constant or a message name), per message
// where the code tells them apart. The Strimzi topics propose the
// resource each stands up.
func TestAsyncAPIContractYieldsFlows(t *testing.T) {
	_, ps, got := stripped(t, claimsEvents, nil, 0.01)
	want(t, got, "contracts/asyncapi.yaml#claimsFiled", "documents", "claim:ClaimFiled")
	want(t, got, "contracts/asyncapi.yaml#claimsDecisions", "documents", "claim:ClaimDecided")
	want(t, got, "contracts/asyncapi.yaml#payouts", "documents", "claim:PayoutEvent")
	want(t, got, "contracts/asyncapi.yaml#fraudAlerts", "documents", "claim:FraudSuspected")
	want(t, got, "platform/kafka/topics/claims-filed.yaml#KafkaTopic/claims.filed.v1", "provisions", ".FiledTopic")
	want(t, got, "platform/kafka/topics/fraud-alerts.yaml#KafkaTopic/fraud.alerts.v1", "provisions", ".FraudAlertsTopic")
	flows := map[string]bool{}
	for _, p := range ps {
		if p.Source == sourceAsyncAPI && p.Scope == "" {
			flows[p.Path+"#"+p.Symbol+" "+p.Relation+" "+p.Target] = true
			if !strings.HasPrefix(p.Basis, "asyncapi/") {
				t.Errorf("%s: basis %s, want asyncapi/<role>", p.Symbol, p.Basis)
			}
		}
	}
	for _, f := range []string{
		// handle tells the filed events apart by record name (FILED_EVENT_TYPES).
		"services/assessment/assessment/consumer.py#handle subscribes claim:MotorClaimFiled",
		"services/assessment/assessment/consumer.py#handle subscribes claim:FireClaimFiled",
		"services/assessment/assessment/consumer.py#handle subscribes claim:ClaimWithdrawn",
		"services/assessment/assessment/consumer.py#handle subscribes claim:FraudSuspected",
		"services/assessment/assessment/producer.py#publish publishes claim:ClaimApproved",
		"services/assessment/assessment/producer.py#publish publishes claim:PayoutAuthorized",
		// Through the filedTopic config alias: the channel's own event.
		"services/claim-intake/src/main/kotlin/com/tidewater/claims/intake/events/ClaimEventPublisher.kt#publishFiled publishes claim:ClaimFiled",
		// A Streams topology reads one topic and writes another.
		"services/fraud-screening/src/main/kotlin/com/tidewater/claims/fraud/ScreeningTopology.kt#screeningTopology subscribes claim:ClaimFiled",
		"services/fraud-screening/src/main/kotlin/com/tidewater/claims/fraud/ScreeningTopology.kt#screeningTopology publishes claim:FraudSuspected",
		"services/payout-notifier/internal/consumer/consumer.go#Consumer.Handle subscribes claim:PayoutAuthorized",
	} {
		if !flows[f] {
			t.Errorf("missing flow %s", f)
		}
	}
	for _, f := range []string{
		// run only opens the subscription handle serves: plumbing.
		"services/assessment/assessment/consumer.py#run subscribes claim:ClaimFiled",
		// A rule that reads the event type neither publishes nor subscribes.
		"services/assessment/assessment/rules.py#assess subscribes claim:FloodClaimFiled",
	} {
		if flows[f] {
			t.Errorf("unwanted flow %s", f)
		}
	}
	if len(flows) == 0 {
		t.Fatal("no flows inferred")
	}
}

// TestSourceFilter: Options.Sources keeps only those sources, and every
// proposal names one.
func TestSourceFilter(t *testing.T) {
	root := t.TempDir()
	copyStripped(t, claimsEvents, root)
	ws, g := build(t, root)
	ps, _, err := Run(ws, g, nil, Options{MinConf: 0.01, Sources: []string{sourceAsyncAPI, SourceTopic}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) == 0 {
		t.Fatal("no proposals")
	}
	for _, p := range ps {
		if p.Source != sourceAsyncAPI && p.Source != SourceTopic {
			t.Errorf("%s: source %q kept by the filter", p.Symbol, p.Source)
		}
	}
	all, _, _ := Run(ws, g, nil, Options{MinConf: 0.01})
	for _, p := range all {
		if !contains(Sources, p.Source) {
			t.Errorf("%s: source %q is not one of %v", p.Symbol, p.Source, Sources)
		}
	}
	if len(all) <= len(ps) {
		t.Errorf("filter kept %d of %d", len(ps), len(all))
	}
}

const openAPISpec = `openapi: 3.0.3
info: { title: Booking API, version: 1.0.0 }
paths:
  /v1/appointments:
    post:
      operationId: bookSlot
      summary: Reserving a slot for a patient.
      requestBody:
        content: { application/json: { schema: { $ref: '#/components/schemas/Appointment' } } }
      responses:
        '201': { description: booked }
  /v1/appointments/{id}:
    get:
      operationId: getAppointment
      responses:
        '200': { description: ok }
components:
  schemas:
    Appointment:
      type: object
      properties: { id: { type: string } }
    Problem:
      type: object
`

// TestOpenAPIOperationsAndRoutes (#260): the OpenAPI spec proposes
// documents on an operation whose id or summary names a process and
// represents on a schema named for an entity; a handler whose route the
// spec covers implements the process the operation documents, and a
// route with no spec behind it is name evidence on its path.
func TestOpenAPIOperationsAndRoutes(t *testing.T) {
	extra := map[string]string{
		"services/booking-api/api/openapi.yaml": openAPISpec,
		"services/booking-api/internal/httpapi/book.go": `package httpapi

import "net/http"

// Register mounts the handlers.
func Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/appointments", PostAppointment)
}

// PostAppointment is the handler.
func PostAppointment(w http.ResponseWriter, r *http.Request) {
	_ = r
}
`,
	}
	_, _, got := stripped(t, clinic, extra, 0.01)
	want(t, got, "services/booking-api/api/openapi.yaml#bookSlot", "documents", "clinic:Booking")
	want(t, got, "services/booking-api/api/openapi.yaml#Appointment", "represents", "clinic:Appointment")
	if _, ok := got["services/booking-api/api/openapi.yaml#Problem"]; ok {
		t.Error("Problem: a schema named for no entity was proposed")
	}
	p, ok := got["services/booking-api/internal/httpapi/book.go#Register"]
	if !ok || p.Relation != "implements" || p.Target != "clinic:Booking" || p.Source != sourceOpenAPI {
		t.Errorf("Register: got %+v, want implements clinic:Booking from openapi", p)
	}
	for k, p := range got {
		if strings.HasSuffix(k, "openapi.yaml#bookSlot") && p.Scope != "block" {
			t.Errorf("%s: scope %q, want block", k, p.Scope)
		}
	}
	// Without the spec the route's own words are the evidence.
	_, _, got = stripped(t, clinic, map[string]string{"services/booking-api/internal/httpapi/book.go": extra["services/booking-api/internal/httpapi/book.go"]}, 0.01)
	if p, ok := got["services/booking-api/internal/httpapi/book.go#Register"]; ok && p.Source != SourceRoute {
		t.Errorf("Register without a spec: source %q, want route", p.Source)
	}
}

// TestORMTableMapping (#260): a type that maps itself to a table
// represents the entity the table represents, else the one the table is
// named for.
func TestORMTableMapping(t *testing.T) {
	extra := map[string]string{
		"services/reminder-worker/worker/models.py": `class ApptRow(Base):
    __tablename__ = "appointments"
    id = Column(String, primary_key=True)
`,
	}
	_, _, got := stripped(t, clinic, extra, 0.01)
	p, ok := got["services/reminder-worker/worker/models.py#ApptRow"]
	if !ok || p.Relation != "represents" || p.Target != "clinic:Appointment" || p.Source != SourceSchema {
		t.Errorf("ApptRow: got %+v, want represents clinic:Appointment from schema", p)
	}
}
