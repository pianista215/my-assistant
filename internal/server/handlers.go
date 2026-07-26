package server

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/display"
	"github.com/pianista215/my-assistant/internal/weeklymenu"
)

// DefaultVisibleDays caps how many of the rotated week's days actually
// render below the agenda/shopping list columns — today plus the next
// day. Deliberately far below the full week (7): with up to 5 lunch + 5
// dinner entries per day, 7 days doesn't fit legibly in the space left
// below the top columns. This is a first pass to validate the integration
// end-to-end; expected to grow once the layout is fine-tuned by
// time-of-day in a later iteration — kept as one constant specifically so
// that revision is a one-line change.
const DefaultVisibleDays = 2

// handleDisplay serves the image the ESP32 should render: today's agenda
// from the reference calendar in a left column, the current shopping
// list in a right column, and the next few days of the weekly menu below
// both — or a rendered error message if the calendar couldn't be fetched
// (so a broken integration is visible on the panel itself, rather than a
// stale image or a bare error status). A shopping list or weekly menu
// fetch failure is less critical — it doesn't take down the whole
// screen, just replaces that section with an error line, so the rest of
// the display stays visible.
func (s *Server) handleDisplay(w http.ResponseWriter, r *http.Request) {
	now := time.Now().In(s.cfg.Location)

	battery, err := parseBatteryPercent(r.URL.Query().Get("battery"))
	if err != nil {
		log.Printf("server: %v", err)
		http.Error(w, "invalid battery", http.StatusBadRequest)
		return
	}
	footer := fmt.Sprintf("%s - %d%%", now.Format("15:04:05"), battery)

	var img *display.GrayImage
	rows, err := s.calendar.FetchToday(r.Context())
	if err != nil {
		log.Printf("server: fetching calendar: %v", err)
		img = display.NewTextRows("Could not load calendar", footer, []string{
			now.Format("2006-01-02 15:04:05"),
			err.Error(),
		})
	} else {
		items, listErr := s.shoppingList.FetchItems(r.Context())
		if listErr != nil {
			log.Printf("server: fetching shopping list: %v", listErr)
		}
		days, menuErr := s.menu.FetchWeek(r.Context())
		if menuErr != nil {
			log.Printf("server: fetching weekly menu: %v", menuErr)
		}
		img = display.NewDailyLayout(spanishHeaderDate(now), footer,
			[]display.Section{{Title: "Eventos", Lines: agendaLines(rows)}},
			[]display.Section{{Title: "Lista de la compra", Lines: shoppingListLines(items, listErr), Bulleted: true}},
			menuSections(days, menuErr),
		)
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

// spanishWeekdays and spanishMonths translate time.Time's English-only
// Weekday()/Month() names, since the standard library has no locale
// support of its own — every other date the app deals with is either a
// fixed layout (agenda timestamps) or verbatim from the spreadsheet
// (weeklymenu.Day.Label), but the header is built from Go's time.Time
// here, so it needs an explicit translation.
var spanishWeekdays = [...]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}

var spanishMonths = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}

// spanishHeaderDate formats now as e.g. "Sábado, 25 de julio de 2026", the
// main header shown above the agenda/shopping-list columns.
func spanishHeaderDate(now time.Time) string {
	weekday := spanishWeekdays[now.Weekday()]
	weekday = strings.ToUpper(weekday[:1]) + weekday[1:]
	month := spanishMonths[now.Month()-1]
	return fmt.Sprintf("%s, %d de %s de %d", weekday, now.Day(), month, now.Year())
}

// parseBatteryPercent parses the ESP32's battery level from the "battery"
// query parameter, required on every request and range-checked as the
// device itself would report it: 1-100 (0 would mean it's already dead
// and couldn't have made the request).
func parseBatteryPercent(raw string) (int, error) {
	if raw == "" {
		return 0, fmt.Errorf("server: battery query parameter is required")
	}
	battery, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("server: invalid battery %q: %w", raw, err)
	}
	if battery < 1 || battery > 100 {
		return 0, fmt.Errorf("server: battery %d out of range 1-100", battery)
	}
	return battery, nil
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

// menuSections turns the fetched week into the Sections NewDailyLayout's
// bottom region renders: one Section per visible day (capped at
// DefaultVisibleDays), titled with the sheet-provided day label, with a
// "Comida: ..." and a "Cena: ..." line. Both lines are always present —
// a day with no planned entries for a meal still shows a placeholder —
// so every day's Section has the same, predictable shape.
func menuSections(days []weeklymenu.Day, err error) []display.Section {
	if err != nil {
		return []display.Section{{Title: "Menú semanal", Lines: []string{"No se pudo cargar"}}}
	}

	visible := days
	if len(visible) > DefaultVisibleDays {
		visible = visible[:DefaultVisibleDays]
	}

	sections := make([]display.Section, len(visible))
	for i, day := range visible {
		sections[i] = display.Section{
			Title: day.Label,
			Lines: []string{
				mealLine("Comida", day.Lunch),
				mealLine("Cena", day.Dinner),
			},
		}
	}
	return sections
}

func mealLine(label string, entries []string) string {
	if len(entries) == 0 {
		return label + ": (sin planificar)"
	}
	return label + ": " + strings.Join(entries, ", ")
}
