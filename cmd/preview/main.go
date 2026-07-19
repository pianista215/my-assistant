// Command preview renders a display buffer (our own binary format, see
// internal/display) either in the terminal or as a real PNG image, so we
// can inspect what the ESP32 would show without owning the actual e-ink
// panel.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/pianista215/my-assistant/internal/display"
)

// ANSI 256-color grayscale ramp (232 darkest .. 255 lightest), one entry
// per panel grayscale level (0=black .. 3=white).
var grayANSI = [4]int{232, 240, 248, 255}

// Evenly spaced 8-bit grayscale values for PNG output, one entry per panel
// grayscale level (0=black .. 3=white).
var grayPNG = [4]uint8{0, 85, 170, 255}

func main() {
	url := flag.String("url", "", "URL of the /api/v1/display endpoint")
	token := flag.String("token", "", "Bearer token to send as Authorization header")
	file := flag.String("file", "", "Path to a file with an already downloaded buffer")
	cols := flag.Int("cols", 120, "Output width in terminal columns (terminal mode only)")
	pngPath := flag.String("png", "", "Save the image as a PNG at this path instead of printing to the terminal")
	open := flag.Bool("open", false, "Save the image as a PNG and open it with the OS default viewer")
	flag.Parse()

	data, err := loadData(*url, *token, *file)
	if err != nil {
		log.Fatalf("preview: %v", err)
	}

	img, err := display.Decode(data)
	if err != nil {
		log.Fatalf("preview: decoding buffer: %v", err)
	}

	switch {
	case *open:
		path := *pngPath
		if path == "" {
			path, err = tempPNGPath()
			if err != nil {
				log.Fatalf("preview: %v", err)
			}
		}
		if err := savePNG(path, img); err != nil {
			log.Fatalf("preview: %v", err)
		}
		if err := openInViewer(path); err != nil {
			log.Fatalf("preview: opening %s: %v", path, err)
		}
		fmt.Println(path)
	case *pngPath != "":
		if err := savePNG(*pngPath, img); err != nil {
			log.Fatalf("preview: %v", err)
		}
		fmt.Println(*pngPath)
	default:
		render(os.Stdout, img, *cols)
	}
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

// savePNG writes img to path as a real 800x480 PNG using the panel's actual
// grayscale values, so it can be inspected at native resolution instead of
// downsampled to terminal columns.
func savePNG(path string, img *display.GrayImage) error {
	out := image.NewGray(image.Rect(0, 0, img.Width, img.Height))
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			out.SetGray(x, y, color.Gray{Y: grayPNG[img.At(x, y)]})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, out)
}

func tempPNGPath() (string, error) {
	f, err := os.CreateTemp("", "eink-preview-*.png")
	if err != nil {
		return "", err
	}
	path := f.Name()
	return path, f.Close()
}

// openInViewer opens path with the OS's default image viewer/browser. It
// doesn't wait for the viewer to exit, since viewers/browsers are typically
// long-lived.
func openInViewer(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
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
