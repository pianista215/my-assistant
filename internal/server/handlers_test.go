package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/display"
)

func TestHandleDisplayRequiresToken(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/display", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleDisplayReturnsEncodedImage(t *testing.T) {
	cases := []struct {
		name            string
		calendarFetcher CalendarFetcher
		shoppingFetcher ShoppingListFetcher
	}{
		{
			"today's agenda",
			fakeCalendarFetcher{rows: []calendar.Row{
				{Summary: "Dentist", Start: time.Now(), End: time.Now().Add(30 * time.Minute)},
			}},
			fakeShoppingListFetcher{},
		},
		{"empty agenda", fakeCalendarFetcher{}, fakeShoppingListFetcher{}},
		{"calendar fetch error", fakeCalendarFetcher{err: errors.New("boom")}, fakeShoppingListFetcher{}},
		{
			"shopping list items",
			fakeCalendarFetcher{},
			fakeShoppingListFetcher{items: []string{"Leche", "Pan"}},
		},
		{"empty shopping list", fakeCalendarFetcher{}, fakeShoppingListFetcher{}},
		{"shopping list fetch error", fakeCalendarFetcher{}, fakeShoppingListFetcher{err: errors.New("boom")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServerWithFetchers(t, tc.calendarFetcher, tc.shoppingFetcher)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/display", nil)
			req.Header.Set("Authorization", "Bearer correct-token")
			rec := httptest.NewRecorder()

			srv.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
				t.Fatalf("Content-Type = %q, want application/octet-stream", ct)
			}

			img, err := display.Decode(rec.Body.Bytes())
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if img.Width != display.Width || img.Height != display.Height {
				t.Fatalf("dimensions = %dx%d, want %dx%d", img.Width, img.Height, display.Width, display.Height)
			}
		})
	}
}
