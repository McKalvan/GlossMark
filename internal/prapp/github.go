// @gloss implements gmi:PullRequestApp scope=file

// Package prapp is the hosted pull-request app (gloss app serve, ADR 0052):
// it receives a code host's pull-request webhooks, checks out the PR, runs
// the gate and posts the result back as the impact comment, a check run and
// a review request to the owners of the HARD changes. GitHub first; the
// GitHub-specific parts (webhook shape, App authentication, REST calls) are
// in this file and webhook.go, so a GitLab backend can sit beside them.
package prapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultAPI is api.github.com; GitHub Enterprise sets GITHUB_API_URL.
const DefaultAPI = "https://api.github.com"

// Client authenticates as a GitHub App and calls the REST API with stdlib
// crypto and net/http: a JWT signed with the App's private key buys an
// installation token, and the token signs every call about a repository.
type Client struct {
	API   string
	AppID string
	Key   *rsa.PrivateKey
	HTTP  *http.Client
	// Now is the clock (tests set it).
	Now func() time.Time

	mu     sync.Mutex
	tokens map[int64]installationToken
}

type installationToken struct {
	token   string
	expires time.Time
}

// NewClient returns a client for api (DefaultAPI when empty).
func NewClient(api, appID string, key *rsa.PrivateKey) *Client {
	if api == "" {
		api = DefaultAPI
	}
	return &Client{API: strings.TrimSuffix(api, "/"), AppID: appID, Key: key, HTTP: &http.Client{Timeout: 30 * time.Second}, Now: time.Now, tokens: map[int64]installationToken{}}
}

// ParsePrivateKey reads the PEM GitHub hands out for an App (PKCS#1) or a
// PKCS#8 re-encoding of it.
func ParsePrivateKey(b []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("private key: no PEM block found")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("private key: %v", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key: not an RSA key")
	}
	return rk, nil
}

// AppJWT signs the App's JWT (RS256): issued a minute in the past for clock
// skew, valid for nine minutes (GitHub allows ten).
func (c *Client) AppJWT() (string, error) {
	now := c.Now().Unix()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"iat": now - 60, "exp": now + 9*60, "iss": c.AppID})
	signing := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.Key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// InstallationToken returns a token for the installation, cached until a
// minute before it expires.
func (c *Client) InstallationToken(ctx context.Context, installationID int64) (string, error) {
	c.mu.Lock()
	if t, ok := c.tokens[installationID]; ok && c.Now().Before(t.expires.Add(-time.Minute)) {
		c.mu.Unlock()
		return t.token, nil
	}
	c.mu.Unlock()
	jwt, err := c.AppJWT()
	if err != nil {
		return "", err
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := c.Do(ctx, "Bearer "+jwt, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", installationID), nil, &out); err != nil {
		return "", fmt.Errorf("installation token: %w", err)
	}
	if out.Token == "" {
		return "", errors.New("installation token: empty token in response")
	}
	c.mu.Lock()
	c.tokens[installationID] = installationToken{out.Token, out.ExpiresAt}
	c.mu.Unlock()
	return out.Token, nil
}

// APIError is a non-2xx answer.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string { return fmt.Sprintf("GitHub API: HTTP %d: %s", e.Status, e.Body) }

// Do calls path under the API with the Authorization value auth and decodes
// a JSON answer into out (nil to discard). A path that is already a URL is
// called as is (pagination links).
func (c *Client) Do(ctx context.Context, auth, method, path string, body, out any) error {
	url := path
	if !strings.HasPrefix(path, "http") {
		url = c.API + path
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "glossmark-app")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{resp.StatusCode, strings.TrimSpace(string(b))}
	}
	if out != nil && len(bytes.TrimSpace(b)) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}

// repoAPI is the repository the installation token can act on.
type repoAPI struct {
	c     *Client
	token string
	owner string
	repo  string
}

func (r repoAPI) do(ctx context.Context, method, path string, body, out any) error {
	return r.c.Do(ctx, "token "+r.token, method, fmt.Sprintf("/repos/%s/%s%s", r.owner, r.repo, path), body, out)
}

// pages calls fn with each page of a list endpoint until a page is short.
func (r repoAPI) pages(ctx context.Context, path string, fn func(json.RawMessage) (int, error)) error {
	for page := 1; ; page++ {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		var raw json.RawMessage
		if err := r.do(ctx, http.MethodGet, fmt.Sprintf("%s%sper_page=100&page=%d", path, sep, page), nil, &raw); err != nil {
			return err
		}
		n, err := fn(raw)
		if err != nil || n < 100 {
			return err
		}
	}
}
