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
// own layout constants in image.go — see "Weather panel sizing and
// layout" in CLAUDE.md for the two rounds of user feedback that produced
// these values: first a size reduction (a 96/40 first pass made the
// weather column taller than the weekly menu's budget below it), then a
// switch from a left-aligned vertical icon+text list to this centered
// "now" block + horizontal forecast row (the vertical list left most of
// the column's width empty, since neither the icon nor the text needed
// it).
const (
	weatherIconLargeSize         = 64
	weatherNowFontSize           = 30
	weatherNowGap                = 16 // gap between the "now" icon and its temperature text
	weatherForecastIconSize      = 32
	weatherForecastLabelFontSize = 13
	weatherForecastTextFontSize  = 15
	weatherForecastGap           = 4 // gap between the label/icon/text stack within one forecast slot
)

// drawWeatherPanel draws panel's title (if any), the large "now" icon and
// temperature centered as a block within width, then panel.Hours as a
// horizontal row of equal-width slots spread across width — each slot's
// time label, icon, and "temp rain%" text all centered within its own
// slot. Returns the y immediately below the last row drawn, so it
// composes with a bottom region the same way drawSections' return value
// does.
func drawWeatherPanel(canvas *image.Gray, x, y, width int, panel WeatherPanel) int {
	if panel.Title != "" {
		drawText(canvas, newBoldFace(sectionTitleFontSize), panel.Title, x, y)
		y += sectionTitleHeight
	}

	tempFace := newBoldFace(weatherNowFontSize)
	tempText := fmt.Sprintf("%.0f°C", panel.Now.TempC)
	nowBlockWidth := weatherIconLargeSize + weatherNowGap + measureWidth(tempFace, tempText)
	nowBlockX := x + (width-nowBlockWidth)/2
	if nowBlockX < x {
		nowBlockX = x
	}
	drawIcon(canvas, panel.Now.IconKey, nowBlockX, y, weatherIconLargeSize)
	tempY := y + (weatherIconLargeSize-tempFace.Metrics().Height.Ceil())/2
	drawText(canvas, tempFace, tempText, nowBlockX+weatherIconLargeSize+weatherNowGap, tempY)
	y += weatherIconLargeSize + sectionGap

	if len(panel.Hours) == 0 {
		return y
	}

	labelFace := newFace(weatherForecastLabelFontSize)
	textFace := newFace(weatherForecastTextFontSize)
	labelY := y
	iconY := labelY + labelFace.Metrics().Height.Ceil() + weatherForecastGap
	textY := iconY + weatherForecastIconSize + weatherForecastGap

	slotWidth := width / len(panel.Hours)
	for i, h := range panel.Hours {
		centerX := x + i*slotWidth + slotWidth/2

		drawText(canvas, labelFace, h.Label, centerX-measureWidth(labelFace, h.Label)/2, labelY)
		drawIcon(canvas, h.IconKey, centerX-weatherForecastIconSize/2, iconY, weatherForecastIconSize)
		text := fmt.Sprintf("%.0f°C %d%%", h.TempC, h.PrecipPct)
		drawText(canvas, textFace, text, centerX-measureWidth(textFace, text)/2, textY)
	}

	return textY + textFace.Metrics().Height.Ceil()
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
