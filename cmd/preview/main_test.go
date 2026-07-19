package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/pianista215/my-assistant/internal/display"
)

func TestSavePNGWritesDecodableNativeResolutionImage(t *testing.T) {
	img := display.NewGrayImage(display.Width, display.Height)
	img.Set(10, 10, display.Black)

	path := filepath.Join(t.TempDir(), "out.png")
	if err := savePNG(path, img); err != nil {
		t.Fatalf("savePNG() error = %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening saved PNG: %v", err)
	}
	defer f.Close()

	decoded, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decoding saved PNG: %v", err)
	}

	bounds := decoded.Bounds()
	if bounds.Dx() != display.Width || bounds.Dy() != display.Height {
		t.Fatalf("saved PNG size = %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), display.Width, display.Height)
	}
}

// A single dark pixel anywhere in the block must win, even when it's a
// lone stroke of 1px-wide text lost by naive nearest-neighbor sampling.
func TestDarkestInBlockFindsLoneDarkPixel(t *testing.T) {
	img := display.NewGrayImage(8, 8)
	for i := range img.Pixels {
		img.Pixels[i] = display.White
	}
	img.Set(5, 5, display.Black)

	if got := darkestInBlock(img, 0, 0, 8); got != display.Black {
		t.Fatalf("darkestInBlock() = %d, want %d (Black)", got, display.Black)
	}
}

func TestDarkestInBlockAllWhite(t *testing.T) {
	img := display.NewGrayImage(4, 4)
	for i := range img.Pixels {
		img.Pixels[i] = display.White
	}

	if got := darkestInBlock(img, 0, 0, 4); got != display.White {
		t.Fatalf("darkestInBlock() = %d, want %d (White)", got, display.White)
	}
}

func TestDarkestInBlockClampsAtImageBounds(t *testing.T) {
	img := display.NewGrayImage(3, 3)
	for i := range img.Pixels {
		img.Pixels[i] = display.White
	}
	img.Set(2, 2, display.Black)

	// Block extends past the image edge; must not panic and must still
	// find the dark pixel within bounds.
	if got := darkestInBlock(img, 0, 0, 10); got != display.Black {
		t.Fatalf("darkestInBlock() = %d, want %d (Black)", got, display.Black)
	}
}
