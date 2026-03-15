package tga

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"
)

func TestEncode_ZeroSize(t *testing.T) {
	empty := image.NewNRGBA(image.Rect(0, 0, 0, 0))
	err := Encode(bytes.NewBuffer(nil), empty)
	if err == nil {
		t.Fatal("expected error for zero-size image")
	}
	if !errors.Is(err, ErrFormat) {
		t.Errorf("expected ErrFormat, got %v", err)
	}

	zeroH := image.NewNRGBA(image.Rect(0, 0, 10, 0))
	if err := Encode(bytes.NewBuffer(nil), zeroH); err == nil {
		t.Fatal("expected error for zero height")
	} else if !errors.Is(err, ErrFormat) {
		t.Errorf("expected ErrFormat, got %v", err)
	}

	zeroW := image.NewNRGBA(image.Rect(0, 0, 0, 10))
	if err := Encode(bytes.NewBuffer(nil), zeroW); err == nil {
		t.Fatal("expected error for zero width")
	} else if !errors.Is(err, ErrFormat) {
		t.Errorf("expected ErrFormat, got %v", err)
	}
}

func TestEncode_TooLarge(t *testing.T) {
	// Width or height > 65535 must be rejected (uint16 overflow in header)
	wide := image.NewNRGBA(image.Rect(0, 0, 70000, 1))
	err := Encode(bytes.NewBuffer(nil), wide)
	if err == nil {
		t.Fatal("expected error for width > 65535")
	}
	if err != ErrFormat {
		t.Errorf("expected ErrFormat, got %v", err)
	}

	tall := image.NewNRGBA(image.Rect(0, 0, 1, 70000))
	err = Encode(bytes.NewBuffer(nil), tall)
	if err == nil {
		t.Fatal("expected error for height > 65535")
	}
}

func TestEncode_OtherImageType(t *testing.T) {
	// *image.RGBA and other types go through default path (convert to NRGBA via draw.Draw)
	rgba := image.NewRGBA(image.Rect(0, 0, 4, 4))
	rgba.Set(1, 1, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	var buf bytes.Buffer
	if err := Encode(&buf, rgba); err != nil {
		t.Fatalf("Encode RGBA: %v", err)
	}
	dec, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	// Decoded as NRGBA; compare RGBA at (1,1)
	r0, g0, b0, a0 := rgba.At(1, 1).RGBA()
	r1, g1, b1, a1 := dec.At(1, 1).RGBA()
	if r0 != r1 || g0 != g1 || b0 != b1 || a0 != a1 {
		t.Errorf("pixel (1,1): got %v, want %v", dec.At(1, 1), rgba.At(1, 1))
	}
}

func TestEncode_NonZeroBounds(t *testing.T) {
	// Image with Min != (0,0) must encode the visible rectangle correctly
	nrgba := image.NewNRGBA(image.Rect(5, 5, 15, 15))
	nrgba.SetNRGBA(5, 5, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	nrgba.SetNRGBA(14, 14, color.NRGBA{R: 4, G: 5, B: 6, A: 255})

	var buf bytes.Buffer
	if err := Encode(&buf, nrgba); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	dec, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if dec.Bounds().Dx() != 10 || dec.Bounds().Dy() != 10 {
		t.Errorf("decoded bounds: got %v", dec.Bounds())
	}
	// Decoded image has Rect(0,0,10,10); pixel that was at (5,5) is now at (0,0)
	c := dec.(*image.NRGBA).NRGBAAt(0, 0)
	if c.R != 1 || c.G != 2 || c.B != 3 {
		t.Errorf("pixel (0,0): got %v", c)
	}
	c = dec.(*image.NRGBA).NRGBAAt(9, 9)
	if c.R != 4 || c.G != 5 || c.B != 6 {
		t.Errorf("pixel (9,9): got %v", c)
	}
}

func TestEncodeWithOptions_RLEGray(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 8, 2))
	for i := 0; i < len(img.Pix); i++ {
		if i < 8 {
			img.Pix[i] = 33
			continue
		}

		img.Pix[i] = byte(i)
	}

	var buf bytes.Buffer
	err := EncodeWithOptions(&buf, img, &EncodeOptions{RLE: true})
	if err != nil {
		t.Fatalf("EncodeWithOptions Gray RLE: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 18 {
		t.Fatalf("encoded data too short: %d", len(data))
	}
	if data[2] != typeRLEGrayscale {
		t.Fatalf("header image type = %d, want %d", data[2], typeRLEGrayscale)
	}

	dec, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode RLE gray: %v", err)
	}

	if !imagesEqual(img, dec) {
		t.Fatal("gray RLE round-trip mismatch")
	}
}

func TestEncodeWithOptions_RLENRGBA(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 8; x++ {
			c := color.NRGBA{R: 10, G: 20, B: 30, A: 255}
			if y == 1 {
				c = color.NRGBA{
					R: byte(x * 20),
					G: byte(y * 40),
					B: byte(x * 15),
					A: byte(200 + x),
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}

	var buf bytes.Buffer
	err := EncodeWithOptions(&buf, img, &EncodeOptions{RLE: true})
	if err != nil {
		t.Fatalf("EncodeWithOptions NRGBA RLE: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 18 {
		t.Fatalf("encoded data too short: %d", len(data))
	}
	if data[2] != typeRLETrueColor {
		t.Fatalf("header image type = %d, want %d", data[2], typeRLETrueColor)
	}

	dec, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode RLE nrgba: %v", err)
	}

	if !imagesEqual(img, dec) {
		t.Fatal("nrgba RLE round-trip mismatch")
	}
}

func TestEncode_DefaultUncompressed(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))

	var buf bytes.Buffer
	if err := Encode(&buf, img); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 18 {
		t.Fatalf("encoded data too short: %d", len(data))
	}

	if data[2] != typeTrueColor {
		t.Fatalf("header image type = %d, want %d", data[2], typeTrueColor)
	}
}

func TestEncodeWithOptions_TrueColor24(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 40})
	img.SetNRGBA(1, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 250})

	var buf bytes.Buffer
	err := EncodeWithOptions(&buf, img, &EncodeOptions{PixelDepth: 24})
	if err != nil {
		t.Fatalf("EncodeWithOptions 24-bit: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 18 {
		t.Fatalf("encoded data too short: %d", len(data))
	}
	if data[2] != typeTrueColor {
		t.Fatalf("header type=%d, want %d", data[2], typeTrueColor)
	}
	if data[16] != 24 {
		t.Fatalf("pixel depth=%d, want 24", data[16])
	}

	dec, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode 24-bit: %v", err)
	}

	got := dec.(*image.NRGBA)
	p0 := got.NRGBAAt(0, 0)
	if p0.R != 10 || p0.G != 20 || p0.B != 30 || p0.A != 255 {
		t.Fatalf("pixel 0 mismatch: %+v", p0)
	}
	p1 := got.NRGBAAt(1, 0)
	if p1.R != 200 || p1.G != 100 || p1.B != 50 || p1.A != 255 {
		t.Fatalf("pixel 1 mismatch: %+v", p1)
	}
}

func TestEncodeWithOptions_TrueColor16(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	src := color.NRGBA{R: 123, G: 45, B: 210, A: 255}
	img.SetNRGBA(0, 0, src)

	var buf bytes.Buffer
	err := EncodeWithOptions(&buf, img, &EncodeOptions{PixelDepth: 16})
	if err != nil {
		t.Fatalf("EncodeWithOptions 16-bit: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 18 {
		t.Fatalf("encoded data too short: %d", len(data))
	}
	if data[16] != 16 {
		t.Fatalf("pixel depth=%d, want 16", data[16])
	}

	dec, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode 16-bit: %v", err)
	}

	v := encodeRGB555(src.R, src.G, src.B, src.A)
	want := decodeRGB555(v)
	got := dec.(*image.NRGBA).NRGBAAt(0, 0)
	if got != want {
		t.Fatalf("16-bit round-trip mismatch: got=%+v want=%+v", got, want)
	}
}

func TestEncode_Paletted(t *testing.T) {
	pal := color.Palette{
		color.NRGBA{R: 0, G: 0, B: 0, A: 255},
		color.NRGBA{R: 255, G: 0, B: 0, A: 255},
	}
	img := image.NewPaletted(image.Rect(0, 0, 4, 1), pal)
	img.Pix = []byte{1, 1, 0, 1}

	var buf bytes.Buffer
	if err := Encode(&buf, img); err != nil {
		t.Fatalf("Encode paletted: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 18 {
		t.Fatalf("encoded data too short: %d", len(data))
	}
	if data[2] != typePaletted {
		t.Fatalf("header type=%d, want %d", data[2], typePaletted)
	}
	if data[16] != 8 {
		t.Fatalf("pixel depth=%d, want 8", data[16])
	}
	if data[7] != 24 {
		t.Fatalf("color map depth=%d, want 24", data[7])
	}

	dec, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode paletted: %v", err)
	}

	got, ok := dec.(*image.Paletted)
	if !ok {
		t.Fatalf("expected *image.Paletted, got %T", dec)
	}
	if len(got.Pix) != len(img.Pix) {
		t.Fatalf("pix length mismatch: got=%d want=%d", len(got.Pix), len(img.Pix))
	}
	for i, v := range got.Pix {
		if v != img.Pix[i] {
			t.Fatalf("pix[%d]=%d, want %d", i, v, img.Pix[i])
		}
	}
}

func TestEncodeWithOptions_PalettedRLE(t *testing.T) {
	pal := color.Palette{
		color.NRGBA{R: 0, G: 0, B: 0, A: 255},
		color.NRGBA{R: 0, G: 255, B: 0, A: 255},
	}
	img := image.NewPaletted(image.Rect(0, 0, 8, 1), pal)
	img.Pix = []byte{1, 1, 1, 1, 0, 0, 1, 1}

	var buf bytes.Buffer
	err := EncodeWithOptions(&buf, img, &EncodeOptions{RLE: true})
	if err != nil {
		t.Fatalf("EncodeWithOptions paletted RLE: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 18 {
		t.Fatalf("encoded data too short: %d", len(data))
	}
	if data[2] != typeRLEPaletted {
		t.Fatalf("header type=%d, want %d", data[2], typeRLEPaletted)
	}

	dec, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode paletted RLE: %v", err)
	}

	got := dec.(*image.Paletted)
	if len(got.Pix) != len(img.Pix) {
		t.Fatalf("pix length mismatch: got=%d want=%d", len(got.Pix), len(img.Pix))
	}
	for i, v := range got.Pix {
		if v != img.Pix[i] {
			t.Fatalf("pix[%d]=%d, want %d", i, v, img.Pix[i])
		}
	}
}

func TestEncodeWithOptions_PalettedColorMap32(t *testing.T) {
	pal := color.Palette{
		color.NRGBA{R: 0, G: 0, B: 0, A: 255},
		color.NRGBA{R: 255, G: 0, B: 0, A: 80},
	}
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), pal)
	img.Pix[0] = 1

	var buf bytes.Buffer
	err := EncodeWithOptions(&buf, img, &EncodeOptions{ColorMapDepth: 32})
	if err != nil {
		t.Fatalf("EncodeWithOptions paletted 32 cmap: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 18 {
		t.Fatalf("encoded data too short: %d", len(data))
	}
	if data[7] != 32 {
		t.Fatalf("color map depth=%d, want 32", data[7])
	}

	dec, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode paletted 32 cmap: %v", err)
	}

	got := color.NRGBAModel.Convert(dec.At(0, 0)).(color.NRGBA)
	if got.R != 255 || got.G != 0 || got.B != 0 || got.A != 80 {
		t.Fatalf("pixel mismatch: %+v", got)
	}
}

func TestEncodeWithOptions_UnsupportedDepth(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	err := EncodeWithOptions(bytes.NewBuffer(nil), img, &EncodeOptions{PixelDepth: 12})
	if err == nil {
		t.Fatal("expected error for unsupported pixel depth")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}

	pal := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black})
	err = EncodeWithOptions(bytes.NewBuffer(nil), pal, &EncodeOptions{ColorMapDepth: 15})
	if err == nil {
		t.Fatal("expected error for unsupported color map depth")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}
