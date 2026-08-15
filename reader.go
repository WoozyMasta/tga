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

const maxInt = int(^uint(0) >> 1)

// DecodeOptions limits resources consumed by DecodeWithOptions.
// A zero limit disables that limit.
type DecodeOptions struct {
	// MaxPixels limits the total decoded image pixels.
	MaxPixels uint64 `json:"max_pixels,omitempty"`
	// MaxDecodedBytes limits storage for decoded pixels:
	// four bytes per NRGBA pixel and one byte per Gray or Paletted pixel.
	MaxDecodedBytes uint64 `json:"max_decoded_bytes,omitempty"`
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

// RegisterFormat registers the TGA format with image.Decode and image.DecodeConfig.
// Because TGA has no magic bytes, it does not play nicely with other formats
// when image.Decode tries them in order; registration is disabled by default.
// Call this explicitly if you need image.Decode to recognize TGA
// (e.g. in a TGA-only context or after other formats).
func RegisterFormat() {
	image.RegisterFormat("tga", "", Decode, DecodeConfig)
}

// DecodeConfig returns the image configuration after consuming only the header
// and color-map data required to validate it. It does not decode image pixels.
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
		if header.pixelDepth == 16 {
			cm = color.NRGBAModel
		} else {
			cm = color.GrayModel
		}
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
	return decode(r, DecodeOptions{})
}

// DecodeWithOptions reads a TGA image from r subject to opts resource limits.
// A zero-value DecodeOptions leaves both limits disabled. Decode does not close r.
func DecodeWithOptions(r io.Reader, opts DecodeOptions) (image.Image, error) {
	return decode(r, opts)
}

func decode(r io.Reader, opts DecodeOptions) (img image.Image, err error) {
	defer func() {
		err = wrapTruncated(err)
	}()

	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	header, err := readHeader(br)
	if err != nil {
		return nil, err
	}
	if err := validateDecodeSize(header, opts); err != nil {
		return nil, err
	}

	if header.idLen > 0 {
		if _, err := br.Discard(header.idLen); err != nil {
			return nil, err
		}
	}

	var palette color.Palette
	if header.hasColorMap {
		// image.Paletted uses zero-based indices,
		// so TGA's ColorMapFirst is applied while decoding pixels
		// rather than by padding the palette.
		entryBytes := (int(header.colorMapDepth) + 7) / 8
		paletteBytes, err := checkedMul(header.colorMapLen, entryBytes)
		if err != nil {
			return nil, err
		}
		rawPalette := make([]byte, paletteBytes)
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

	// TGA stores pixels from the descriptor-selected corner; normalize all
	// supported origins to Go's top-left coordinate system after decoding.
	flipY := (header.descriptor & maskOriginTop) == 0
	flipX := header.descriptor&maskOriginRight != 0

	switch header.imageType {
	case typeTrueColor:
		return decodeUncompressed(
			br,
			header.width,
			header.height,
			header.pixelDepth,
			header.hasAlpha,
			flipX,
			flipY,
		)

	case typeGrayscale:
		if header.pixelDepth == 16 {
			return decodeGray16(br, header.width, header.height, flipX, flipY)
		}
		return decodeUncompressed(br, header.width, header.height, header.pixelDepth, false, flipX, flipY)

	case typeRLETrueColor:
		return decodeRLE(
			br,
			header.width,
			header.height,
			header.pixelDepth,
			header.hasAlpha,
			flipX,
			flipY,
		)

	case typeRLEGrayscale:
		if header.pixelDepth == 16 {
			return decodeRLEGray16(br, header.width, header.height, flipX, flipY)
		}
		return decodeRLE(br, header.width, header.height, header.pixelDepth, false, flipX, flipY)

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

// validateDecodeSize checks decoded image allocation size and configured resource limits.
func validateDecodeSize(header parsedHeader, opts DecodeOptions) error {
	// Validate the final image allocation, not the compressed input length:
	// a tiny RLE stream can expand to a large image.
	pixels, err := checkedMul(header.width, header.height)
	if err != nil {
		return err
	}

	// #nosec G115 -- checkedMul returns a non-negative result.
	if opts.MaxPixels > 0 && uint64(pixels) > opts.MaxPixels {
		return ErrResourceLimit
	}

	bytesPerPixel := 4
	switch header.imageType {
	case typePaletted, typeRLEPaletted:
		bytesPerPixel = 1
	case typeGrayscale, typeRLEGrayscale:
		if header.pixelDepth == 8 {
			bytesPerPixel = 1
		}
	}

	decodedBytes, err := checkedMul(pixels, bytesPerPixel)
	if err != nil {
		return err
	}

	// #nosec G115 -- checkedMul returns a non-negative result.
	if opts.MaxDecodedBytes > 0 && uint64(decodedBytes) > opts.MaxDecodedBytes {
		return ErrResourceLimit
	}

	return nil
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
		// Only 0 and 1 are defined;
		// treating other values as "no map" would silently reinterpret malformed indexed images.
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
		if pixelDepth != 8 && pixelDepth != 16 {
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

// checkedMul multiplies non-negative native integers without overflow.
func checkedMul(a, b int) (int, error) {
	if a < 0 || b < 0 || (a != 0 && b > maxInt/a) {
		return 0, ErrFormat
	}

	return a * b, nil
}

// decodeUncompressed reads uncompressed true-color or grayscale image data.
func decodeUncompressed(r io.Reader, w, h, depth int, hasAlpha, flipX, flipY bool) (image.Image, error) {
	bytesPerPixel := (depth + 7) / 8
	rowSize, err := checkedMul(w, bytesPerPixel)
	if err != nil {
		return nil, err
	}
	totalPixels, err := checkedMul(w, h)
	if err != nil {
		return nil, err
	}
	if depth != 8 {
		if _, err := checkedMul(totalPixels, 4); err != nil {
			return nil, err
		}
	}

	if depth == 8 {
		gray := image.NewGray(image.Rect(0, 0, w, h))

		for y := range h {
			destY := y
			if flipY {
				destY = h - 1 - y
			}

			destOffset := destY * gray.Stride

			row := gray.Pix[destOffset : destOffset+rowSize]
			if _, err := io.ReadFull(r, row); err != nil {
				return nil, err
			}
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

// decodeGray16 decodes luminance/alpha pairs into straight-alpha NRGBA pixels.
func decodeGray16(r io.Reader, w, h int, flipX, flipY bool) (image.Image, error) {
	rowSize, err := checkedMul(w, 2)
	if err != nil {
		return nil, err
	}
	totalPixels, err := checkedMul(w, h)
	if err != nil {
		return nil, err
	}
	if _, err := checkedMul(totalPixels, 4); err != nil {
		return nil, err
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	row := make([]byte, rowSize)

	for y := range h {
		if _, err := io.ReadFull(r, row); err != nil {
			return nil, err
		}

		dstY := y
		if flipY {
			dstY = h - 1 - y
		}

		dst := img.Pix[dstY*img.Stride:]
		// TGA Gray16 stores one luminance byte followed by one alpha byte.
		for x := range w {
			l, a := row[x*2], row[x*2+1]
			dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = l, l, l, a
		}
	}

	if flipX {
		flipImageHorizontally(img, w, h)
	}

	return img, nil
}

// decodeRLEGray16 decodes RLE luminance/alpha pairs into straight-alpha NRGBA pixels.
func decodeRLEGray16(r *bufio.Reader, w, h int, flipX, flipY bool) (image.Image, error) {
	total, err := checkedMul(w, h)
	if err != nil {
		return nil, err
	}
	if _, err := checkedMul(total, 4); err != nil {
		return nil, err
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	pixel := make([]byte, 2)
	out := 0

	for out < total {
		head, err := r.ReadByte()
		if err != nil {
			return nil, err
		}

		count := int(head&0x7f) + 1
		if out+count > total {
			return nil, ErrRLEOverrun
		}

		// Raw packets read one pair per pixel; RLE packets reuse their first pair.
		for n := range count {
			if head&0x80 == 0 || n == 0 {
				if _, err := io.ReadFull(r, pixel); err != nil {
					return nil, err
				}
			}
			i := out * 4
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pixel[0], pixel[0], pixel[0], pixel[1]
			out++
		}
	}

	if flipY {
		flipImageVertically(img, w, h)
	}
	if flipX {
		flipImageHorizontally(img, w, h)
	}

	return img, nil
}

// decodeUncompressedPaletted reads uncompressed color-mapped image data.
func decodeUncompressedPaletted(r io.Reader, w, h, depth int, flipX, flipY bool, pal color.Palette, colorMapStart, colorMapLen int) (image.Image, error) {
	if depth != 8 {
		return nil, ErrUnsupported
	}
	if _, err := checkedMul(w, h); err != nil {
		return nil, err
	}

	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	stride := img.Stride

	for y := range h {
		destY := y
		if flipY {
			destY = h - 1 - y
		}
		destOffset := destY * stride
		row := img.Pix[destOffset : destOffset+w]

		if _, err := io.ReadFull(r, row); err != nil {
			return nil, err
		}
		if err := normalizePaletteIndices(
			row,
			row,
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
// RLE packets may cross scan lines; chunks are placed into logical rows directly.
func decodeRLE(r *bufio.Reader, w, h, depth int, hasAlpha, flipX, flipY bool) (image.Image, error) {
	bytesPerPixel := (depth + 7) / 8
	totalPixels, err := checkedMul(w, h)
	if err != nil {
		return nil, err
	}
	if _, err := checkedMul(totalPixels, 4); err != nil {
		return nil, err
	}

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
	rawBufferSize, err := checkedMul(128, bytesPerPixel)
	if err != nil {
		return nil, err
	}
	var rawBuf []byte
	if depth != 8 || flipY {
		rawBuf = make([]byte, rawBufferSize)
	}
	pixelsRead := 0

	for pixelsRead < totalPixels {
		packetHeader, err := r.ReadByte()
		if err != nil {
			return nil, err
		}

		packetType := packetHeader & 0x80
		count := int(packetHeader&0x7F) + 1

		// Packets are allowed to cross rows, but never the declared image end.
		if pixelsRead+count > totalPixels {
			return nil, ErrRLEOverrun
		}

		if packetType != 0 {
			if _, err := io.ReadFull(r, pixelBuf); err != nil {
				return nil, err
			}

			if isGray {
				if flipY {
					writeRLEGrayRun(outPix, w, pixelsRead, count, pixelBuf[0])
				} else {
					dst := outPix[pixelsRead : pixelsRead+count]
					dst[0] = pixelBuf[0]
					replicatePattern(dst, 1)
				}
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

				if flipY {
					writeRLETrueColorRun(outPix, w, pixelsRead, count, rv, gv, bv, av)
				} else {
					dst := outPix[pixelsRead*4 : (pixelsRead+count)*4]
					dst[0], dst[1], dst[2], dst[3] = rv, gv, bv, av
					replicatePattern(dst, 4)
				}
			}
		} else {
			if isGray {
				if flipY {
					buf := rawBuf[:count]
					if _, err := io.ReadFull(r, buf); err != nil {
						return nil, err
					}
					writeRLEGrayRaw(outPix, w, pixelsRead, buf)
				} else {
					if _, err := io.ReadFull(r, outPix[pixelsRead:pixelsRead+count]); err != nil {
						return nil, err
					}
				}
			} else {
				rawLen, err := checkedMul(count, bytesPerPixel)
				if err != nil {
					return nil, err
				}
				buf := rawBuf[:rawLen]
				if _, err := io.ReadFull(r, buf); err != nil {
					return nil, err
				}

				if flipY {
					writeRLETrueColorRaw(outPix, w, pixelsRead, buf, count, depth, hasAlpha)
				} else {
					convertBufferToNRGBA(outPix[pixelsRead*4:], buf, count, depth, hasAlpha)
				}
			}
		}

		pixelsRead += count
	}

	if flipX {
		flipImageHorizontally(img, w, h)
	}

	return img, nil
}

// writeRLEGrayRun writes a repeated grayscale value into logical destination rows.
func writeRLEGrayRun(dst []byte, w, start, count int, value byte) {
	for count > 0 {
		x := start % w
		y := start / w
		chunk := min(count, w-x)
		y = (len(dst)/w - 1) - y
		row := dst[y*w+x : y*w+x+chunk]
		row[0] = value
		replicatePattern(row, 1)
		start += chunk
		count -= chunk
	}
}

// writeRLEGrayRaw writes raw grayscale bytes into logical destination rows.
func writeRLEGrayRaw(dst []byte, w, start int, src []byte) {
	for len(src) > 0 {
		x := start % w
		y := start / w
		chunk := min(len(src), w-x)
		y = (len(dst)/w - 1) - y
		copy(dst[y*w+x:y*w+x+chunk], src[:chunk])
		start += chunk
		src = src[chunk:]
	}
}

// writeRLETrueColorRun writes a repeated converted pixel into logical rows.
func writeRLETrueColorRun(dst []byte, w, start, count int, r, g, b, a byte) {
	pixel := [4]byte{r, g, b, a}
	for count > 0 {
		x := start % w
		y := start / w
		chunk := min(count, w-x)
		y = (len(dst)/(w*4) - 1) - y
		row := dst[(y*w+x)*4 : (y*w+x+chunk)*4]
		copy(row[:4], pixel[:])
		replicatePattern(row, 4)
		start += chunk
		count -= chunk
	}
}

// writeRLETrueColorRaw converts raw pixels into logical destination rows.
func writeRLETrueColorRaw(dst []byte, w, start int, src []byte, count, depth int, hasAlpha bool) {
	bytesPerPixel := (depth + 7) / 8
	srcOffset := 0
	for count > 0 {
		x := start % w
		y := start / w
		chunk := min(count, w-x)
		y = (len(dst)/(w*4) - 1) - y
		dstOffset := (y*w + x) * 4
		srcEnd := srcOffset + chunk*bytesPerPixel
		convertBufferToNRGBA(dst[dstOffset:], src[srcOffset:srcEnd], chunk, depth, hasAlpha)
		start += chunk
		count -= chunk
		srcOffset = srcEnd
	}
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

	if _, err := checkedMul(w, h); err != nil {
		return nil, err
	}

	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	outPix := img.Pix
	totalPixels, err := checkedMul(w, h)
	if err != nil {
		return nil, err
	}
	pixelsRead := 0
	rawBuf := make([]byte, 128)

	// Decode RLE packets
	for pixelsRead < totalPixels {
		packetHeader, err := r.ReadByte()
		if err != nil {
			return nil, err
		}

		packetType := packetHeader & 0x80
		count := int(packetHeader&0x7F) + 1

		// Palette packets share the same linear stream semantics as true color.
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

			if flipY {
				writeRLEPaletteRun(outPix, w, pixelsRead, count, val)
			} else {
				dst := outPix[pixelsRead : pixelsRead+count]
				dst[0] = val
				replicatePattern(dst, 1)
			}
		} else {
			buf := rawBuf[:count]
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, err
			}

			if flipY {
				if err := writeRLEPaletteRaw(
					outPix,
					w,
					pixelsRead,
					buf,
					colorMapStart,
					colorMapLen,
				); err != nil {
					return nil, err
				}
			} else if err := normalizePaletteIndices(
				outPix[pixelsRead:pixelsRead+count],
				buf,
				colorMapStart,
				colorMapLen,
			); err != nil {
				return nil, err
			}
		}
		pixelsRead += count
	}

	if flipX {
		flipImageHorizontally(img, w, h)
	}

	return img, nil
}

// writeRLEPaletteRun writes a normalized palette index into logical rows.
func writeRLEPaletteRun(dst []byte, w, start, count int, value byte) {
	for count > 0 {
		x := start % w
		y := start / w
		chunk := min(count, w-x)
		y = (len(dst)/w - 1) - y
		row := dst[y*w+x : y*w+x+chunk]
		row[0] = value
		replicatePattern(row, 1)
		start += chunk
		count -= chunk
	}
}

// writeRLEPaletteRaw validates and writes raw palette indices into logical rows.
func writeRLEPaletteRaw(dst []byte, w, start int, src []byte, colorMapStart, colorMapLen int) error {
	srcOffset := 0
	for len(src) > 0 {
		x := start % w
		y := start / w
		chunk := min(len(src), w-x)
		y = (len(dst)/w - 1) - y
		row := dst[y*w+x : y*w+x+chunk]

		for i := range chunk {
			value, err := normalizePaletteIndex(src[i], colorMapStart, colorMapLen)
			if err != nil {
				return err
			}

			row[i] = value
		}

		start += chunk
		srcOffset += chunk
		src = src[chunk:]
	}

	return nil
}

// normalizePaletteIndices validates TGA color-map indices and converts them to local palette indices.
func normalizePaletteIndices(dst, src []byte, colorMapStart, colorMapLen int) error {
	if colorMapStart == 0 && colorMapLen == 256 {
		return nil
	}
	if colorMapStart == 0 {
		for i, index := range src {
			if int(index) >= colorMapLen {
				return ErrPaletteIndex
			}
			dst[i] = index
		}
		return nil
	}

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
	if hasAlpha {
		convertRow16ToNRGBAAlpha(dst, src)
		return
	}

	convertRow16ToNRGBAOpaque(dst, src)
}

// convertRow16ToNRGBAOpaque converts RGB555 pixels without per-pixel alpha checks.
func convertRow16ToNRGBAOpaque(dst []byte, src []byte) {
	di := 0
	for si := 0; si < len(src); si += 2 {
		v := uint16(src[si]) | uint16(src[si+1])<<8
		r := byte((v >> 10) & 0x1f)
		g := byte((v >> 5) & 0x1f)
		b := byte(v & 0x1f)
		dst[di+0] = (r << 3) | (r >> 2)
		dst[di+1] = (g << 3) | (g >> 2)
		dst[di+2] = (b << 3) | (b >> 2)
		dst[di+3] = 0xff
		di += 4
	}
}

// convertRow16ToNRGBAAlpha converts A1R5G5B5 pixels with their alpha bit.
func convertRow16ToNRGBAAlpha(dst []byte, src []byte) {
	di := 0
	for si := 0; si < len(src); si += 2 {
		v := uint16(src[si]) | uint16(src[si+1])<<8
		r := byte((v >> 10) & 0x1f)
		g := byte((v >> 5) & 0x1f)
		b := byte(v & 0x1f)
		dst[di+0] = (r << 3) | (r >> 2)
		dst[di+1] = (g << 3) | (g >> 2)
		dst[di+2] = (b << 3) | (b >> 2)
		dst[di+3] = byte((v >> 15) * 0xff)
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
