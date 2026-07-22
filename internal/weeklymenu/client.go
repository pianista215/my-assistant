// Package weeklymenu reads the planned lunch/dinner menu for the week
// from a single, fixed Google Sheet: the same spreadsheet as the shopping
// list (internal/shoppinglist), but its second tab, found by position
// (not by a hardcoded tab name) since there's no other way to reference a
// tab other than the first via A1 notation alone.
package weeklymenu

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Client fetches the current week's menu from one fixed reference
// spreadsheet's second tab.
type Client struct {
	svc           *sheets.Service
	spreadsheetID string
	loc           *time.Location
}

// NewClient builds a Client authenticated with the credentials stored at
// credentialsFile (same authorized_user file used by internal/calendar
// and internal/shoppinglist), reading the menu from spreadsheetID and
// determining "today" (for rotating the week) in loc.
func NewClient(ctx context.Context, credentialsFile, spreadsheetID string, loc *time.Location) (*Client, error) {
	svc, err := sheets.NewService(ctx,
		option.WithCredentialsFile(credentialsFile),
		option.WithScopes(sheets.SpreadsheetsReadonlyScope),
	)
	if err != nil {
		return nil, fmt.Errorf("weeklymenu: creating client: %w", err)
	}
	return &Client{svc: svc, spreadsheetID: spreadsheetID, loc: loc}, nil
}

// FetchWeek returns the current week's days, rotated to start at today.
func (c *Client) FetchWeek(ctx context.Context) ([]Day, error) {
	return FetchWeek(ctx, c.svc, c.spreadsheetID, time.Now().In(c.loc).Weekday())
}
