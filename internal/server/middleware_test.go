package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/config"
)

// fakeCalendarFetcher lets tests control what handleDisplay sees without
// making a real Google Calendar API call.
type fakeCalendarFetcher struct {
	rows []calendar.Row
	err  error
}

func (f fakeCalendarFetcher) FetchToday(ctx context.Context) ([]calendar.Row, error) {
	return f.rows, f.err
}

// fakeShoppingListFetcher lets tests control what handleDisplay sees
// without making a real Google Sheets API call.
type fakeShoppingListFetcher struct {
	items []string
	err   error
}

func (f fakeShoppingListFetcher) FetchItems(ctx context.Context) ([]string, error) {
	return f.items, f.err
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return newTestServerWithFetchers(t, fakeCalendarFetcher{}, fakeShoppingListFetcher{})
}

func newTestServerWithFetchers(t *testing.T, calendarFetcher CalendarFetcher, shoppingListFetcher ShoppingListFetcher) *Server {
	t.Helper()
	cfg := &config.Config{AuthToken: "correct-token", Port: "0", Location: time.UTC}
	return New(cfg, calendarFetcher, shoppingListFetcher)
}

func TestRequireAuth(t *testing.T) {
	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"missing header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic correct-token", http.StatusUnauthorized},
		{"wrong token", "Bearer wrong-token", http.StatusUnauthorized},
		{"correct token", "Bearer correct-token", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)

			called := false
			protected := srv.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/display", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()

			protected.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			wantCalled := tc.wantStatus == http.StatusOK
			if called != wantCalled {
				t.Fatalf("next handler called = %v, want %v", called, wantCalled)
			}
		})
	}
}
