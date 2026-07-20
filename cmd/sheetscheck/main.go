// Command sheetscheck dumps the raw values read from the configured
// shopping-list sheet as JSON, for inspecting how Google actually
// represents rows (e.g. how a genuinely blank row comes through) and for
// general debugging of the data feeding the display. It talks to the
// Sheets API directly (bypassing internal/shoppinglist's parsing) since
// the whole point is to see the raw shape Google sends.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/pianista215/my-assistant/internal/config"
)

const itemsRange = "A2:A"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	svc, err := sheets.NewService(ctx,
		option.WithCredentialsFile(cfg.GoogleCredentialsFile),
		option.WithScopes(sheets.SpreadsheetsReadonlyScope),
	)
	if err != nil {
		log.Fatalf("shoppinglist: creating client: %v", err)
	}

	resp, err := svc.Spreadsheets.Values.Get(cfg.GoogleSheetID, itemsRange).Context(ctx).Do()
	if err != nil {
		log.Fatalf("shoppinglist: reading values: %v", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(resp.Values); err != nil {
		log.Fatalf("encoding output: %v", err)
	}
}
