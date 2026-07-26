package weather

import (
	"testing"
	"time"
)

func TestParseForecast(t *testing.T) {
	var raw openMeteoResponse
	raw.Hourly.Time = []string{"2026-07-26T14:00", "not-a-time", "2026-07-26T15:00"}
	raw.Hourly.Temperature2m = []float64{20.5, 99, 21.0}
	raw.Hourly.WeatherCode = []int{0, 99, 61}
	raw.Hourly.PrecipitationProbability = []int{5, 99, 40}

	points, err := parseForecast(raw, time.UTC)
	if err != nil {
		t.Fatalf("parseForecast() error = %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("len(points) = %d, want 2 (malformed timestamp dropped)", len(points))
	}
	if points[0].TempC != 20.5 || points[0].Code != 0 || points[0].PrecipPct != 5 {
		t.Errorf("points[0] = %+v, unexpected", points[0])
	}
	if points[1].TempC != 21.0 || points[1].Code != 61 || points[1].PrecipPct != 40 {
		t.Errorf("points[1] = %+v, unexpected", points[1])
	}
}

func TestParseForecastShortArraysLeaveZeroValues(t *testing.T) {
	var raw openMeteoResponse
	raw.Hourly.Time = []string{"2026-07-26T14:00"}
	// Temperature2m/WeatherCode/PrecipitationProbability left empty.

	points, err := parseForecast(raw, time.UTC)
	if err != nil {
		t.Fatalf("parseForecast() error = %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	if points[0].TempC != 0 || points[0].Code != 0 || points[0].PrecipPct != 0 {
		t.Errorf("points[0] = %+v, want zero values", points[0])
	}
}

func TestParseForecastNoUsableData(t *testing.T) {
	var raw openMeteoResponse
	raw.Hourly.Time = []string{"garbage"}

	_, err := parseForecast(raw, time.UTC)
	if err == nil {
		t.Fatalf("parseForecast() error = nil, want an error")
	}
}

func TestParseForecastEmptyResponse(t *testing.T) {
	_, err := parseForecast(openMeteoResponse{}, time.UTC)
	if err == nil {
		t.Fatalf("parseForecast() error = nil, want an error")
	}
}
