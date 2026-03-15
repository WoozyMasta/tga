// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"io"
)

// EncodeOptions controls optional TGA encoding features.
type EncodeOptions struct {
	// RLE enables TGA RLE packet compression (types 10/11).
	RLE bool
	// PixelDepth sets true-color depth for non-grayscale/non-paletted output.
	// Supported values: 16, 24, 32. Zero means default 32.
	PixelDepth int
	// ColorMapDepth sets palette entry depth for paletted output.
	// Supported values: 24, 32. Zero means auto (24 unless palette has alpha).
	ColorMapDepth int
}

// Encode writes the image m in TGA format to w.
// Supports *image.Gray (8-bit grayscale), *image.Paletted (8-bit indexed),
// and true-color images in 16/24/32-bit depth (default 32-bit).
// Other image types are converted to NRGBA. Origin is top-left (descriptor bit 5 set).
// No TGA 2.0 footer or extension area is written.
func Encode(w io.Writer, m image.Image) error {
	return EncodeWithOptions(w, m, nil)
}

// EncodeWithOptions writes the image m in TGA format to w using opts.
// Supports *image.Gray (8-bit grayscale), *image.Paletted (8-bit indexed),
// and true-color images in 16/24/32-bit depth (default 32-bit).
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
	trueColorDepth, err := resolveTrueColorDepth(settings.PixelDepth)
	if err != nil {
		return err
	}

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

	case *image.Paletted:
		cMapDepth, err := resolveColorMapDepth(settings.ColorMapDepth, src.Palette)
		if err != nil {
			return err
		}

		if settings.RLE {
			header[2] = typeRLEPaletted
		} else {
			header[2] = typePaletted
		}
		header[1] = 1
		if len(src.Palette) == 0 || len(src.Palette) > 256 {
			return ErrFormat
		}

		binary.LittleEndian.PutUint16(header[3:5], 0)
		// len(src.Palette) is checked above.
		// #nosec G115 -- validated by range checks.
		palLen := uint16(len(src.Palette))
		binary.LittleEndian.PutUint16(header[5:7], palLen)
		if cMapDepth == 24 {
			header[7] = 24
		} else {
			header[7] = 32
		}
		header[16] = 8
		if _, err := w.Write(header[:]); err != nil {
			return err
		}

		if err := writePalette(w, src.Palette, cMapDepth); err != nil {
			return err
		}

		if settings.RLE {
			return encodePalettedRLE(w, src, b)
		}

		return encodePaletted(w, src, b)

	case *image.NRGBA:
		if settings.RLE {
			header[2] = typeRLETrueColor
		} else {
			header[2] = typeTrueColor
		}
		// True-color descriptor alpha bits.
		// 16-bit uses one attribute bit, 32-bit uses eight, 24-bit uses none.
		switch trueColorDepth {
		case 16:
			header[17] = 0x21
		case 24:
			header[17] = 0x20
		default:
			header[17] = 0x28
		}
		switch trueColorDepth {
		case 16:
			header[16] = 16
		case 24:
			header[16] = 24
		default:
			header[16] = 32
		}
		if _, err := w.Write(header[:]); err != nil {
			return err
		}

		if settings.RLE {
			return encodeNRGBARLE(w, src, b, trueColorDepth)
		}

		return encodeNRGBA(w, src, b, trueColorDepth)

	default:
		if settings.RLE {
			header[2] = typeRLETrueColor
		} else {
			header[2] = typeTrueColor
		}
		switch trueColorDepth {
		case 16:
			header[17] = 0x21
		case 24:
			header[17] = 0x20
		default:
			header[17] = 0x28
		}
		switch trueColorDepth {
		case 16:
			header[16] = 16
		case 24:
			header[16] = 24
		default:
			header[16] = 32
		}
		if _, err := w.Write(header[:]); err != nil {
			return err
		}

		dst := image.NewNRGBA(b)
		draw.Draw(dst, b, m, b.Min, draw.Src)

		if settings.RLE {
			return encodeNRGBARLE(w, dst, b, trueColorDepth)
		}

		return encodeNRGBA(w, dst, b, trueColorDepth)
	}
}

// effectiveEncodeOptions resolves nil options to defaults.
func effectiveEncodeOptions(opts *EncodeOptions) EncodeOptions {
	if opts == nil {
		return EncodeOptions{}
	}

	return *opts
}

// resolveTrueColorDepth validates and resolves the output true-color depth.
func resolveTrueColorDepth(pixelDepth int) (int, error) {
	if pixelDepth == 0 {
		return 32, nil
	}

	switch pixelDepth {
	case 16, 24, 32:
		return pixelDepth, nil
	default:
		return 0, ErrUnsupported
	}
}

// resolveColorMapDepth validates and resolves palette entry depth.
func resolveColorMapDepth(cMapDepth int, pal color.Palette) (int, error) {
	if cMapDepth == 0 {
		for _, c := range pal {
			_, _, _, a := c.RGBA()
			if a != 0xffff {
				return 32, nil
			}
		}

		return 24, nil
	}

	switch cMapDepth {
	case 24, 32:
		return cMapDepth, nil
	default:
		return 0, ErrUnsupported
	}
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

// encodeNRGBA writes true-color pixel data in row order (top to bottom).
func encodeNRGBA(w io.Writer, m *image.NRGBA, b image.Rectangle, depth int) error {
	width := b.Dx()
	switch depth {
	case 32:
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

	case 24:
		row := make([]byte, width*3)

		for y := b.Min.Y; y < b.Max.Y; y++ {
			i0 := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X-m.Rect.Min.X)*4
			src := m.Pix[i0 : i0+width*4]
			di := 0
			for si := 0; si < len(src); si += 4 {
				row[di+0] = src[si+2]
				row[di+1] = src[si+1]
				row[di+2] = src[si+0]
				di += 3
			}

			if _, err := w.Write(row); err != nil {
				return err
			}
		}

	case 16:
		row := make([]byte, width*2)

		for y := b.Min.Y; y < b.Max.Y; y++ {
			i0 := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X-m.Rect.Min.X)*4
			src := m.Pix[i0 : i0+width*4]
			di := 0
			for si := 0; si < len(src); si += 4 {
				v := encodeRGB555(
					src[si+0],
					src[si+1],
					src[si+2],
					src[si+3],
				)
				binary.LittleEndian.PutUint16(row[di:di+2], v)
				di += 2
			}

			if _, err := w.Write(row); err != nil {
				return err
			}
		}

	default:
		return ErrUnsupported
	}

	return nil
}

// encodeNRGBARLE writes true-color pixel data using TGA RLE packets.
func encodeNRGBARLE(w io.Writer, m *image.NRGBA, b image.Rectangle, depth int) error {
	packed, bytesPerPixel, err := packNRGBAPixels(m, b, depth)
	if err != nil {
		return err
	}

	return encodeRLEPackets(w, packed, bytesPerPixel)
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

// packNRGBAPixels converts NRGBA rows to contiguous true-color bytes.
func packNRGBAPixels(m *image.NRGBA, b image.Rectangle, depth int) ([]byte, int, error) {
	width := b.Dx()
	height := b.Dy()
	var bytesPerPixel int
	switch depth {
	case 16:
		bytesPerPixel = 2
	case 24:
		bytesPerPixel = 3
	case 32:
		bytesPerPixel = 4
	default:
		return nil, 0, ErrUnsupported
	}

	packed := make([]byte, width*height*bytesPerPixel)
	dst := 0

	for y := b.Min.Y; y < b.Max.Y; y++ {
		srcOffset := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X-m.Rect.Min.X)*4
		row := m.Pix[srcOffset : srcOffset+width*4]

		for i := 0; i < len(row); i += 4 {
			switch depth {
			case 16:
				v := encodeRGB555(
					row[i+0],
					row[i+1],
					row[i+2],
					row[i+3],
				)
				binary.LittleEndian.PutUint16(packed[dst:dst+2], v)
				dst += 2

			case 24:
				packed[dst+0] = row[i+2] // B
				packed[dst+1] = row[i+1] // G
				packed[dst+2] = row[i+0] // R
				dst += 3

			case 32:
				packed[dst+0] = row[i+2] // B
				packed[dst+1] = row[i+1] // G
				packed[dst+2] = row[i+0] // R
				packed[dst+3] = row[i+3] // A
				dst += 4
			}
		}
	}

	return packed, bytesPerPixel, nil
}

// encodePaletted writes uncompressed 8-bit paletted pixel data.
func encodePaletted(w io.Writer, m *image.Paletted, b image.Rectangle) error {
	width := b.Dx()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i0 := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X - m.Rect.Min.X)
		row := m.Pix[i0 : i0+width]
		if _, err := w.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// encodePalettedRLE writes 8-bit indexed pixel data using TGA RLE packets.
func encodePalettedRLE(w io.Writer, m *image.Paletted, b image.Rectangle) error {
	packed := packPalettedPixels(m, b)
	return encodeRLEPackets(w, packed, 1)
}

// packPalettedPixels copies index data into contiguous top-to-bottom buffer.
func packPalettedPixels(m *image.Paletted, b image.Rectangle) []byte {
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

// writePalette writes a color map in BGR/BGRA order.
func writePalette(w io.Writer, pal color.Palette, depth int) error {
	for _, c := range pal {
		converted := color.NRGBAModel.Convert(c).(color.NRGBA)
		r := converted.R
		g := converted.G
		b := converted.B
		a := converted.A

		switch depth {
		case 24:
			if _, err := w.Write([]byte{b, g, r}); err != nil {
				return err
			}

		case 32:
			if _, err := w.Write([]byte{b, g, r, a}); err != nil {
				return err
			}

		default:
			return ErrUnsupported
		}
	}

	return nil
}

// encodeRGB555 encodes 8-bit RGBA to A1R5G5B5 little-endian value.
func encodeRGB555(r, g, b, a uint8) uint16 {
	r5 := uint16(r >> 3)
	g5 := uint16(g >> 3)
	b5 := uint16(b >> 3)
	alphaBit := uint16(0)
	if a >= 128 {
		alphaBit = 1 << 15
	}

	return alphaBit | (r5 << 10) | (g5 << 5) | b5
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
