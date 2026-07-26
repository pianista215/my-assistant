package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/config"
	"github.com/pianista215/my-assistant/internal/weather"
	"github.com/pianista215/my-assistant/internal/weeklymenu"
)

// fakeCalendarFetcher lets tests control what handleDisplay sees without
// making a real Google Calendar API call. FetchForDay ignores day and
// returns the same fixed rows/err as FetchToday — tests that care about
// which day was requested assert on it separately, via a dedicated fake.
type fakeCalendarFetcher struct {
	rows []calendar.Row
	err  error
}

func (f fakeCalendarFetcher) FetchToday(ctx context.Context) ([]calendar.Row, error) {
	return f.rows, f.err
}

func (f fakeCalendarFetcher) FetchForDay(ctx context.Context, day time.Time) ([]calendar.Row, error) {
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

// fakeMenuFetcher lets tests control what handleDisplay sees without
// making a real Google Sheets API call. FetchWeekFrom ignores day and
// returns the same fixed week/err as FetchWeek, same rationale as
// fakeCalendarFetcher.FetchForDay above.
type fakeMenuFetcher struct {
	week []weeklymenu.Day
	err  error
}

func (f fakeMenuFetcher) FetchWeek(ctx context.Context) ([]weeklymenu.Day, error) {
	return f.week, f.err
}

func (f fakeMenuFetcher) FetchWeekFrom(ctx context.Context, day time.Time) ([]weeklymenu.Day, error) {
	return f.week, f.err
}

// fakeWeatherFetcher lets tests control what handleDisplay sees without
// making a real Open-Meteo API call.
type fakeWeatherFetcher struct {
	points []weather.HourPoint
	err    error
}

func (f fakeWeatherFetcher) FetchForecast(ctx context.Context) ([]weather.HourPoint, error) {
	return f.points, f.err
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return newTestServerWithFetchers(t, fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{})
}

func newTestServerWithFetchers(t *testing.T, calendarFetcher CalendarFetcher, shoppingListFetcher ShoppingListFetcher, menuFetcher MenuFetcher, weatherFetcher WeatherFetcher) *Server {
	t.Helper()
	cfg := &config.Config{AuthToken: "correct-token", Port: "0", Location: time.UTC}
	return New(cfg, calendarFetcher, shoppingListFetcher, menuFetcher, weatherFetcher)
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
