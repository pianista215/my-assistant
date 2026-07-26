package weather

import "time"

// ForecastHorizonHours is how far ahead Summarize looks past the current
// hour — the "next 7h" the user asked for.
const ForecastHorizonHours = 7

// Summarize picks current (the point covering now's hour) and forecast (up
// to maxForecastEntries points covering the next ForecastHorizonHours
// hours after it) out of points. If that many hours fit within
// maxForecastEntries, forecast has one point per hour; otherwise it's
// reduced to maxForecastEntries representative points, one per
// roughly-equal-sized bucket across the horizon (each bucket's middle
// point, so it's representative of the bucket rather than just its first
// hour). ok is false if points has nothing at or after now's hour, meaning
// there's nothing to show.
func Summarize(points []HourPoint, now time.Time, maxForecastEntries int) (current HourPoint, forecast []HourPoint, ok bool) {
	currentHour := now.Truncate(time.Hour)

	idx := -1
	for i, p := range points {
		if !p.Time.Before(currentHour) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return HourPoint{}, nil, false
	}
	current = points[idx]

	rest := points[idx+1:]
	if len(rest) > ForecastHorizonHours {
		rest = rest[:ForecastHorizonHours]
	}
	if len(rest) <= maxForecastEntries {
		return current, rest, true
	}
	return current, groupInto(rest, maxForecastEntries), true
}

// groupInto splits points into up to groups roughly-equal-sized buckets
// and returns each bucket's middle point as its representative.
func groupInto(points []HourPoint, groups int) []HourPoint {
	if groups <= 0 {
		groups = 1
	}
	n := len(points)
	result := make([]HourPoint, 0, groups)
	for g := 0; g < groups; g++ {
		start := g * n / groups
		end := (g + 1) * n / groups
		if start >= end {
			continue
		}
		result = append(result, points[start+(end-start)/2])
	}
	return result
}
