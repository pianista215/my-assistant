// Package display builds and encodes the images shown on the e-ink panel.
package display

import (
	"image"
	"image/color"
	"image/draw"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Panel dimensions and grayscale depth of the target e-ink hardware
// (Seeed reTerminal E1001 / GDEY075T7 panel, UC8179 controller):
// 800x480 pixels, 4 grayscale levels (2 bits per pixel).
const (
	Width  = 800
	Height = 480
)

// Grayscale levels, matching the panel's TFT_GRAY_0..3 constants.
const (
	Black uint8 = iota
	DarkGray
	LightGray
	White
)

// GrayImage is a raster image using one of the panel's 4 grayscale levels
// per pixel, stored row-major.
type GrayImage struct {
	Width  int
	Height int
	Pixels []uint8
}

func NewGrayImage(width, height int) *GrayImage {
	return &GrayImage{
		Width:  width,
		Height: height,
		Pixels: make([]uint8, width*height),
	}
}

func (img *GrayImage) At(x, y int) uint8 {
	return img.Pixels[y*img.Width+x]
}

func (img *GrayImage) Set(x, y int, level uint8) {
	if x < 0 || x >= img.Width || y < 0 || y >= img.Height {
		return
	}
	img.Pixels[y*img.Width+x] = level & 0x03
}

// baseFont and boldFont are the embedded Go Mono TTFs (regular and bold),
// parsed once at package init. Unlike the bitmap font used before
// (basicfont.Face7x13, ASCII-only), these cover full Unicode — accented
// letters like "ñ" render correctly — and rasterize directly at whatever
// point size is needed, with no integer-upscale step.
var (
	baseFont = mustParseFont(gomono.TTF)
	boldFont = mustParseFont(gomonobold.TTF)
)

func mustParseFont(ttf []byte) *opentype.Font {
	f, err := opentype.Parse(ttf)
	if err != nil {
		// The embedded font data is fixed at compile time; a parse failure
		// here would mean the vendored TTF itself is corrupt, not a
		// reachable runtime condition.
		panic("display: parsing embedded font: " + err.Error())
	}
	return f
}

// newFace rasterizes baseFont at the given point size (DPI fixed at 72, so
// size reads directly as an approximate pixel line height).
func newFace(size float64) font.Face { return mustNewFace(baseFont, size) }

// newBoldFace is newFace's bold counterpart, used for section sub-headers
// that need to stand out from regular body rows.
func newBoldFace(size float64) font.Face { return mustNewFace(boldFont, size) }

func mustNewFace(f *opentype.Font, size float64) font.Face {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		panic("display: creating font face: " + err.Error())
	}
	return face
}

// NewHelloWorld builds the placeholder image for this first iteration:
// "Hello World" plus the current time, rendered on the panel canvas.
// It will be replaced by real Google Calendar/Sheets content later.
func NewHelloWorld(now time.Time) *GrayImage {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	drawText(canvas, newFace(48), "Hello World", 40, 90)
	drawText(canvas, newFace(24), now.Format("2006-01-02 15:04:05"), 40, 180)

	return fromGray(canvas)
}

// Section is a titled group of body rows within a NewSections image. An
// empty Title renders no sub-header — used by NewTextRows's single-section
// wrapper, whose only caller (the calendar-fetch-error fallback) has
// nothing of its own to label. Every section NewDailyLayout renders
// (agenda, shopping list, each weekly-menu day) has an explicit Title.
type Section struct {
	Title string
	Lines []string

	// Bulleted, when true, renders Lines as a wrapped bullet list — each
	// item prefixed with its own small filled circle, and consecutive
	// short items packed onto the same line — instead of the default one
	// item per row. Meant for flat, unordered item lists where cramming
	// several onto a line wastes no meaning (e.g. the shopping list); not
	// for content like the agenda where each line's position/time matters.
	Bulleted bool

	// Events, when non-empty, renders as a bulleted list with each
	// entry's Time in bold and Text word-wrapped in the regular weight —
	// used for the agenda, where a summary can run long enough to need
	// real wrapping (unlike Bulleted's short shopping-list items, which
	// are packed several-per-line instead of wrapped). Takes precedence
	// over Lines/Bulleted when set.
	Events []EventLine
}

// NewTextRows renders a header line followed by a single untitled section
// of body rows. A thin convenience wrapper around NewSections for the
// common single-section case (e.g. an error message).
func NewTextRows(header, footer string, rows []string) *GrayImage {
	return NewSections(header, footer, []Section{{Lines: rows}})
}

// Layout constants shared by NewSections and NewDailyLayout.
const (
	marginX              = 24
	headerFontSize       = 28
	headerY              = 20
	headerLineHeight     = 44
	rowFontSize          = 18
	rowHeight            = 24
	sectionTitleFontSize = 24
	sectionTitleHeight   = 34
	sectionGap           = 12
)

// leftColumnX and rightColumnX are the two side-by-side columns
// NewDailyLayout's left/right sections render at. drawText doesn't wrap
// or clip text to a width, so a line wider than a column will simply
// overflow into (or past) its neighbor — an accepted limitation for this
// first pass, not something solved here.
const (
	leftColumnX  = marginX
	rightColumnX = Width/2 + marginX/2
)

// Column/content widths available to drawSections, used to decide when a
// Bulleted section needs to wrap to a new line. leftColumnWidth trims a
// further marginX/2 off the gap up to rightColumnX so wrapped bullet text
// doesn't butt directly against the right column's content.
const (
	leftColumnWidth  = rightColumnX - leftColumnX - marginX/2
	rightColumnWidth = Width - marginX - rightColumnX
	fullContentWidth = Width - 2*marginX
)

// Bullet-list layout constants, used by drawBulletedLines to pack a
// Section's items — each marked with a small filled circle, matching a
// markdown-style list — multiple per line instead of one per row.
const (
	bulletRadius    = 3
	bulletGap       = 6
	bulletUnitWidth = bulletRadius*2 + bulletGap
	itemGap         = 20
)

// Footer layout constants: footerFontSize is deliberately smaller than
// rowFontSize (18), the previous smallest text on the panel, since the
// footer is a diagnostic aside (generation time + battery level) that
// shouldn't compete visually with real content. footerMarginBottom is the
// gap from the panel's bottom edge to the footer's own bottom edge; the
// footer reuses marginX for its right-edge gap rather than adding a
// second "right margin" constant that would always equal the same value.
const (
	footerFontSize     = 12
	footerMarginBottom = 12
)

// NewSections renders a main header followed by one or more sections, each
// optionally with its own bold sub-header line. It's a generic enough
// primitive to serve any content source that reduces to "a title plus a
// few grouped rows" (today's agenda, the shopping list, an error message,
// and future content sources alike) without this package needing to know
// anything about where each section's text came from.
func NewSections(header, footer string, sections []Section) *GrayImage {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	y := drawHeader(canvas, header)
	drawSections(canvas, marginX, y, fullContentWidth, sections)
	drawFooter(canvas, footer)

	return fromGray(canvas)
}

// NewDailyLayout renders the main header, then left and right as two
// side-by-side columns (each only as tall as its own content), then
// bottom as a single full-width region starting below whichever column
// ends lower. Meant for the daily display's actual content (agenda |
// shopping list, weekly menu below); NewSections/NewTextRows are kept
// separate and unchanged since they're still used for single-column
// content like the calendar-error fallback.
func NewDailyLayout(header, footer string, left, right, bottom []Section) *GrayImage {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	topY := drawHeader(canvas, header)
	leftEndY := drawSections(canvas, leftColumnX, topY, leftColumnWidth, left)
	rightEndY := drawSections(canvas, rightColumnX, topY, rightColumnWidth, right)

	bottomY := leftEndY
	if rightEndY > bottomY {
		bottomY = rightEndY
	}
	drawSections(canvas, marginX, bottomY+sectionGap, fullContentWidth, bottom)
	drawFooter(canvas, footer)

	return fromGray(canvas)
}

// drawHeader draws the shared main header line and returns the y
// immediately below it, where body content should start.
func drawHeader(canvas *image.Gray, header string) int {
	drawText(canvas, newBoldFace(headerFontSize), header, marginX, headerY)
	return headerY + headerLineHeight
}

// drawSections stacks sections vertically at horizontal offset x starting
// at y, wrapping within width (needed by Bulleted and Events sections, to
// decide when a line is full), and returns the y immediately below the
// last line drawn, so a caller can chain another region beneath it (or
// beneath several, side by side, as NewDailyLayout does for its bottom
// region).
func drawSections(canvas *image.Gray, x, y, width int, sections []Section) int {
	rowFace := newFace(rowFontSize)
	boldRowFace := newBoldFace(rowFontSize)
	sectionTitleFace := newBoldFace(sectionTitleFontSize)
	for i, sec := range sections {
		if sec.Title != "" {
			if i > 0 {
				y += sectionGap
			}
			drawText(canvas, sectionTitleFace, sec.Title, x, y)
			y += sectionTitleHeight
		}
		switch {
		case len(sec.Events) > 0:
			y = drawEventLines(canvas, boldRowFace, rowFace, x, y, width, sec.Events)
		case sec.Bulleted:
			y = drawBulletedLines(canvas, rowFace, x, y, width, sec.Lines)
		default:
			for _, line := range sec.Lines {
				drawText(canvas, rowFace, line, x, y)
				y += rowHeight
			}
		}
	}
	return y
}

// drawBulletedLines renders items as a markdown-style bullet list: each
// item gets its own small filled circle, and consecutive items are packed
// onto the same line — separated by itemGap — until the next one would
// overflow width, instead of giving every item a full-width row to itself.
// Returns the y immediately below the last line drawn (unchanged from y if
// items is empty, matching the plain-line loop's behavior for no lines).
func drawBulletedLines(canvas *image.Gray, face font.Face, x, y, width int, items []string) int {
	if len(items) == 0 {
		return y
	}
	lineX := x
	for _, item := range items {
		unitWidth := bulletUnitWidth + measureWidth(face, item)
		if lineX > x && lineX+unitWidth > x+width {
			y += rowHeight
			lineX = x
		}
		drawBullet(canvas, face, lineX, y)
		drawText(canvas, face, item, lineX+bulletUnitWidth, y)
		lineX += unitWidth + itemGap
	}
	return y + rowHeight
}

// drawBullet draws the small filled circle marking one bulleted item, at
// (x, y) being the same top-left coordinate drawText takes for the row's
// text, vertically centered on face's line height.
func drawBullet(dst *image.Gray, face font.Face, x, y int) {
	cy := y + face.Metrics().Height.Ceil()/2
	cx := x + bulletRadius
	for dy := -bulletRadius; dy <= bulletRadius; dy++ {
		for dx := -bulletRadius; dx <= bulletRadius; dx++ {
			if dx*dx+dy*dy <= bulletRadius*bulletRadius {
				dst.SetGray(cx+dx, cy+dy, color.Gray{Y: 0})
			}
		}
	}
}

// measureWidth returns the rendered pixel width of s in face, rounded to
// the nearest whole pixel.
func measureWidth(face font.Face, s string) int {
	return font.MeasureString(face, s).Round()
}

// drawFooter draws footer right-aligned to the panel's bottom-right
// corner, marginX from the right edge and footerMarginBottom above the
// bottom edge. A blank footer draws nothing, matching drawBulletedLines's
// empty-input guard.
func drawFooter(canvas *image.Gray, footer string) {
	if footer == "" {
		return
	}
	face := newFace(footerFontSize)
	x := Width - marginX - measureWidth(face, footer)
	y := Height - footerMarginBottom - face.Metrics().Height.Ceil()
	drawText(canvas, face, footer, x, y)
}

// drawText draws s with face onto dst, with (x, y) as the top-left corner
// of the line (not the baseline the underlying font.Drawer works in).
// Glyph edges are anti-aliased grayscale rather than a crisp 1-bit bitmap
// — the panel has 4 real gray levels (see quantize), so this is simply
// more detail, not something that needs flattening away.
func drawText(dst *image.Gray, face font.Face, s string, x, y int) {
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.Black,
		Face: face,
		Dot:  fixed.P(x, y+face.Metrics().Ascent.Ceil()),
	}
	d.DrawString(s)
}

func fromGray(g *image.Gray) *GrayImage {
	bounds := g.Bounds()
	img := NewGrayImage(bounds.Dx(), bounds.Dy())
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			img.Set(x, y, quantize(g.GrayAt(bounds.Min.X+x, bounds.Min.Y+y).Y))
		}
	}
	return img
}

// quantize maps an 8-bit grayscale value to one of the panel's 4 levels.
func quantize(v uint8) uint8 {
	switch {
	case v < 64:
		return Black
	case v < 128:
		return DarkGray
	case v < 192:
		return LightGray
	default:
		return White
	}
}
