package server

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

	"golang.org/x/oauth2"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/config"
	"github.com/pianista215/my-assistant/internal/display"
	"github.com/pianista215/my-assistant/internal/oauthrenewal"
	"github.com/pianista215/my-assistant/internal/weather"
)

// invalidGrantErr is what a dead/revoked refresh token surfaces as,
// wrapped the same way internal/calendar/shoppinglist/weeklymenu wrap
// their underlying API errors with %w.
var invalidGrantErr = fmt.Errorf("calendar: listing events: %w", &oauth2.RetrieveError{ErrorCode: "invalid_grant"})

// fakeDeviceAuthServer serves a minimal, working Device Authorization
// Grant (device-code + token endpoints), so tests can exercise
// Server.handleReauth's real polling/renewal orchestration without
// hitting Google. Mirrors internal/oauthrenewal's own test double.
type fakeDeviceAuthServer struct {
	*httptest.Server
	tokenAttempts      atomic.Int32
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

func (f *fakeDeviceAuthServer) endpoint() oauth2.Endpoint {
	return oauth2.Endpoint{
		DeviceAuthURL: f.URL + "/device/code",
		TokenURL:      f.URL + "/token",
	}
}

// newReauthTestServer builds a Server with a fake calendar fetcher that
// always fails with invalid_grant, a credentials file under t.TempDir(),
// a GoogleClientsBuilder that just returns fresh fakes (recording that it
// was called), and startDeviceAuth wired to fake's endpoint instead of
// the real Google one.
func newReauthTestServer(t *testing.T, fake *fakeDeviceAuthServer) (srv *Server, rebuildCount *int32, credentialsPath string) {
	t.Helper()
	credentialsPath = filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(credentialsPath, []byte(`{"client_id":"device-client-id","client_secret":"device-client-secret","refresh_token":"dead","type":"authorized_user"}`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{AuthToken: "correct-token", Port: "0", Location: time.UTC, GoogleCredentialsFile: credentialsPath}
	srv = New(cfg, fakeCalendarFetcher{err: invalidGrantErr}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{points: []weather.HourPoint{{Time: time.Now(), TempC: 20, Code: 0}}}, TLSInfo{})

	var calls int32
	rebuildCount = &calls
	srv.SetGoogleClientsBuilder(func(ctx context.Context, credentialsFile string) (CalendarFetcher, ShoppingListFetcher, MenuFetcher, error) {
		atomic.AddInt32(&calls, 1)
		return fakeCalendarFetcher{rows: []calendar.Row{{Summary: "Renewed", Start: time.Now(), End: time.Now()}}}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, nil
	})

	if fake != nil {
		endpoint := fake.endpoint()
		srv.startDeviceAuth = func(ctx context.Context, credentialsFile string) (*oauthrenewal.PendingAuth, error) {
			return oauthrenewal.StartDeviceAuthWithEndpoint(ctx, "device-client-id", "device-client-secret", endpoint)
		}
	}

	return srv, rebuildCount, credentialsPath
}

func TestHandleDisplayInvalidGrantWithoutBuilderFallsBackToPlainError(t *testing.T) {
	// Regression guard: a Server that never calls SetGoogleClientsBuilder
	// (every pre-existing test, and any future caller that forgets to
	// wire it) must behave exactly as it did before this feature existed
	// — the generic calendar-error screen, no attempt at renewal.
	srv := newTestServerWithFetchers(t, fakeCalendarFetcher{err: invalidGrantErr}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{points: []weather.HourPoint{{Time: time.Now(), TempC: 20, Code: 0}}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/display?battery=87", nil)
	req.Header.Set("Authorization", "Bearer correct-token")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	img, err := display.Decode(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if img.Width != display.Width || img.Height != display.Height {
		t.Fatalf("dimensions = %dx%d, want %dx%d", img.Width, img.Height, display.Width, display.Height)
	}
}

func TestHandleDisplayInvalidGrantShowsReauthScreenWhenPending(t *testing.T) {
	fake := newFakeDeviceAuthServer(100) // never succeeds within this test
	defer fake.Close()
	srv, rebuildCount, _ := newReauthTestServer(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/display?battery=87", nil)
	req.Header.Set("Authorization", "Bearer correct-token")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if atomic.LoadInt32(rebuildCount) != 0 {
		t.Fatalf("rebuildCount = %d, want 0 (nothing should have been approved yet)", *rebuildCount)
	}

	srv.reauthMu.Lock()
	pending := srv.reauthPending
	srv.reauthMu.Unlock()
	if pending == nil {
		t.Fatal("srv.reauthPending is nil after a request that should have started a device authorization")
	}
	if pending.VerificationURL != "https://example.org/device" || pending.UserCode != "ABCD-EFGH" {
		t.Fatalf("pending = %+v, want the fake server's verification URL/user code", pending)
	}
}

func TestHandleDisplayInvalidGrantCompletesRenewalAndRetries(t *testing.T) {
	// tokenSucceedsAfter=0: the very first poll already succeeds, so a
	// single request should both discover the pending is done and retry
	// the calendar fetch against the freshly rebuilt (fake) clients.
	fake := newFakeDeviceAuthServer(0)
	defer fake.Close()
	srv, rebuildCount, credentialsPath := newReauthTestServer(t, fake)

	// Seed a pending auth as if a previous request had already started
	// one (handleReauth's "poll an existing one" branch) — otherwise this
	// first request would only start the device auth and poll nothing.
	pending, err := oauthrenewal.StartDeviceAuthWithEndpoint(context.Background(), "device-client-id", "device-client-secret", fake.endpoint())
	if err != nil {
		t.Fatalf("seeding pending auth: %v", err)
	}
	srv.reauthMu.Lock()
	srv.reauthPending = pending
	srv.reauthMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/display?battery=87", nil)
	req.Header.Set("Authorization", "Bearer correct-token")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if atomic.LoadInt32(rebuildCount) != 1 {
		t.Fatalf("rebuildCount = %d, want exactly 1", *rebuildCount)
	}

	srv.reauthMu.Lock()
	stillPending := srv.reauthPending
	srv.reauthMu.Unlock()
	if stillPending != nil {
		t.Fatalf("srv.reauthPending = %+v, want nil after a successful renewal", stillPending)
	}

	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatalf("reading rewritten credentials file: %v", err)
	}
	var stored struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.RefreshToken != "refresh-token-xyz" {
		t.Fatalf("stored refresh_token = %q, want refresh-token-xyz", stored.RefreshToken)
	}
}

func TestCompleteReauthPersistsAndSwapsClients(t *testing.T) {
	credentialsPath := filepath.Join(t.TempDir(), "credentials.json")
	cfg := &config.Config{AuthToken: "t", Location: time.UTC, GoogleCredentialsFile: credentialsPath}
	srv := New(cfg, fakeCalendarFetcher{err: errors.New("stale")}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{}, TLSInfo{})

	newCalendar := fakeCalendarFetcher{rows: []calendar.Row{{Summary: "Fresh"}}}
	srv.SetGoogleClientsBuilder(func(ctx context.Context, credentialsFile string) (CalendarFetcher, ShoppingListFetcher, MenuFetcher, error) {
		if credentialsFile != credentialsPath {
			t.Errorf("rebuildClients called with %q, want %q", credentialsFile, credentialsPath)
		}
		return newCalendar, fakeShoppingListFetcher{}, fakeMenuFetcher{}, nil
	})

	pending := &oauthrenewal.PendingAuth{ClientID: "cid", ClientSecret: "csecret"}
	token := &oauth2.Token{RefreshToken: "renewed-refresh-token"}

	if err := srv.completeReauth(context.Background(), pending, token); err != nil {
		t.Fatalf("completeReauth: %v", err)
	}

	clients := srv.clients.Load()
	gotRows, err := clients.calendar.FetchToday(context.Background())
	if err != nil || len(gotRows) != 1 || gotRows[0].Summary != "Fresh" {
		t.Fatalf("clients.calendar not swapped to the rebuilt fetcher: rows=%v err=%v", gotRows, err)
	}

	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatalf("reading credentials file: %v", err)
	}
	var stored struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.ClientID != "cid" || stored.ClientSecret != "csecret" || stored.RefreshToken != "renewed-refresh-token" {
		t.Fatalf("stored credentials = %+v, want ClientID=cid ClientSecret=csecret RefreshToken=renewed-refresh-token", stored)
	}
}

func TestCompleteReauthPropagatesRebuildError(t *testing.T) {
	credentialsPath := filepath.Join(t.TempDir(), "credentials.json")
	cfg := &config.Config{AuthToken: "t", Location: time.UTC, GoogleCredentialsFile: credentialsPath}
	srv := New(cfg, fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{}, TLSInfo{})
	srv.SetGoogleClientsBuilder(func(ctx context.Context, credentialsFile string) (CalendarFetcher, ShoppingListFetcher, MenuFetcher, error) {
		return nil, nil, nil, errors.New("boom")
	})

	pending := &oauthrenewal.PendingAuth{ClientID: "cid", ClientSecret: "csecret"}
	token := &oauth2.Token{RefreshToken: "renewed-refresh-token"}
	if err := srv.completeReauth(context.Background(), pending, token); err == nil {
		t.Fatal("completeReauth returned nil error when rebuildClients failed")
	}
}
