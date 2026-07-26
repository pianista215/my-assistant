package display

import (
	"image"
	"testing"
)

func TestNewDailyLayoutWithWeatherProducesPanelSizedImage(t *testing.T) {
	cases := []struct {
		name    string
		left    []Section
		weather WeatherPanel
		bottom  []Section
	}{
		{
			"agenda and weather",
			[]Section{{Title: "Eventos", Lines: []string{"09:00  Dentist"}}},
			WeatherPanel{
				Title: "Tiempo",
				Now:   WeatherNow{TempC: 21, IconKey: "clear"},
				Hours: []WeatherHour{
					{Label: "16h", TempC: 22, PrecipPct: 10, IconKey: "partly-cloudy"},
					{Label: "17h", TempC: 20, PrecipPct: 40, IconKey: "rain"},
				},
			},
			[]Section{{Title: "Lunes", Lines: []string{"Comida: Lentejas", "Cena: Tortilla"}}},
		},
		{
			"empty weather panel",
			nil,
			WeatherPanel{},
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := NewDailyLayoutWithWeather("Sábado, 25 de julio de 2026", "15:04:05 - 87%", tc.left, tc.weather, tc.bottom)

			if img.Width != Width || img.Height != Height {
				t.Fatalf("dimensions = %dx%d, want %dx%d", img.Width, img.Height, Width, Height)
			}

			var sawNonWhite bool
			for _, level := range img.Pixels {
				if level != White {
					sawNonWhite = true
					break
				}
			}
			if !sawNonWhite {
				t.Fatal("expected the rendered content to produce at least one non-white pixel")
			}
		})
	}
}

func TestDrawIconNoopsOnUnknownKey(t *testing.T) {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	// Left at zero-value, i.e. fully black (color.Gray{Y:0}), so a no-op
	// leaves every pixel black rather than white — the opposite of the
	// blank-canvas convention used elsewhere, chosen so "nothing was
	// drawn" is unambiguous either way this test's assumption could fail.
	for i := range canvas.Pix {
		canvas.Pix[i] = 255
	}

	drawIcon(canvas, "not-a-real-icon-key", 100, 100, 50)

	for _, v := range canvas.Pix {
		if v != 255 {
			t.Fatalf("drawIcon with an unknown key modified the canvas, want no-op")
		}
	}
}

func TestWeatherIconsCoverAllKnownKeys(t *testing.T) {
	for _, key := range []string{"clear", "partly-cloudy", "cloudy", "fog", "rain", "snow", "storm"} {
		if _, ok := weatherIcons[key]; !ok {
			t.Errorf("weatherIcons missing embedded asset for key %q", key)
		}
	}
}
