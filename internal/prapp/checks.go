// @gloss implements gmi:PullRequestApp scope=file

package prapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mckalvan/glossmark/internal/gate"
)

// CheckName is the check run the app owns on each head commit.
const CheckName = "GlossMark gate"

// ApprovalLabel is the label a reviewer puts on a PR to record the human
// decision a HARD change needs; the Action's workflow uses the same one.
const ApprovalLabel = "ontology-approved"

// Conclusion maps the enforced gate to a check-run conclusion. AUTO and
// SOFT pass. HARD is action_required: GitHub shows it as a check that
// needs attention rather than a failure, it blocks merging when the check
// is required, and the label clears it (ADR 0052). An approved HARD passes
// as neutral, so the record shows the decision was taken, not that there
// was nothing to decide.
func Conclusion(class gate.Class, approved bool) string {
	switch {
	case class != gate.HARD:
		return "success"
	case approved:
		return "neutral"
	}
	return "action_required"
}

// CheckOutput is the check run's title and summary.
type CheckOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Text    string `json:"text,omitempty"`
}

type checkRun struct {
	ID  int64 `json:"id"`
	App struct {
		ID int64 `json:"id"`
	} `json:"app"`
	Status  string `json:"status"`
	HTMLURL string `json:"html_url"`
}

// findCheckRun returns this app's CheckName run on the commit, or nil.
func (r repoAPI) findCheckRun(ctx context.Context, sha string) (*checkRun, error) {
	var found *checkRun
	err := r.pages(ctx, "/commits/"+sha+"/check-runs?check_name="+url.QueryEscape(CheckName), func(raw json.RawMessage) (int, error) {
		var page struct {
			CheckRuns []checkRun `json:"check_runs"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return 0, err
		}
		for i := range page.CheckRuns {
			cr := &page.CheckRuns[i]
			if found == nil && (cr.App.ID == 0 || strconv.FormatInt(cr.App.ID, 10) == r.c.AppID) {
				found = cr
			}
		}
		return len(page.CheckRuns), nil
	})
	return found, err
}

// StartCheck creates or resets the check run as in_progress on the head
// commit, so the PR shows the gate is running, and returns its id.
func (r repoAPI) StartCheck(ctx context.Context, sha string) (int64, error) {
	existing, err := r.findCheckRun(ctx, sha)
	if err != nil {
		return 0, err
	}
	body := map[string]any{"name": CheckName, "head_sha": sha, "status": "in_progress", "output": CheckOutput{Title: "running gloss diff", Summary: "Checking out the pull request and classifying its changes."}}
	var out checkRun
	if existing != nil {
		err = r.do(ctx, http.MethodPatch, "/check-runs/"+itoa64(existing.ID), body, &out)
	} else {
		err = r.do(ctx, http.MethodPost, "/check-runs", body, &out)
	}
	return out.ID, err
}

// FinishCheck completes the run. id is 0 when StartCheck did not run (the
// run is then looked up or created).
func (r repoAPI) FinishCheck(ctx context.Context, id int64, sha, conclusion, detailsURL string, out CheckOutput) error {
	body := map[string]any{"name": CheckName, "head_sha": sha, "status": "completed", "conclusion": conclusion, "output": out}
	if detailsURL != "" {
		body["details_url"] = detailsURL
	}
	if id == 0 {
		existing, err := r.findCheckRun(ctx, sha)
		if err != nil {
			return err
		}
		if existing != nil {
			id = existing.ID
		}
	}
	if id != 0 {
		return r.do(ctx, http.MethodPatch, "/check-runs/"+itoa64(id), body, nil)
	}
	return r.do(ctx, http.MethodPost, "/check-runs", body, nil)
}

// RequestReviews asks the users and teams to review, leaving out the PR's
// author and anyone already asked.
func (r repoAPI) RequestReviews(ctx context.Context, pr PullRequest, users, teams []string) error {
	asked := map[string]bool{pr.User.Login: true}
	for _, a := range pr.RequestedReviewers {
		asked[a.Login] = true
	}
	for _, t := range pr.RequestedTeams {
		asked["@/"+t.Slug] = true
	}
	var u, t []string
	for _, x := range users {
		if !asked[x] {
			u = append(u, x)
		}
	}
	for _, x := range teams {
		if !asked["@/"+x] {
			t = append(t, x)
		}
	}
	if len(u) == 0 && len(t) == 0 {
		return nil
	}
	body := map[string]any{}
	if len(u) > 0 {
		body["reviewers"] = u
	}
	if len(t) > 0 {
		body["team_reviewers"] = t
	}
	return r.do(ctx, http.MethodPost, fmt.Sprintf("/pulls/%d/requested_reviewers", pr.Number), body, nil)
}

func itoa(n int) string     { return strconv.Itoa(n) }
func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
