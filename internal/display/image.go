// Package display builds and encodes the images shown on the e-ink panel.
package display

import (
	"image"
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
// empty Title renders no sub-header — used for a section that sits
// directly under the main header with nothing of its own to label (e.g.
// today's agenda, the first section on the panel).
type Section struct {
	Title string
	Lines []string
}

// NewTextRows renders a header line followed by a single untitled section
// of body rows. A thin convenience wrapper around NewSections for the
// common single-section case (e.g. an error message).
func NewTextRows(header string, rows []string) *GrayImage {
	return NewSections(header, []Section{{Lines: rows}})
}

// NewSections renders a main header followed by one or more sections, each
// optionally with its own bold sub-header line. It's a generic enough
// primitive to serve any content source that reduces to "a title plus a
// few grouped rows" (today's agenda, the shopping list, an error message,
// and future content sources alike) without this package needing to know
// anything about where each section's text came from.
func NewSections(header string, sections []Section) *GrayImage {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

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

	drawText(canvas, newFace(headerFontSize), header, marginX, headerY)

	rowFace := newFace(rowFontSize)
	sectionTitleFace := newBoldFace(sectionTitleFontSize)
	y := headerY + headerLineHeight
	for i, sec := range sections {
		if sec.Title != "" {
			if i > 0 {
				y += sectionGap
			}
			drawText(canvas, sectionTitleFace, sec.Title, marginX, y)
			y += sectionTitleHeight
		}
		for _, line := range sec.Lines {
			drawText(canvas, rowFace, line, marginX, y)
			y += rowHeight
		}
	}

	return fromGray(canvas)
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
