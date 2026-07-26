package display

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// weatherIconFS embeds the weather icon set: one black-on-transparent PNG
// per icon key (matching weather.IconKeyFor's return values), drawn as
// simple geometric glyphs (sun, cloud, rain, snow, storm shapes) rather
// than sourced from a licensed icon set — see internal/weather's package
// doc for why this integration needed no API key/vendor dependency either.
//
//go:embed assets/weathericons/*.png
var weatherIconFS embed.FS

// weatherIcons maps an icon key (the PNG's filename, without extension) to
// its decoded image, resolved once at package init the same way
// baseFont/boldFont are.
var weatherIcons = mustLoadWeatherIcons()

func mustLoadWeatherIcons() map[string]image.Image {
	entries, err := weatherIconFS.ReadDir("assets/weathericons")
	if err != nil {
		panic("display: reading embedded weather icons: " + err.Error())
	}
	icons := make(map[string]image.Image, len(entries))
	for _, entry := range entries {
		data, err := weatherIconFS.ReadFile("assets/weathericons/" + entry.Name())
		if err != nil {
			panic("display: reading embedded weather icon " + entry.Name() + ": " + err.Error())
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			panic("display: decoding embedded weather icon " + entry.Name() + ": " + err.Error())
		}
		icons[strings.TrimSuffix(entry.Name(), ".png")] = img
	}
	return icons
}

// drawIcon scales the icon for key to a size x size square at (x, y) and
// composites it onto canvas using its alpha channel, so the transparent
// background leaves the canvas untouched. An unrecognized key is a no-op
// rather than a panic — the same tolerance drawFooter/drawBulletedLines
// apply to their own edge cases — since a bad icon key is a data problem
// (an unmapped weather.IconKeyFor result), not a programming error worth
// crashing the whole display over.
func drawIcon(canvas *image.Gray, key string, x, y, size int) {
	src, ok := weatherIcons[key]
	if !ok {
		return
	}
	target := image.Rect(x, y, x+size, y+size)
	xdraw.BiLinear.Scale(canvas, target, src, src.Bounds(), xdraw.Over, nil)
}

// WeatherNow is the current conditions, shown as a large icon next to the
// temperature — the panel's headline, Google-Weather-style.
type WeatherNow struct {
	TempC   float64
	IconKey string
}

// WeatherHour is one entry in the small forecast list below WeatherNow:
// either a single upcoming hour or a representative hour for a grouped
// time range (see weather.Summarize) — rendered identically either way,
// since the panel doesn't distinguish the two cases visually.
type WeatherHour struct {
	Label     string // e.g. "16h"
	TempC     float64
	PrecipPct int
	IconKey   string
}

// WeatherPanel is the weather column's content, rendered by
// NewDailyLayoutWithWeather in place of a plain Section list. Represents
// only the happy path — a fetch error is rendered as an ordinary
// error Section through NewDailyLayout instead, the same as the shopping
// list's/weekly menu's own non-fatal fetch-error handling.
type WeatherPanel struct {
	Title string
	Now   WeatherNow
	Hours []WeatherHour
}

// Weather panel layout constants, alongside NewSections/NewDailyLayout's
// own layout constants in image.go.
const (
	weatherIconLargeSize = 96
	weatherIconSmallSize = 28
	weatherNowFontSize   = 40
	weatherRowHeight     = 34
)

// drawWeatherPanel draws panel's title (if any), the large "now" icon and
// temperature, then a vertical list of small icon+time+temperature+rain%
// rows — one per WeatherHour, stacked like drawSections' plain rows.
// Returns the y immediately below the last row drawn, so it composes with
// a bottom region the same way drawSections' return value does.
func drawWeatherPanel(canvas *image.Gray, x, y, width int, panel WeatherPanel) int {
	if panel.Title != "" {
		drawText(canvas, newBoldFace(sectionTitleFontSize), panel.Title, x, y)
		y += sectionTitleHeight
	}

	drawIcon(canvas, panel.Now.IconKey, x, y, weatherIconLargeSize)
	tempFace := newBoldFace(weatherNowFontSize)
	tempY := y + (weatherIconLargeSize-tempFace.Metrics().Height.Ceil())/2
	drawText(canvas, tempFace, fmt.Sprintf("%.0f°C", panel.Now.TempC), x+weatherIconLargeSize+16, tempY)
	y += weatherIconLargeSize + sectionGap

	rowFace := newFace(rowFontSize)
	for _, h := range panel.Hours {
		drawIcon(canvas, h.IconKey, x, y, weatherIconSmallSize)
		text := fmt.Sprintf("%s  %.0f°C  %d%%", h.Label, h.TempC, h.PrecipPct)
		textY := y + (weatherIconSmallSize-rowFace.Metrics().Height.Ceil())/2
		drawText(canvas, rowFace, text, x+weatherIconSmallSize+12, textY)
		y += weatherRowHeight
	}

	return y
}

// NewDailyLayoutWithWeather is NewDailyLayout's counterpart for the
// phases (morning, night, or an empty-shopping-list midday) that show the
// weather panel instead of the shopping list in the right column. Kept as
// a separate function rather than folding a "right column kind" switch
// into NewDailyLayout itself, consistent with how NewDailyLayout was
// added alongside (not instead of) NewSections/NewTextRows.
func NewDailyLayoutWithWeather(header, footer string, left []Section, weather WeatherPanel, bottom []Section) *GrayImage {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	topY := drawHeader(canvas, header)
	leftEndY := drawSections(canvas, leftColumnX, topY, leftColumnWidth, left)
	rightEndY := drawWeatherPanel(canvas, rightColumnX, topY, rightColumnWidth, weather)

	bottomY := leftEndY
	if rightEndY > bottomY {
		bottomY = rightEndY
	}
	drawSections(canvas, marginX, bottomY+sectionGap, fullContentWidth, bottom)
	drawFooter(canvas, footer)

	return fromGray(canvas)
}
