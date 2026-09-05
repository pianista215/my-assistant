package display

import (
	"image"
	"image/draw"
	"testing"
)

// countBlackPixels returns how many pixels of canvas are pure black,
// used below to assert a QR code actually got drawn without depending on
// its exact module layout.
func countBlackPixels(canvas *image.Gray) int {
	count := 0
	for _, v := range canvas.Pix {
		if v == 0 {
			count++
		}
	}
	return count
}

func TestDrawQRCodeDrawsWithinBounds(t *testing.T) {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	const x, y, size = 100, 50, 200
	if err := drawQRCode(canvas, "https://example.org/device?code=ABCD-EFGH", x, y, size); err != nil {
		t.Fatalf("drawQRCode: %v", err)
	}

	if got := countBlackPixels(canvas); got == 0 {
		t.Fatal("drawQRCode drew no black pixels, want a visible code")
	}

	// No black pixel should land outside the requested box (modules are
	// sized to fit within it, never past it).
	bounds := canvas.Bounds()
	for py := bounds.Min.Y; py < bounds.Max.Y; py++ {
		for px := bounds.Min.X; px < bounds.Max.X; px++ {
			if canvas.GrayAt(px, py).Y != 0 {
				continue
			}
			if px < x || px >= x+size || py < y || py >= y+size {
				t.Fatalf("black pixel at (%d, %d) outside requested box [%d,%d)-[%d,%d)", px, py, x, x+size, y, y+size)
			}
		}
	}
}

func TestDrawQRCodeEmptyContentErrors(t *testing.T) {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	if err := drawQRCode(canvas, "", 0, 0, 100); err == nil {
		t.Fatal("drawQRCode(\"\") = nil error, want an error encoding empty content")
	}
}

func TestNewReauthScreenDimensions(t *testing.T) {
	img := NewReauthScreen("Reautorización necesaria", "12:00:00 - 80%",
		[]string{"Escanea el código con el móvil.", "O entra en la URL e introduce el código."},
		"https://www.google.com/device?user_code=ABCD-EFGH", "ABCD-EFGH")

	if img.Width != Width || img.Height != Height {
		t.Fatalf("NewReauthScreen size = %dx%d, want %dx%d", img.Width, img.Height, Width, Height)
	}

	hasBlack := false
	for _, v := range img.Pixels {
		if v == Black {
			hasBlack = true
			break
		}
	}
	if !hasBlack {
		t.Fatal("NewReauthScreen produced an image with no black pixels at all")
	}
}

func TestNewReauthScreenWithoutUserCodeStillRenders(t *testing.T) {
	// VerificationURIComplete case: no manual code needed, userCode is
	// empty — should render fine without drawing a (blank) code line.
	img := NewReauthScreen("Reautorización necesaria", "", []string{"Escanea el código."},
		"https://www.google.com/device?user_code=ABCD-EFGH", "")
	if img == nil {
		t.Fatal("NewReauthScreen returned nil")
	}
}
