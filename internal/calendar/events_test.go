package calendar

import (
	"testing"
	"time"

	googlecalendar "google.golang.org/api/calendar/v3"
)

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("time.LoadLocation(%q) error = %v", name, err)
	}
	return loc
}

func TestToRowClassification(t *testing.T) {
	loc := mustLoadLocation(t, "Europe/Madrid")

	cases := []struct {
		name         string
		item         *googlecalendar.Event
		wantAllDay   bool
		wantReminder bool
	}{
		{
			name: "reminder (start == end)",
			item: &googlecalendar.Event{
				Summary: "Take pills",
				Start:   &googlecalendar.EventDateTime{DateTime: "2026-07-19T09:00:00+02:00"},
				End:     &googlecalendar.EventDateTime{DateTime: "2026-07-19T09:00:00+02:00"},
			},
			wantReminder: true,
		},
		{
			name: "event with duration",
			item: &googlecalendar.Event{
				Summary: "Standup",
				Start:   &googlecalendar.EventDateTime{DateTime: "2026-07-19T10:00:00+02:00"},
				End:     &googlecalendar.EventDateTime{DateTime: "2026-07-19T10:30:00+02:00"},
			},
			wantReminder: false,
		},
		{
			name: "all-day event",
			item: &googlecalendar.Event{
				Summary: "Public holiday",
				Start:   &googlecalendar.EventDateTime{Date: "2026-07-19"},
				End:     &googlecalendar.EventDateTime{Date: "2026-07-20"},
			},
			wantAllDay: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row, err := toRow(tc.item, loc)
			if err != nil {
				t.Fatalf("toRow() error = %v", err)
			}
			if row.Summary != tc.item.Summary {
				t.Fatalf("Summary = %q, want %q", row.Summary, tc.item.Summary)
			}
			if row.AllDay != tc.wantAllDay {
				t.Fatalf("AllDay = %v, want %v", row.AllDay, tc.wantAllDay)
			}
			if row.IsReminder() != tc.wantReminder {
				t.Fatalf("IsReminder() = %v, want %v", row.IsReminder(), tc.wantReminder)
			}
		})
	}
}

func TestRowString(t *testing.T) {
	base := time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		row  Row
		want string
	}{
		{"reminder", Row{Summary: "Take pills", Start: base, End: base}, "09:00  Take pills"},
		{"event", Row{Summary: "Standup", Start: base, End: base.Add(30 * time.Minute)}, "09:00-09:30  Standup"},
		{"all day", Row{Summary: "Public holiday", AllDay: true}, "All day  Public holiday"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.row.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsVisible(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		row  Row
		want bool
	}{
		{"upcoming", Row{Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}, true},
		{"ongoing", Row{Start: now.Add(-30 * time.Minute), End: now.Add(30 * time.Minute)}, true},
		{"ended 30 min ago", Row{Start: now.Add(-90 * time.Minute), End: now.Add(-30 * time.Minute)}, true},
		{"ended 90 min ago", Row{Start: now.Add(-3 * time.Hour), End: now.Add(-90 * time.Minute)}, false},
		{"ended exactly 1h ago", Row{Start: now.Add(-2 * time.Hour), End: now.Add(-time.Hour)}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isVisible(tc.row, now); got != tc.want {
				t.Fatalf("isVisible() = %v, want %v", got, tc.want)
			}
		})
	}
}
