// @gloss implements gmi:PullRequestApp scope=file

package prapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

// VerifySignature checks GitHub's X-Hub-Signature-256 header (sha256=<hex
// HMAC of the body with the webhook secret>) in constant time.
func VerifySignature(secret, body []byte, header string) bool {
	hexSig, ok := strings.CutPrefix(header, "sha256=")
	if !ok || len(secret) == 0 {
		return false
	}
	want, err := hex.DecodeString(hexSig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}

// Account is a GitHub user, bot or organization.
type Account struct {
	Login string `json:"login"`
	Type  string `json:"type"` // User, Bot, Organization
}

// Repository is the part of a webhook's repository object the app reads.
type Repository struct {
	FullName string  `json:"full_name"`
	CloneURL string  `json:"clone_url"`
	Owner    Account `json:"owner"`
	Name     string  `json:"name"`
}

// Ref is a pull request's head or base.
type Ref struct {
	Ref  string      `json:"ref"`
	SHA  string      `json:"sha"`
	Repo *Repository `json:"repo"`
}

// PullRequest is the part of a pull_request object the app reads.
type PullRequest struct {
	Number int     `json:"number"`
	Title  string  `json:"title"`
	User   Account `json:"user"`
	Head   Ref     `json:"head"`
	Base   Ref     `json:"base"`
	Draft  bool    `json:"draft"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	RequestedReviewers []Account `json:"requested_reviewers"`
	RequestedTeams     []struct {
		Slug string `json:"slug"`
	} `json:"requested_teams"`
}

// Event is one pull request to judge: what the handler hands to the
// server, whatever webhook it came from.
type Event struct {
	Delivery     string
	Action       string
	Installation int64
	Repo         Repository
	PR           PullRequest
}

type webhookPayload struct {
	Action       string             `json:"action"`
	Installation struct{ ID int64 } `json:"installation"`
	Repository   Repository         `json:"repository"`
	PullRequest  *PullRequest       `json:"pull_request"`
	CheckSuite   *struct {
		PullRequests []struct {
			Number int `json:"number"`
		} `json:"pull_requests"`
	} `json:"check_suite"`
}

// prActions are the pull_request actions that run the gate: a new or
// updated PR, and the ontology-approved label going on or off, as in the
// Action's workflow.
var prActions = map[string]bool{"opened": true, "synchronize": true, "reopened": true, "ready_for_review": true, "labeled": true, "unlabeled": true}

// Handler is the webhook endpoint. It verifies the signature, turns a
// pull_request or check_suite delivery into Events and hands each to
// Process, which must return at once: GitHub gives a webhook ten seconds,
// and the answer is 202 before any work is done.
// A delivery seen before is acknowledged and not processed again.
type Handler struct {
	Secret  []byte
	Process func(Event)
	// Fetch resolves a check_suite re-request to its pull requests when the
	// payload lists none (nil: such a re-request is ignored).
	Fetch func(installation int64, repo Repository, number int) (*PullRequest, error)
	Log   *log.Logger

	mu   sync.Mutex
	seen []string
	idx  map[string]bool
}

const maxBody = 10 << 20

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !VerifySignature(h.Secret, body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	delivery := r.Header.Get("X-GitHub-Delivery")
	if delivery != "" && h.duplicate(delivery) {
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, "already delivered\n")
		return
	}
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	var events []Event
	switch r.Header.Get("X-GitHub-Event") {
	case "ping":
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "pong\n")
		return
	case "pull_request":
		if p.PullRequest == nil || !prActions[p.Action] {
			break
		}
		events = append(events, Event{Delivery: delivery, Action: p.Action, Installation: p.Installation.ID, Repo: p.Repository, PR: *p.PullRequest})
	case "check_suite":
		if p.Action != "rerequested" || p.CheckSuite == nil {
			break
		}
		for _, pr := range p.CheckSuite.PullRequests {
			if h.Fetch == nil {
				continue
			}
			full, err := h.Fetch(p.Installation.ID, p.Repository, pr.Number)
			if err != nil {
				h.logf("delivery %s: pull request #%d: %v", delivery, pr.Number, err)
				continue
			}
			events = append(events, Event{Delivery: delivery, Action: "rerequested", Installation: p.Installation.ID, Repo: p.Repository, PR: *full})
		}
	}
	w.WriteHeader(http.StatusAccepted)
	if len(events) == 0 {
		io.WriteString(w, "ignored\n")
		return
	}
	io.WriteString(w, "queued\n")
	for _, ev := range events {
		h.Process(ev)
	}
}

// duplicate records a delivery id and reports whether it was seen among the
// last thousand: GitHub redelivers on a timeout or by hand.
func (h *Handler) duplicate(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.idx == nil {
		h.idx = map[string]bool{}
	}
	if h.idx[id] {
		return true
	}
	h.idx[id] = true
	h.seen = append(h.seen, id)
	if len(h.seen) > 1000 {
		delete(h.idx, h.seen[0])
		h.seen = h.seen[1:]
	}
	return false
}

func (h *Handler) logf(format string, args ...any) {
	if h.Log != nil {
		h.Log.Printf(format, args...)
	}
}
