// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"encoding/binary"
	"image"
	"image/draw"
	"io"
)

// EncodeOptions controls optional TGA encoding features.
type EncodeOptions struct {
	// RLE enables TGA RLE packet compression (types 10/11).
	RLE bool
}

// Encode writes the image m in TGA format to w.
// Supports *image.Gray (8-bit grayscale, type 3) and *image.NRGBA (32-bit true color, type 2).
// Other image types are converted to NRGBA. Origin is top-left (descriptor bit 5 set).
// No TGA 2.0 footer or extension area is written.
func Encode(w io.Writer, m image.Image) error {
	return EncodeWithOptions(w, m, nil)
}

// EncodeWithOptions writes the image m in TGA format to w using opts.
// Supports *image.Gray (8-bit grayscale) and *image.NRGBA (32-bit true color).
// Other image types are converted to NRGBA. Origin is top-left (descriptor bit 5 set).
// No TGA 2.0 footer or extension area is written.
func EncodeWithOptions(w io.Writer, m image.Image, opts *EncodeOptions) error {
	b := m.Bounds()
	mw := b.Dx()
	mh := b.Dy()

	if mw <= 0 || mh <= 0 {
		return ErrFormat
	}

	if mw > 0xffff || mh > 0xffff {
		return ErrFormat
	}

	// Bounds check above guarantees safe conversion to uint16.
	// #nosec G115 -- validated by range checks.
	mw16 := uint16(mw)
	// #nosec G115 -- validated by range checks.
	mh16 := uint16(mh)

	// 18-byte header: id=0, no color map, then image spec
	header := [18]byte{
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0,
		0, 0,
		0,
		0x20, // bit 5: top-left origin
	}
	binary.LittleEndian.PutUint16(header[12:14], mw16)
	binary.LittleEndian.PutUint16(header[14:16], mh16)

	settings := effectiveEncodeOptions(opts)

	switch src := m.(type) {
	case *image.Gray:
		if settings.RLE {
			header[2] = typeRLEGrayscale
		} else {
			header[2] = typeGrayscale
		}
		header[16] = 8
		if _, err := w.Write(header[:]); err != nil {
			return err
		}

		if settings.RLE {
			return encodeGrayRLE(w, src, b)
		}

		return encodeGray(w, src, b)

	case *image.NRGBA:
		if settings.RLE {
			header[2] = typeRLETrueColor
		} else {
			header[2] = typeTrueColor
		}
		header[16] = 32
		header[17] = 0x28 // 8 attribute bits, top-left
		if _, err := w.Write(header[:]); err != nil {
			return err
		}

		if settings.RLE {
			return encodeNRGBARLE(w, src, b)
		}

		return encodeNRGBA(w, src, b)

	default:
		if settings.RLE {
			header[2] = typeRLETrueColor
		} else {
			header[2] = typeTrueColor
		}
		header[16] = 32
		header[17] = 0x28
		if _, err := w.Write(header[:]); err != nil {
			return err
		}

		dst := image.NewNRGBA(b)
		draw.Draw(dst, b, m, b.Min, draw.Src)

		if settings.RLE {
			return encodeNRGBARLE(w, dst, b)
		}

		return encodeNRGBA(w, dst, b)
	}
}

// effectiveEncodeOptions resolves nil options to defaults.
func effectiveEncodeOptions(opts *EncodeOptions) EncodeOptions {
	if opts == nil {
		return EncodeOptions{}
	}

	return *opts
}

// encodeGray writes 8-bit grayscale pixel data in row order (top to bottom).
func encodeGray(w io.Writer, m *image.Gray, b image.Rectangle) error {
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := m.Pix[(y-m.Rect.Min.Y)*m.Stride : (y-m.Rect.Min.Y)*m.Stride+b.Dx()]
		if _, err := w.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// encodeGrayRLE writes 8-bit grayscale pixel data using TGA RLE packets.
func encodeGrayRLE(w io.Writer, m *image.Gray, b image.Rectangle) error {
	packed := packGrayPixels(m, b)
	return encodeRLEPackets(w, packed, 1)
}

// encodeNRGBA writes 32-bit BGRA pixel data in row order (top to bottom).
func encodeNRGBA(w io.Writer, m *image.NRGBA, b image.Rectangle) error {
	width := b.Dx()
	row := make([]byte, width*4)

	for y := b.Min.Y; y < b.Max.Y; y++ {
		i0 := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X-m.Rect.Min.X)*4
		copy(row, m.Pix[i0:i0+width*4])
		for i := 0; i < width*4; i += 4 {
			row[i+0], row[i+2] = row[i+2], row[i+0] // RGBA -> BGRA
		}

		if _, err := w.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// encodeNRGBARLE writes 32-bit BGRA pixel data using TGA RLE packets.
func encodeNRGBARLE(w io.Writer, m *image.NRGBA, b image.Rectangle) error {
	packed := packNRGBABGRAPixels(m, b)
	return encodeRLEPackets(w, packed, 4)
}

// packGrayPixels copies grayscale pixels into a contiguous top-to-bottom buffer.
func packGrayPixels(m *image.Gray, b image.Rectangle) []byte {
	width := b.Dx()
	height := b.Dy()
	packed := make([]byte, width*height)
	dst := 0

	for y := b.Min.Y; y < b.Max.Y; y++ {
		srcOffset := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X - m.Rect.Min.X)
		copy(packed[dst:dst+width], m.Pix[srcOffset:srcOffset+width])
		dst += width
	}

	return packed
}

// packNRGBABGRAPixels converts NRGBA rows to contiguous BGRA bytes.
func packNRGBABGRAPixels(m *image.NRGBA, b image.Rectangle) []byte {
	width := b.Dx()
	height := b.Dy()
	packed := make([]byte, width*height*4)
	dst := 0

	for y := b.Min.Y; y < b.Max.Y; y++ {
		srcOffset := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X-m.Rect.Min.X)*4
		row := m.Pix[srcOffset : srcOffset+width*4]

		for i := 0; i < len(row); i += 4 {
			packed[dst+0] = row[i+2] // B
			packed[dst+1] = row[i+1] // G
			packed[dst+2] = row[i+0] // R
			packed[dst+3] = row[i+3] // A
			dst += 4
		}
	}

	return packed
}

// encodeRLEPackets writes TGA RLE packets from packed pixels.
func encodeRLEPackets(w io.Writer, packed []byte, bytesPerPixel int) error {
	totalPixels := len(packed) / bytesPerPixel
	i := 0

	for i < totalPixels {
		runLen := findRunLength(packed, bytesPerPixel, i, totalPixels)

		if runLen > 1 {
			packetHeader, err := makePacketHeader(runLen, true)
			if err != nil {
				return err
			}

			header := []byte{packetHeader}
			if _, err := w.Write(header); err != nil {
				return err
			}

			start := i * bytesPerPixel
			if _, err := w.Write(packed[start : start+bytesPerPixel]); err != nil {
				return err
			}

			i += runLen
			continue
		}

		rawStart := i
		rawLen := 1
		i++

		for rawLen < 128 && i < totalPixels {
			runLen = findRunLength(packed, bytesPerPixel, i, totalPixels)
			if runLen > 1 {
				break
			}

			rawLen++
			i++
		}

		packetHeader, err := makePacketHeader(rawLen, false)
		if err != nil {
			return err
		}

		header := []byte{packetHeader}
		if _, err := w.Write(header); err != nil {
			return err
		}

		start := rawStart * bytesPerPixel
		end := start + rawLen*bytesPerPixel
		if _, err := w.Write(packed[start:end]); err != nil {
			return err
		}
	}

	return nil
}

// findRunLength returns equal-pixel run length limited to TGA max packet size.
func findRunLength(packed []byte, bytesPerPixel int, start, totalPixels int) int {
	runLen := 1
	for runLen < 128 && start+runLen < totalPixels {
		if !pixelsEqualAt(packed, bytesPerPixel, start, start+runLen) {
			break
		}
		runLen++
	}

	return runLen
}

// pixelsEqualAt compares two pixels in a packed pixel buffer.
func pixelsEqualAt(packed []byte, bytesPerPixel int, i, j int) bool {
	ii := i * bytesPerPixel
	jj := j * bytesPerPixel

	for k := range bytesPerPixel {
		if packed[ii+k] != packed[jj+k] {
			return false
		}
	}

	return true
}

// makePacketHeader builds one TGA packet header byte for given pixel count.
func makePacketHeader(count int, rle bool) (byte, error) {
	if count < 1 || count > 128 {
		return 0, ErrFormat
	}

	// count range check above guarantees safe conversion.
	// #nosec G115 -- validated by range checks.
	header := byte(count - 1)
	if rle {
		header |= 0x80
	}

	return header, nil
}
