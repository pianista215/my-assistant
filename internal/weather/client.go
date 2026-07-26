// Package weather reads the hourly forecast for a single, fixed location
// from Open-Meteo. Chosen over Google Weather API because it needs no API
// key or billing-enabled Cloud project — a plain HTTP GET, matching this
// codebase's existing no-SDK, no-heavyweight-auth style for every other
// integration (though calendar/shoppinglist/weeklymenu do need OAuth,
// that's inherent to reading a specific private Google account's data;
// weather has no such requirement).
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// HourPoint is one hour's forecast.
type HourPoint struct {
	Time      time.Time
	TempC     float64
	Code      int // WMO weather code, kept for cmd/weathercheck and tests
	PrecipPct int
}

// Client fetches the hourly forecast for one fixed reference location.
type Client struct {
	httpClient *http.Client
	latitude   float64
	longitude  float64
	loc        *time.Location
}

// NewClient builds a Client for the given coordinates, formatting hourly
// timestamps in loc.
func NewClient(latitude, longitude float64, loc *time.Location) *Client {
	return &Client{httpClient: http.DefaultClient, latitude: latitude, longitude: longitude, loc: loc}
}

const forecastURL = "https://api.open-meteo.com/v1/forecast"

// FetchForecast returns the hourly forecast from today's start through the
// next day, trimmed down to whatever's actually shown by Summarize.
func (c *Client) FetchForecast(ctx context.Context) ([]HourPoint, error) {
	u, err := url.Parse(forecastURL)
	if err != nil {
		return nil, fmt.Errorf("weather: building request: %w", err)
	}
	q := u.Query()
	q.Set("latitude", strconv.FormatFloat(c.latitude, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(c.longitude, 'f', -1, 64))
	q.Set("hourly", "temperature_2m,weathercode,precipitation_probability")
	q.Set("timezone", c.loc.String())
	q.Set("forecast_days", "2")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("weather: building request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weather: fetching forecast: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather: unexpected status %s", resp.Status)
	}

	var raw openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("weather: decoding response: %w", err)
	}

	return parseForecast(raw, c.loc)
}
