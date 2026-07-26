// Command weathercheck dumps the raw Open-Meteo forecast response for the
// configured location as JSON, for inspecting the shape of fields
// internal/weather doesn't parse, and for general debugging of the
// weather data feeding the display. It builds the same request
// internal/weather.Client.FetchForecast does but skips its parsing
// entirely, since the whole point is to see the raw shape Open-Meteo
// sends.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/pianista215/my-assistant/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	u, err := url.Parse("https://api.open-meteo.com/v1/forecast")
	if err != nil {
		log.Fatalf("weathercheck: building request: %v", err)
	}
	q := u.Query()
	q.Set("latitude", strconv.FormatFloat(cfg.WeatherLatitude, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(cfg.WeatherLongitude, 'f', -1, 64))
	q.Set("hourly", "temperature_2m,weathercode,precipitation_probability")
	q.Set("timezone", cfg.Location.String())
	q.Set("forecast_days", "2")
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		log.Fatalf("weathercheck: fetching forecast: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("weathercheck: reading response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "weathercheck: unexpected status %s\n", resp.Status)
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		log.Fatalf("weathercheck: formatting response: %v", err)
	}
	os.Stdout.Write(pretty.Bytes())
	fmt.Println()
}
