package calendar

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/calendar/v3"
)

// Row is a single agenda entry, decoupled from the shape of the Google
// Calendar API's own Event type.
type Row struct {
	Summary string
	Start   time.Time
	End     time.Time // equal to Start for a reminder/point-in-time item
	AllDay  bool
}

// IsReminder reports whether this row is a point-in-time item (as opposed
// to an event with a real duration), based on Start and End being equal.
func (r Row) IsReminder() bool {
	return !r.AllDay && r.Start.Equal(r.End)
}

// String formats the row for display: just the start time for a
// reminder, start-end for an event, and an "All day" marker for all-day
// items.
func (r Row) String() string {
	switch {
	case r.AllDay:
		return fmt.Sprintf("All day  %s", r.Summary)
	case r.IsReminder():
		return fmt.Sprintf("%s  %s", r.Start.Format("15:04"), r.Summary)
	default:
		return fmt.Sprintf("%s-%s  %s", r.Start.Format("15:04"), r.End.Format("15:04"), r.Summary)
	}
}

// visibleAfterEnd is how long a past row stays on the display once it has
// finished, so the panel keeps showing what just happened for a while
// instead of dropping it the instant it ends.
const visibleAfterEnd = time.Hour

// FetchToday returns today's agenda rows (in loc's timezone) from
// calendarID, excluding rows that finished more than an hour before now.
func FetchToday(ctx context.Context, svc *calendar.Service, calendarID string, loc *time.Location, now time.Time) ([]Row, error) {
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	call := svc.Events.List(calendarID).
		Context(ctx).
		TimeMin(dayStart.Format(time.RFC3339)).
		TimeMax(dayEnd.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime")

	events, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("calendar: listing events: %w", err)
	}

	rows := make([]Row, 0, len(events.Items))
	for _, item := range events.Items {
		row, err := toRow(item, loc)
		if err != nil {
			return nil, fmt.Errorf("calendar: parsing event %q: %w", item.Id, err)
		}
		if !isVisible(row, now) {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// isVisible reports whether row should still be shown at now: anything
// upcoming or ongoing, plus anything that finished within visibleAfterEnd.
func isVisible(row Row, now time.Time) bool {
	return !row.End.Add(visibleAfterEnd).Before(now)
}

func toRow(item *calendar.Event, loc *time.Location) (Row, error) {
	if item.Start.Date != "" {
		start, err := time.ParseInLocation("2006-01-02", item.Start.Date, loc)
		if err != nil {
			return Row{}, fmt.Errorf("parsing all-day start %q: %w", item.Start.Date, err)
		}
		end := start.Add(24 * time.Hour)
		if item.End != nil && item.End.Date != "" {
			if parsedEnd, err := time.ParseInLocation("2006-01-02", item.End.Date, loc); err == nil {
				end = parsedEnd
			}
		}
		return Row{Summary: item.Summary, Start: start, End: end, AllDay: true}, nil
	}

	start, err := time.Parse(time.RFC3339, item.Start.DateTime)
	if err != nil {
		return Row{}, fmt.Errorf("parsing start %q: %w", item.Start.DateTime, err)
	}
	end, err := time.Parse(time.RFC3339, item.End.DateTime)
	if err != nil {
		return Row{}, fmt.Errorf("parsing end %q: %w", item.End.DateTime, err)
	}

	return Row{
		Summary: item.Summary,
		Start:   start.In(loc),
		End:     end.In(loc),
	}, nil
}
