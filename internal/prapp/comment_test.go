// @gloss verifies gmi:PullRequestApp scope=file

package prapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mckalvan/glossmark/internal/gate"
)

func TestMarkerAndFind(t *testing.T) {
	for in, want := range map[string]string{"": "<!-- glossmark-impact:. -->", ".": "<!-- glossmark-impact:. -->", "./svc/": "<!-- glossmark-impact:svc -->", "a/b": "<!-- glossmark-impact:a/b -->"} {
		if got := Marker(in); got != want {
			t.Errorf("Marker(%q) = %q", in, got)
		}
	}
	comments := []Comment{
		{ID: 1, Body: "unrelated"},
		{ID: 2, Body: "<!-- glossmark-impact:svc -->\n## Ontology impact"},
		{ID: 3, Body: "<!-- glossmark-impact -->\n## Ontology impact (legacy)"},
		{ID: 4, Body: "<!-- glossmark-impact:. -->\n<!-- glossmark-impact -->\n## Ontology impact"},
	}
	if c := FindComment(comments, "svc"); c == nil || c.ID != 2 {
		t.Errorf("svc: %+v", c)
	}
	// The first comment that carries the root's key or the legacy marker,
	// in the Action's order, so both post to the same one.
	if c := FindComment(comments, "."); c == nil || c.ID != 3 {
		t.Errorf("root: %+v", c)
	}
	if c := FindComment(append(comments[:2:2], comments[3]), "."); c == nil || c.ID != 4 {
		t.Errorf("keyed: %+v", c)
	}
	if c := FindComment(comments, "other"); c != nil {
		t.Errorf("other: %+v", c)
	}
	body := CommentBody("svc", "<!-- glossmark-impact -->\n## Ontology impact — gate **HARD**\n\nx\n")
	if !strings.HasPrefix(body, "<!-- glossmark-impact:svc -->\n<!-- glossmark-impact -->\n## Ontology impact — gate **HARD** · `svc`\n") {
		t.Errorf("body:\n%s", body)
	}
}

// fakeComments is the issue-comments API of one pull request.
type fakeComments struct {
	mu       sync.Mutex
	comments []Comment
	next     int64
	calls    []string
}

func (f *fakeComments) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		var body struct{ Body string }
		json.NewDecoder(r.Body).Decode(&body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/issues/7/comments":
			if r.URL.Query().Get("page") != "1" {
				json.NewEncoder(w).Encode([]Comment{})
				return
			}
			json.NewEncoder(w).Encode(f.comments)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/7/comments":
			f.next++
			c := Comment{ID: f.next, Body: body.Body, HTMLURL: fmt.Sprintf("https://github.example/o/r/pull/7#issuecomment-%d", f.next)}
			f.comments = append(f.comments, c)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(c)
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/repos/o/r/issues/comments/"):
			for i := range f.comments {
				if itoa64(f.comments[i].ID) == strings.TrimPrefix(r.URL.Path, "/repos/o/r/issues/comments/") {
					f.comments[i].Body = body.Body
					json.NewEncoder(w).Encode(f.comments[i])
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.Error(w, r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	})
}

// TestUpsertComment: the first run creates the comment, the next updates
// it in place, and an identical body is not written again.
func TestUpsertComment(t *testing.T) {
	f := &fakeComments{comments: []Comment{{ID: 1, Body: "unrelated"}}, next: 1}
	api := httptest.NewServer(f.handler())
	defer api.Close()
	r := repoAPI{c: NewClient(api.URL, "1", nil), token: "t", owner: "o", repo: "r"}
	ctx := context.Background()
	url, err := r.UpsertComment(ctx, 7, ".", CommentBody(".", "<!-- glossmark-impact -->\n## Ontology impact — gate **SOFT**\n"))
	if err != nil || !strings.HasSuffix(url, "issuecomment-2") {
		t.Fatalf("%q %v", url, err)
	}
	if _, err := r.UpsertComment(ctx, 7, ".", CommentBody(".", "<!-- glossmark-impact -->\n## Ontology impact — gate **HARD**\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.UpsertComment(ctx, 7, ".", CommentBody(".", "<!-- glossmark-impact -->\n## Ontology impact — gate **HARD**\n")); err != nil {
		t.Fatal(err)
	}
	if len(f.comments) != 2 || !strings.Contains(f.comments[1].Body, "**HARD**") {
		t.Fatalf("comments: %+v", f.comments)
	}
	writes := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, "POST") || strings.HasPrefix(c, "PATCH") {
			writes++
		}
	}
	if writes != 2 {
		t.Fatalf("want one create and one update, got calls %v", f.calls)
	}
}

func TestConclusion(t *testing.T) {
	for _, c := range []struct {
		class    gate.Class
		approved bool
		want     string
	}{{gate.AUTO, false, "success"}, {gate.SOFT, false, "success"}, {gate.HARD, false, "action_required"}, {gate.HARD, true, "neutral"}, {gate.SOFT, true, "success"}} {
		if got := Conclusion(c.class, c.approved); got != c.want {
			t.Errorf("%s approved=%v: %s", c.class, c.approved, got)
		}
	}
}

func TestActorAndApproved(t *testing.T) {
	if a := Actor(Account{Login: "Alice", Type: "User"}); a != "human:alice" {
		t.Error(a)
	}
	if a := Actor(Account{Login: "dependabot[bot]", Type: "Bot"}); a != "agent:dependabot" {
		t.Error(a)
	}
	labelled := PullRequest{Labels: []struct {
		Name string `json:"name"`
	}{{Name: ApprovalLabel}}}
	if !Approved(Event{Action: "labeled", PR: labelled}) || Approved(Event{Action: "synchronize", PR: labelled}) || Approved(Event{Action: "opened"}) {
		t.Error("approval")
	}
}
