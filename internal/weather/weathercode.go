package weather

// IconKeyFor maps a WMO weather code (as returned by Open-Meteo's
// "weathercode" field) to one of the icon keys embedded in
// internal/display's weather icon assets: "clear", "partly-cloudy",
// "cloudy", "fog", "rain", "snow", "storm". An unrecognized code falls
// back to "cloudy" rather than an empty key, so display.drawIcon always
// has something to draw.
func IconKeyFor(code int) string {
	switch {
	case code == 0 || code == 1:
		return "clear"
	case code == 2:
		return "partly-cloudy"
	case code == 3:
		return "cloudy"
	case code == 45 || code == 48:
		return "fog"
	case isStorm(code):
		return "storm"
	case isSnow(code):
		return "snow"
	case isRain(code):
		return "rain"
	default:
		return "cloudy"
	}
}

// isRain covers drizzle and rain (including freezing and shower variants):
// WMO codes 51-67 and 80-82.
func isRain(code int) bool {
	switch code {
	case 51, 53, 55, 56, 57, 61, 63, 65, 66, 67, 80, 81, 82:
		return true
	default:
		return false
	}
}

// isSnow covers snowfall, snow grains, and snow showers: WMO codes 71-77
// and 85-86.
func isSnow(code int) bool {
	switch code {
	case 71, 73, 75, 77, 85, 86:
		return true
	default:
		return false
	}
}

// isStorm covers thunderstorms, with or without hail: WMO codes 95, 96, 99.
func isStorm(code int) bool {
	switch code {
	case 95, 96, 99:
		return true
	default:
		return false
	}
}
