// Command server runs the REST API the ESP32 polls to know what to display.
package main

import (
	"log"
	"net/http"

	"github.com/pianista215/my-assistant/internal/config"
	"github.com/pianista215/my-assistant/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	srv := server.New(cfg)

	addr := ":" + cfg.Port
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("server: %v", err)
	}
}
