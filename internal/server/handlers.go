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
	"github.com/pianista215/my-assistant/internal/weather"
	"github.com/pianista215/my-assistant/internal/weeklymenu"
)

// DefaultVisibleDays caps how many of the rotated week's days actually
// render below the top columns — today (or, in the night phase,
// tomorrow) plus the next day. Deliberately far below the full week (7):
// with up to 5 lunch + 5 dinner entries per day, 7 days doesn't fit
// legibly in the space left below the top columns. This is a first pass
// to validate the integration end-to-end; expected to grow once the
// layout is fine-tuned by time-of-day in a later iteration — kept as one
// constant specifically so that revision is a one-line change.
const DefaultVisibleDays = 2

// WeatherMaxForecastEntries caps how many rows the weather panel's small
// forecast list shows — see weather.Summarize, which reduces the raw
// hourly forecast to this many representative entries once it no longer
// fits one row per hour.
const WeatherMaxForecastEntries = 4

// dayPhase selects which panel handleDisplay renders. phaseMorning and
// phaseNight always show the weather panel; phaseMidday normally shows
// the shopping list, except handleDisplay falls back to weather there too
// when the list turns out to be empty.
type dayPhase int

const (
	phaseMorning dayPhase = iota // 00:00-15:00: agenda | weather, today
	phaseMidday                  // 15:00-21:00: agenda | shopping list (or weather if empty), today
	phaseNight                   // 21:00-00:00: agenda | weather, both referencing tomorrow
)

// phaseFor picks the phase for the given wall-clock time.
func phaseFor(now time.Time) dayPhase {
	switch h := now.Hour(); {
	case h < 15:
		return phaseMorning
	case h < 21:
		return phaseMidday
	default:
		return phaseNight
	}
}

// handleDisplay serves the image the ESP32 should render: the agenda in a
// left column, and in the right column either the current shopping list
// (phaseMidday, unless it's empty) or a visual weather panel (every other
// case) — or a rendered error message if the calendar couldn't be fetched
// (so a broken integration is visible on the panel itself, rather than a
// stale image or a bare error status). A shopping-list, weather, or
// weekly-menu fetch failure is less critical — it doesn't take down the
// whole screen, just replaces that section with an error line, so the
// rest of the display stays visible. In the night phase, the agenda and
// weekly menu both reference tomorrow instead of today, and the header
// reads "Mañana, <date>" via tomorrowHeaderDate instead of
// spanishHeaderDate.
func (s *Server) handleDisplay(w http.ResponseWriter, r *http.Request) {
	now := time.Now().In(s.cfg.Location)
	if raw := r.URL.Query().Get("demo_time"); raw != "" {
		override, err := parseDemoTime(raw, s.cfg.Location)
		if err != nil {
			log.Printf("server: %v", err)
			http.Error(w, "invalid demo_time", http.StatusBadRequest)
			return
		}
		now = override
	}

	battery, err := parseBatteryPercent(r.URL.Query().Get("battery"))
	if err != nil {
		log.Printf("server: %v", err)
		http.Error(w, "invalid battery", http.StatusBadRequest)
		return
	}
	footer := fmt.Sprintf("%s - %d%%", now.Format("15:04:05"), battery)

	phase := phaseFor(now)
	header := spanishHeaderDate(now)
	referenceDay := now
	if phase == phaseNight {
		referenceDay = now.AddDate(0, 0, 1)
		header = tomorrowHeaderDate(referenceDay)
	}

	var rows []calendar.Row
	if phase == phaseNight {
		rows, err = s.calendar.FetchForDay(r.Context(), referenceDay)
	} else {
		rows, err = s.calendar.FetchToday(r.Context())
	}

	var img *display.GrayImage
	if err != nil {
		log.Printf("server: fetching calendar: %v", err)
		img = display.NewTextRows("Could not load calendar", footer, []string{
			now.Format("2006-01-02 15:04:05"),
			err.Error(),
		})
	} else {
		var days []weeklymenu.Day
		var menuErr error
		if phase == phaseNight {
			days, menuErr = s.menu.FetchWeekFrom(r.Context(), referenceDay)
		} else {
			days, menuErr = s.menu.FetchWeek(r.Context())
		}
		if menuErr != nil {
			log.Printf("server: fetching weekly menu: %v", menuErr)
		}

		left := []display.Section{{Title: "Eventos", Lines: agendaLines(rows)}}
		bottom := menuSections(days, menuErr)

		showWeather := phase != phaseMidday
		var items []string
		var listErr error
		if phase == phaseMidday {
			items, listErr = s.shoppingList.FetchItems(r.Context())
			switch {
			case listErr != nil:
				log.Printf("server: fetching shopping list: %v", listErr)
			case len(items) == 0:
				showWeather = true
			}
		}

		switch {
		case !showWeather:
			img = display.NewDailyLayout(header, footer, left,
				[]display.Section{{Title: "Lista de la compra", Lines: shoppingListLines(items, listErr), Bulleted: true}},
				bottom,
			)
		default:
			img = s.buildWeatherImage(r, header, footer, left, bottom, now)
		}
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

// buildWeatherImage fetches the forecast and renders the weather panel
// variant of the daily layout, falling back to a plain error Section
// (through the ordinary NewDailyLayout) on a fetch failure or unusable
// data — the same non-fatal isolation as the shopping list/weekly menu.
func (s *Server) buildWeatherImage(r *http.Request, header, footer string, left, bottom []display.Section, now time.Time) *display.GrayImage {
	points, err := s.weather.FetchForecast(r.Context())
	if err != nil {
		log.Printf("server: fetching weather: %v", err)
		return display.NewDailyLayout(header, footer, left,
			[]display.Section{{Title: "Tiempo", Lines: []string{"No se pudo cargar"}}},
			bottom,
		)
	}

	panel, ok := buildWeatherPanel(points, now)
	if !ok {
		return display.NewDailyLayout(header, footer, left,
			[]display.Section{{Title: "Tiempo", Lines: []string{"No se pudo cargar"}}},
			bottom,
		)
	}
	return display.NewDailyLayoutWithWeather(header, footer, left, panel, bottom)
}

// buildWeatherPanel turns the raw hourly forecast into the
// display.WeatherPanel the weather column renders: weather.Summarize
// picks the current hour plus up to WeatherMaxForecastEntries
// representative upcoming hours, each mapped through weather.IconKeyFor
// to the icon key display.drawIcon expects. ok is false when there's
// nothing usable to show (empty or entirely-stale forecast).
func buildWeatherPanel(points []weather.HourPoint, now time.Time) (display.WeatherPanel, bool) {
	current, forecast, ok := weather.Summarize(points, now, WeatherMaxForecastEntries)
	if !ok {
		return display.WeatherPanel{}, false
	}

	hours := make([]display.WeatherHour, len(forecast))
	for i, p := range forecast {
		hours[i] = display.WeatherHour{
			Label:     fmt.Sprintf("%dh", p.Time.Hour()),
			TempC:     p.TempC,
			PrecipPct: p.PrecipPct,
			IconKey:   weather.IconKeyFor(p.Code),
		}
	}

	return display.WeatherPanel{
		Title: "Tiempo",
		Now: display.WeatherNow{
			TempC:   current.TempC,
			IconKey: weather.IconKeyFor(current.Code),
		},
		Hours: hours,
	}, true
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
// main header shown above the top columns for the morning/midday phases.
func spanishHeaderDate(now time.Time) string {
	weekday := spanishWeekdays[now.Weekday()]
	weekday = strings.ToUpper(weekday[:1]) + weekday[1:]
	month := spanishMonths[now.Month()-1]
	return fmt.Sprintf("%s, %d de %s de %d", weekday, now.Day(), month, now.Year())
}

// tomorrowHeaderDate formats day (the night phase's reference day, always
// tomorrow relative to the request) as e.g. "Mañana, 27 de julio de
// 2026" — spanishHeaderDate's format with the weekday name replaced by
// the word "Mañana" ("tomorrow" in Spanish), since the night phase's
// agenda/menu reference the next day rather than today.
func tomorrowHeaderDate(day time.Time) string {
	month := spanishMonths[day.Month()-1]
	return fmt.Sprintf("Mañana, %d de %s de %d", day.Day(), month, day.Year())
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

// parseDemoTime parses the optional "demo_time" query parameter: an
// "HH:MM" wall-clock time, combined with today's real date in loc. Exists
// purely for testing/preview (cmd/preview's --demo-time flag forwards
// it) — the ESP32 never sends it, so production requests always use the
// real time.Now() and this branch is never exercised.
func parseDemoTime(raw string, loc *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation("15:04", raw, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid demo_time %q: %w", raw, err)
	}
	today := time.Now().In(loc)
	return time.Date(today.Year(), today.Month(), today.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc), nil
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

// menuSections turns the fetched week into the Sections the bottom region
// renders: one Section per visible day (capped at DefaultVisibleDays),
// titled with the sheet-provided day label, with a "Comida: ..." and a
// "Cena: ..." line. Both lines are always present — a day with no planned
// entries for a meal still shows a placeholder — so every day's Section
// has the same, predictable shape.
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
