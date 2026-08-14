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

	"github.com/woozymasta/tga/internal/simd"
)

// EncodeOptions controls optional TGA encoding features.
type EncodeOptions struct {
	// Metadata enables writing TGA 2.0 footer/extension/developer areas.
	// Metadata.AttributesType can be used for advanced alpha semantics in TGA 2.0.
	Metadata *TGA2Metadata
	// ImageID writes the optional image ID field after the 18-byte header.
	ImageID []byte
	// ColorMapDepth sets palette entry depth for paletted output.
	// Supported values: 24, 32. Zero means auto (24 unless palette has alpha).
	ColorMapDepth int
	// PixelDepth sets true-color depth for non-grayscale/non-paletted output.
	// Supported values: 16, 24, 32. Zero means default 32.
	PixelDepth int
	// OriginBottom sets image origin to bottom-left when true.
	// Default is top-left.
	OriginBottom bool
	// RLE enables TGA RLE packet compression (types 10/11).
	RLE bool
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
// TGA 2.0 footer/extension areas are written when metadata options are enabled.
func EncodeWithOptions(w io.Writer, m image.Image, opts *EncodeOptions) error {
	settings := effectiveEncodeOptions(opts)
	if err := validateMetadata(settings.Metadata); err != nil {
		return err
	}
	meta := settings.Metadata

	out := w
	var cw *countingWriter
	if meta != nil {
		cw = &countingWriter{w: w}
		out = cw
	}

	b := m.Bounds()
	mw := b.Dx()
	mh := b.Dy()

	if mw <= 0 || mh <= 0 {
		return ErrFormat
	}

	if mw > 0xffff || mh > 0xffff {
		return ErrFormat
	}

	idLen, err := byteFromInt(len(settings.ImageID))
	if err != nil {
		return err
	}

	// Bounds check above guarantees safe conversion to uint16.
	// #nosec G115 -- validated by range checks.
	mw16 := uint16(mw)
	// #nosec G115 -- validated by range checks.
	mh16 := uint16(mh)

	// 18-byte header: id/cmap fields then image spec.
	header := [18]byte{
		idLen, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0,
		0, 0,
		0, 0,
	}
	binary.LittleEndian.PutUint16(header[12:14], mw16)
	binary.LittleEndian.PutUint16(header[14:16], mh16)

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

		alphaBits, err := resolveDescriptorAlphaBits(8)
		if err != nil {
			return err
		}
		header[17] = buildImageDescriptor(settings.OriginBottom, alphaBits)

		if _, err := out.Write(header[:]); err != nil {
			return err
		}
		if err := writeImageID(out, settings.ImageID); err != nil {
			return err
		}

		if settings.RLE {
			if err := encodeGrayRLE(out, src, b, settings.OriginBottom); err != nil {
				return err
			}
		} else {
			if err := encodeGray(out, src, b, settings.OriginBottom); err != nil {
				return err
			}
		}

		return writeTGA2TailIfNeeded(cw, meta)

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

		alphaBits, err := resolveDescriptorAlphaBits(8)
		if err != nil {
			return err
		}
		header[17] = buildImageDescriptor(settings.OriginBottom, alphaBits)

		if _, err := out.Write(header[:]); err != nil {
			return err
		}
		if err := writeImageID(out, settings.ImageID); err != nil {
			return err
		}

		if err := writePalette(out, src.Palette, cMapDepth); err != nil {
			return err
		}

		if settings.RLE {
			if err := encodePalettedRLE(out, src, b, settings.OriginBottom); err != nil {
				return err
			}
		} else {
			if err := encodePaletted(out, src, b, settings.OriginBottom); err != nil {
				return err
			}
		}

		return writeTGA2TailIfNeeded(cw, meta)

	case *image.NRGBA:
		if settings.RLE {
			header[2] = typeRLETrueColor
		} else {
			header[2] = typeTrueColor
		}

		switch trueColorDepth {
		case 16:
			header[16] = 16
		case 24:
			header[16] = 24
		default:
			header[16] = 32
		}

		alphaBits, err := resolveDescriptorAlphaBits(trueColorDepth)
		if err != nil {
			return err
		}
		header[17] = buildImageDescriptor(settings.OriginBottom, alphaBits)

		if _, err := out.Write(header[:]); err != nil {
			return err
		}
		if err := writeImageID(out, settings.ImageID); err != nil {
			return err
		}

		if settings.RLE {
			if err := encodeNRGBARLE(out, src, b, trueColorDepth, settings.OriginBottom); err != nil {
				return err
			}
		} else {
			if err := encodeNRGBA(out, src, b, trueColorDepth, settings.OriginBottom); err != nil {
				return err
			}
		}

		return writeTGA2TailIfNeeded(cw, meta)

	default:
		// Convert arbitrary image.Image once to a canonical straight-alpha
		// representation before choosing the true-color packing path.
		if settings.RLE {
			header[2] = typeRLETrueColor
		} else {
			header[2] = typeTrueColor
		}

		switch trueColorDepth {
		case 16:
			header[16] = 16
		case 24:
			header[16] = 24
		default:
			header[16] = 32
		}

		alphaBits, err := resolveDescriptorAlphaBits(trueColorDepth)
		if err != nil {
			return err
		}
		header[17] = buildImageDescriptor(settings.OriginBottom, alphaBits)

		if _, err := out.Write(header[:]); err != nil {
			return err
		}
		if err := writeImageID(out, settings.ImageID); err != nil {
			return err
		}

		dst := image.NewNRGBA(b)
		draw.Draw(dst, b, m, b.Min, draw.Src)

		if settings.RLE {
			if err := encodeNRGBARLE(out, dst, b, trueColorDepth, settings.OriginBottom); err != nil {
				return err
			}
		} else {
			if err := encodeNRGBA(out, dst, b, trueColorDepth, settings.OriginBottom); err != nil {
				return err
			}
		}

		return writeTGA2TailIfNeeded(cw, meta)
	}
}

// effectiveEncodeOptions resolves nil options to defaults.
func effectiveEncodeOptions(opts *EncodeOptions) EncodeOptions {
	if opts == nil {
		return EncodeOptions{}
	}

	return *opts
}

// writeTGA2TailIfNeeded writes TGA 2.0 tail only when metadata is enabled.
func writeTGA2TailIfNeeded(cw *countingWriter, meta *TGA2Metadata) error {
	if cw == nil || meta == nil {
		return nil
	}

	return writeTGA2Tail(cw, meta)
}

// validateMetadata validates supported metadata field ranges.
func validateMetadata(meta *TGA2Metadata) error {
	if meta == nil {
		return nil
	}
	if meta.AttributesType > 4 {
		return ErrFormat
	}

	return nil
}

// writeImageID writes optional TGA Image ID field.
func writeImageID(w io.Writer, imageID []byte) error {
	if len(imageID) == 0 {
		return nil
	}

	_, err := w.Write(imageID)
	return err
}

// resolveDescriptorAlphaBits returns descriptor low-nibble alpha bits.
func resolveDescriptorAlphaBits(depth int) (uint8, error) {
	alphaBits, ok := descriptorAlphaBitsByDepth(depth)
	if !ok {
		return 0, ErrUnsupported
	}

	return alphaBits, nil
}

// descriptorAlphaBitsByDepth returns descriptor alpha bits for known depths.
func descriptorAlphaBitsByDepth(depth int) (uint8, bool) {
	switch depth {
	case 8, 24:
		return 0, true
	case 16:
		return 1, true
	case 32:
		return 8, true
	default:
		return 0, false
	}
}

// buildImageDescriptor builds descriptor from origin and alpha bits.
func buildImageDescriptor(originBottom bool, alphaBits uint8) uint8 {
	desc := alphaBits & 0x0f
	if !originBottom {
		desc |= maskOriginTop
	}

	return desc
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
		// TGA palette depth is selected from actual palette alpha,
		// preserving alpha only when at least one entry needs it.
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

// encodeGray writes 8-bit grayscale pixel data in configured row order.
func encodeGray(w io.Writer, m *image.Gray, b image.Rectangle, originBottom bool) error {
	for rowIndex := 0; rowIndex < b.Dy(); rowIndex++ {
		y := b.Min.Y + rowIndex
		if originBottom {
			y = b.Max.Y - 1 - rowIndex
		}

		rowData := m.Pix[(y-m.Rect.Min.Y)*m.Stride : (y-m.Rect.Min.Y)*m.Stride+b.Dx()]
		if _, err := w.Write(rowData); err != nil {
			return err
		}
	}

	return nil
}

// encodeGrayRLE writes 8-bit grayscale pixel data using TGA RLE packets.
func encodeGrayRLE(w io.Writer, m *image.Gray, b image.Rectangle, originBottom bool) error {
	header := []byte{0}
	value := []byte{0}

	width := b.Dx()
	for rowIndex := 0; rowIndex < b.Dy(); rowIndex++ {
		y := b.Min.Y + rowIndex
		if originBottom {
			y = b.Max.Y - 1 - rowIndex
		}

		srcOffset := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X - m.Rect.Min.X)
		rowData := m.Pix[srcOffset : srcOffset+width]
		if err := encodeRLEPackets1WithScratch(w, rowData, header, value); err != nil {
			return err
		}
	}

	return nil
}

// trueColorBytesPerPixel returns packed bytes per pixel for a true-color depth.
func trueColorBytesPerPixel(depth int) (int, error) {
	switch depth {
	case 16:
		return 2, nil
	case 24:
		return 3, nil
	case 32:
		return 4, nil
	default:
		return 0, ErrUnsupported
	}
}

// packNRGBARow converts one NRGBA row (RGBA bytes, len width*4)
// into packed TGA true-color bytes: BGRA (32), BGR (24) or little-endian RGB555 (16).
// len(dst) must equal width*bytesPerPixel for the given depth.
// This is the single point where pixel layout conversion happens for true-color encoding.
func packNRGBARow(dst, src []byte, depth int) {
	switch depth {
	case 32:
		simd.SwapRB32(dst, src) // RGBA -> BGRA

	case 24:
		simd.RGBAToBGR(dst, src) // RGBA -> BGR

	case 16:
		for si, di := 0, 0; si < len(src); si, di = si+4, di+2 {
			v := encodeRGB555(src[si+0], src[si+1], src[si+2], src[si+3])
			binary.LittleEndian.PutUint16(dst[di:di+2], v)
		}
	}
}

// encodeNRGBA writes true-color pixel data in configured row order.
// The depth switch is hoisted out of the row loop and the per-row conversion is
// kept inline (no per-row call) so the compiler can tightly optimize the hot path;
// the RLE path shares packNRGBARow instead.
func encodeNRGBA(w io.Writer, m *image.NRGBA, b image.Rectangle, depth int, originBottom bool) error {
	width := b.Dx()
	switch depth {
	case 32:
		row := make([]byte, width*4)

		for rowIndex := 0; rowIndex < b.Dy(); rowIndex++ {
			y := b.Min.Y + rowIndex
			if originBottom {
				y = b.Max.Y - 1 - rowIndex
			}

			i0 := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X-m.Rect.Min.X)*4
			simd.SwapRB32(row, m.Pix[i0:i0+width*4]) // RGBA -> BGRA

			if _, err := w.Write(row); err != nil {
				return err
			}
		}

	case 24:
		row := make([]byte, width*3)

		for rowIndex := 0; rowIndex < b.Dy(); rowIndex++ {
			y := b.Min.Y + rowIndex
			if originBottom {
				y = b.Max.Y - 1 - rowIndex
			}

			i0 := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X-m.Rect.Min.X)*4
			simd.RGBAToBGR(row, m.Pix[i0:i0+width*4]) // RGBA -> BGR

			if _, err := w.Write(row); err != nil {
				return err
			}
		}

	case 16:
		row := make([]byte, width*2)

		for rowIndex := 0; rowIndex < b.Dy(); rowIndex++ {
			y := b.Min.Y + rowIndex
			if originBottom {
				y = b.Max.Y - 1 - rowIndex
			}

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
// Rows are packed and RLE-encoded one at a time, so packets never cross scan
// lines (TGA 2.0 friendly) and no whole-image scratch buffer is allocated.
func encodeNRGBARLE(w io.Writer, m *image.NRGBA, b image.Rectangle, depth int, originBottom bool) error {
	bytesPerPixel, err := trueColorBytesPerPixel(depth)
	if err != nil {
		return err
	}

	width := b.Dx()
	rowPacked := make([]byte, width*bytesPerPixel)
	header := []byte{0} // reused across rows to avoid a per-row allocation

	for rowIndex := 0; rowIndex < b.Dy(); rowIndex++ {
		y := b.Min.Y + rowIndex
		if originBottom {
			y = b.Max.Y - 1 - rowIndex
		}

		i0 := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X-m.Rect.Min.X)*4
		packNRGBARow(rowPacked, m.Pix[i0:i0+width*4], depth)

		if err := encodeRLEPackets(w, rowPacked, bytesPerPixel, header); err != nil {
			return err
		}
	}

	return nil
}

// encodePaletted writes uncompressed 8-bit paletted pixel data.
func encodePaletted(w io.Writer, m *image.Paletted, b image.Rectangle, originBottom bool) error {
	width := b.Dx()
	for rowIndex := 0; rowIndex < b.Dy(); rowIndex++ {
		y := b.Min.Y + rowIndex
		if originBottom {
			y = b.Max.Y - 1 - rowIndex
		}

		i0 := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X - m.Rect.Min.X)
		rowData := m.Pix[i0 : i0+width]
		if _, err := w.Write(rowData); err != nil {
			return err
		}
	}

	return nil
}

// encodePalettedRLE writes 8-bit indexed pixel data using TGA RLE packets.
func encodePalettedRLE(w io.Writer, m *image.Paletted, b image.Rectangle, originBottom bool) error {
	header := []byte{0}
	value := []byte{0}

	width := b.Dx()
	for rowIndex := 0; rowIndex < b.Dy(); rowIndex++ {
		y := b.Min.Y + rowIndex
		if originBottom {
			y = b.Max.Y - 1 - rowIndex
		}

		i0 := (y-m.Rect.Min.Y)*m.Stride + (b.Min.X - m.Rect.Min.X)
		rowData := m.Pix[i0 : i0+width]
		if err := encodeRLEPackets1WithScratch(w, rowData, header, value); err != nil {
			return err
		}
	}

	return nil
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
			var entry [3]byte
			entry[0] = b
			entry[1] = g
			entry[2] = r
			if _, err := w.Write(entry[:]); err != nil {
				return err
			}

		case 32:
			var entry [4]byte
			entry[0] = b
			entry[1] = g
			entry[2] = r
			entry[3] = a
			if _, err := w.Write(entry[:]); err != nil {
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

// encodeRLEPackets writes TGA RLE packets from packed pixels
// using a caller-provided 1-byte header scratch,
// so per-row callers avoid allocating once per row.
func encodeRLEPackets(w io.Writer, packed []byte, bytesPerPixel int, header []byte) error {
	if bytesPerPixel == 1 {
		return encodeRLEPackets1WithScratch(w, packed, header, []byte{0})
	}

	totalPixels := len(packed) / bytesPerPixel
	i := 0

	for i < totalPixels {
		runLen := findRunLength(packed, bytesPerPixel, i, totalPixels)

		if runLen > 1 {
			packetHeader, err := makePacketHeader(runLen, true)
			if err != nil {
				return err
			}

			header[0] = packetHeader
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
				// Leave a repeated run for the next iteration
				// so it gets its own RLE packet instead of being hidden inside a raw packet.
				break
			}

			rawLen++
			i++
		}

		packetHeader, err := makePacketHeader(rawLen, false)
		if err != nil {
			return err
		}

		header[0] = packetHeader
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

// encodeRLEPackets1WithScratch writes one-byte-pixel RLE using caller scratch.
func encodeRLEPackets1WithScratch(w io.Writer, packed []byte, header, value []byte) error {
	totalPixels := len(packed)
	i := 0

	for i < totalPixels {
		runLen := 1
		pv := packed[i]
		for runLen < 128 && i+runLen < totalPixels && packed[i+runLen] == pv {
			runLen++
		}

		if runLen > 1 {
			packetHeader, err := makePacketHeader(runLen, true)
			if err != nil {
				return err
			}

			header[0] = packetHeader
			if _, err := w.Write(header); err != nil {
				return err
			}

			value[0] = pv
			if _, err := w.Write(value); err != nil {
				return err
			}

			i += runLen
			continue
		}

		rawStart := i
		rawLen := 1
		i++

		for rawLen < 128 && i < totalPixels {
			runLen = 1
			pv = packed[i]
			for runLen < 128 && i+runLen < totalPixels && packed[i+runLen] == pv {
				runLen++
			}
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

		header[0] = packetHeader
		if _, err := w.Write(header); err != nil {
			return err
		}

		if _, err := w.Write(packed[rawStart : rawStart+rawLen]); err != nil {
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

	switch bytesPerPixel {
	case 1:
		return packed[ii] == packed[jj]

	case 2:
		return packed[ii] == packed[jj] &&
			packed[ii+1] == packed[jj+1]

	case 3:
		return packed[ii] == packed[jj] &&
			packed[ii+1] == packed[jj+1] &&
			packed[ii+2] == packed[jj+2]

	case 4:
		return packed[ii] == packed[jj] &&
			packed[ii+1] == packed[jj+1] &&
			packed[ii+2] == packed[jj+2] &&
			packed[ii+3] == packed[jj+3]

	default:
		for k := range bytesPerPixel {
			if packed[ii+k] != packed[jj+k] {
				return false
			}
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
