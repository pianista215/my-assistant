// Command preview renders a display buffer (our own binary format, see
// internal/display) in the terminal, so we can inspect what the ESP32
// would show without owning the actual e-ink panel.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/pianista215/my-assistant/internal/display"
)

// ANSI 256-color grayscale ramp (232 darkest .. 255 lightest), one entry
// per panel grayscale level (0=black .. 3=white).
var grayANSI = [4]int{232, 240, 248, 255}

func main() {
	url := flag.String("url", "", "URL of the /api/v1/display endpoint")
	token := flag.String("token", "", "Bearer token to send as Authorization header")
	file := flag.String("file", "", "Path to a file with an already downloaded buffer")
	cols := flag.Int("cols", 120, "Output width in terminal columns")
	flag.Parse()

	data, err := loadData(*url, *token, *file)
	if err != nil {
		log.Fatalf("preview: %v", err)
	}

	img, err := display.Decode(data)
	if err != nil {
		log.Fatalf("preview: decoding buffer: %v", err)
	}

	render(os.Stdout, img, *cols)
}

func loadData(url, token, file string) ([]byte, error) {
	switch {
	case file != "":
		return os.ReadFile(file)
	case url != "":
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %s", resp.Status)
		}
		return io.ReadAll(resp.Body)
	default:
		return nil, fmt.Errorf("must provide --url or --file")
	}
}

// render prints the image using half-block characters: the foreground
// color paints the top pixel of a terminal cell, the background paints
// the bottom pixel, doubling the effective vertical resolution. The
// image is downsampled by nearest-neighbor to fit the requested width.
func render(w io.Writer, img *display.GrayImage, cols int) {
	if cols <= 0 || cols > img.Width {
		cols = img.Width
	}
	scale := img.Width / cols
	if scale < 1 {
		scale = 1
	}
	rowsOut := img.Height / (scale * 2)

	for ry := 0; ry < rowsOut; ry++ {
		for rx := 0; rx < cols; rx++ {
			top := darkestInBlock(img, rx*scale, ry*scale*2, scale)
			bottom := darkestInBlock(img, rx*scale, ry*scale*2+scale, scale)
			fmt.Fprintf(w, "\x1b[38;5;%dm\x1b[48;5;%dm▀", grayANSI[top], grayANSI[bottom])
		}
		fmt.Fprint(w, "\x1b[0m\n")
	}
}

// darkestInBlock returns the darkest (lowest) grayscale level found in the
// size x size block starting at (x, y). Downsampling with a single nearest
// pixel would routinely skip over 1px-wide text strokes; taking the darkest
// pixel per block keeps thin content visible instead of aliasing it away.
func darkestInBlock(img *display.GrayImage, x, y, size int) uint8 {
	darkest := display.White
	for dy := 0; dy < size; dy++ {
		py := y + dy
		if py >= img.Height {
			break
		}
		for dx := 0; dx < size; dx++ {
			px := x + dx
			if px >= img.Width {
				break
			}
			if level := img.At(px, py); level < darkest {
				darkest = level
			}
		}
	}
	return darkest
}
