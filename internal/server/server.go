// Package server implements the HTTP API the ESP32 polls to know what to
// display.
package server

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/config"
	"github.com/pianista215/my-assistant/internal/oauthrenewal"
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

// googleClients groups the three Google-credential-backed fetchers so
// they can be swapped in atomically after a successful oauthrenewal (see
// handleReauth/completeReauth in handlers.go). weather is excluded: it
// talks to Open-Meteo, which needs no credential at all, so it's never
// affected by a renewal.
//
// This indirection exists because option.WithCredentialsFile (used by
// internal/calendar/internal/shoppinglist/internal/weeklymenu's
// NewClient) reads the credentials file exactly once, at client
// construction time — overwriting the file on disk after a renewal does
// nothing for a *Client already built against the old (now-invalid)
// in-memory refresh token. The only way for the already-running process
// to actually start using a renewed credential is to construct brand new
// clients against the rewritten file and swap them in, which is what
// completeReauth does.
type googleClients struct {
	calendar     CalendarFetcher
	shoppingList ShoppingListFetcher
	menu         MenuFetcher
}

// GoogleClientsBuilder constructs the three Google-credential-backed
// fetchers from a credentials file — the same shape of call
// cmd/server/main.go already makes once at startup to build the fetchers
// passed into New. Server never imports internal/calendar/shoppinglist/
// weeklymenu directly (same as it never did before this feature — see
// the Fetcher interfaces above), so it asks the caller for this function
// instead of constructing clients itself.
type GoogleClientsBuilder func(ctx context.Context, credentialsFile string) (CalendarFetcher, ShoppingListFetcher, MenuFetcher, error)

type Server struct {
	cfg     *config.Config
	weather WeatherFetcher
	tls     TLSInfo
	mux     *http.ServeMux

	clients atomic.Pointer[googleClients]

	// rebuildClients is set via SetGoogleClientsBuilder, normally right
	// after New — kept as a post-construction setter rather than a New
	// parameter so every test/caller that doesn't care about renewal
	// (the overwhelming majority) is unaffected. A nil rebuildClients
	// (the default) simply means handleDisplay never attempts a renewal
	// on an expired credential — it falls back to the same plain error
	// screen as any other calendar-fetch failure.
	rebuildClients GoogleClientsBuilder

	// startDeviceAuth begins a new device authorization request — set to
	// oauthrenewal.StartDeviceAuth by New, and only ever overridden by
	// this package's own tests (to point handleReauth's cold-start path
	// at a fake Google instead of the real network, the same reason
	// oauthrenewal.StartDeviceAuthWithEndpoint exists). Not something
	// cmd/server ever needs to configure, unlike rebuildClients, so it
	// has no public setter.
	startDeviceAuth func(ctx context.Context, credentialsFile string) (*oauthrenewal.PendingAuth, error)

	// menuClearMu guards menuClearedDate, the date (in cfg.Location) the
	// outgoing day's menu was last cleared, so the night phase's ~3
	// hourly polls only trigger one clear per calendar day instead of
	// repeatedly wiping an entry the user just refilled for next week.
	menuClearMu     sync.Mutex
	menuClearedDate string

	// reauthMu guards reauthPending, the in-flight device authorization
	// request (if any) started the last time handleDisplay saw an
	// expired-credential error — see handleReauth. Kept across requests
	// (rather than started fresh on every request) so the same QR code
	// stays valid and pollable across the ESP32's hourly polls, instead
	// of abandoning a code the user might already be part-way through
	// scanning.
	reauthMu      sync.Mutex
	reauthPending *oauthrenewal.PendingAuth
}

func New(cfg *config.Config, calendarFetcher CalendarFetcher, shoppingListFetcher ShoppingListFetcher, menuFetcher MenuFetcher, weatherFetcher WeatherFetcher, tlsInfo TLSInfo) *Server {
	s := &Server{cfg: cfg, weather: weatherFetcher, tls: tlsInfo, mux: http.NewServeMux(), startDeviceAuth: oauthrenewal.StartDeviceAuth}
	s.clients.Store(&googleClients{calendar: calendarFetcher, shoppingList: shoppingListFetcher, menu: menuFetcher})
	s.routes()
	return s
}

// SetGoogleClientsBuilder installs the function handleDisplay uses to
// reconstruct calendar/shoppinglist/weeklymenu clients after a successful
// oauthrenewal. cmd/server/main.go calls this once, right after New, with
// a closure that mirrors the same three NewClient calls it already makes
// to build the fetchers passed into New itself.
func (s *Server) SetGoogleClientsBuilder(build GoogleClientsBuilder) {
	s.rebuildClients = build
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
