package display

import (
	"image"
	"image/color"
	"image/draw"

	qrcode "github.com/skip2/go-qrcode"
)

// reauthQRSize is the QR code's rendered pixel size, and reauthCodeFontSize
// the size of the manual-entry code shown below it — both picked to leave
// comfortable room for the header/instructions above and the footer below
// at the panel's 480px height (see NewReauthScreen's layout comment).
const (
	reauthQRSize         = 240
	reauthCodeFontSize   = 32
	reauthInstructionGap = sectionGap
)

// drawQRCode renders data as a QR code within a size x size square at
// (x, y): one solid black square per module, no antialiasing or scaling
// blur (unlike drawIcon's BiLinear-scaled PNGs) — a blurred module edge
// risks a phone camera misreading it. Each module is drawn at an integer
// pixel width so every module stays the same size; the rendered code may
// end up a little smaller than size to keep that true, rather than
// stretching unevenly to fill it exactly. The background is left
// untouched (already white, like every other New* canvas), matching
// drawBullet's "only draw the foreground" approach.
func drawQRCode(canvas *image.Gray, data string, x, y, size int) error {
	qr, err := qrcode.New(data, qrcode.Medium)
	if err != nil {
		return err
	}
	bitmap := qr.Bitmap()
	modules := len(bitmap)
	if modules == 0 {
		return nil
	}
	modulePx := size / modules
	if modulePx < 1 {
		modulePx = 1
	}
	for row := range bitmap {
		for col, dark := range bitmap[row] {
			if !dark {
				continue
			}
			px0, py0 := x+col*modulePx, y+row*modulePx
			for py := py0; py < py0+modulePx; py++ {
				for px := px0; px < px0+modulePx; px++ {
					canvas.SetGray(px, py, color.Gray{Y: 0})
				}
			}
		}
	}
	return nil
}

// NewReauthScreen is the whole-screen takeover shown in place of the
// generic calendar-fetch-error fallback when the stored Google credential
// has actually expired (see internal/oauthrenewal): lines explains what's
// happening, a QR code points at verificationURL so scanning it from a
// phone opens the renewal page directly, and — only when Google didn't
// embed the code in the URL itself (VerificationURIComplete empty) —
// userCode is shown as large text to type in by hand. Content-agnostic
// like every other New* entry point: internal/server supplies all the
// Spanish copy and decides whether userCode is needed; this function only
// lays it out, reusing drawHeader/drawFooter so the screen stays visually
// consistent with the rest of the app.
func NewReauthScreen(header, footer string, lines []string, verificationURL, userCode string) *GrayImage {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	y := drawHeader(canvas, header)
	rowFace := newFace(rowFontSize)
	for _, line := range lines {
		drawText(canvas, rowFace, line, marginX, y)
		y += rowHeight
	}
	y += reauthInstructionGap

	qrX := (Width - reauthQRSize) / 2
	if err := drawQRCode(canvas, verificationURL, qrX, y, reauthQRSize); err != nil {
		drawText(canvas, rowFace, "No se pudo generar el código QR", marginX, y)
	}
	y += reauthQRSize + reauthInstructionGap

	if userCode != "" {
		codeFace := newBoldFace(reauthCodeFontSize)
		codeX := (Width - measureWidth(codeFace, userCode)) / 2
		drawText(canvas, codeFace, userCode, codeX, y)
	}

	drawFooter(canvas, footer)
	return fromGray(canvas)
}
