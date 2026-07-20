package server

import (
	"log"
	"net/http"
	"time"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/display"
)

// handleDisplay serves the image the ESP32 should render: today's agenda
// from the reference calendar followed by the current shopping list, or a
// rendered error message if the calendar couldn't be fetched (so a broken
// integration is visible on the panel itself, rather than a stale image
// or a bare error status). A shopping list fetch failure is less
// critical — it doesn't take down the whole screen, just replaces that
// section with an error line, so the agenda stays visible.
func (s *Server) handleDisplay(w http.ResponseWriter, r *http.Request) {
	now := time.Now().In(s.cfg.Location)

	var img *display.GrayImage
	rows, err := s.calendar.FetchToday(r.Context())
	if err != nil {
		log.Printf("server: fetching calendar: %v", err)
		img = display.NewTextRows("Could not load calendar", []string{
			now.Format("2006-01-02 15:04:05"),
			err.Error(),
		})
	} else {
		items, err := s.shoppingList.FetchItems(r.Context())
		if err != nil {
			log.Printf("server: fetching shopping list: %v", err)
		}
		img = display.NewSections(now.Format("Monday, 2 January 2006"), []display.Section{
			{Lines: agendaLines(rows)},
			{Title: "Lista de la compra", Lines: shoppingListLines(items, err)},
		})
	}

	data, err := display.Encode(img)
	if err != nil {
		log.Printf("server: encoding display image: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

func agendaLines(rows []calendar.Row) []string {
	if len(rows) == 0 {
		return []string{"No events today"}
	}
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = row.String()
	}
	return lines
}

func shoppingListLines(items []string, err error) []string {
	switch {
	case err != nil:
		return []string{"No se pudo cargar"}
	case len(items) == 0:
		return []string{"(vacía)"}
	default:
		return items
	}
}
