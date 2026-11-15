// Package shoppinglist reads the current shopping list from a single,
// fixed Google Sheet: one product per row, starting at row 2 (row 1 is a
// human-only header, ignored here).
package shoppinglist

import (
	"context"
	"fmt"

	"github.com/pianista215/my-assistant/internal/oauthrenewal"
	"google.golang.org/api/sheets/v4"
)

// itemsRange is the column read on every fetch. It has no sheet-tab name
// prefix, so the Sheets API resolves it against the spreadsheet's first
// tab; starting at row 2 skips the header row without any special-case
// logic in the parsing code.
const itemsRange = "A2:A"

// Client fetches the current shopping list from one fixed reference sheet.
type Client struct {
	svc           *sheets.Service
	spreadsheetID string
}

// NewClient builds a Client authenticated with the credentials stored at
// credentialsFile (same authorized_user file used by internal/calendar,
// loaded through oauthrenewal.ClientOption), reading products
// from spreadsheetID. The credentials are scoped to drive.file — per-file
// access to whichever spreadsheet was picked during cmd/oauthsetup, not
// account-wide — so spreadsheetID must be that same file.
func NewClient(ctx context.Context, credentialsFile, spreadsheetID string) (*Client, error) {
	// oauthrenewal.ClientOption, not option.WithCredentialsFile: see its
	// doc comment for the hourly-poll/async-refresh race it avoids.
	authOpt, err := oauthrenewal.ClientOption(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("shoppinglist: loading credentials: %w", err)
	}
	svc, err := sheets.NewService(ctx, authOpt)
	if err != nil {
		return nil, fmt.Errorf("shoppinglist: creating client: %w", err)
	}
	return &Client{svc: svc, spreadsheetID: spreadsheetID}, nil
}

// FetchItems returns the current shopping list items, in sheet order.
func (c *Client) FetchItems(ctx context.Context) ([]string, error) {
	return FetchItems(ctx, c.svc, c.spreadsheetID)
}
