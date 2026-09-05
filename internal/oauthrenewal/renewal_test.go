package oauthrenewal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	googleauth "cloud.google.com/go/auth"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

func TestClientCredentialsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	token := &oauth2.Token{RefreshToken: "refresh-123"}
	if err := SaveCredentials(path, "client-id", "client-secret", token); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	gotID, gotSecret, err := ClientCredentials(path)
	if err != nil {
		t.Fatalf("ClientCredentials: %v", err)
	}
	if gotID != "client-id" || gotSecret != "client-secret" {
		t.Fatalf("ClientCredentials = (%q, %q), want (\"client-id\", \"client-secret\")", gotID, gotSecret)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	var stored storedCredentials
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("unmarshalling written file: %v", err)
	}
	if stored.RefreshToken != "refresh-123" || stored.Type != "authorized_user" {
		t.Fatalf("stored credentials = %+v, want refresh_token=refresh-123 type=authorized_user", stored)
	}
}

func TestClientCredentialsMissingFile(t *testing.T) {
	if _, _, err := ClientCredentials(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ClientCredentials on a missing file returned nil error")
	}
}

func TestClientCredentialsIncompleteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"type":"authorized_user"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ClientCredentials(path); err == nil {
		t.Fatal("ClientCredentials on a file with no client_id/client_secret returned nil error")
	}
}

func TestIsReauthErrorDetectsInvalidGrant(t *testing.T) {
	wrapped := fmt.Errorf("calendar: listing events: %w", &oauth2.RetrieveError{ErrorCode: "invalid_grant"})
	if !IsReauthError(wrapped) {
		t.Error("IsReauthError(invalid_grant) = false, want true")
	}
}

func TestIsReauthErrorIgnoresOtherRetrieveErrors(t *testing.T) {
	wrapped := fmt.Errorf("calendar: listing events: %w", &oauth2.RetrieveError{ErrorCode: "invalid_request"})
	if IsReauthError(wrapped) {
		t.Error("IsReauthError(invalid_request) = true, want false")
	}
}

func TestIsReauthErrorIgnoresPlainErrors(t *testing.T) {
	if IsReauthError(errors.New("network timeout")) {
		t.Error("IsReauthError(plain error) = true, want false")
	}
}

// TestIsReauthErrorDetectsGoogleAuthInvalidGrant is a regression test:
// live testing (letting a real refresh attempt happen against a revoked
// credential) surfaced a *cloud.google.com/go/auth.Error, formatted as
// `auth: "invalid_grant" "Token has been expired or revoked."` — a
// different type from golang.org/x/oauth2's own *oauth2.RetrieveError,
// since google.golang.org/api now routes token refresh through the newer
// library internally. Its `code`/`description` fields are unexported (no
// public constructor sets them from outside the package either), but
// `Body` is exported, so a real *googleauth.Error carrying the raw JSON
// response body can still be built directly here.
func TestIsReauthErrorDetectsGoogleAuthInvalidGrant(t *testing.T) {
	authErr := &googleauth.Error{
		Body: []byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`),
	}
	wrapped := fmt.Errorf("calendar: listing events: %w", authErr)
	if !IsReauthError(wrapped) {
		t.Error("IsReauthError(googleauth invalid_grant) = false, want true")
	}
}

func TestIsReauthErrorIgnoresOtherGoogleAuthErrors(t *testing.T) {
	authErr := &googleauth.Error{
		Body: []byte(`{"error":"invalid_request","error_description":"something else"}`),
	}
	wrapped := fmt.Errorf("calendar: listing events: %w", authErr)
	if IsReauthError(wrapped) {
		t.Error("IsReauthError(googleauth invalid_request) = true, want false")
	}
}

func TestTokenErrorCodeParsesRawBody(t *testing.T) {
	body := []byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`)
	if got := tokenErrorCode(body); got != "invalid_grant" {
		t.Fatalf("tokenErrorCode(%s) = %q, want invalid_grant", body, got)
	}
}

func TestTokenErrorCodeIgnoresUnparsableBody(t *testing.T) {
	if got := tokenErrorCode([]byte("not json")); got != "" {
		t.Fatalf("tokenErrorCode(garbage) = %q, want empty string", got)
	}
	if got := tokenErrorCode(nil); got != "" {
		t.Fatalf("tokenErrorCode(nil) = %q, want empty string", got)
	}
}

// TestIsReauthErrorDetectsGoogleAPI401 is a regression test: revoking
// access in a live test surfaced a *googleapi.Error{Code: 401} instead of
// the expected *oauth2.RetrieveError{ErrorCode: "invalid_grant"} — the
// access token cached by the transport hadn't itself expired yet, so it
// was sent straight to the Calendar API (and rejected there) without ever
// attempting a refresh.
func TestIsReauthErrorDetectsGoogleAPI401(t *testing.T) {
	wrapped := fmt.Errorf("calendar: listing events: %w", &googleapi.Error{
		Code:    401,
		Message: "Invalid Credentials",
	})
	if !IsReauthError(wrapped) {
		t.Error("IsReauthError(googleapi 401) = false, want true")
	}
}

func TestIsReauthErrorIgnoresOtherGoogleAPIErrors(t *testing.T) {
	wrapped := fmt.Errorf("calendar: listing events: %w", &googleapi.Error{
		Code:    403,
		Message: "Forbidden",
	})
	if IsReauthError(wrapped) {
		t.Error("IsReauthError(googleapi 403) = true, want false")
	}

	wrapped500 := fmt.Errorf("calendar: listing events: %w", &googleapi.Error{
		Code:    500,
		Message: "Internal error",
	})
	if IsReauthError(wrapped500) {
		t.Error("IsReauthError(googleapi 500) = true, want false")
	}
}

// fakeDeviceAuthServer serves both the device-code and token endpoints of
// a Device Authorization Grant, so StartDeviceAuth/Poll can be exercised
// end to end without hitting Google. tokenAttempts counts calls to
// /token so tests can make Poll return "authorization_pending" on the
// first attempt and succeed on a later one, like a real user completing
// the flow between two ESP32 polls.
type fakeDeviceAuthServer struct {
	*httptest.Server
	tokenAttempts atomic.Int32
	// tokenSucceedsAfter is how many /token calls return
	// authorization_pending before the next one succeeds.
	tokenSucceedsAfter int32
}

func newFakeDeviceAuthServer(tokenSucceedsAfter int32) *fakeDeviceAuthServer {
	f := &fakeDeviceAuthServer{tokenSucceedsAfter: tokenSucceedsAfter}
	mux := http.NewServeMux()
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "device-code-123",
			"user_code":        "ABCD-EFGH",
			"verification_uri": "https://example.org/device",
			"expires_in":       1800,
			"interval":         5,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		n := f.tokenAttempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n <= f.tokenSucceedsAfter {
			json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-token-xyz",
			"refresh_token": "refresh-token-xyz",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})
	f.Server = httptest.NewServer(mux)
	return f
}

func TestStartDeviceAuthAndPollFullFlow(t *testing.T) {
	fake := newFakeDeviceAuthServer(1)
	defer fake.Close()

	endpoint := oauth2.Endpoint{
		DeviceAuthURL: fake.URL + "/device/code",
		TokenURL:      fake.URL + "/token",
	}

	pending, err := StartDeviceAuthWithEndpoint(context.Background(), "test-client-id", "test-client-secret", endpoint)
	if err != nil {
		t.Fatalf("startDeviceAuth: %v", err)
	}
	if pending.DeviceCode != "device-code-123" || pending.UserCode != "ABCD-EFGH" {
		t.Fatalf("pending = %+v, want DeviceCode=device-code-123 UserCode=ABCD-EFGH", pending)
	}
	if !pending.NeedsUserCode || pending.VerificationURL != "https://example.org/device" {
		t.Fatalf("pending.NeedsUserCode/VerificationURL = %v/%q, want true/https://example.org/device", pending.NeedsUserCode, pending.VerificationURL)
	}
	if pending.Expired() {
		t.Fatal("pending.Expired() = true immediately after StartDeviceAuth")
	}

	// First poll: still pending (the fake user hasn't "approved" yet).
	token, done, err := pending.Poll(context.Background())
	if err != nil || done || token != nil {
		t.Fatalf("first Poll = (%v, %v, %v), want (nil, false, nil)", token, done, err)
	}

	// Second poll: approved.
	token, done, err = pending.Poll(context.Background())
	if err != nil {
		t.Fatalf("second Poll error: %v", err)
	}
	if !done || token == nil {
		t.Fatalf("second Poll = (%v, %v, %v), want a token with done=true", token, done, err)
	}
	if token.RefreshToken != "refresh-token-xyz" {
		t.Fatalf("token.RefreshToken = %q, want refresh-token-xyz", token.RefreshToken)
	}
}

func TestPollAccessDenied(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": "access_denied"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	pending := &PendingAuth{
		config: &oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: server.URL + "/token"}},
	}
	token, done, err := pending.Poll(context.Background())
	if err == nil || !done || token != nil {
		t.Fatalf("Poll(access_denied) = (%v, %v, %v), want (nil, true, err)", token, done, err)
	}
}

func TestPollSuccessWithoutRefreshTokenIsAnError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "abc", "token_type": "Bearer", "expires_in": 3600})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	pending := &PendingAuth{
		config: &oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: server.URL + "/token"}},
	}
	token, done, err := pending.Poll(context.Background())
	if err == nil || !done || token != nil {
		t.Fatalf("Poll(no refresh_token) = (%v, %v, %v), want (nil, true, err)", token, done, err)
	}
}

func TestWaitForTokenSucceedsAfterPending(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 5 * time.Millisecond
	defer func() { waitPollInterval = old }()

	fake := newFakeDeviceAuthServer(2)
	defer fake.Close()
	endpoint := oauth2.Endpoint{DeviceAuthURL: fake.URL + "/device/code", TokenURL: fake.URL + "/token"}

	pending, err := StartDeviceAuthWithEndpoint(context.Background(), "test-client-id", "test-client-secret", endpoint)
	if err != nil {
		t.Fatalf("StartDeviceAuthWithEndpoint: %v", err)
	}

	token, err := pending.WaitForToken(context.Background())
	if err != nil {
		t.Fatalf("WaitForToken: %v", err)
	}
	if token.RefreshToken != "refresh-token-xyz" {
		t.Fatalf("token.RefreshToken = %q, want refresh-token-xyz", token.RefreshToken)
	}
}

func TestWaitForTokenReturnsErrorOnDenial(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 5 * time.Millisecond
	defer func() { waitPollInterval = old }()

	mux := http.NewServeMux()
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"device_code": "d", "user_code": "u", "verification_uri": "https://example.org/device",
			"expires_in": 1800, "interval": 5,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": "access_denied"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	pending, err := StartDeviceAuthWithEndpoint(context.Background(), "test-client-id", "test-client-secret",
		oauth2.Endpoint{DeviceAuthURL: server.URL + "/device/code", TokenURL: server.URL + "/token"})
	if err != nil {
		t.Fatalf("StartDeviceAuthWithEndpoint: %v", err)
	}

	if _, err := pending.WaitForToken(context.Background()); err == nil {
		t.Fatal("WaitForToken returned nil error after access_denied")
	}
}
