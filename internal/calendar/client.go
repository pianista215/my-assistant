// Package calendar reads today's agenda from a single, fixed Google
// Calendar using a service account (the calendar is shared with the
// service account's email; no OAuth/user-consent flow is involved).
package calendar

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// Client fetches today's agenda from one fixed reference calendar.
type Client struct {
	svc        *calendar.Service
	calendarID string
	loc        *time.Location
}

// NewClient builds a Client authenticated as the service account whose key
// is stored at credentialsFile, reading calendarID and formatting/bounding
// "today" in loc.
func NewClient(ctx context.Context, credentialsFile, calendarID string, loc *time.Location) (*Client, error) {
	svc, err := calendar.NewService(ctx,
		option.WithCredentialsFile(credentialsFile),
		option.WithScopes(calendar.CalendarReadonlyScope),
	)
	if err != nil {
		return nil, fmt.Errorf("calendar: creating client: %w", err)
	}
	return &Client{svc: svc, calendarID: calendarID, loc: loc}, nil
}

// FetchToday returns today's agenda rows as of now.
func (c *Client) FetchToday(ctx context.Context) ([]Row, error) {
	return FetchToday(ctx, c.svc, c.calendarID, c.loc, time.Now().In(c.loc))
}
