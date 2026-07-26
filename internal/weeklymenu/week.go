package weeklymenu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/sheets/v4"
)

// Day is one day's parsed menu entries, in sheet order. Lunch/Dinner drop
// blank entries rather than padding to a fixed length, so a day with
// fewer than 5 planned dishes just has a shorter slice.
type Day struct {
	Label  string   // the sheet's header cell text for this day's column, verbatim
	Lunch  []string // up to 5 "comida" entries
	Dinner []string // up to 5 "cena" entries
}

// Row layout of the second tab (fixed): row 1 is the header, rows 2-6 are
// lunch entries, row 7 is a blank spacer (never read), rows 8-12 are
// dinner entries. Expressed here as 0-indexed slice offsets into
// resp.Values.
const (
	headerRow      = 0
	lunchStartRow  = 1
	lunchEndRow    = 5
	dinnerStartRow = 7
	dinnerEndRow   = 11
	weekColumns    = 7
)

// FetchWeek returns the week's Days, rotated to start at today's column.
// Unlike shoppinglist's single-call fetch, the second tab's name can't be
// assumed, so this first discovers it by position (the second sheet,
// regardless of its title) before reading its values.
func FetchWeek(ctx context.Context, svc *sheets.Service, spreadsheetID string, today time.Weekday) ([]Day, error) {
	meta, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.properties.title").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("weeklymenu: reading spreadsheet metadata: %w", err)
	}
	if len(meta.Sheets) < 2 {
		return nil, fmt.Errorf("weeklymenu: spreadsheet has fewer than 2 tabs")
	}

	resp, err := svc.Spreadsheets.Values.Get(spreadsheetID, menuRange(meta.Sheets[1].Properties.Title)).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("weeklymenu: reading values: %w", err)
	}
	return parseWeek(resp.Values, today), nil
}

// ClearDay blanks the given day's "comida"/"cena" entries (rows 2-12 of
// its column) in the sheet, so the user has to fill them in again before
// that weekday comes back around next week. Mirrors FetchWeek's two-step
// shape: discover the second tab's title by position, then act on it.
func ClearDay(ctx context.Context, svc *sheets.Service, spreadsheetID string, day time.Weekday) error {
	meta, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets.properties.title").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("weeklymenu: reading spreadsheet metadata: %w", err)
	}
	if len(meta.Sheets) < 2 {
		return fmt.Errorf("weeklymenu: spreadsheet has fewer than 2 tabs")
	}

	rng := menuColumnRange(meta.Sheets[1].Properties.Title, day)
	if _, err := svc.Spreadsheets.Values.Clear(spreadsheetID, rng, &sheets.ClearValuesRequest{}).Context(ctx).Do(); err != nil {
		return fmt.Errorf("weeklymenu: clearing day: %w", err)
	}
	return nil
}

// menuRange builds the A1-notation range for the given tab title, quoted
// (required for any tab name with spaces or punctuation) with embedded
// single quotes doubled, as Sheets' quoting rules require.
func menuRange(tabTitle string) string {
	return quotedTab(tabTitle) + "!A1:G12"
}

// menuColumnRange builds the A1-notation range covering one weekday's
// entire column (rows 2-12: lunch, the blank spacer, and dinner — clearing
// the spacer too is harmless since it's never read). Reuses parseWeek's
// Sunday=0..Saturday=6 -> Monday=0..Sunday=6 shift to pick the column, then
// converts it to a letter; a single letter always suffices since the sheet
// only ever has 7 day columns (A-G).
func menuColumnRange(tabTitle string, day time.Weekday) string {
	col := string(rune('A' + (int(day)+6)%7))
	return fmt.Sprintf("%s!%s2:%s12", quotedTab(tabTitle), col, col)
}

// quotedTab quotes a tab title for use in A1 notation, doubling any
// embedded single quotes as Sheets' quoting rules require.
func quotedTab(tabTitle string) string {
	return "'" + strings.ReplaceAll(tabTitle, "'", "''") + "'"
}

// parseWeek extracts each weekday's column from the raw grid, then
// rotates the 7 resulting Days to start at today's column and wrap
// through the week. Go's time.Weekday is Sunday=0..Saturday=6, but the
// sheet's columns are Monday-first, hence the +6 shift before the mod.
func parseWeek(values [][]interface{}, today time.Weekday) []Day {
	days := make([]Day, weekColumns)
	for col := 0; col < weekColumns; col++ {
		days[col] = Day{
			Label:  cellString(values, headerRow, col),
			Lunch:  collectColumn(values, lunchStartRow, lunchEndRow, col),
			Dinner: collectColumn(values, dinnerStartRow, dinnerEndRow, col),
		}
	}

	start := (int(today) + 6) % 7
	rotated := make([]Day, weekColumns)
	for i := 0; i < weekColumns; i++ {
		rotated[i] = days[(start+i)%weekColumns]
	}
	return rotated
}

// collectColumn reads column col across rows startRow..endRow (inclusive,
// 0-indexed), dropping any row that's missing, non-string, or blank once
// trimmed — the same tolerance as shoppinglist.parseItems.
func collectColumn(values [][]interface{}, startRow, endRow, col int) []string {
	entries := make([]string, 0, endRow-startRow+1)
	for row := startRow; row <= endRow; row++ {
		if text, ok := cellStringOK(values, row, col); ok {
			entries = append(entries, text)
		}
	}
	return entries
}

// cellString is cellStringOK without the presence flag, for callers (like
// the header row) that treat a missing cell the same as an empty label.
func cellString(values [][]interface{}, row, col int) string {
	text, _ := cellStringOK(values, row, col)
	return text
}

// cellStringOK returns the trimmed string at (row, col) and whether it was
// present and non-blank: a row beyond len(values), a cell beyond that
// row's length (Sheets omits trailing empty cells), a non-string cell, or
// a whitespace-only string all report false.
func cellStringOK(values [][]interface{}, row, col int) (string, bool) {
	if row < 0 || row >= len(values) {
		return "", false
	}
	line := values[row]
	if col < 0 || col >= len(line) {
		return "", false
	}
	text, ok := line[col].(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	return text, true
}
