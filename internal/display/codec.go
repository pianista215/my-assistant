package display

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// Custom wire format for the ESP32: a small header followed by the pixel
// data packed at 2 bits per pixel (4 pixels per byte, MSB first), with no
// per-row padding. There's no off-the-shelf image format worth adopting
// for a 4-level grayscale panel this small, so we define our own.
//
//	offset  size  field
//	0       4     magic "EINK"
//	4       1     format version
//	5       2     width  (big-endian uint16)
//	7       2     height (big-endian uint16)
//	9       1     bits per pixel
//	10      ...   packed pixel data
const (
	headerLen     = 10
	formatVersion = 1
	bitsPerPixel  = 2
)

var magic = [4]byte{'E', 'I', 'N', 'K'}

func Encode(img *GrayImage) ([]byte, error) {
	if img.Width <= 0 || img.Height <= 0 || img.Width > 0xFFFF || img.Height > 0xFFFF {
		return nil, errors.New("display: invalid image dimensions")
	}

	buf := new(bytes.Buffer)
	buf.Write(magic[:])
	buf.WriteByte(formatVersion)
	_ = binary.Write(buf, binary.BigEndian, uint16(img.Width))
	_ = binary.Write(buf, binary.BigEndian, uint16(img.Height))
	buf.WriteByte(bitsPerPixel)

	var b byte
	var bitCount uint
	for _, level := range img.Pixels {
		b = (b << 2) | (level & 0x03)
		bitCount += 2
		if bitCount == 8 {
			buf.WriteByte(b)
			b, bitCount = 0, 0
		}
	}
	if bitCount > 0 {
		b <<= 8 - bitCount
		buf.WriteByte(b)
	}

	return buf.Bytes(), nil
}

func Decode(data []byte) (*GrayImage, error) {
	if len(data) < headerLen {
		return nil, errors.New("display: data too short for header")
	}
	if !bytes.Equal(data[:4], magic[:]) {
		return nil, errors.New("display: invalid magic header")
	}

	version := data[4]
	if version != formatVersion {
		return nil, fmt.Errorf("display: unsupported format version %d", version)
	}

	width := int(binary.BigEndian.Uint16(data[5:7]))
	height := int(binary.BigEndian.Uint16(data[7:9]))
	bpp := data[9]
	if bpp != bitsPerPixel {
		return nil, fmt.Errorf("display: unsupported bits per pixel %d", bpp)
	}

	img := NewGrayImage(width, height)
	totalPixels := width * height
	pixelIdx := 0

	for _, b := range data[headerLen:] {
		for shift := 6; shift >= 0 && pixelIdx < totalPixels; shift -= 2 {
			img.Pixels[pixelIdx] = (b >> uint(shift)) & 0x03
			pixelIdx++
		}
	}

	if pixelIdx != totalPixels {
		return nil, errors.New("display: payload shorter than expected")
	}

	return img, nil
}
