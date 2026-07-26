// Package server implements the HTTP API the ESP32 polls to know what to
// display.
package server

import (
	"context"
	"net/http"
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
// Satisfied in production by internal/weeklymenu.Client; tests can supply
// a fake instead of hitting the network.
type MenuFetcher interface {
	FetchWeek(ctx context.Context) ([]weeklymenu.Day, error)
	FetchWeekFrom(ctx context.Context, day time.Time) ([]weeklymenu.Day, error)
}

// WeatherFetcher returns the hourly forecast for the configured location.
// Satisfied in production by internal/weather.Client; tests can supply a
// fake instead of hitting the network.
type WeatherFetcher interface {
	FetchForecast(ctx context.Context) ([]weather.HourPoint, error)
}

type Server struct {
	cfg          *config.Config
	calendar     CalendarFetcher
	shoppingList ShoppingListFetcher
	menu         MenuFetcher
	weather      WeatherFetcher
	mux          *http.ServeMux
}

func New(cfg *config.Config, calendarFetcher CalendarFetcher, shoppingListFetcher ShoppingListFetcher, menuFetcher MenuFetcher, weatherFetcher WeatherFetcher) *Server {
	s := &Server{cfg: cfg, calendar: calendarFetcher, shoppingList: shoppingListFetcher, menu: menuFetcher, weather: weatherFetcher, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
