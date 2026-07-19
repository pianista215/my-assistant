package server

import (
	"log"
	"net/http"
	"time"

	"github.com/pianista215/my-assistant/internal/display"
)

// handleDisplay serves the image the ESP32 should render. For this first
// iteration it always returns the "Hello World" + current time placeholder;
// later iterations will build it from Google Calendar/Sheets data instead.
func (s *Server) handleDisplay(w http.ResponseWriter, r *http.Request) {
	img := display.NewHelloWorld(time.Now())

	data, err := display.Encode(img)
	if err != nil {
		log.Printf("server: encoding display image: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}
