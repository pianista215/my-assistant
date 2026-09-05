// Command server runs the REST API the ESP32 polls to know what to display.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/pianista215/my-assistant/internal/calendar"
	"github.com/pianista215/my-assistant/internal/config"
	"github.com/pianista215/my-assistant/internal/server"
	"github.com/pianista215/my-assistant/internal/shoppinglist"
	"github.com/pianista215/my-assistant/internal/weather"
	"github.com/pianista215/my-assistant/internal/weeklymenu"
)

func main() {
	insecure := flag.Bool("insecure", false, "Serve plain HTTP instead of HTTPS (skips the self-signed certificate at secrets/tls-cert.pem / secrets/tls-key.pem)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// buildGoogleClients is used both for the initial construction below
	// and, later, by internal/server to rebuild calendar/shoppinglist/
	// weeklymenu against a renewed credentials file once the QR-code
	// reauthorization flow (internal/oauthrenewal) completes — see
	// server.SetGoogleClientsBuilder.
	buildGoogleClients := func(ctx context.Context, credentialsFile string) (server.CalendarFetcher, server.ShoppingListFetcher, server.MenuFetcher, error) {
		cal, err := calendar.NewClient(ctx, credentialsFile, cfg.CalendarID, cfg.Location)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("calendar: %w", err)
		}
		shoppingList, err := shoppinglist.NewClient(ctx, credentialsFile, cfg.GoogleSheetID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("shoppinglist: %w", err)
		}
		menu, err := weeklymenu.NewClient(ctx, credentialsFile, cfg.GoogleSheetID, cfg.Location)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("weeklymenu: %w", err)
		}
		return cal, shoppingList, menu, nil
	}

	calClient, shoppingListClient, menuClient, err := buildGoogleClients(context.Background(), cfg.GoogleCredentialsFile)
	if err != nil {
		log.Fatalf("building google clients: %v", err)
	}

	weatherClient := weather.NewClient(cfg.WeatherLatitude, cfg.WeatherLongitude, cfg.Location)

	var tlsInfo server.TLSInfo
	if !*insecure {
		fingerprint, certPEM, err := ensureTLSCert(tlsCertPath, tlsKeyPath)
		if err != nil {
			log.Fatalf("tls: %v", err)
		}
		log.Printf("tls: certificate ready, sha256 fingerprint: %s", fingerprint)
		tlsInfo = server.TLSInfo{Fingerprint: fingerprint, CertPEM: certPEM}
	}

	srv := server.New(cfg, calClient, shoppingListClient, menuClient, weatherClient, tlsInfo)
	srv.SetGoogleClientsBuilder(buildGoogleClients)

	addr := ":" + cfg.Port
	if *insecure {
		log.Printf("listening on %s (http, insecure)", addr)
		err = http.ListenAndServe(addr, srv)
	} else {
		log.Printf("listening on %s (https)", addr)
		err = http.ListenAndServeTLS(addr, tlsCertPath, tlsKeyPath, srv)
	}
	if err != nil {
		log.Fatalf("server: %v", err)
	}
}
