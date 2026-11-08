// @gloss verifies gmi:PullRequestApp scope=file

package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/mckalvan/glossmark/internal/prapp"
)

// fakeGitHub is the part of the REST API the app talks to, for one
// installation and one pull request, recording what was written.
type fakeGitHub struct {
	mu        sync.Mutex
	comments  []map[string]any
	checkRuns []map[string]any
	reviewers map[string][]string
	calls     []string
}

func (f *fakeGitHub) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		body := map[string]any{}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		auth := r.Header.Get("Authorization")
		p := r.URL.Path
		switch {
		case r.Method == "POST" && p == "/app/installations/42/access_tokens":
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "no JWT", 401)
				return
			}
			w.WriteHeader(201)
			fmt.Fprint(w, `{"token":"ghs_test","expires_at":"2999-01-01T00:00:00Z"}`)
			return
		}
		if auth != "token ghs_test" {
			http.Error(w, "no installation token", 401)
			return
		}
		switch {
		case r.Method == "GET" && p == "/repos/larkspur/clinic-booking/issues/7/comments":
			if r.URL.Query().Get("page") != "1" {
				fmt.Fprint(w, "[]")
				return
			}
			json.NewEncoder(w).Encode(f.comments)
		case r.Method == "POST" && p == "/repos/larkspur/clinic-booking/issues/7/comments":
			c := map[string]any{"id": len(f.comments) + 1, "body": body["body"], "html_url": fmt.Sprintf("https://github.example/larkspur/clinic-booking/pull/7#issuecomment-%d", len(f.comments)+1)}
			f.comments = append(f.comments, c)
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(c)
		case r.Method == "PATCH" && strings.HasPrefix(p, "/repos/larkspur/clinic-booking/issues/comments/"):
			for _, c := range f.comments {
				if fmt.Sprint(c["id"]) == strings.TrimPrefix(p, "/repos/larkspur/clinic-booking/issues/comments/") {
					c["body"] = body["body"]
					json.NewEncoder(w).Encode(c)
					return
				}
			}
			http.NotFound(w, r)
		case r.Method == "GET" && strings.HasPrefix(p, "/repos/larkspur/clinic-booking/commits/") && strings.HasSuffix(p, "/check-runs"):
			if r.URL.Query().Get("check_name") != prapp.CheckName {
				t.Errorf("check-runs listed without check_name: %s", r.URL)
			}
			sha := strings.TrimSuffix(strings.TrimPrefix(p, "/repos/larkspur/clinic-booking/commits/"), "/check-runs")
			var runs []map[string]any
			if r.URL.Query().Get("page") == "1" {
				for _, cr := range f.checkRuns {
					if cr["head_sha"] == sha {
						runs = append(runs, cr)
					}
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"total_count": len(runs), "check_runs": runs})
		case r.Method == "POST" && p == "/repos/larkspur/clinic-booking/check-runs":
			body["id"] = len(f.checkRuns) + 1
			body["app"] = map[string]any{"id": 99}
			f.checkRuns = append(f.checkRuns, body)
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(body)
		case r.Method == "PATCH" && strings.HasPrefix(p, "/repos/larkspur/clinic-booking/check-runs/"):
			for _, cr := range f.checkRuns {
				if fmt.Sprint(cr["id"]) == strings.TrimPrefix(p, "/repos/larkspur/clinic-booking/check-runs/") {
					for k, v := range body {
						cr[k] = v
					}
					json.NewEncoder(w).Encode(cr)
					return
				}
			}
			http.NotFound(w, r)
		case r.Method == "POST" && p == "/repos/larkspur/clinic-booking/pulls/7/requested_reviewers":
			if f.reviewers == nil {
				f.reviewers = map[string][]string{}
			}
			for _, k := range []string{"reviewers", "team_reviewers"} {
				for _, v := range asStrings(body[k]) {
					f.reviewers[k] = append(f.reviewers[k], v)
				}
			}
			w.WriteHeader(201)
			fmt.Fprint(w, "{}")
		default:
			t.Errorf("unexpected API call %s %s", r.Method, p)
			http.NotFound(w, r)
		}
	})
}

func asStrings(v any) []string {
	var out []string
	if xs, ok := v.([]any); ok {
		for _, x := range xs {
			out = append(out, fmt.Sprint(x))
		}
	}
	return out
}

// clinicFixture is a reduced copy of showcase/clinic-booking: two
// namespaces, the booking API with its manifest and markers, owners in
// three places (owns:, manifests, CODEOWNERS) and an app.owners mapping
// for one of them. It returns the repository and the sha of main.
func clinicFixture(t *testing.T) (root string, git func(args ...string), write func(rel, content string)) {
	root, write, git = gitRepo(t)
	write("glossmark.yaml", `grammar: 1
repo-iri: https://onto.larkspur.example/
owns:
  clinic: { iri: https://onto.larkspur.example/core/, tier: core, version: 0.1.0, owner: clinical-ops }
  lark: { iri: https://onto.larkspur.example/impl/, tier: implementation, version: 0.1.0, owner: booking-platform }
policy:
  soft-conf-threshold: 0.7
app:
  owners:
    clinical-ops: ["@larkspur/clinical", "carol"]
`)
	write("ontology/clinic.gloss.yaml", `namespace: clinic
version: 0.1.0
concepts:
  Patient: { kind: entity, criticality: high, doc: "A person who books appointments." }
  Slot: { kind: entity, doc: "A bookable period in a practitioner's schedule." }
  Appointment: { kind: entity, criticality: high, doc: "A patient's reservation of one slot." }
  Booking: { kind: process, consumes: [Slot], produces: [Appointment], criticality: high, doc: "A patient reserves an open slot." }
  Cancellation: { kind: process, consumes: [Appointment], reverses: Booking, doc: "A patient gives up an appointment and the slot is released." }
  CancellationWindow: { kind: constraint, constrains: [Cancellation], criticality: critical, doc: "How long before the start a patient may cancel without it counting as late." }
`)
	write("ontology/lark.gloss.yaml", `namespace: lark
version: 0.1.0
concepts:
  BookingApiService: { kind: system-type, realizes: [clinic:Booking, clinic:Cancellation], doc: "The Go booking API." }
  BookingHandler: { kind: process, realizes: clinic:Booking, doc: "Service.Book." }
  CancellationHandler: { kind: process, realizes: clinic:Cancellation, doc: "Service.Cancel." }
  CancellationPolicy: { kind: constraint, realizes: clinic:CancellationWindow, published-as: clinic:CancellationWindow, doc: "The 24 hour rule as code." }
`)
	write("ontology/views.yaml", `views:
  business:
    include: [clinic:*]
    collapse: { "lark:*": via realizes }
    zoom: system
  published-values:
    include: [clinic:CancellationWindow, lark:CancellationPolicy]
    zoom: code
`)
	write(".gloss.yaml", "boundary: clinicb:clinic-booking\nlevel: system\nlabel: Larkspur clinic booking\nrealizes: inherit\ndefault-namespace: lark\nowner: booking-platform\n")
	write("services/booking-api/.gloss.yaml", `boundary: clinicb:booking-api
level: container
label: Booking API
part-of: clinicb:clinic-booking
realizes: [clinic:Booking, clinic:Cancellation, lark:CancellationPolicy]
holds: [clinic:Appointment, clinic:Patient, clinic:Slot]
default-namespace: lark
owner: booking-platform
criticality: high
`)
	// The markers are assembled so this repository's own scanner does not
	// read the fixture's links as its own.
	m := "// @" + "gloss"
	write("services/booking-api/booking.go", fmt.Sprintf(`package booking

%[1]s implements lark:BookingHandler
func Book(patient, slot string) error { return nil }

%[1]s implements lark:CancellationHandler
func Cancel(appointment string) error { return nil }

%[1]s publishes-value lark:CancellationPolicy
const CancellationWindowHours = 24
`, m))
	write("services/booking-api/booking_test.go", "package booking\n\n"+m+" verifies clinic:Booking\nfunc TestBook() {}\n")
	write(".github/CODEOWNERS", "services/booking-api/ @alice\n")
	git("checkout", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-qm", "base")
	return root, git, write
}

// TestAppEndToEnd feeds a pull_request webhook to the app with a fake
// GitHub API and a local repository: a PR that changes a critical
// constraint gets the impact comment, an action_required check run and a
// review request to the owners, with no workflow file; a redelivery
// changes nothing, a push updates the same comment and run, and the
// ontology-approved label turns the run neutral.
func TestAppEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root, git, write := clinicFixture(t)
	git("checkout", "-q", "-b", "feature")
	// A HARD change: the published rule itself (ontology/** is a hard
	// path, and the concept is critical), plus the manifest of a boundary
	// whose owner has no app.owners entry, so CODEOWNERS stands in.
	write("ontology/clinic.gloss.yaml", strings.Replace(mustRead(t, root+"/ontology/clinic.gloss.yaml"), "without it counting as late.", "without it counting as late; 48 hours from January.", 1))
	write("services/booking-api/.gloss.yaml", strings.Replace(mustRead(t, root+"/services/booking-api/.gloss.yaml"), "criticality: high", "criticality: critical", 1))
	git("add", "-A")
	git("commit", "-qm", "widen the cancellation window")
	git("update-ref", "refs/pull/7/head", "HEAD")
	headSHA := gitRev(t, root, "HEAD")
	baseSHA := gitRev(t, root, "main")

	gh := &fakeGitHub{}
	api := httptest.NewServer(gh.handler(t))
	defer api.Close()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	srv := prapp.NewServer(prapp.NewClient(api.URL, "99", key), appRunner, t.TempDir(), log.New(&logs, "", 0))
	secret := []byte("hush")
	h := srv.Handler(secret)
	payload := func(action, delivery string, labels ...string) *httptest.ResponseRecorder {
		ls := []map[string]string{}
		for _, l := range labels {
			ls = append(ls, map[string]string{"name": l})
		}
		// GitHub lists the reviewers already asked on the PR object.
		gh.mu.Lock()
		asked, askedTeams := []map[string]string{}, []map[string]string{}
		for _, u := range gh.reviewers["reviewers"] {
			asked = append(asked, map[string]string{"login": u})
		}
		for _, s := range gh.reviewers["team_reviewers"] {
			askedTeams = append(askedTeams, map[string]string{"slug": s})
		}
		gh.mu.Unlock()
		body, _ := json.Marshal(map[string]any{
			"action":       action,
			"installation": map[string]any{"id": 42},
			"repository":   map[string]any{"full_name": "larkspur/clinic-booking", "name": "clinic-booking", "owner": map[string]any{"login": "larkspur"}, "clone_url": "file://" + root},
			"pull_request": map[string]any{
				"number": 7, "title": "widen the cancellation window", "user": map[string]any{"login": "Pat", "type": "User"},
				"head": map[string]any{"ref": "feature", "sha": headSHA}, "base": map[string]any{"ref": "main", "sha": baseSHA},
				"labels": ls, "requested_reviewers": asked, "requested_teams": askedTeams,
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("X-GitHub-Event", "pull_request")
		req.Header.Set("X-GitHub-Delivery", delivery)
		mac := hmac.New(sha256.New, secret)
		mac.Write(body)
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		srv.Wait()
		return rec
	}
	if rec := payload("opened", "d-1"); rec.Code != http.StatusAccepted {
		t.Fatalf("webhook: %d %s", rec.Code, rec.Body)
	}
	if logs.Len() > 0 {
		t.Fatalf("errors logged:\n%s", logs.String())
	}
	gh.mu.Lock()
	if len(gh.comments) != 1 {
		t.Fatalf("want one comment, got %d (calls %v)", len(gh.comments), gh.calls)
	}
	body := fmt.Sprint(gh.comments[0]["body"])
	for _, want := range []string{"<!-- glossmark-impact:. -->\n<!-- glossmark-impact -->\n## Ontology impact — gate **HARD**", "criticality **critical**", "clinic:CancellationWindow", "Recent history"} {
		if !strings.Contains(body, want) {
			t.Errorf("comment lacks %q:\n%s", want, body)
		}
	}
	if len(gh.checkRuns) != 1 {
		t.Fatalf("want one check run, got %d", len(gh.checkRuns))
	}
	cr := gh.checkRuns[0]
	if cr["name"] != prapp.CheckName || cr["head_sha"] != headSHA || cr["status"] != "completed" || cr["conclusion"] != "action_required" {
		t.Errorf("check run: %v", cr)
	}
	if cr["details_url"] != gh.comments[0]["html_url"] {
		t.Errorf("details_url %v, want the comment %v", cr["details_url"], gh.comments[0]["html_url"])
	}
	out, _ := cr["output"].(map[string]any)
	if s := fmt.Sprint(out["summary"]); !strings.Contains(s, "HARD changes need a human decision") || !strings.Contains(s, "ontology-approved") || !strings.Contains(s, "@carol") || !strings.Contains(s, "team clinical") {
		t.Errorf("summary: %s", s)
	}
	// clinical-ops maps to a team and a login; booking-platform has no
	// entry and CODEOWNERS names alice for the manifest's directory.
	if got := strings.Join(gh.reviewers["reviewers"], ","); got != "alice,carol" {
		t.Errorf("reviewers %q", got)
	}
	if got := strings.Join(gh.reviewers["team_reviewers"], ","); got != "clinical" {
		t.Errorf("team reviewers %q", got)
	}
	writes := countWrites(gh.calls)
	gh.mu.Unlock()

	// The same delivery again: acknowledged, nothing written.
	if rec := payload("opened", "d-1"); rec.Body.String() != "already delivered\n" {
		t.Fatalf("redelivery: %s", rec.Body)
	}
	gh.mu.Lock()
	if n := countWrites(gh.calls); n != writes {
		t.Errorf("redelivery wrote %d more calls", n-writes)
	}
	gh.mu.Unlock()

	// A push: the comment and the run are updated in place.
	payload("synchronize", "d-2")
	gh.mu.Lock()
	if len(gh.comments) != 1 || len(gh.checkRuns) != 1 {
		t.Errorf("after synchronize: %d comments, %d check runs", len(gh.comments), len(gh.checkRuns))
	}
	gh.mu.Unlock()

	// A reviewer labels the PR: the gate is approved, no new request.
	payload("labeled", "d-3", prapp.ApprovalLabel)
	gh.mu.Lock()
	defer gh.mu.Unlock()
	if c := gh.checkRuns[0]["conclusion"]; c != "neutral" {
		t.Errorf("approved conclusion %v", c)
	}
	if out, _ := gh.checkRuns[0]["output"].(map[string]any); !strings.Contains(fmt.Sprint(out["summary"]), "approved") {
		t.Errorf("approved summary: %v", out)
	}
	if len(gh.reviewers["reviewers"]) != 2 {
		t.Errorf("reviewers asked again: %v", gh.reviewers)
	}
	if logs.Len() > 0 {
		t.Errorf("errors logged:\n%s", logs.String())
	}
}

func countWrites(calls []string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c, "POST") || strings.HasPrefix(c, "PATCH") {
			n++
		}
	}
	return n
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func gitRev(t *testing.T, root, ref string) string {
	t.Helper()
	out, err := gitOut(root, "rev-parse", ref)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}
