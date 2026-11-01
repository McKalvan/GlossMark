// @gloss verifies gmi:PullRequestApp scope=file

package prapp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestParsePrivateKey(t *testing.T) {
	k := testKey(t)
	pkcs1 := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	der8, _ := x509.MarshalPKCS8PrivateKey(k)
	pkcs8 := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der8})
	for name, b := range map[string][]byte{"pkcs1": pkcs1, "pkcs8": pkcs8} {
		got, err := ParsePrivateKey(b)
		if err != nil || got.N.Cmp(k.N) != 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := ParsePrivateKey([]byte("not a key")); err == nil {
		t.Error("garbage parsed")
	}
}

// TestAppJWT: the token is an RS256 JWT for the App, verifiable with the
// public key, issued a minute early and valid for nine.
func TestAppJWT(t *testing.T) {
	k := testKey(t)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	c := NewClient("", "12345", k)
	c.Now = func() time.Time { return now }
	tok, err := c.AppJWT()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("%d parts", len(parts))
	}
	hdr, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if string(hdr) != `{"alg":"RS256","typ":"JWT"}` {
		t.Errorf("header %s", hdr)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Iat, Exp int64
		Iss      string
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Iss != "12345" || claims.Iat != now.Unix()-60 || claims.Exp != now.Unix()+540 {
		t.Errorf("claims %+v", claims)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&k.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Errorf("signature: %v", err)
	}
}

// TestInstallationToken exchanges the JWT for an installation token
// against a fake API and caches it until it is about to expire.
func TestInstallationToken(t *testing.T) {
	k := testKey(t)
	var calls int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/app/installations/42/access_tokens" {
			http.Error(w, r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.Count(auth, ".") != 2 {
			http.Error(w, "no JWT", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			http.Error(w, "accept", http.StatusBadRequest)
			return
		}
		n := atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"token": "ghs_" + itoa(int(n)), "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
	}))
	defer api.Close()
	c := NewClient(api.URL, "1", k)
	ctx := context.Background()
	tok, err := c.InstallationToken(ctx, 42)
	if err != nil || tok != "ghs_1" {
		t.Fatalf("%q %v", tok, err)
	}
	if tok, _ := c.InstallationToken(ctx, 42); tok != "ghs_1" || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("not cached: %q, %d calls", tok, calls)
	}
	// Near expiry, a fresh one.
	c.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if tok, _ := c.InstallationToken(ctx, 42); tok != "ghs_2" {
		t.Fatalf("not refreshed: %q", tok)
	}
	if _, err := c.InstallationToken(ctx, 7); err == nil {
		t.Fatal("unknown installation succeeded")
	} else if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("error: %v", err)
	}
}
