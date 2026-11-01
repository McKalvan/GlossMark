// @gloss implements gmi:PullRequestApp scope=file

package prapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// fetchDepth is how much history each side gets: enough for a merge base
// on an ordinary PR and for the comment's recent-history section.
const fetchDepth = 200

// Checkout fetches the PR's head and base into a fresh directory under
// workdir (shallow, token in an HTTP header rather than the URL) and
// returns the directory, with the head checked out, and the ref to diff
// against: the merge base when the shallow history reaches it, else the
// tip of the base branch as fetched. The caller removes the directory.
func Checkout(ctx context.Context, workdir, cloneURL, token string, pr PullRequest) (dir, base string, err error) {
	dir, err = os.MkdirTemp(workdir, "gloss-pr-")
	if err != nil {
		return "", "", err
	}
	git := func(args ...string) (string, error) {
		all := []string{"-C", dir}
		if token != "" {
			all = append(all, "-c", "http.extraheader=Authorization: basic "+base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token)))
		}
		all = append(all, args...)
		cmd := exec.CommandContext(ctx, "git", all...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1")
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return strings.TrimSpace(out.String()), nil
	}
	fail := func(e error) (string, string, error) {
		os.RemoveAll(dir)
		return "", "", e
	}
	if _, err := git("init", "-q"); err != nil {
		return fail(err)
	}
	// The repository's own name for a merge base on its origin, so
	// gloss diff's history section reads as it would locally.
	if _, err := git("remote", "add", "origin", cloneURL); err != nil {
		return fail(err)
	}
	headRef := fmt.Sprintf("refs/pull/%d/head", pr.Number)
	baseBranch := strings.TrimPrefix(pr.Base.Ref, "refs/heads/")
	if _, err := git("fetch", "-q", "--no-tags", "--depth", itoa(fetchDepth), "origin",
		"+"+headRef+":refs/remotes/origin/pr-head", "+refs/heads/"+baseBranch+":refs/remotes/origin/"+baseBranch); err != nil {
		return fail(err)
	}
	head := pr.Head.SHA
	if head == "" {
		head = "refs/remotes/origin/pr-head"
	}
	if _, err := git("checkout", "-q", "--detach", head); err != nil {
		// A push since the event: the ref GitHub serves is what to judge.
		if _, err2 := git("checkout", "-q", "--detach", "refs/remotes/origin/pr-head"); err2 != nil {
			return fail(err)
		}
	}
	base = "origin/" + baseBranch
	if mb, err := git("merge-base", "HEAD", base); err == nil && mb != "" {
		base = mb
	}
	return dir, base, nil
}

// ReadCodeowners returns the checkout's CODEOWNERS text, or "".
func ReadCodeowners(dir string) string {
	for _, p := range CodeownersPaths {
		if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p))); err == nil {
			return string(b)
		}
	}
	return ""
}
