package weather

import (
	"fmt"
	"time"
)

// openMeteoResponse is the slice of Open-Meteo's JSON response this
// package actually reads: the hourly arrays, aligned by index. cmd/
// weathercheck bypasses this and dumps the full raw JSON instead, for
// inspecting fields this package doesn't parse.
type openMeteoResponse struct {
	Hourly struct {
		Time                     []string  `json:"time"`
		Temperature2m            []float64 `json:"temperature_2m"`
		WeatherCode              []int     `json:"weathercode"`
		PrecipitationProbability []int     `json:"precipitation_probability"`
	} `json:"hourly"`
}

// openMeteoTimeLayout is the format Open-Meteo uses for hourly timestamps
// when timezone is a named zone (as opposed to "GMT"/"auto"): local wall
// time, no offset.
const openMeteoTimeLayout = "2006-01-02T15:04"

// parseForecast converts the raw Open-Meteo response into HourPoints,
// parsing each hourly timestamp in loc. A timestamp that fails to parse is
// dropped rather than failing the whole fetch, the same tolerance
// shoppinglist.parseItems/weeklymenu.parseWeek apply to their own rows;
// the arrays are trusted to be index-aligned per Open-Meteo's contract, so
// a short array simply leaves the corresponding field zero-valued rather
// than being treated as an error.
func parseForecast(raw openMeteoResponse, loc *time.Location) ([]HourPoint, error) {
	points := make([]HourPoint, 0, len(raw.Hourly.Time))
	for i, ts := range raw.Hourly.Time {
		t, err := time.ParseInLocation(openMeteoTimeLayout, ts, loc)
		if err != nil {
			continue
		}
		point := HourPoint{Time: t}
		if i < len(raw.Hourly.Temperature2m) {
			point.TempC = raw.Hourly.Temperature2m[i]
		}
		if i < len(raw.Hourly.WeatherCode) {
			point.Code = raw.Hourly.WeatherCode[i]
		}
		if i < len(raw.Hourly.PrecipitationProbability) {
			point.PrecipPct = raw.Hourly.PrecipitationProbability[i]
		}
		points = append(points, point)
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("weather: no usable hourly data in response")
	}
	return points, nil
}
