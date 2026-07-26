package display

import (
	"image"
	"image/draw"
	"strings"
	"testing"
)

func TestWrapTextStaysWithinMaxWidth(t *testing.T) {
	face := newFace(rowFontSize)
	text := "This is a moderately long sentence that should wrap across several lines within a narrow column width."
	maxWidth := 150

	lines := wrapText(face, text, maxWidth)
	if len(lines) < 2 {
		t.Fatalf("len(lines) = %d, want >= 2 for a long sentence wrapped to a narrow width", len(lines))
	}
	for _, line := range lines {
		if w := measureWidth(face, line); w > maxWidth {
			t.Errorf("line %q has width %d, want <= %d", line, w, maxWidth)
		}
	}
	if got := strings.Join(lines, " "); got != text {
		t.Errorf("rejoined lines = %q, want %q", got, text)
	}
}

func TestWrapTextEmpty(t *testing.T) {
	if lines := wrapText(newFace(rowFontSize), "", 100); lines != nil {
		t.Fatalf("wrapText(\"\") = %v, want nil", lines)
	}
}

func TestWrapTextSingleOverlongWordUnsplit(t *testing.T) {
	face := newFace(rowFontSize)
	word := "Supercalifragilisticexpialidocious"

	lines := wrapText(face, word, 10)
	if len(lines) != 1 || lines[0] != word {
		t.Fatalf("wrapText(%q, 10) = %v, want a single unsplit line", word, lines)
	}
}

// TestDrawEventLinesWrapsWithinColumnWidth is a regression test for the
// agenda overflowing into the right column when an event's summary was
// long: it draws a long-summary event into the left column and asserts
// no pixel is touched past the column's right boundary.
func TestDrawEventLinesWrapsWithinColumnWidth(t *testing.T) {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	longText := strings.Repeat("Very long summary text that would previously overflow the column boundary. ", 5)
	events := []EventLine{{Time: "09:00-10:00", Text: longText}}

	startY := 100
	endY := drawEventLines(canvas, newBoldFace(rowFontSize), newFace(rowFontSize), leftColumnX, startY, leftColumnWidth, events)
	if endY <= startY {
		t.Fatalf("endY = %d, want > %d (should have wrapped to at least one line)", endY, startY)
	}

	boundary := leftColumnX + leftColumnWidth
	for y := 0; y < Height; y++ {
		for x := boundary; x < Width; x++ {
			if canvas.GrayAt(x, y).Y != 255 {
				t.Fatalf("found non-white pixel at (%d,%d), past the column boundary at x=%d — text overflowed", x, y, boundary)
			}
		}
	}
}

func TestDrawEventLinesBoldTimePrefix(t *testing.T) {
	canvas := image.NewGray(image.Rect(0, 0, Width, Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	events := []EventLine{{Time: "09:00", Text: "Dentist"}}
	endY := drawEventLines(canvas, newBoldFace(rowFontSize), newFace(rowFontSize), leftColumnX, 100, leftColumnWidth, events)
	if endY != 100+rowHeight {
		t.Fatalf("endY = %d, want %d for a single unwrapped event", endY, 100+rowHeight)
	}

	var sawNonWhite bool
	for y := 100; y < endY; y++ {
		for x := leftColumnX; x < leftColumnX+leftColumnWidth; x++ {
			if canvas.GrayAt(x, y).Y != 255 {
				sawNonWhite = true
			}
		}
	}
	if !sawNonWhite {
		t.Fatal("expected the bullet/time/text to produce at least one non-white pixel")
	}
}
