// @gloss verifies gmi:PullRequestApp scope=file

package prapp

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	secret := []byte("s3cret")
	body := []byte(`{"action":"opened"}`)
	good := sign(secret, body)
	for _, c := range []struct {
		name   string
		secret []byte
		body   []byte
		header string
		want   bool
	}{
		{"valid", secret, body, good, true},
		{"other body", secret, []byte(`{"action":"closed"}`), good, false},
		{"other secret", []byte("nope"), body, good, false},
		{"sha1 header", secret, body, "sha1=" + good[7:], false},
		{"no prefix", secret, body, good[7:], false},
		{"bad hex", secret, body, "sha256=zz", false},
		{"empty header", secret, body, "", false},
		{"empty secret", nil, body, sign(nil, body), false},
	} {
		if got := VerifySignature(c.secret, c.body, c.header); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// TestHandler: a signed pull_request delivery is queued once, a redelivery
// of the same id is acknowledged and not run again, an unsigned one is
// refused, and events the app does not act on are ignored.
func TestHandler(t *testing.T) {
	secret := []byte("s")
	var mu sync.Mutex
	var got []Event
	done := make(chan struct{}, 10)
	h := &Handler{Secret: secret, Process: func(ev Event) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
		done <- struct{}{}
	}}
	body := []byte(`{"action":"opened","installation":{"id":42},"repository":{"full_name":"o/r","name":"r","owner":{"login":"o"}},"pull_request":{"number":7,"user":{"login":"alice","type":"User"},"head":{"sha":"h"},"base":{"ref":"main","sha":"b"}}}`)
	post := func(event, delivery string, body []byte, signed bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("X-GitHub-Event", event)
		req.Header.Set("X-GitHub-Delivery", delivery)
		if signed {
			req.Header.Set("X-Hub-Signature-256", sign(secret, body))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := post("pull_request", "d1", body, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned: %d", rec.Code)
	}
	if rec := post("pull_request", "d1", body, true); rec.Code != http.StatusAccepted {
		t.Fatalf("signed: %d %s", rec.Code, rec.Body)
	}
	<-done
	if rec := post("pull_request", "d1", body, true); rec.Code != http.StatusAccepted || rec.Body.String() != "already delivered\n" {
		t.Fatalf("redelivery: %d %s", rec.Code, rec.Body)
	}
	closed := bytes.Replace(body, []byte(`"opened"`), []byte(`"closed"`), 1)
	if rec := post("pull_request", "d2", closed, true); rec.Body.String() != "ignored\n" {
		t.Fatalf("closed: %s", rec.Body)
	}
	if rec := post("ping", "d3", []byte(`{"zen":"x"}`), true); rec.Code != http.StatusOK {
		t.Fatalf("ping: %d", rec.Code)
	}
	if rec := post("issues", "d4", body, true); rec.Body.String() != "ignored\n" {
		t.Fatalf("issues: %s", rec.Body)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].PR.Number != 7 || got[0].Installation != 42 || got[0].Delivery != "d1" || got[0].Repo.FullName != "o/r" {
		t.Fatalf("events: %+v", got)
	}
}

// TestHandlerCheckSuite: a re-requested check suite names its pull
// requests by number; the handler fetches each and runs it.
func TestHandlerCheckSuite(t *testing.T) {
	secret := []byte("s")
	done := make(chan Event, 1)
	h := &Handler{Secret: secret, Process: func(ev Event) { done <- ev },
		Fetch: func(_ int64, _ Repository, n int) (*PullRequest, error) {
			return &PullRequest{Number: n, User: Account{Login: "bob"}}, nil
		}}
	body := []byte(`{"action":"rerequested","installation":{"id":1},"repository":{"full_name":"o/r"},"check_suite":{"pull_requests":[{"number":9}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "check_suite")
	req.Header.Set("X-Hub-Signature-256", sign(secret, body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	ev := <-done
	if ev.PR.Number != 9 || ev.Action != "rerequested" || ev.PR.User.Login != "bob" {
		t.Fatalf("%+v", ev)
	}
}
