package display

import (
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
