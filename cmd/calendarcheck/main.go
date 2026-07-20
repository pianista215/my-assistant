// Command calendarcheck dumps today's raw events from the configured
// reference calendar as JSON, for inspecting how Google actually
// represents reminders vs. events vs. all-day items, and for general
// debugging of the calendar data feeding the display. It talks to the
// Calendar API directly (bypassing internal/calendar's Row abstraction)
// since the whole point is to see the raw shape Google sends.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	googlecalendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/pianista215/my-assistant/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	svc, err := googlecalendar.NewService(ctx,
		option.WithCredentialsFile(cfg.GoogleCredentialsFile),
		option.WithScopes(googlecalendar.CalendarReadonlyScope),
	)
	if err != nil {
		log.Fatalf("calendar: creating client: %v", err)
	}

	now := time.Now().In(cfg.Location)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cfg.Location)
	dayEnd := dayStart.Add(24 * time.Hour)

	events, err := svc.Events.List(cfg.CalendarID).
		Context(ctx).
		TimeMin(dayStart.Format(time.RFC3339)).
		TimeMax(dayEnd.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Do()
	if err != nil {
		log.Fatalf("calendar: listing events: %v", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(events.Items); err != nil {
		log.Fatalf("encoding output: %v", err)
	}
}
