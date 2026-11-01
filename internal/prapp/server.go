// @gloss implements gmi:PullRequestApp scope=file
// @gloss consumes gm:Graph, gm:Impact scope=file

package prapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/load"
)

// Config is what gloss app serve reads from the environment (docs/APP.md).
type Config struct {
	AppID         string // APP_ID
	PrivateKey    []byte // PRIVATE_KEY (PEM) or the file PRIVATE_KEY_FILE names
	WebhookSecret []byte // WEBHOOK_SECRET
	ListenAddr    string // LISTEN_ADDR, default :8080
	Workdir       string // WORKDIR, where checkouts are made; default the system temp directory
	API           string // GITHUB_API_URL, default api.github.com
}

// FromEnv reads the configuration, naming the first variable missing.
func FromEnv() (*Config, error) {
	c := &Config{AppID: os.Getenv("APP_ID"), WebhookSecret: []byte(os.Getenv("WEBHOOK_SECRET")), ListenAddr: os.Getenv("LISTEN_ADDR"), Workdir: os.Getenv("WORKDIR"), API: os.Getenv("GITHUB_API_URL")}
	if c.AppID == "" {
		return nil, errors.New("APP_ID is not set; it is the App ID on the GitHub App's settings page")
	}
	switch key, file := os.Getenv("PRIVATE_KEY"), os.Getenv("PRIVATE_KEY_FILE"); {
	case key != "":
		c.PrivateKey = []byte(strings.ReplaceAll(key, `\n`, "\n"))
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("PRIVATE_KEY_FILE: %v", err)
		}
		c.PrivateKey = b
	default:
		return nil, errors.New("PRIVATE_KEY or PRIVATE_KEY_FILE is not set; generate a private key on the GitHub App's settings page")
	}
	if len(c.WebhookSecret) == 0 {
		return nil, errors.New("WEBHOOK_SECRET is not set; it is the secret given when the App was registered")
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	return c, nil
}

// Result is what the gate says about a checkout: what gloss diff computes.
type Result struct {
	Impact   *gate.Impact
	Markdown string // gloss diff --format markdown, history included
	WS       *load.Workspace
}

// Runner runs the gate on a checkout: gloss build and gloss diff --base
// base --actor actor, in process. cmd/gloss supplies it.
type Runner func(ctx context.Context, dir, base, actor string) (*Result, error)

// Server judges pull requests: Process does everything one event needs.
type Server struct {
	Client  *Client
	Run     Runner
	Workdir string
	Log     *log.Logger
	// Timeout bounds one pull request (checkout, build, diff, API calls).
	Timeout time.Duration

	mu    sync.Mutex
	locks map[string]*sync.Mutex
	wg    sync.WaitGroup
}

// NewServer returns a server; Handler wires it to the webhook endpoint.
func NewServer(c *Client, run Runner, workdir string, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.New(os.Stderr, "gloss app: ", log.LstdFlags)
	}
	return &Server{Client: c, Run: run, Workdir: workdir, Log: logger, Timeout: 15 * time.Minute, locks: map[string]*sync.Mutex{}}
}

// Handler returns the webhook handler for the secret.
func (s *Server) Handler(secret []byte) *Handler {
	return &Handler{Secret: secret, Process: s.Process, Fetch: s.fetchPR, Log: s.Log}
}

// Wait blocks until every event handed to Process is done (tests, shutdown).
func (s *Server) Wait() { s.wg.Wait() }

// Process judges one pull request in the background and reports the
// outcome to GitHub. Events for the same PR run one at a time, in arrival
// order, so a re-delivery or a quick second push cannot post over a newer
// result.
func (s *Server) Process(ev Event) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.processLocked(ev)
	}()
}

func (s *Server) processLocked(ev Event) {
	key := fmt.Sprintf("%s#%d", ev.Repo.FullName, ev.PR.Number)
	s.mu.Lock()
	l := s.locks[key]
	if l == nil {
		l = &sync.Mutex{}
		s.locks[key] = l
	}
	s.mu.Unlock()
	l.Lock()
	defer l.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	defer cancel()
	if err := s.process(ctx, ev); err != nil {
		s.Log.Printf("%s (%s, delivery %s): %v", key, ev.Action, ev.Delivery, err)
	}
}

// Actor is the --actor for a PR author: agent:<name> for a bot account,
// human:<login> otherwise, as the Action derives it (#186).
func Actor(a Account) string {
	if a.Type == "Bot" {
		return "agent:" + strings.TrimSuffix(a.Login, "[bot]")
	}
	return "human:" + strings.ToLower(a.Login)
}

// Approved reports whether a reviewer's ontology-approved label clears
// the gate for this event: a push after the label (synchronize) is gated
// again, as in the Action's workflow.
func Approved(ev Event) bool {
	if ev.Action == "synchronize" {
		return false
	}
	for _, l := range ev.PR.Labels {
		if l.Name == ApprovalLabel {
			return true
		}
	}
	return false
}

func (s *Server) process(ctx context.Context, ev Event) error {
	token, err := s.Client.InstallationToken(ctx, ev.Installation)
	if err != nil {
		return err
	}
	api := repoAPI{c: s.Client, token: token, owner: ev.Repo.Owner.Login, repo: ev.Repo.Name}
	if api.owner == "" || api.repo == "" {
		api.owner, api.repo, _ = strings.Cut(ev.Repo.FullName, "/")
	}
	checkID, err := api.StartCheck(ctx, ev.PR.Head.SHA)
	if err != nil {
		s.Log.Printf("%s#%d: check run not started: %v", ev.Repo.FullName, ev.PR.Number, err)
	}
	finish := func(conclusion, details string, out CheckOutput) error {
		return api.FinishCheck(ctx, checkID, ev.PR.Head.SHA, conclusion, details, out)
	}
	dir, base, err := Checkout(ctx, s.Workdir, ev.Repo.CloneURL, token, ev.PR)
	if err != nil {
		finish("failure", "", CheckOutput{Title: "checkout failed", Summary: "```\n" + err.Error() + "\n```"})
		return err
	}
	defer os.RemoveAll(dir)
	if _, err := os.Stat(dir + "/" + load.ConfigName); err != nil {
		// Not a GlossMark repository (yet): nothing to judge, say so once.
		return finish("neutral", "", CheckOutput{Title: "no glossmark.yaml", Summary: "This repository has no `glossmark.yaml` at its root, so there is nothing to gate. `gloss init` scaffolds one (docs/QUICKSTART.md)."})
	}
	res, err := s.Run(ctx, dir, base, Actor(ev.PR.User))
	if err != nil {
		finish("failure", "", CheckOutput{Title: "gloss diff failed", Summary: "```\n" + err.Error() + "\n```"})
		return err
	}
	im := res.Impact
	commentURL, err := api.UpsertComment(ctx, ev.PR.Number, ".", CommentBody(".", res.Markdown))
	if err != nil {
		finish("failure", "", CheckOutput{Title: "comment failed", Summary: "```\n" + err.Error() + "\n```"})
		return err
	}
	approved := Approved(ev)
	var rv Reviewers
	if im.Enforced == gate.HARD && !approved {
		rv = MapOwners(res.WS.Config.App, OwnersOf(res.WS, im), im, ReadCodeowners(dir))
		if err := api.RequestReviews(ctx, ev.PR, rv.Users, rv.Teams); err != nil {
			s.Log.Printf("%s#%d: review request failed: %v", ev.Repo.FullName, ev.PR.Number, err)
		}
	}
	return finish(Conclusion(im.Enforced, approved), commentURL, Summary(im, approved, rv))
}

// Summary is the check run's title and summary: the gate, what it asks
// for and who was asked.
func Summary(im *gate.Impact, approved bool, rv Reviewers) CheckOutput {
	out := CheckOutput{Title: "gate " + string(im.Enforced)}
	if im.Criticality != "" {
		out.Title += " · criticality " + im.Criticality
	}
	hard, soft := 0, 0
	for _, ch := range im.Changes {
		switch ch.Gate {
		case gate.HARD:
			hard++
		case gate.SOFT:
			soft++
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d change(s): %d HARD, %d SOFT, %d AUTO. The impact comment on the pull request has the detail.\n", len(im.Changes), hard, soft, len(im.Changes)-hard-soft)
	switch {
	case im.Enforced == gate.HARD && approved:
		fmt.Fprintf(&sb, "\nA reviewer labelled the pull request `%s`: the HARD changes are approved. A new push gates them again.\n", ApprovalLabel)
	case im.Enforced == gate.HARD:
		fmt.Fprintf(&sb, "\nHARD changes need a human decision. A reviewer who owns the area approves by labelling the pull request `%s`.\n", ApprovalLabel)
		if len(rv.Users)+len(rv.Teams) > 0 {
			var who []string
			for _, u := range rv.Users {
				who = append(who, "@"+u)
			}
			for _, t := range rv.Teams {
				who = append(who, "team "+t)
			}
			fmt.Fprintf(&sb, "Review requested from %s (owners: %s).\n", strings.Join(who, ", "), strings.Join(rv.Owners, ", "))
		}
		if len(rv.Unmapped) > 0 {
			fmt.Fprintf(&sb, "No reviewer is known for owner %s: map it under `app.owners` in glossmark.yaml, or add a CODEOWNERS rule for the changed paths.\n", strings.Join(rv.Unmapped, ", "))
		}
	}
	if im.Advisory != "" {
		sb.WriteString("\n" + im.Advisory + "\n")
	}
	out.Summary = sb.String()
	return out
}

// fetchPR reads a pull request for a check_suite re-request, whose payload
// names only the number.
func (s *Server) fetchPR(installation int64, repo Repository, number int) (*PullRequest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	token, err := s.Client.InstallationToken(ctx, installation)
	if err != nil {
		return nil, err
	}
	api := repoAPI{c: s.Client, token: token, owner: repo.Owner.Login, repo: repo.Name}
	var pr PullRequest
	if err := api.do(ctx, "GET", "/pulls/"+itoa(number), nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}
