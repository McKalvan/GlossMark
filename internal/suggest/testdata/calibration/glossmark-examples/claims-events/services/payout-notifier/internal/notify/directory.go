package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PolicyAdminDirectory looks up contact preferences in policy administration.
type PolicyAdminDirectory struct {
	baseURL string
	http    *http.Client
}

// NewPolicyAdminDirectory returns a directory backed by the policy admin API.
func NewPolicyAdminDirectory(baseURL string) *PolicyAdminDirectory {
	return &PolicyAdminDirectory{baseURL: baseURL, http: &http.Client{Timeout: 2 * time.Second}}
}

// EmailFor implements Directory.
func (d *PolicyAdminDirectory) EmailFor(ctx context.Context, policyholderID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/policyholders/"+policyholderID+"/contact", nil)
	if err != nil {
		return "", err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("policy admin: %s", resp.Status)
	}
	var contact struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&contact); err != nil {
		return "", err
	}
	return contact.Email, nil
}
