// Command menucheck dumps the raw values read from the weekly-menu
// spreadsheet's second tab as JSON, plus which tab title tab discovery
// picked, bypassing internal/weeklymenu's column-extraction/rotation
// parsing (see cmd/sheetscheck for the analogous shopping-list tool).
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/pianista215/my-assistant/internal/config"
)

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
		log.Fatalf("weeklymenu: creating client: %v", err)
	}

	meta, err := svc.Spreadsheets.Get(cfg.GoogleSheetID).Fields("sheets.properties.title").Context(ctx).Do()
	if err != nil {
		log.Fatalf("weeklymenu: reading spreadsheet metadata: %v", err)
	}
	if len(meta.Sheets) < 2 {
		log.Fatalf("weeklymenu: spreadsheet has fewer than 2 tabs")
	}
	tab := meta.Sheets[1].Properties.Title
	valuesRange := "'" + strings.ReplaceAll(tab, "'", "''") + "'!A1:G12"

	resp, err := svc.Spreadsheets.Values.Get(cfg.GoogleSheetID, valuesRange).Context(ctx).Do()
	if err != nil {
		log.Fatalf("weeklymenu: reading values: %v", err)
	}

	out := struct {
		Tab    string          `json:"tab"`
		Values [][]interface{} `json:"values"`
	}{Tab: tab, Values: resp.Values}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		log.Fatalf("encoding output: %v", err)
	}
}
