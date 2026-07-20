// Package server implements the HTTP API the ESP32 polls to know what to
// display.
package server

import (
	"context"
	"net/http"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/config"
)

// CalendarFetcher returns today's agenda rows. Satisfied in production by
// internal/calendar.FetchToday bound to the real Google Calendar client;
// tests can supply a fake instead of hitting the network.
type CalendarFetcher interface {
	FetchToday(ctx context.Context) ([]calendar.Row, error)
}

type Server struct {
	cfg      *config.Config
	calendar CalendarFetcher
	mux      *http.ServeMux
}

func New(cfg *config.Config, fetcher CalendarFetcher) *Server {
	s := &Server{cfg: cfg, calendar: fetcher, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
