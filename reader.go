// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"bufio"
	"image"
	"image/color"
	"io"

	"github.com/woozymasta/tga/internal/simd"
)

const (
	headerSize = 18 // Header size.

	typePaletted     = 1  // Color-mapped image.
	typeTrueColor    = 2  // True-color image.
	typeGrayscale    = 3  // Grayscale image.
	typeRLEPaletted  = 9  // RLE color-mapped image.
	typeRLETrueColor = 10 // RLE true-color image.
	typeRLEGrayscale = 11 // RLE grayscale image.

	maskOriginTop   = 0x20 // Bit 5 of image descriptor: 0 = bottom-left, 1 = top-left origin.
	maskOriginRight = 0x10 // Bit 4 of image descriptor: 0 = left, 1 = right origin.
	maskInterleave  = 0xc0 // Bits 6-7 of image descriptor: interleave mode.
)

// RegisterFormat registers the TGA format with image.Decode and image.DecodeConfig.
// Because TGA has no magic bytes, it does not play nicely with other formats
// when image.Decode tries them in order; registration is disabled by default.
// Call this explicitly if you need image.Decode to recognize TGA
// (e.g. in a TGA-only context or after other formats).
func RegisterFormat() {
	image.RegisterFormat("tga", "", Decode, DecodeConfig)
}

// DecodeConfig returns the image configuration without decoding pixel data.
func DecodeConfig(r io.Reader) (image.Config, error) {
	header, err := readHeader(r)
	if err != nil {
		return image.Config{}, err
	}

	var cm color.Model
	switch header.imageType {
	case typeTrueColor, typeRLETrueColor:
		cm = color.NRGBAModel
	case typeGrayscale, typeRLEGrayscale:
		cm = color.GrayModel
	case typePaletted, typeRLEPaletted:
		cm = color.Palette{}
	}

	return image.Config{
		ColorModel: cm,
		Width:      header.width,
		Height:     header.height,
	}, nil
}

// Decode reads a TGA image from r.
func Decode(r io.Reader) (image.Image, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	header, err := readHeader(br)
	if err != nil {
		return nil, err
	}

	if header.idLen > 0 {
		if _, err := br.Discard(header.idLen); err != nil {
			return nil, err
		}
	}

	var palette color.Palette
	if header.hasColorMap {
		entryBytes := (int(header.colorMapDepth) + 7) / 8
		rawPalette := make([]byte, header.colorMapLen*entryBytes)
		if _, err := io.ReadFull(br, rawPalette); err != nil {
			return nil, err
		}

		palette = make(color.Palette, header.colorMapLen)

		// Create the color palette
		for i := range header.colorMapLen {
			offset := i * entryBytes
			switch header.colorMapDepth {
			case 24:
				palette[i] = color.NRGBA{
					R: rawPalette[offset+2],
					G: rawPalette[offset+1],
					B: rawPalette[offset+0],
					A: 0xff,
				}

			case 32:
				palette[i] = color.NRGBA{
					R: rawPalette[offset+2],
					G: rawPalette[offset+1],
					B: rawPalette[offset+0],
					A: rawPalette[offset+3],
				}

			case 15, 16:
				v := uint16(rawPalette[offset]) | uint16(rawPalette[offset+1])<<8
				palette[i] = decodeRGB555(v)
			}
		}
	}

	// Go uses a top-left origin.
	flipY := (header.descriptor & maskOriginTop) == 0
	flipX := header.descriptor&maskOriginRight != 0

	switch header.imageType {
	case typeTrueColor, typeGrayscale:
		return decodeUncompressed(
			br,
			header.width,
			header.height,
			header.pixelDepth,
			header.hasAlpha,
			flipX,
			flipY,
		)

	case typeRLETrueColor, typeRLEGrayscale:
		return decodeRLE(
			br,
			header.width,
			header.height,
			header.pixelDepth,
			header.hasAlpha,
			flipX,
			flipY,
		)

	case typePaletted:
		return decodeUncompressedPaletted(
			br,
			header.width,
			header.height,
			header.pixelDepth,
			flipX,
			flipY,
			palette,
			header.colorMapStart,
			header.colorMapLen,
		)

	case typeRLEPaletted:
		return decodeRLEPaletted(
			br,
			header.width,
			header.height,
			header.pixelDepth,
			flipX,
			flipY,
			palette,
			header.colorMapStart,
			header.colorMapLen,
		)

	default:
		return nil, ErrUnsupported
	}
}

// parsedHeader is the validated fixed-size TGA image specification.
type parsedHeader struct {
	idLen         int   // idLen is the byte length of the image ID field following the header.
	colorMapStart int   // colorMapStart is the first palette index declared by the color map.
	colorMapLen   int   // colorMapLen is the number of entries declared by the color map.
	width         int   // width is the image width in pixels.
	height        int   // height is the image height in pixels.
	pixelDepth    int   // pixelDepth is the bit depth of each encoded image pixel.
	imageType     uint8 // imageType identifies the pixel encoding and compression mode.
	colorMapDepth uint8 // colorMapDepth is the bit depth of each color-map entry.
	descriptor    uint8 // descriptor contains alpha attributes and image origin bits.
	hasColorMap   bool  // hasColorMap reports whether the header declares a color map.
	hasAlpha      bool  // hasAlpha reports whether 16-bit pixels use an A1 alpha bit.
}

// readHeader reads, parses, and validates the fixed-size TGA header.
func readHeader(r io.Reader) (parsedHeader, error) {
	var raw [headerSize]byte
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return parsedHeader{}, ErrHeaderTooShort
		}
		return parsedHeader{}, err
	}

	return parseHeader(raw)
}

// parseHeader converts a raw TGA header into a validated image specification.
func parseHeader(raw [headerSize]byte) (parsedHeader, error) {
	header := parsedHeader{
		idLen:         int(raw[0]),
		imageType:     raw[2],
		colorMapStart: int(raw[3]) | int(raw[4])<<8,
		colorMapLen:   int(raw[5]) | int(raw[6])<<8,
		colorMapDepth: raw[7],
		width:         int(raw[12]) | int(raw[13])<<8,
		height:        int(raw[14]) | int(raw[15])<<8,
		pixelDepth:    int(raw[16]),
		descriptor:    raw[17],
	}

	switch raw[1] {
	case 0:
	case 1:
		header.hasColorMap = true
	default:
		return parsedHeader{}, ErrFormat
	}

	if header.width == 0 || header.height == 0 {
		return parsedHeader{}, ErrFormat
	}
	if err := validateImageSpec(header.imageType, header.pixelDepth, header.hasColorMap, header.colorMapLen); err != nil {
		return parsedHeader{}, err
	}
	if header.hasColorMap && !isColorMapDepth(header.colorMapDepth) {
		return parsedHeader{}, ErrUnsupported
	}
	if header.descriptor&maskInterleave != 0 {
		return parsedHeader{}, ErrUnsupported
	}

	header.hasAlpha = header.pixelDepth == 16 && header.descriptor&0x0f == 1
	return header, nil
}

// isColorMapDepth reports whether depth is supported for color-map entries.
func isColorMapDepth(depth uint8) bool {
	switch depth {
	case 15, 16, 24, 32:
		return true
	default:
		return false
	}
}

// validateImageSpec checks that image type and bit depth combination is supported.
func validateImageSpec(imgType uint8, pixelDepth int, hasCMap bool, cMapLen int) error {
	switch imgType {
	case typeTrueColor, typeRLETrueColor:
		if !isTrueColorDepth(pixelDepth) {
			return ErrUnsupported
		}
		if hasCMap {
			return ErrFormat
		}

	case typeGrayscale, typeRLEGrayscale:
		if pixelDepth != 8 {
			return ErrUnsupported
		}
		if hasCMap {
			return ErrFormat
		}

	case typePaletted, typeRLEPaletted:
		if pixelDepth != 8 {
			return ErrUnsupported
		}
		if !hasCMap || cMapLen == 0 {
			return ErrFormat
		}

	default:
		return ErrUnsupported
	}

	return nil
}

// isTrueColorDepth reports whether depth is one of supported true-color bit depths.
func isTrueColorDepth(pixelDepth int) bool {
	switch pixelDepth {
	case 15, 16, 24, 32:
		return true
	default:
		return false
	}
}

// decodeUncompressed reads uncompressed true-color or grayscale image data.
func decodeUncompressed(r io.Reader, w, h, depth int, hasAlpha, flipX, flipY bool) (image.Image, error) {
	bytesPerPixel := (depth + 7) / 8
	rowSize := w * bytesPerPixel

	if depth == 8 {
		gray := image.NewGray(image.Rect(0, 0, w, h))
		rowBuf := make([]byte, rowSize)

		for y := range h {
			destY := y
			if flipY {
				destY = h - 1 - y
			}

			destOffset := destY * gray.Stride

			if _, err := io.ReadFull(r, rowBuf); err != nil {
				return nil, err
			}

			copy(gray.Pix[destOffset:destOffset+w], rowBuf)
		}

		if flipX {
			flipImageHorizontally(gray, w, h)
		}

		return gray, nil
	}

	nrgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	rowBuf := make([]byte, rowSize)

	for y := range h {
		destY := y
		if flipY {
			destY = h - 1 - y
		}

		if _, err := io.ReadFull(r, rowBuf); err != nil {
			return nil, err
		}

		destOffset := destY * nrgba.Stride
		convertRowToNRGBA(nrgba.Pix[destOffset:destOffset+w*4], rowBuf, w, depth, hasAlpha)
	}

	if flipX {
		flipImageHorizontally(nrgba, w, h)
	}

	return nrgba, nil
}

// decodeUncompressedPaletted reads uncompressed color-mapped image data.
func decodeUncompressedPaletted(r io.Reader, w, h, depth int, flipX, flipY bool, pal color.Palette, colorMapStart, colorMapLen int) (image.Image, error) {
	if depth != 8 {
		return nil, ErrUnsupported
	}

	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	stride := img.Stride
	rowBuf := make([]byte, w)

	for y := range h {
		destY := y
		if flipY {
			destY = h - 1 - y
		}
		destOffset := destY * stride

		if _, err := io.ReadFull(r, rowBuf); err != nil {
			return nil, err
		}
		if err := normalizePaletteIndices(
			img.Pix[destOffset:destOffset+w],
			rowBuf,
			colorMapStart,
			colorMapLen,
		); err != nil {
			return nil, err
		}
	}

	if flipX {
		flipImageHorizontally(img, w, h)
	}

	return img, nil
}

// decodeRLE decodes RLE-compressed true-color or grayscale data.
// RLE packets may cross scan lines; we decode linearly then flip if needed.
func decodeRLE(r *bufio.Reader, w, h, depth int, hasAlpha, flipX, flipY bool) (image.Image, error) {
	bytesPerPixel := (depth + 7) / 8
	totalPixels := w * h

	var img image.Image
	var outPix []byte
	var isGray bool

	if depth == 8 {
		gray := image.NewGray(image.Rect(0, 0, w, h))
		img = gray
		outPix = gray.Pix
		isGray = true
	} else {
		nrgba := image.NewNRGBA(image.Rect(0, 0, w, h))
		img = nrgba
		outPix = nrgba.Pix
	}

	pixelBuf := make([]byte, bytesPerPixel)
	rawBuf := make([]byte, 128*bytesPerPixel)
	pixelsRead := 0
	outIdx := 0

	for pixelsRead < totalPixels {
		packetHeader, err := r.ReadByte()
		if err != nil {
			return nil, err
		}

		packetType := packetHeader & 0x80
		count := int(packetHeader&0x7F) + 1

		if pixelsRead+count > totalPixels {
			return nil, ErrRLEOverrun
		}

		if packetType != 0 {
			if _, err := io.ReadFull(r, pixelBuf); err != nil {
				return nil, err
			}

			if isGray {
				dst := outPix[outIdx : outIdx+count]
				dst[0] = pixelBuf[0]
				replicatePattern(dst, 1)
				outIdx += count
			} else {
				var rv, gv, bv, av uint8

				// Convert the pixel buffer to NRGBA
				switch depth {
				case 24:
					bv, gv, rv, av = pixelBuf[0], pixelBuf[1], pixelBuf[2], 0xff

				case 32:
					bv, gv, rv, av = pixelBuf[0], pixelBuf[1], pixelBuf[2], pixelBuf[3]

				case 15, 16:
					c := decode16BitTrueColor(
						uint16(pixelBuf[0])|uint16(pixelBuf[1])<<8,
						hasAlpha,
					)
					rv, gv, bv, av = c.R, c.G, c.B, c.A
				}

				// Write one pixel, then replicate it across the run via memmove.
				dst := outPix[outIdx : outIdx+count*4]
				dst[0], dst[1], dst[2], dst[3] = rv, gv, bv, av
				replicatePattern(dst, 4)
				outIdx += count * 4
			}
		} else {
			if isGray {
				target := outPix[outIdx : outIdx+count]
				if _, err := io.ReadFull(r, target); err != nil {
					return nil, err
				}

				outIdx += count
			} else {
				rawLen := count * bytesPerPixel
				buf := rawBuf[:rawLen]
				if _, err := io.ReadFull(r, buf); err != nil {
					return nil, err
				}

				convertBufferToNRGBA(outPix[outIdx:], buf, count, depth, hasAlpha)
				outIdx += count * 4
			}
		}

		pixelsRead += count
	}

	if flipY {
		flipImageVertically(img, w, h)
	}
	if flipX {
		flipImageHorizontally(img, w, h)
	}

	return img, nil
}

// decodeRLEPaletted decodes RLE-compressed color-mapped image data.
func decodeRLEPaletted(
	r *bufio.Reader,
	w, h, depth int,
	flipX, flipY bool,
	pal color.Palette,
	colorMapStart, colorMapLen int,
) (image.Image, error) {
	if depth != 8 {
		return nil, ErrUnsupported
	}

	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	outPix := img.Pix
	totalPixels := w * h
	pixelsRead := 0
	outIdx := 0
	rawBuf := make([]byte, 128)

	// Decode RLE packets
	for pixelsRead < totalPixels {
		packetHeader, err := r.ReadByte()
		if err != nil {
			return nil, err
		}

		packetType := packetHeader & 0x80
		count := int(packetHeader&0x7F) + 1

		if pixelsRead+count > totalPixels {
			return nil, ErrRLEOverrun
		}

		// If packetType is not 0, it's an RLE packet.
		if packetType != 0 {
			val, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			val, err = normalizePaletteIndex(val, colorMapStart, colorMapLen)
			if err != nil {
				return nil, err
			}

			dst := outPix[outIdx : outIdx+count]
			dst[0] = val
			replicatePattern(dst, 1)
			outIdx += count
		} else {
			buf := rawBuf[:count]
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, err
			}

			if err := normalizePaletteIndices(
				outPix[outIdx:outIdx+count],
				buf,
				colorMapStart,
				colorMapLen,
			); err != nil {
				return nil, err
			}

			outIdx += count
		}
		pixelsRead += count
	}

	if flipY {
		flipImageVertically(img, w, h)
	}
	if flipX {
		flipImageHorizontally(img, w, h)
	}

	return img, nil
}

// normalizePaletteIndices validates TGA color-map indices and converts them to local palette indices.
func normalizePaletteIndices(dst, src []byte, colorMapStart, colorMapLen int) error {
	for i, index := range src {
		normalized, err := normalizePaletteIndex(index, colorMapStart, colorMapLen)
		if err != nil {
			return err
		}
		dst[i] = normalized
	}

	return nil
}

// normalizePaletteIndex validates one TGA color-map index and converts it to a local palette index.
func normalizePaletteIndex(index byte, colorMapStart, colorMapLen int) (byte, error) {
	if colorMapStart < 0 || colorMapStart > 0xff || colorMapLen <= 0 {
		return 0, ErrPaletteIndex
	}

	value := int(index)
	if value < colorMapStart || value >= colorMapStart+colorMapLen {
		return 0, ErrPaletteIndex
	}

	normalized := value - colorMapStart
	return byte(normalized), nil // #nosec G115 -- normalized is within [0, 255].
}

// convertRowToNRGBA converts one row of TGA BGR/BGRA bytes to NRGBA (RGBA).
// dst must have length w*4.
func convertRowToNRGBA(dst []byte, src []byte, w int, depth int, hasAlpha bool) {
	switch depth {
	case 24:
		simd.BGRToRGBA(dst[:w*4], src[:w*3])

	case 32:
		simd.SwapRB32(dst[:w*4], src[:w*4])

	case 15, 16:
		convertRow16ToNRGBA(dst[:w*4], src[:w*2], hasAlpha)
	}
}

// convertRow16ToNRGBA converts one 15/16-bit BGR555 row to 32-bit RGBA.
func convertRow16ToNRGBA(dst []byte, src []byte, hasAlpha bool) {
	di := 0
	for si := 0; si < len(src); si += 2 {
		v := uint16(src[si]) | uint16(src[si+1])<<8
		c := decode16BitTrueColor(v, hasAlpha)
		dst[di+0] = c.R
		dst[di+1] = c.G
		dst[di+2] = c.B
		dst[di+3] = c.A
		di += 4
	}
}

// convertBufferToNRGBA converts a chunk of TGA pixel bytes to NRGBA (used by RLE raw packets).
func convertBufferToNRGBA(dst []byte, src []byte, count int, depth int, hasAlpha bool) {
	convertRowToNRGBA(dst, src, count, depth, hasAlpha)
}

// decode16BitTrueColor decodes a 15-bit RGB555 or 16-bit A1R5G5B5 pixel.
func decode16BitTrueColor(v uint16, hasAlpha bool) color.NRGBA {
	c := decodeRGB555(v)
	if hasAlpha && v&(1<<15) == 0 {
		c.A = 0
	}

	return c
}

// decodeRGB555 converts a 16-bit word (ARRRRRGGGGGBBBBB) to NRGBA.
// Alpha bit is typically ignored; we output 0xff.
// 5-bit fields are in 0..31, safe for uint8.
func decodeRGB555(v uint16) color.NRGBA {
	const mask5 = 0x1f
	r := byte((v >> 10) & mask5)
	g := byte((v >> 5) & mask5)
	b := byte(v & mask5)

	r = (r << 3) | (r >> 2)
	g = (g << 3) | (g >> 2)
	b = (b << 3) | (b >> 2)

	return color.NRGBA{R: r, G: g, B: b, A: 0xff}
}

// replicatePattern fills dst with repeated copies of its first unit bytes.
// dst[:unit] must already hold the pattern and len(dst) must be a multiple of unit.
// The filled region grows exponentially via copy (runtime.memmove),
// which is far faster than storing the pattern element-by-element.
func replicatePattern(dst []byte, unit int) {
	for n := unit; n < len(dst); {
		n += copy(dst[n:], dst[:n])
	}
}

// flipImageVertically flips the image in place along the horizontal axis.
func flipImageVertically(img image.Image, _, h int) {
	var pix []uint8
	var stride int

	// Get pixel data
	switch m := img.(type) {
	case *image.NRGBA:
		pix = m.Pix
		stride = m.Stride

	case *image.Gray:
		pix = m.Pix
		stride = m.Stride

	case *image.Paletted:
		pix = m.Pix
		stride = m.Stride

	default:
		return
	}

	rowBuf := make([]byte, stride)
	halfH := h / 2

	// Flip the image vertically
	for y := range halfH {
		y1 := y * stride
		y2 := (h - 1 - y) * stride

		copy(rowBuf, pix[y1:y1+stride])
		copy(pix[y1:y1+stride], pix[y2:y2+stride])
		copy(pix[y2:y2+stride], rowBuf)
	}
}

// flipImageHorizontally flips the image in place along the vertical axis.
func flipImageHorizontally(img image.Image, w, h int) {
	var pix []uint8
	var stride int
	var pixelSize int

	switch m := img.(type) {
	case *image.NRGBA:
		pix = m.Pix
		stride = m.Stride
		pixelSize = 4

	case *image.Gray:
		pix = m.Pix
		stride = m.Stride
		pixelSize = 1

	case *image.Paletted:
		pix = m.Pix
		stride = m.Stride
		pixelSize = 1

	default:
		return
	}

	for y := range h {
		row := pix[y*stride : y*stride+w*pixelSize]
		for x := 0; x < w/2; x++ {
			left := x * pixelSize
			right := (w - 1 - x) * pixelSize
			for i := range pixelSize {
				row[left+i], row[right+i] = row[right+i], row[left+i]
			}
		}
	}
}
