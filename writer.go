package tga

import (
	"image"
	"image/draw"
	"io"
)

// Encode writes the image m in TGA format to w.
// Supports *image.Gray (8-bit grayscale, type 3) and *image.NRGBA (24- or 32-bit true color, type 2).
// Other image types are converted to NRGBA. Origin is top-left (descriptor bit 5 set).
// No TGA 2.0 footer or extension area is written.
func Encode(w io.Writer, m image.Image) error {
	b := m.Bounds()
	mw := b.Dx()
	mh := b.Dy()

	if mw <= 0 || mh <= 0 {
		return ErrFormat
	}

	if mw > 0xffff || mh > 0xffff {
		return ErrFormat
	}

	// 18-byte header: id=0, no color map, then image spec
	header := [18]byte{
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0,
		byte(mw), byte(mw >> 8),
		byte(mh), byte(mh >> 8),
		0,
		0x20, // bit 5: top-left origin
	}

	switch src := m.(type) {
	case *image.Gray:
		header[2] = typeGrayscale
		header[16] = 8
		if _, err := w.Write(header[:]); err != nil {
			return err
		}

		return encodeGray(w, src, b)

	case *image.NRGBA:
		header[2] = typeTrueColor
		header[16] = 32
		header[17] = 0x28 // 8 attribute bits, top-left
		if _, err := w.Write(header[:]); err != nil {
			return err
		}

		return encodeNRGBA(w, src, b)

	default:
		header[2] = typeTrueColor
		header[16] = 32
		header[17] = 0x28
		if _, err := w.Write(header[:]); err != nil {
			return err
		}

		dst := image.NewNRGBA(b)
		draw.Draw(dst, b, m, b.Min, draw.Src)

		return encodeNRGBA(w, dst, b)
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
