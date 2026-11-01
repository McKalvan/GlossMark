// @gloss verifies gmi:SpecInference scope=file

package infer

import (
	"strings"
	"testing"
)

func TestRoutesParse(t *testing.T) {
	for _, tc := range []struct {
		line         string
		method, path string
	}{
		{`@app.route("/upload", methods=["POST"])`, "post", "/upload"},
		{`@router.get("/{location_id}/{station}", response_model=list[KitchenTicket])`, "get", "/{location_id}/{station}"},
		{`@GetMapping(value = "/shipments/{id}/label", produces = "x-application/zpl")`, "get", "/shipments/{id}/label"},
		{`@PostMapping`, "", ""},
		{`app.post('/pickups/:id/confirm', async (req, reply) => {`, "post", "/pickups/:id/confirm"},
		{`mux.HandleFunc("/healthz", health)`, "", "/healthz"},
		{`        post("/{id}/withdrawal") {`, "post", "/{id}/withdrawal"},
	} {
		rs := Routes([]string{tc.line}, "")
		if tc.path == "" {
			if len(rs) != 0 {
				t.Errorf("%s: parsed %+v", tc.line, rs)
			}
			continue
		}
		if len(rs) != 1 || rs[0].Method != tc.method || rs[0].Path != tc.path {
			t.Errorf("%s: got %+v, want %s %s", tc.line, rs, tc.method, tc.path)
		}
	}
	if got := NormPath("/pickups/:id/confirm/"); got != "/pickups/{}/confirm" {
		t.Errorf("NormPath: %s", got)
	}
	if got := PathWords("/api/v1/shipments/{id}/label"); got != "shipments label" {
		t.Errorf("PathWords: %q", got)
	}
}

func TestParseBinding(t *testing.T) {
	for _, tc := range []struct {
		line  string
		ident string
		vals  []string
	}{
		{`    filed_topic: str = "claims.filed.v1"`, "filed_topic", []string{"claims.filed.v1"}},
		{`const val FILED_TOPIC = "claims.filed.v1"`, "FILED_TOPIC", []string{"claims.filed.v1"}},
		{`  filedTopic = "claims.filed.v1"`, "filedTopic", []string{"claims.filed.v1"}},
		{`topic: payouts.v1`, "topic", []string{"payouts.v1"}},
		{`FILED_EVENT_TYPES = ("MotorClaimFiled", "FloodClaimFiled")`, "FILED_EVENT_TYPES", []string{"MotorClaimFiled", "FloodClaimFiled"}},
		{`if (input.eventType == "MotorClaimFiled" && x) {`, "", nil},
		{`            event_type="MotorClaimFiled",`, "", nil},
		{`    val publisher = ClaimEventPublisher.fromConfig(config.config("kafka"))`, "", nil},
	} {
		b, ok := ParseBinding(tc.line)
		if tc.ident == "" {
			if ok {
				t.Errorf("%s: bound %q", tc.line, b.Ident)
			}
			continue
		}
		if !ok {
			t.Errorf("%s: no binding", tc.line)
			continue
		}
		if b.Ident != tc.ident || strings.Join(b.Values, ",") != strings.Join(tc.vals, ",") {
			t.Errorf("%s: got %s = %v, want %s = %v", tc.line, b.Ident, b.Values, tc.ident, tc.vals)
		}
	}
}

func TestKindOf(t *testing.T) {
	if k := kindOf([]string{"# contract", "asyncapi: 3.0.0"}); k != "asyncapi" {
		t.Errorf("asyncapi: %q", k)
	}
	if k := kindOf([]string{"{", `  "openapi": "3.0.3",`}); k != "openapi" {
		t.Errorf("openapi json: %q", k)
	}
	if k := kindOf([]string{"apiVersion: v1", "kind: ConfigMap"}); k != "" {
		t.Errorf("configmap: %q", k)
	}
}
