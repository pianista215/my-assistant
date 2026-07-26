package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/display"
	"github.com/pianista215/my-assistant/internal/weather"
	"github.com/pianista215/my-assistant/internal/weeklymenu"
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

func TestHandleDisplayRequiresValidBattery(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"missing", ""},
		{"non numeric", "?battery=abc"},
		{"zero", "?battery=0"},
		{"over 100", "?battery=101"},
		{"negative", "?battery=-5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/display"+tc.query, nil)
			req.Header.Set("Authorization", "Bearer correct-token")
			rec := httptest.NewRecorder()

			srv.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleDisplayReturnsEncodedImage(t *testing.T) {
	somePoints := []weather.HourPoint{{Time: time.Now(), TempC: 20, Code: 0}}

	cases := []struct {
		name            string
		demoTime        string // "" leaves phase to whatever the real clock is; the shopping-list-specific cases below pin it to midday so they're deterministic.
		calendarFetcher CalendarFetcher
		shoppingFetcher ShoppingListFetcher
		menuFetcher     MenuFetcher
		weatherFetcher  WeatherFetcher
	}{
		{
			"today's agenda",
			"", fakeCalendarFetcher{rows: []calendar.Row{
				{Summary: "Dentist", Start: time.Now(), End: time.Now().Add(30 * time.Minute)},
			}},
			fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{points: somePoints},
		},
		{"empty agenda", "", fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{points: somePoints}},
		{"calendar fetch error", "", fakeCalendarFetcher{err: errors.New("boom")}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{points: somePoints}},
		{
			"shopping list items",
			"17:00", fakeCalendarFetcher{}, fakeShoppingListFetcher{items: []string{"Leche", "Pan"}}, fakeMenuFetcher{}, fakeWeatherFetcher{points: somePoints},
		},
		{
			"empty shopping list falls back to weather",
			"17:00", fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{points: somePoints},
		},
		{
			"shopping list fetch error",
			"17:00", fakeCalendarFetcher{}, fakeShoppingListFetcher{err: errors.New("boom")}, fakeMenuFetcher{}, fakeWeatherFetcher{points: somePoints},
		},
		{
			"weekly menu days",
			"", fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{week: []weeklymenu.Day{
				{Label: "Lunes", Lunch: []string{"Lentejas"}, Dinner: []string{"Tortilla"}},
			}}, fakeWeatherFetcher{points: somePoints},
		},
		{"empty weekly menu", "", fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{points: somePoints}},
		{"weekly menu fetch error", "", fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{err: errors.New("boom")}, fakeWeatherFetcher{points: somePoints}},
		{"weather fetch error (morning)", "10:00", fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{err: errors.New("boom")}},
		{"weather panel (morning)", "10:00", fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{points: somePoints}},
		{"night phase, tomorrow's agenda/menu", "22:00", fakeCalendarFetcher{}, fakeShoppingListFetcher{}, fakeMenuFetcher{}, fakeWeatherFetcher{points: somePoints}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServerWithFetchers(t, tc.calendarFetcher, tc.shoppingFetcher, tc.menuFetcher, tc.weatherFetcher)

			target := "/api/v1/display?battery=87"
			if tc.demoTime != "" {
				target += "&demo_time=" + tc.demoTime
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
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

func TestHandleDisplayRequiresValidDemoTime(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/display?battery=87&demo_time=not-a-time", nil)
	req.Header.Set("Authorization", "Bearer correct-token")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleDisplayNightPhaseClearsOutgoingMenuOnce(t *testing.T) {
	var cleared []time.Weekday
	menuFetcher := fakeMenuFetcher{cleared: &cleared}
	srv := newTestServerWithFetchers(t, fakeCalendarFetcher{}, fakeShoppingListFetcher{}, menuFetcher, fakeWeatherFetcher{points: []weather.HourPoint{{Time: time.Now(), TempC: 20, Code: 0}}})

	for _, demoTime := range []string{"21:00", "22:00", "23:00"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/display?battery=87&demo_time="+demoTime, nil)
		req.Header.Set("Authorization", "Bearer correct-token")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("demo_time=%s: status = %d, want %d", demoTime, rec.Code, http.StatusOK)
		}
	}

	if len(cleared) != 1 {
		t.Fatalf("cleared = %v, want exactly one call", cleared)
	}
	wantDay := time.Now().Weekday()
	if cleared[0] != wantDay {
		t.Fatalf("cleared[0] = %v, want %v", cleared[0], wantDay)
	}
}

func TestHandleDisplayNonNightPhaseNeverClearsMenu(t *testing.T) {
	var cleared []time.Weekday
	menuFetcher := fakeMenuFetcher{cleared: &cleared}
	srv := newTestServerWithFetchers(t, fakeCalendarFetcher{}, fakeShoppingListFetcher{}, menuFetcher, fakeWeatherFetcher{points: []weather.HourPoint{{Time: time.Now(), TempC: 20, Code: 0}}})

	for _, demoTime := range []string{"10:00", "17:00"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/display?battery=87&demo_time="+demoTime, nil)
		req.Header.Set("Authorization", "Bearer correct-token")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("demo_time=%s: status = %d, want %d", demoTime, rec.Code, http.StatusOK)
		}
	}

	if len(cleared) != 0 {
		t.Fatalf("cleared = %v, want no calls", cleared)
	}
}

func TestAgendaSectionEmptyFallsBackToPlainLines(t *testing.T) {
	sec := agendaSection(nil)
	if sec.Title != "Eventos" {
		t.Errorf("Title = %q, want %q", sec.Title, "Eventos")
	}
	if len(sec.Events) != 0 {
		t.Errorf("Events = %v, want empty", sec.Events)
	}
	if len(sec.Lines) != 1 || sec.Lines[0] != "No hay eventos" {
		t.Errorf("Lines = %v, want [\"No hay eventos\"]", sec.Lines)
	}
}

func TestAgendaSectionBuildsEventsWithSeparateTimeAndText(t *testing.T) {
	base := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	rows := []calendar.Row{
		{Summary: "Dentist", Start: base, End: base.Add(30 * time.Minute)},
	}

	sec := agendaSection(rows)
	if len(sec.Lines) != 0 {
		t.Errorf("Lines = %v, want empty", sec.Lines)
	}
	if len(sec.Events) != 1 {
		t.Fatalf("len(Events) = %d, want 1", len(sec.Events))
	}
	if sec.Events[0].Time != "09:00-09:30" || sec.Events[0].Text != "Dentist" {
		t.Errorf("Events[0] = %+v, want Time=09:00-09:30 Text=Dentist", sec.Events[0])
	}
}

func TestPhaseForBoundaries(t *testing.T) {
	cases := []struct {
		hour int
		want dayPhase
	}{
		{0, phaseMorning},
		{14, phaseMorning},
		{15, phaseMidday},
		{20, phaseMidday},
		{21, phaseNight},
		{23, phaseNight},
	}
	for _, tc := range cases {
		now := time.Date(2026, 7, 25, tc.hour, 0, 0, 0, time.UTC)
		if got := phaseFor(now); got != tc.want {
			t.Errorf("phaseFor(hour=%d) = %v, want %v", tc.hour, got, tc.want)
		}
	}
}

func TestParseDemoTime(t *testing.T) {
	loc := time.UTC

	got, err := parseDemoTime("22:30", loc)
	if err != nil {
		t.Fatalf("parseDemoTime() error = %v", err)
	}
	if got.Hour() != 22 || got.Minute() != 30 {
		t.Fatalf("parsed = %v, want hour=22 minute=30", got)
	}
	today := time.Now().In(loc)
	if got.Year() != today.Year() || got.Month() != today.Month() || got.Day() != today.Day() {
		t.Fatalf("parsed date = %v, want today (%v)", got, today)
	}

	invalid := []string{"", "25:00", "abc", "10:70", "10"}
	for _, raw := range invalid {
		if _, err := parseDemoTime(raw, loc); err == nil {
			t.Errorf("parseDemoTime(%q) expected an error", raw)
		}
	}
}
