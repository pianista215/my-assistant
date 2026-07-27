// Package server implements the HTTP API the ESP32 polls to know what to
// display.
package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/config"
	"github.com/pianista215/my-assistant/internal/weather"
	"github.com/pianista215/my-assistant/internal/weeklymenu"
)

// CalendarFetcher returns a day's agenda rows: FetchToday for today,
// FetchForDay for an arbitrary reference day (used by the night phase to
// show tomorrow's agenda instead). Satisfied in production by
// internal/calendar.Client; tests can supply a fake instead of hitting the
// network.
type CalendarFetcher interface {
	FetchToday(ctx context.Context) ([]calendar.Row, error)
	FetchForDay(ctx context.Context, day time.Time) ([]calendar.Row, error)
}

// ShoppingListFetcher returns the current shopping list items. Satisfied
// in production by internal/shoppinglist.Client; tests can supply a fake
// instead of hitting the network.
type ShoppingListFetcher interface {
	FetchItems(ctx context.Context) ([]string, error)
}

// MenuFetcher returns the current week's menu: FetchWeek rotated to start
// at today, FetchWeekFrom rotated to start at an arbitrary reference day
// (used by the night phase the same way CalendarFetcher.FetchForDay is).
// ClearDay blanks one weekday's entries, used to reset the outgoing day's
// menu once the night phase is reached. Satisfied in production by
// internal/weeklymenu.Client; tests can supply a fake instead of hitting
// the network.
type MenuFetcher interface {
	FetchWeek(ctx context.Context) ([]weeklymenu.Day, error)
	FetchWeekFrom(ctx context.Context, day time.Time) ([]weeklymenu.Day, error)
	ClearDay(ctx context.Context, day time.Weekday) error
}

// WeatherFetcher returns the hourly forecast for the configured location.
// Satisfied in production by internal/weather.Client; tests can supply a
// fake instead of hitting the network.
type WeatherFetcher interface {
	FetchForecast(ctx context.Context) ([]weather.HourPoint, error)
}

// TLSInfo carries the running server's TLS certificate details, for the
// /api/v1/tls-cert endpoint. The zero value (Fingerprint == "") means the
// server is running plain HTTP — routes() uses that to decide whether to
// register the endpoint at all, since there's no certificate to report
// otherwise. Server never generates or parses the certificate itself
// (that's cmd/server's job, see cmd/server/tls.go) — it only serves
// whatever strings it's handed, the same shallow-parameter style as the
// fetcher interfaces above.
type TLSInfo struct {
	// Fingerprint is the certificate's SHA-256 fingerprint, formatted as
	// colon-separated uppercase hex (the same format `openssl x509
	// -fingerprint -sha256` prints).
	Fingerprint string
	// CertPEM is the certificate's raw PEM text — what actually gets
	// embedded in ESP32 firmware (e.g. via WiFiClientSecure::setCACert()
	// on Arduino, or esp_tls_cfg_t.cacert_buf on ESP-IDF) to trust it.
	CertPEM string
}

type Server struct {
	cfg          *config.Config
	calendar     CalendarFetcher
	shoppingList ShoppingListFetcher
	menu         MenuFetcher
	weather      WeatherFetcher
	tls          TLSInfo
	mux          *http.ServeMux

	// menuClearMu guards menuClearedDate, the date (in cfg.Location) the
	// outgoing day's menu was last cleared, so the night phase's ~3
	// hourly polls only trigger one clear per calendar day instead of
	// repeatedly wiping an entry the user just refilled for next week.
	menuClearMu     sync.Mutex
	menuClearedDate string
}

func New(cfg *config.Config, calendarFetcher CalendarFetcher, shoppingListFetcher ShoppingListFetcher, menuFetcher MenuFetcher, weatherFetcher WeatherFetcher, tlsInfo TLSInfo) *Server {
	s := &Server{cfg: cfg, calendar: calendarFetcher, shoppingList: shoppingListFetcher, menu: menuFetcher, weather: weatherFetcher, tls: tlsInfo, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
