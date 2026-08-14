// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"testing"
)

type writeCallCounter struct {
	bytes.Buffer
	calls int
}

type genericTestImage struct {
	image.Image
}

func (w *writeCallCounter) Write(p []byte) (int, error) {
	w.calls++
	return w.Buffer.Write(p)
}

func TestEncode_RLEPacketsUseSingleWritePerPacket(t *testing.T) {
	var oneByte writeCallCounter
	if err := encodeRLEPackets1WithScratch(&oneByte, []byte{7, 7, 1, 2, 3}, make([]byte, 129)); err != nil {
		t.Fatalf("encode one-byte packets: %v", err)
	}
	if oneByte.calls != 2 {
		t.Fatalf("one-byte packet writes=%d, want=2", oneByte.calls)
	}
	wantOneByte := []byte{0x81, 7, 0x02, 1, 2, 3}
	if !bytes.Equal(oneByte.Bytes(), wantOneByte) {
		t.Fatalf("one-byte packets=%x, want=%x", oneByte.Bytes(), wantOneByte)
	}

	packed := []byte{
		1, 2, 3, 1, 2, 3,
		4, 5, 6,
		7, 8, 9,
	}
	var multiByte writeCallCounter
	if err := encodeRLEPackets(&multiByte, packed, 3, make([]byte, 385)); err != nil {
		t.Fatalf("encode multi-byte packets: %v", err)
	}
	if multiByte.calls != 2 {
		t.Fatalf("multi-byte packet writes=%d, want=2", multiByte.calls)
	}
}

func TestWritePaletteUsesSingleWrite(t *testing.T) {
	var counter writeCallCounter
	pal := color.Palette{color.Black, color.White}
	if err := writePalette(&counter, pal, 32); err != nil {
		t.Fatalf("write palette: %v", err)
	}
	if counter.calls != 1 {
		t.Fatalf("palette writes=%d, want=1", counter.calls)
	}
}

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
	rgba := image.NewRGBA(image.Rect(0, 0, 4, 4))
	rgba.Set(1, 1, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	rgba.SetRGBA(2, 2, color.RGBA{R: 100, G: 50, B: 25, A: 128})

	var buf bytes.Buffer
	if err := Encode(&buf, rgba); err != nil {
		t.Fatalf("Encode RGBA: %v", err)
	}
	dec, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	// Decoded as NRGBA; compare straight-alpha values at opaque and alpha pixels.
	for _, point := range []image.Point{{X: 1, Y: 1}, {X: 2, Y: 2}} {
		want := color.NRGBAModel.Convert(rgba.At(point.X, point.Y)).(color.NRGBA)
		got := dec.(*image.NRGBA).NRGBAAt(point.X, point.Y)
		if got != want {
			t.Errorf("pixel %v: got %v, want %v", point, got, want)
		}
	}
}

func TestEncode_GenericImageRows(t *testing.T) {
	src := image.NewNRGBA(image.Rect(5, 7, 9, 10))
	for y := 7; y < 10; y++ {
		for x := 5; x < 9; x++ {
			src.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 11), G: uint8(y * 13), B: uint8(x + y), A: uint8(80 + x),
			})
		}
	}

	for _, originBottom := range []bool{false, true} {
		var buf bytes.Buffer
		opts := &EncodeOptions{OriginBottom: originBottom, PixelDepth: 32, RLE: true}
		if err := EncodeWithOptions(&buf, genericTestImage{Image: src}, opts); err != nil {
			t.Fatalf("Encode generic image, bottom=%t: %v", originBottom, err)
		}
		decoded, err := Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("Decode generic image, bottom=%t: %v", originBottom, err)
		}
		for y := 0; y < src.Bounds().Dy(); y++ {
			for x := 0; x < src.Bounds().Dx(); x++ {
				want := src.At(x+src.Bounds().Min.X, y+src.Bounds().Min.Y)
				got := decoded.At(x, y)
				wr, wg, wb, wa := want.RGBA()
				gr, gg, gb, ga := got.RGBA()
				if wr != gr || wg != gg || wb != gb || wa != ga {
					t.Fatalf("pixel (%d,%d), bottom=%t: got %v, want %v", x, y, originBottom, got, want)
				}
			}
		}
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
	if data[17]&0x0f != 8 {
		t.Fatalf("descriptor alpha bits=%d, want 8", data[17]&0x0f)
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
	if data[17]&0x0f != 0 {
		t.Fatalf("descriptor alpha bits=%d, want 0", data[17]&0x0f)
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
	if data[17]&0x0f != 1 {
		t.Fatalf("descriptor alpha bits=%d, want 1", data[17]&0x0f)
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

	err = EncodeWithOptions(bytes.NewBuffer(nil), img, &EncodeOptions{
		Metadata: &TGA2Metadata{
			AttributesType: 9,
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid alpha attribute type")
	}
	if !errors.Is(err, ErrFormat) {
		t.Fatalf("expected ErrFormat, got %v", err)
	}
}

func TestEncodeWithOptions_IgnoresIrrelevantDepthOptions(t *testing.T) {
	tests := []struct {
		name string
		img  image.Image
		opts EncodeOptions
	}{
		{
			name: "gray ignores pixel and color map depth",
			img:  image.NewGray(image.Rect(0, 0, 1, 1)),
			opts: EncodeOptions{PixelDepth: 12, ColorMapDepth: 15},
		},
		{
			name: "paletted ignores pixel depth",
			img:  image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black}),
			opts: EncodeOptions{PixelDepth: 12, ColorMapDepth: 24},
		},
		{
			name: "true color ignores color map depth",
			img:  image.NewNRGBA(image.Rect(0, 0, 1, 1)),
			opts: EncodeOptions{PixelDepth: 24, ColorMapDepth: 15},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := EncodeWithOptions(&buf, test.img, &test.opts); err != nil {
				t.Fatalf("EncodeWithOptions: %v", err)
			}
			if _, err := Decode(bytes.NewReader(buf.Bytes())); err != nil {
				t.Fatalf("Decode: %v", err)
			}
		})
	}
}

func TestEncodeWithOptions_ImageID(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	imageID := []byte("example-id")

	var buf bytes.Buffer
	err := EncodeWithOptions(&buf, img, &EncodeOptions{ImageID: imageID})
	if err != nil {
		t.Fatalf("EncodeWithOptions ImageID: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 18+len(imageID) {
		t.Fatalf("encoded data too short: %d", len(data))
	}
	if int(data[0]) != len(imageID) {
		t.Fatalf("header id length=%d, want %d", data[0], len(imageID))
	}
	if !bytes.Equal(data[18:18+len(imageID)], imageID) {
		t.Fatalf("image id bytes mismatch")
	}

	dec, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode with ImageID: %v", err)
	}
	if !imagesEqual(img, dec) {
		t.Fatal("round-trip mismatch with image id")
	}
}

func TestEncodeWithOptions_OriginBottom(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})

	var buf bytes.Buffer
	err := EncodeWithOptions(&buf, img, &EncodeOptions{OriginBottom: true})
	if err != nil {
		t.Fatalf("EncodeWithOptions OriginBottom: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 18 {
		t.Fatalf("encoded data too short: %d", len(data))
	}
	if data[17]&maskOriginTop != 0 {
		t.Fatalf("expected bottom-left origin bit clear, descriptor=0x%02x", data[17])
	}

	dec, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode with OriginBottom: %v", err)
	}
	if !imagesEqual(img, dec) {
		t.Fatal("round-trip mismatch with bottom origin")
	}
}

func TestEncodeWithOptions_MetadataAttributeType(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 255})

	var buf bytes.Buffer
	err := EncodeWithOptions(&buf, img, &EncodeOptions{
		Metadata: &TGA2Metadata{
			AttributesType: 3,
		},
	})
	if err != nil {
		t.Fatalf("EncodeWithOptions Metadata AttributesType: %v", err)
	}

	data := buf.Bytes()
	if len(data) < tga2FooterSize {
		t.Fatalf("encoded data too short: %d", len(data))
	}
	footer := data[len(data)-tga2FooterSize:]
	if string(footer[8:26]) != tga2FooterSignature {
		t.Fatal("expected TGA 2.0 footer signature")
	}

	extOffset := int(binary.LittleEndian.Uint32(footer[0:4]))
	if extOffset <= 0 || extOffset+tga2ExtensionSize > len(data) {
		t.Fatal("invalid extension offset")
	}
	ext := data[extOffset : extOffset+tga2ExtensionSize]
	if ext[tga2OffAttrType] != 3 {
		t.Fatalf("extension attribute type=%d, want 3", ext[tga2OffAttrType])
	}
}

func TestEncodeWithOptions_DerivesAlphaMetadataFromOutput(t *testing.T) {
	tests := []struct {
		name          string
		img           image.Image
		opts          EncodeOptions
		wantAttr      byte
		wantAlphaBits byte
	}{
		{name: "nrgba 32-bit", img: image.NewNRGBA(image.Rect(0, 0, 1, 1)), opts: EncodeOptions{Metadata: &TGA2Metadata{}}, wantAttr: 3, wantAlphaBits: 8},
		{name: "nrgba 16-bit", img: image.NewNRGBA(image.Rect(0, 0, 1, 1)), opts: EncodeOptions{PixelDepth: 16, Metadata: &TGA2Metadata{}}, wantAttr: 3, wantAlphaBits: 1},
		{name: "nrgba 24-bit", img: image.NewNRGBA(image.Rect(0, 0, 1, 1)), opts: EncodeOptions{PixelDepth: 24, Metadata: &TGA2Metadata{}}, wantAttr: 0, wantAlphaBits: 0},
		{name: "rgba generic", img: image.NewRGBA(image.Rect(0, 0, 1, 1)), opts: EncodeOptions{Metadata: &TGA2Metadata{}}, wantAttr: 3, wantAlphaBits: 8},
		{name: "paletted 32-bit", img: image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black}), opts: EncodeOptions{ColorMapDepth: 32, Metadata: &TGA2Metadata{}}, wantAttr: 3, wantAlphaBits: 8},
		{name: "paletted 24-bit", img: image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black}), opts: EncodeOptions{ColorMapDepth: 24, Metadata: &TGA2Metadata{}}, wantAttr: 0, wantAlphaBits: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := EncodeWithOptions(&buf, test.img, &test.opts); err != nil {
				t.Fatalf("EncodeWithOptions: %v", err)
			}
			data := buf.Bytes()
			footer := data[len(data)-tga2FooterSize:]
			extOffset := binary.LittleEndian.Uint32(footer[:4])
			ext := data[extOffset:]
			if got := ext[tga2OffAttrType]; got != test.wantAttr {
				t.Fatalf("attributes type=%d, want=%d", got, test.wantAttr)
			}
			if got := data[17] & 0x0f; got != test.wantAlphaBits {
				t.Fatalf("descriptor alpha bits=%d, want=%d", got, test.wantAlphaBits)
			}
		})
	}
}

func TestEncodeWithOptions_RejectsContradictoryAlphaMetadata(t *testing.T) {
	tests := []struct {
		name string
		img  image.Image
		opts EncodeOptions
	}{
		{name: "premultiplied alpha output", img: image.NewNRGBA(image.Rect(0, 0, 1, 1)), opts: EncodeOptions{Metadata: &TGA2Metadata{AttributesType: 4}}},
		{name: "straight alpha without channel", img: image.NewNRGBA(image.Rect(0, 0, 1, 1)), opts: EncodeOptions{PixelDepth: 24, Metadata: &TGA2Metadata{AttributesType: 3}}},
		{name: "straight alpha grayscale", img: image.NewGray(image.Rect(0, 0, 1, 1)), opts: EncodeOptions{Metadata: &TGA2Metadata{AttributesType: 3}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := EncodeWithOptions(&buf, test.img, &test.opts)
			if !errors.Is(err, ErrMetadata) || !errors.Is(err, ErrFormat) {
				t.Fatalf("error=%v, want ErrMetadata and ErrFormat", err)
			}
			if buf.Len() != 0 {
				t.Fatalf("validation wrote %d bytes", buf.Len())
			}
		})
	}
}

func TestEncodeWithOptions_ImageIDTooLong(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	longID := make([]byte, 256)
	err := EncodeWithOptions(bytes.NewBuffer(nil), img, &EncodeOptions{ImageID: longID})
	if err == nil {
		t.Fatal("expected error for image ID > 255 bytes")
	}
	if !errors.Is(err, ErrFormat) {
		t.Fatalf("expected ErrFormat, got %v", err)
	}
}
