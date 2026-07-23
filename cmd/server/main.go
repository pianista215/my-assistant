// Command server runs the REST API the ESP32 polls to know what to display.
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/config"
	"github.com/pianista215/my-assistant/internal/server"
	"github.com/pianista215/my-assistant/internal/shoppinglist"
	"github.com/pianista215/my-assistant/internal/weeklymenu"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	calClient, err := calendar.NewClient(context.Background(), cfg.GoogleCredentialsFile, cfg.CalendarID, cfg.Location)
	if err != nil {
		log.Fatalf("calendar: %v", err)
	}

	shoppingListClient, err := shoppinglist.NewClient(context.Background(), cfg.GoogleCredentialsFile, cfg.GoogleSheetID)
	if err != nil {
		log.Fatalf("shoppinglist: %v", err)
	}

	menuClient, err := weeklymenu.NewClient(context.Background(), cfg.GoogleCredentialsFile, cfg.GoogleSheetID, cfg.Location)
	if err != nil {
		log.Fatalf("weeklymenu: %v", err)
	}

	srv := server.New(cfg, calClient, shoppingListClient, menuClient)

	addr := ":" + cfg.Port
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("server: %v", err)
	}
}
