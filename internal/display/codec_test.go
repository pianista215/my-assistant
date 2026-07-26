package display

import (
	"image"
	"image/draw"
	"testing"
	"time"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	cases := []struct {
		name          string
		width, height int
	}{
		{"small square", 4, 4},
		{"non multiple of 4 pixels", 5, 3},
		{"panel size", Width, Height},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := NewGrayImage(tc.width, tc.height)
			for i := range img.Pixels {
				img.Pixels[i] = uint8(i % 4)
			}

			encoded, err := Encode(img)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			if decoded.Width != img.Width || decoded.Height != img.Height {
				t.Fatalf("dimensions mismatch: got %dx%d, want %dx%d",
					decoded.Width, decoded.Height, img.Width, img.Height)
			}

			for i := range img.Pixels {
				if decoded.Pixels[i] != img.Pixels[i] {
					t.Fatalf("pixel %d mismatch: got %d, want %d", i, decoded.Pixels[i], img.Pixels[i])
				}
			}
		})
	}
}

func TestDecodeRejectsInvalidData(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"too short", []byte{0x01, 0x02}},
		{"bad magic", append([]byte("NOPE"), make([]byte, 6)...)},
		{"truncated payload", func() []byte {
			img := NewGrayImage(10, 10)
			encoded, _ := Encode(img)
			return encoded[:len(encoded)-1]
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decode(tc.data); err == nil {
				t.Fatal("Decode() expected an error, got nil")
			}
		})
	}
}

func TestNewHelloWorldProducesPanelSizedImage(t *testing.T) {
	img := NewHelloWorld(time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC))

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
		t.Fatal("expected the rendered text to produce at least one non-white pixel")
	}
}

func TestNewTextRowsProducesPanelSizedImage(t *testing.T) {
	cases := []struct {
		name   string
		header string
		footer string
		rows   []string
	}{
		{"multiple rows", "Monday, 19 July 2026", "15:04:05 - 87%", []string{"09:00  Dentist", "10:00-11:00  Standup"}},
		{"no rows", "Monday, 19 July 2026", "15:04:05 - 87%", []string{"No events today"}},
		{"error message", "Could not load calendar", "15:04:05 - 87%", []string{"2026-07-19 15:04:05", "boom"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := NewTextRows(tc.header, tc.footer, tc.rows)

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
				t.Fatal("expected the rendered text to produce at least one non-white pixel")
			}
		})
	}
}

func TestNewDailyLayoutProducesPanelSizedImage(t *testing.T) {
	cases := []struct {
		name   string
		header string
		footer string
		left   []Section
		right  []Section
		bottom []Section
	}{
		{
			"agenda, shopping list and menu",
			"Wednesday, 22 July 2026",
			"15:04:05 - 87%",
			[]Section{{Title: "Eventos", Lines: []string{"09:00  Dentist", "10:00-11:00  Standup"}}},
			[]Section{{Title: "Lista de la compra", Lines: []string{"Leche", "Pan"}}},
			[]Section{
				{Title: "Miércoles", Lines: []string{"Comida: Lentejas", "Cena: Tortilla"}},
				{Title: "Jueves", Lines: []string{"Comida: Pasta", "Cena: (sin planificar)"}},
			},
		},
		{
			"all sections empty",
			"Wednesday, 22 July 2026",
			"15:04:05 - 87%",
			nil,
			nil,
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := NewDailyLayout(tc.header, tc.footer, tc.left, tc.right, tc.bottom)

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
				t.Fatal("expected the rendered text to produce at least one non-white pixel")
			}
		})
	}
}

func TestDrawFooterRightAligned(t *testing.T) {
	newBlankCanvas := func() *image.Gray {
		canvas := image.NewGray(image.Rect(0, 0, Width, Height))
		draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
		return canvas
	}

	t.Run("empty footer draws nothing", func(t *testing.T) {
		canvas := newBlankCanvas()
		drawFooter(canvas, "")

		for _, pix := range canvas.Pix {
			if pix != 255 {
				t.Fatal(`expected drawFooter("") to leave the canvas untouched`)
			}
		}
	})

	t.Run("footer text is right-aligned to the bottom-right corner", func(t *testing.T) {
		canvas := newBlankCanvas()
		drawFooter(canvas, "15:04:05 - 87%")

		face := newFace(footerFontSize)
		bandTop := Height - footerMarginBottom - face.Metrics().Height.Ceil()
		bandBottom := Height - footerMarginBottom

		var sawNonWhiteLeft, sawNonWhiteRight bool
		for y := bandTop; y < bandBottom; y++ {
			for x := 0; x < Width/2; x++ {
				if canvas.GrayAt(x, y).Y != 255 {
					sawNonWhiteLeft = true
				}
			}
			for x := Width / 2; x < Width; x++ {
				if canvas.GrayAt(x, y).Y != 255 {
					sawNonWhiteRight = true
				}
			}
		}
		if sawNonWhiteLeft {
			t.Fatal("expected no footer content in the left half of the panel")
		}
		if !sawNonWhiteRight {
			t.Fatal("expected footer content in the right half of the panel")
		}
	})
}
