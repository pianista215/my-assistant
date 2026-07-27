// Command server runs the REST API the ESP32 polls to know what to display.
package main

import (
	"context"
	"flag"
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
	httpsEnabled := flag.Bool("https", false, "Serve over HTTPS using a self-signed certificate (generated once at secrets/tls-cert.pem / secrets/tls-key.pem and reused on every restart)")
	flag.Parse()

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

	weatherClient := weather.NewClient(cfg.WeatherLatitude, cfg.WeatherLongitude, cfg.Location)

	var tlsInfo server.TLSInfo
	if *httpsEnabled {
		fingerprint, certPEM, err := ensureTLSCert(tlsCertPath, tlsKeyPath)
		if err != nil {
			log.Fatalf("tls: %v", err)
		}
		log.Printf("tls: certificate ready, sha256 fingerprint: %s", fingerprint)
		tlsInfo = server.TLSInfo{Fingerprint: fingerprint, CertPEM: certPEM}
	}

	srv := server.New(cfg, calClient, shoppingListClient, menuClient, weatherClient, tlsInfo)

	addr := ":" + cfg.Port
	if *httpsEnabled {
		log.Printf("listening on %s (https)", addr)
		err = http.ListenAndServeTLS(addr, tlsCertPath, tlsKeyPath, srv)
	} else {
		log.Printf("listening on %s", addr)
		err = http.ListenAndServe(addr, srv)
	}
	if err != nil {
		log.Fatalf("server: %v", err)
	}
}
