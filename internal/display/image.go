// Package display builds and encodes the images shown on the e-ink panel.
package display

import (
	"image"
	"image/draw"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
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

// NewHelloWorld builds the placeholder image for this first iteration:
// "Hello World" plus the current time, rendered on the panel canvas.
// It will be replaced by real Google Calendar/Sheets content later.
func NewHelloWorld(now time.Time) *GrayImage {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	// basicfont.Face7x13 is only 7x13px per glyph: unreadably small on an
	// 800x480 panel (and on a downsampled terminal preview) if drawn at
	// native size, so it's rendered small and then upscaled with nearest
	// neighbor to a size that's actually legible.
	drawScaledText(canvas, "Hello World", 40, 100, 5)
	drawScaledText(canvas, now.Format("2006-01-02 15:04:05"), 40, 220, 3)

	return fromGray(canvas)
}

// drawScaledText renders s with basicfont.Face7x13 onto a small offscreen
// canvas, then blits it onto dst at (x, y) scaled up by an integer factor
// using nearest-neighbor, so it stays a crisp 1-bit-per-pixel bitmap font
// (no gray anti-aliasing, which the 4-level panel can't represent well).
func drawScaledText(dst *image.Gray, s string, x, y, scale int) {
	face := basicfont.Face7x13
	metrics := face.Metrics()
	width := font.MeasureString(face, s).Ceil()
	height := (metrics.Ascent + metrics.Descent).Ceil()

	small := image.NewGray(image.Rect(0, 0, width, height))
	draw.Draw(small, small.Bounds(), image.White, image.Point{}, draw.Src)

	d := &font.Drawer{
		Dst:  small,
		Src:  image.Black,
		Face: face,
		Dot:  fixed.P(0, metrics.Ascent.Ceil()),
	}
	d.DrawString(s)

	dstBounds := dst.Bounds()
	for sy := 0; sy < height; sy++ {
		for sx := 0; sx < width; sx++ {
			level := small.GrayAt(sx, sy)
			for oy := 0; oy < scale; oy++ {
				for ox := 0; ox < scale; ox++ {
					px, py := x+sx*scale+ox, y+sy*scale+oy
					if px < dstBounds.Dx() && py < dstBounds.Dy() {
						dst.SetGray(px, py, level)
					}
				}
			}
		}
	}
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
