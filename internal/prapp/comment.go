// @gloss implements gmi:PullRequestApp scope=file

package prapp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Comment is an issue comment as the app reads it.
type Comment struct {
	ID      int64  `json:"id"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
}

// Marker keys the impact comment of one workspace, exactly as the Action
// does, so a repository that runs both never gets two comments.
func Marker(root string) string {
	root = strings.TrimSuffix(strings.TrimLeft(strings.TrimPrefix(root, "./"), "/"), "/")
	if root == "" {
		root = "."
	}
	return "<!-- glossmark-impact:" + root + " -->"
}

// LegacyMarker is what gloss diff --format markdown writes at the top; a
// comment from before the per-root key belongs to the root workspace.
const LegacyMarker = "<!-- glossmark-impact -->"

// FindComment returns the comment that holds the workspace's impact, or nil.
func FindComment(comments []Comment, root string) *Comment {
	marker := Marker(root)
	for i := range comments {
		c := &comments[i]
		if strings.Contains(c.Body, marker) {
			return c
		}
		if root == "." && strings.Contains(c.Body, LegacyMarker) && !strings.Contains(c.Body, "<!-- glossmark-impact:") {
			return c
		}
	}
	return nil
}

// CommentBody is the marker line above the rendered impact.
func CommentBody(root, markdown string) string {
	body := Marker(root) + "\n" + markdown
	if root != "." && root != "" {
		// Name the workspace in the heading, as the Action does.
		if i := strings.Index(body, "\n## "); i >= 0 {
			if j := strings.Index(body[i+1:], "\n"); j >= 0 {
				body = body[:i+1+j] + " · `" + strings.TrimSuffix(root, "/") + "`" + body[i+1+j:]
			}
		}
	}
	return body
}

// UpsertComment posts the impact as the PR's one comment for the
// workspace, updating the existing one in place. It returns the comment's
// URL for the check run's details link.
func (r repoAPI) UpsertComment(ctx context.Context, number int, root, body string) (string, error) {
	var all []Comment
	err := r.pages(ctx, "/issues/"+itoa(number)+"/comments", func(raw json.RawMessage) (int, error) {
		var page []Comment
		if err := json.Unmarshal(raw, &page); err != nil {
			return 0, err
		}
		all = append(all, page...)
		return len(page), nil
	})
	if err != nil {
		return "", err
	}
	var out Comment
	if c := FindComment(all, root); c != nil {
		if c.Body == body {
			return c.HTMLURL, nil
		}
		err = r.do(ctx, http.MethodPatch, "/issues/comments/"+itoa64(c.ID), map[string]string{"body": body}, &out)
	} else {
		err = r.do(ctx, http.MethodPost, "/issues/"+itoa(number)+"/comments", map[string]string{"body": body}, &out)
	}
	return out.HTMLURL, err
}
