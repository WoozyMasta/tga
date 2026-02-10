package tga

import (
	"bufio"
	"image"
	"image/color"
	"io"
)

const (
	headerSize = 18 // Header size.

	typePaletted     = 1  // Color-mapped image.
	typeTrueColor    = 2  // True-color image.
	typeGrayscale    = 3  // Grayscale image.
	typeRLEPaletted  = 9  // RLE color-mapped image.
	typeRLETrueColor = 10 // RLE true-color image.
	typeRLEGrayscale = 11 // RLE grayscale image.

	maskOriginTop = 0x20 // Bit 5 of image descriptor: 0 = bottom-left, 1 = top-left origin.
)

// RegisterFormat registers the TGA format with image.Decode and image.DecodeConfig.
// Because TGA has no magic bytes, it does not play nicely with other formats when
// image.Decode tries them in order; registration is disabled by default. Call this
// explicitly if you need image.Decode to recognize TGA (e.g. in a TGA-only context
// or after other formats).
func RegisterFormat() {
	image.RegisterFormat("tga", "", Decode, DecodeConfig)
}

// DecodeConfig returns the image configuration without decoding pixel data.
func DecodeConfig(r io.Reader) (image.Config, error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return image.Config{}, err
	}

	width := int(header[12]) | int(header[13])<<8
	height := int(header[14]) | int(header[15])<<8
	bpp := header[16]

	// Determine the color model based on the bits per pixel
	var cm color.Model
	switch bpp {
	case 8:
		imageType := header[2]
		if imageType == typePaletted || imageType == typeRLEPaletted {
			cm = color.Palette{}
		} else {
			cm = color.GrayModel
		}

	case 15, 16, 24, 32:
		cm = color.NRGBAModel

	default:
		return image.Config{}, ErrUnsupported
	}

	return image.Config{
		ColorModel: cm,
		Width:      width,
		Height:     height,
	}, nil
}

// Decode reads a TGA image from r.
func Decode(r io.Reader) (image.Image, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	var header [headerSize]byte
	if _, err := io.ReadFull(br, header[:]); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, ErrHeaderTooShort
		}
		return nil, err
	}

	idLen := int(header[0])
	hasCMap := header[1] == 1
	imgType := header[2]
	cMapStart := int(header[3]) | int(header[4])<<8
	cMapLen := int(header[5]) | int(header[6])<<8
	cMapDepth := header[7]

	width := int(header[12]) | int(header[13])<<8
	height := int(header[14]) | int(header[15])<<8
	pixelDepth := int(header[16])
	desc := header[17]

	if width == 0 || height == 0 {
		return nil, ErrFormat
	}

	if idLen > 0 {
		if _, err := br.Discard(idLen); err != nil {
			return nil, err
		}
	}

	var palette color.Palette
	if hasCMap && cMapLen > 0 {
		entryBytes := (int(cMapDepth) + 7) / 8
		rawPalette := make([]byte, cMapLen*entryBytes)
		if _, err := io.ReadFull(br, rawPalette); err != nil {
			return nil, err
		}

		palSize := cMapStart + cMapLen
		palette = make(color.Palette, palSize)
		for i := 0; i < cMapStart; i++ {
			palette[i] = color.NRGBA{}
		}

		// Create the color palette
		for i := 0; i < cMapLen; i++ {
			offset := i * entryBytes
			idx := cMapStart + i
			switch cMapDepth {
			case 24:
				palette[idx] = color.NRGBA{
					R: rawPalette[offset+2],
					G: rawPalette[offset+1],
					B: rawPalette[offset+0],
					A: 0xff,
				}

			case 32:
				palette[idx] = color.NRGBA{
					R: rawPalette[offset+2],
					G: rawPalette[offset+1],
					B: rawPalette[offset+0],
					A: rawPalette[offset+3],
				}

			case 15, 16:
				v := uint16(rawPalette[offset]) | uint16(rawPalette[offset+1])<<8
				palette[idx] = decodeRGB555(v)

			default:
				palette[idx] = color.Gray{Y: rawPalette[offset]}
			}
		}
	}

	// Bit 5 of descriptor: 0 = lower-left origin, 1 = upper-left. Go uses top-left.
	flipY := (desc & maskOriginTop) == 0

	switch imgType {
	case typeTrueColor, typeGrayscale:
		return decodeUncompressed(br, width, height, pixelDepth, flipY)
	case typeRLETrueColor, typeRLEGrayscale:
		return decodeRLE(br, width, height, pixelDepth, flipY)
	case typePaletted:
		return decodeUncompressedPaletted(br, width, height, pixelDepth, flipY, palette)
	case typeRLEPaletted:
		return decodeRLEPaletted(br, width, height, pixelDepth, flipY, palette)
	default:
		return nil, ErrUnsupported
	}
}

// decodeUncompressed reads uncompressed true-color or grayscale image data.
func decodeUncompressed(r io.Reader, w, h, depth int, flipY bool) (image.Image, error) {
	bytesPerPixel := (depth + 7) / 8
	rowSize := w * bytesPerPixel

	var img image.Image
	var pix []uint8
	var stride int
	var isGray bool

	if depth == 8 {
		gray := image.NewGray(image.Rect(0, 0, w, h))
		img = gray
		pix = gray.Pix
		stride = gray.Stride
		isGray = true
	} else {
		nrgba := image.NewNRGBA(image.Rect(0, 0, w, h))
		img = nrgba
		pix = nrgba.Pix
		stride = nrgba.Stride
	}

	rowBuf := make([]byte, rowSize)

	for y := 0; y < h; y++ {
		destY := y
		if flipY {
			destY = h - 1 - y
		}

		destOffset := destY * stride

		if _, err := io.ReadFull(r, rowBuf); err != nil {
			return nil, err
		}

		if isGray {
			copy(pix[destOffset:], rowBuf)
		} else {
			convertRowToNRGBA(pix[destOffset:], rowBuf, w, depth)
		}
	}

	return img, nil
}

// decodeUncompressedPaletted reads uncompressed color-mapped image data.
func decodeUncompressedPaletted(r io.Reader, w, h, depth int, flipY bool, pal color.Palette) (image.Image, error) {
	if depth != 8 {
		return nil, ErrUnsupported
	}

	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	stride := img.Stride
	rowBuf := make([]byte, w)

	for y := 0; y < h; y++ {
		destY := y
		if flipY {
			destY = h - 1 - y
		}
		destOffset := destY * stride

		if _, err := io.ReadFull(r, rowBuf); err != nil {
			return nil, err
		}
		copy(img.Pix[destOffset:], rowBuf)
	}

	return img, nil
}

// decodeRLE decodes RLE-compressed true-color or grayscale data.
// RLE packets may cross scan lines; we decode linearly then flip if needed.
func decodeRLE(r *bufio.Reader, w, h, depth int, flipY bool) (image.Image, error) {
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
				val := pixelBuf[0]
				for i := 0; i < count; i++ {
					outPix[outIdx] = val
					outIdx++
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
					c := decodeRGB555(uint16(pixelBuf[0]) | uint16(pixelBuf[1])<<8)
					rv, gv, bv, av = c.R, c.G, c.B, c.A
				}

				for i := 0; i < count; i++ {
					outPix[outIdx+0] = rv
					outPix[outIdx+1] = gv
					outPix[outIdx+2] = bv
					outPix[outIdx+3] = av
					outIdx += 4
				}
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
				buf := make([]byte, rawLen)
				if _, err := io.ReadFull(r, buf); err != nil {
					return nil, err
				}

				convertBufferToNRGBA(outPix[outIdx:], buf, count, depth)
				outIdx += count * 4
			}
		}

		pixelsRead += count
	}

	if flipY {
		flipImageVertically(img, w, h)
	}

	return img, nil
}

// decodeRLEPaletted decodes RLE-compressed color-mapped image data.
func decodeRLEPaletted(r *bufio.Reader, w, h, depth int, flipY bool, pal color.Palette) (image.Image, error) {
	if depth != 8 {
		return nil, ErrUnsupported
	}

	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	outPix := img.Pix
	totalPixels := w * h
	pixelsRead := 0
	outIdx := 0

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

		// If packetType is not 0, it's a raw packet
		if packetType != 0 {
			val, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			for i := 0; i < count; i++ {
				outPix[outIdx] = val
				outIdx++
			}
		} else {
			if _, err := io.ReadFull(r, outPix[outIdx:outIdx+count]); err != nil {
				return nil, err
			}
			outIdx += count
		}
		pixelsRead += count
	}

	if flipY {
		flipImageVertically(img, w, h)
	}

	return img, nil
}

// convertRowToNRGBA converts one row of TGA BGR/BGRA bytes to NRGBA (RGBA).
// dst must have length w*4.
func convertRowToNRGBA(dst []byte, src []byte, w int, depth int) {
	di := 0
	si := 0

	switch depth {
	case 24:
		for i := 0; i < w; i++ {
			b := src[si]
			g := src[si+1]
			r := src[si+2]
			dst[di+0] = r
			dst[di+1] = g
			dst[di+2] = b
			dst[di+3] = 0xff
			si += 3
			di += 4
		}

	case 32:
		for i := 0; i < w; i++ {
			b := src[si]
			g := src[si+1]
			r := src[si+2]
			a := src[si+3]
			dst[di+0] = r
			dst[di+1] = g
			dst[di+2] = b
			dst[di+3] = a
			si += 4
			di += 4
		}

	case 15, 16:
		for i := 0; i < w; i++ {
			v := uint16(src[si]) | uint16(src[si+1])<<8
			c := decodeRGB555(v)
			dst[di+0] = c.R
			dst[di+1] = c.G
			dst[di+2] = c.B
			dst[di+3] = c.A
			si += 2
			di += 4
		}
	}
}

// convertBufferToNRGBA converts a chunk of TGA pixel bytes to NRGBA (used by RLE raw packets).
func convertBufferToNRGBA(dst []byte, src []byte, count int, depth int) {
	convertRowToNRGBA(dst, src, count, depth)
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
	for y := 0; y < halfH; y++ {
		y1 := y * stride
		y2 := (h - 1 - y) * stride

		copy(rowBuf, pix[y1:y1+stride])
		copy(pix[y1:y1+stride], pix[y2:y2+stride])
		copy(pix[y2:y2+stride], rowBuf)
	}
}
