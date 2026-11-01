// @gloss verifies gmi:PullRequestApp scope=file

package prapp

import (
	"reflect"
	"testing"

	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

const codeowners = `# comment
*                    @org/everyone
/docs/               @org/docs
services/booking-api/ @alice   # trailing comment
*.tf                 @bob @org/platform
/ontology/clinic.gloss.yaml @carol
`

func TestCodeowners(t *testing.T) {
	rules := ParseCodeowners(codeowners)
	if len(rules) != 5 {
		t.Fatalf("%d rules", len(rules))
	}
	for path, want := range map[string][]string{
		"README.md":                            {"@org/everyone"},
		"docs/APP.md":                          {"@org/docs"},
		"services/booking-api/main.go":         {"@alice"},
		"x/services/booking-api/main.go":       {"@alice"},
		"infra/main.tf":                        {"@bob", "@org/platform"},
		"ontology/clinic.gloss.yaml":           {"@carol"},
		"other/ontology/clinic.gloss.yaml":     {"@org/everyone"},
		"services/booking-api/deploy/infra.tf": {"@bob", "@org/platform"}, // the later rule wins
	} {
		if got := CodeownersFor(rules, path); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", path, got, want)
		}
	}
}

func TestOwnersAndMapping(t *testing.T) {
	ws := &load.Workspace{
		Config: &model.Config{Owns: map[string]*model.NamespaceDecl{"clinic": {Owner: "clinical-ops"}, "lark": {}}},
		Manifests: []*model.Manifest{
			{Boundary: "clinicb:booking-api", Owner: "booking-platform"},
			{Boundary: "clinicb:web", Owner: "web-team"},
		},
	}
	im := &gate.Impact{Changes: []gate.Change{
		{Gate: gate.HARD, Kind: "concept-changed", Subject: "clinic:CancellationWindow", Path: "ontology/clinic.gloss.yaml"},
		{Gate: gate.HARD, Kind: "link-added", Subject: "services/booking-api/x.go", Object: "lark:Thing, clinic:Booking", Path: "services/booking-api/x.go", Boundary: "clinicb:booking-api"},
		{Gate: gate.SOFT, Kind: "link-added", Object: "clinic:Patient", Path: "apps/web/p.ts", Boundary: "clinicb:web"},
		{Gate: gate.HARD, Kind: "boundary-changed", Subject: "clinicb:unowned", Path: "infra/main.tf"},
	}}
	owners := OwnersOf(ws, im)
	if want := []string{"booking-platform", "clinical-ops"}; !reflect.DeepEqual(owners, want) {
		t.Fatalf("owners %v", owners)
	}
	cfg := &model.AppConfig{Owners: map[string]model.StringList{"booking-platform": {"@org/booking", "alice"}, "web-team": {"dave"}}}
	rv := MapOwners(cfg, owners, im, codeowners)
	// clinical-ops has no entry: CODEOWNERS for every HARD path stands in
	// (the ontology file's owner, the Terraform owners, the API's).
	if want := []string{"alice", "bob", "carol"}; !reflect.DeepEqual(rv.Users, want) {
		t.Errorf("users %v", rv.Users)
	}
	if want := []string{"booking", "platform"}; !reflect.DeepEqual(rv.Teams, want) {
		t.Errorf("teams %v", rv.Teams)
	}
	if !reflect.DeepEqual(rv.Unmapped, []string{"clinical-ops"}) {
		t.Errorf("unmapped %v", rv.Unmapped)
	}
	// Every owner mapped: CODEOWNERS is not consulted.
	cfg.Owners["clinical-ops"] = model.StringList{"@carol"}
	rv = MapOwners(cfg, owners, im, codeowners)
	if !reflect.DeepEqual(rv.Users, []string{"alice", "carol"}) || !reflect.DeepEqual(rv.Teams, []string{"booking"}) || rv.Unmapped != nil {
		t.Errorf("mapped: %+v", rv)
	}
	// No app: block at all: CODEOWNERS alone.
	rv = MapOwners(nil, nil, im, codeowners)
	if !reflect.DeepEqual(rv.Users, []string{"alice", "bob", "carol"}) || !reflect.DeepEqual(rv.Teams, []string{"platform"}) {
		t.Errorf("codeowners only: %+v", rv)
	}
}
