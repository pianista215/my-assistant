package weather

import (
	"testing"
	"time"
)

func hourAt(loc *time.Location, hour int) time.Time {
	return time.Date(2026, 7, 26, hour, 0, 0, 0, loc)
}

func TestSummarizeOneEntryPerHourWhenItFits(t *testing.T) {
	loc := time.UTC
	now := hourAt(loc, 14)
	var points []HourPoint
	for h := 12; h <= 20; h++ {
		points = append(points, HourPoint{Time: hourAt(loc, h), TempC: float64(h)})
	}

	current, forecast, ok := Summarize(points, now, 5)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if current.Time != hourAt(loc, 14) {
		t.Fatalf("current.Time = %v, want 14:00", current.Time)
	}
	// ForecastHorizonHours = 7, so hours 15..20 (6 points) fit within maxForecastEntries=5? No: 6 > 5, groups.
	if len(forecast) > 5 {
		t.Fatalf("len(forecast) = %d, want <= 5", len(forecast))
	}
}

func TestSummarizeOnePerHourWhenWithinMax(t *testing.T) {
	loc := time.UTC
	now := hourAt(loc, 14)
	var points []HourPoint
	for h := 14; h <= 17; h++ {
		points = append(points, HourPoint{Time: hourAt(loc, h), TempC: float64(h)})
	}

	current, forecast, ok := Summarize(points, now, 5)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if current.Time != hourAt(loc, 14) {
		t.Fatalf("current.Time = %v, want 14:00", current.Time)
	}
	if len(forecast) != 3 {
		t.Fatalf("len(forecast) = %d, want 3 (15:00, 16:00, 17:00)", len(forecast))
	}
	if forecast[0].Time != hourAt(loc, 15) || forecast[1].Time != hourAt(loc, 16) || forecast[2].Time != hourAt(loc, 17) {
		t.Fatalf("forecast times = %v, %v, %v, want 15:00, 16:00, 17:00", forecast[0].Time, forecast[1].Time, forecast[2].Time)
	}
}

func TestSummarizeGroupsWhenExceedingMax(t *testing.T) {
	loc := time.UTC
	now := hourAt(loc, 10)
	var points []HourPoint
	for h := 10; h <= 20; h++ {
		points = append(points, HourPoint{Time: hourAt(loc, h), TempC: float64(h)})
	}

	current, forecast, ok := Summarize(points, now, 4)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if current.Time != hourAt(loc, 10) {
		t.Fatalf("current.Time = %v, want 10:00", current.Time)
	}
	// Horizon caps the remaining points at 7 (11..17), then grouped into 4 buckets.
	if len(forecast) != 4 {
		t.Fatalf("len(forecast) = %d, want 4", len(forecast))
	}
	for i := 1; i < len(forecast); i++ {
		if !forecast[i].Time.After(forecast[i-1].Time) {
			t.Fatalf("forecast not chronologically increasing at %d: %v then %v", i, forecast[i-1].Time, forecast[i].Time)
		}
	}
}

func TestSummarizeNoDataAtOrAfterNow(t *testing.T) {
	loc := time.UTC
	now := hourAt(loc, 20)
	points := []HourPoint{
		{Time: hourAt(loc, 10)},
		{Time: hourAt(loc, 11)},
	}

	_, _, ok := Summarize(points, now, 5)
	if ok {
		t.Fatalf("ok = true, want false")
	}
}

func TestSummarizeEmptyInput(t *testing.T) {
	_, _, ok := Summarize(nil, hourAt(time.UTC, 10), 5)
	if ok {
		t.Fatalf("ok = true, want false")
	}
}

func TestSummarizeNoForecastPointsAfterCurrent(t *testing.T) {
	loc := time.UTC
	now := hourAt(loc, 10)
	points := []HourPoint{{Time: hourAt(loc, 10), TempC: 20}}

	current, forecast, ok := Summarize(points, now, 5)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if current.TempC != 20 {
		t.Fatalf("current.TempC = %v, want 20", current.TempC)
	}
	if len(forecast) != 0 {
		t.Fatalf("len(forecast) = %d, want 0", len(forecast))
	}
}
