package shoppinglist

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"
)

// FetchItems returns the current shopping list items read from
// spreadsheetID, in sheet order, with blank rows dropped.
func FetchItems(ctx context.Context, svc *sheets.Service, spreadsheetID string) ([]string, error) {
	resp, err := svc.Spreadsheets.Values.Get(spreadsheetID, itemsRange).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("shoppinglist: reading values: %w", err)
	}
	return parseItems(resp.Values), nil
}

// parseItems extracts non-blank item names from raw Sheets API row values,
// tolerating rows left empty by accident: a row with no cells, a first
// cell that isn't a string, or a string that's blank once trimmed are all
// silently skipped rather than producing an empty entry.
func parseItems(values [][]interface{}) []string {
	items := make([]string, 0, len(values))
	for _, row := range values {
		if len(row) == 0 {
			continue
		}
		text, ok := row[0].(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		items = append(items, text)
	}
	return items
}
