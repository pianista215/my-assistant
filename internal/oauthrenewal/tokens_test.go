package oauthrenewal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2"
)

// fakeRefreshServer is a token endpoint answering refresh_token grants
// with a new access token ("access-1", "access-2", ...) of the given
// lifetime, or with invalid_grant when dead is set.
type fakeRefreshServer struct {
	*httptest.Server
	refreshes atomic.Int32
}

func newFakeRefreshServer(t *testing.T, expiresIn int, dead bool) *fakeRefreshServer {
	t.Helper()
	f := &fakeRefreshServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "good-refresh" {
			t.Errorf("unexpected token request: %v %v", err, r.Form)
		}
		n := f.refreshes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if dead {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant", "error_description": "Token has been expired or revoked."})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("access-%d", n),
			"token_type":   "Bearer",
			"expires_in":   expiresIn,
		})
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeRefreshServer) endpoint() oauth2.Endpoint {
	return oauth2.Endpoint{TokenURL: f.URL + "/token"}
}

func TestBlockingTokenProviderReusesFreshToken(t *testing.T) {
	fake := newFakeRefreshServer(t, 3600, false)
	tp := newBlockingTokenProvider("cid", "csecret", "good-refresh", fake.endpoint())

	for i := 0; i < 3; i++ {
		tok, err := tp.Token(context.Background())
		if err != nil {
			t.Fatalf("Token #%d: %v", i, err)
		}
		if tok.Value != "access-1" {
			t.Fatalf("Token #%d = %q, want the cached access-1", i, tok.Value)
		}
	}
	if got := fake.refreshes.Load(); got != 1 {
		t.Fatalf("refreshes = %d, want 1", got)
	}
}

func TestBlockingTokenProviderRefreshesNearExpiryTokenSynchronously(t *testing.T) {
	// A 60s token is already inside the early-refresh window (3m45s), the
	// exact situation an hourly poll hits at the previous token's expiry:
	// the very next call must block on a refresh and return the *new*
	// token, never hand back the about-to-expire one (which the default
	// asynchronous cache does).
	fake := newFakeRefreshServer(t, 60, false)
	tp := newBlockingTokenProvider("cid", "csecret", "good-refresh", fake.endpoint())

	first, err := tp.Token(context.Background())
	if err != nil {
		t.Fatalf("first Token: %v", err)
	}
	second, err := tp.Token(context.Background())
	if err != nil {
		t.Fatalf("second Token: %v", err)
	}
	if first.Value != "access-1" || second.Value != "access-2" {
		t.Fatalf("tokens = %q, %q, want access-1, access-2", first.Value, second.Value)
	}
}

func TestBlockingTokenProviderDeadRefreshTokenIsReauthError(t *testing.T) {
	fake := newFakeRefreshServer(t, 3600, true)
	tp := newBlockingTokenProvider("cid", "csecret", "good-refresh", fake.endpoint())

	_, err := tp.Token(context.Background())
	if err == nil {
		t.Fatal("Token returned nil error for a dead refresh token")
	}
	if !IsReauthError(err) {
		t.Fatalf("IsReauthError(%v) = false, want true", err)
	}
}

func TestClientOptionRequiresRefreshToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"client_id":"cid","client_secret":"csecret","type":"authorized_user"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ClientOption(path); err == nil {
		t.Fatal("ClientOption returned nil error for a file without refresh_token")
	}
}
