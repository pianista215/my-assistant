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

// ShoppingListFetcher returns the current shopping list items. Satisfied
// in production by internal/shoppinglist.Client; tests can supply a fake
// instead of hitting the network.
type ShoppingListFetcher interface {
	FetchItems(ctx context.Context) ([]string, error)
}

type Server struct {
	cfg          *config.Config
	calendar     CalendarFetcher
	shoppingList ShoppingListFetcher
	mux          *http.ServeMux
}

func New(cfg *config.Config, calendarFetcher CalendarFetcher, shoppingListFetcher ShoppingListFetcher) *Server {
	s := &Server{cfg: cfg, calendar: calendarFetcher, shoppingList: shoppingListFetcher, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
