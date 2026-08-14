package tga

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// makeRawTGA24 builds a minimal valid 24-bit uncompressed TGA (type 2) with given dimensions.
// Pixel data is BGR; fill with zeros for tests that only need valid structure.
func makeRawTGA24(width, height int, originTop bool) []byte {
	header := [18]byte{
		0, 0,
		2,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		byte(width), byte(width >> 8),
		byte(height), byte(height >> 8),
		24,
		0,
	}

	if originTop {
		header[17] = 0x20
	}

	size := len(header) + width*height*3
	out := make([]byte, size)
	copy(out, header[:])

	return out
}

func TestDecodeConfig(t *testing.T) {
	data := makeRawTGA24(10, 20, true)
	cfg, err := DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if cfg.Width != 10 || cfg.Height != 20 {
		t.Errorf("expected 10x20, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestDecodeAndConfig_HeaderValidation(t *testing.T) {
	validTrueColor := makeRawTGA24(1, 1, true)
	validRLETrueColor := append([]byte{
		0, 0, typeRLETrueColor,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		1, 0, 1, 0,
		24, maskOriginTop,
	}, 0x00, 0, 0, 0)
	validPaletted := append([]byte{
		0, 1, typePaletted,
		0, 0,
		1, 0,
		24,
		0, 0, 0, 0,
		1, 0, 1, 0,
		8, maskOriginTop,
	}, 0, 0, 0, 0)

	invalidColorMapType := append([]byte(nil), validTrueColor...)
	invalidColorMapType[1] = 2
	trueColorWithColorMap := append([]byte(nil), validTrueColor...)
	trueColorWithColorMap[1] = 1
	unsupportedDepth := append([]byte(nil), validTrueColor...)
	unsupportedDepth[16] = 12
	zeroColorMapDepth := append([]byte(nil), validPaletted...)
	zeroColorMapDepth[7] = 0
	zeroWidth := append([]byte(nil), validTrueColor...)
	zeroWidth[12], zeroWidth[13] = 0, 0
	palettedWithoutColorMap := append([]byte(nil), validTrueColor...)
	palettedWithoutColorMap[2], palettedWithoutColorMap[16] = typePaletted, 8

	for _, tt := range []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{name: "true_color", data: validTrueColor},
		{name: "rle_true_color", data: validRLETrueColor},
		{name: "paletted", data: validPaletted},
		{name: "short", data: validTrueColor[:headerSize-1], wantErr: ErrHeaderTooShort},
		{name: "invalid_color_map_type", data: invalidColorMapType, wantErr: ErrFormat},
		{name: "true_color_with_color_map", data: trueColorWithColorMap, wantErr: ErrFormat},
		{name: "unsupported_depth", data: unsupportedDepth, wantErr: ErrUnsupported},
		{name: "zero_color_map_depth", data: zeroColorMapDepth, wantErr: ErrUnsupported},
		{name: "zero_width", data: zeroWidth, wantErr: ErrFormat},
		{name: "paletted_without_color_map", data: palettedWithoutColorMap, wantErr: ErrFormat},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, configErr := DecodeConfig(bytes.NewReader(tt.data))
			_, decodeErr := Decode(bytes.NewReader(tt.data))

			if (configErr == nil) != (decodeErr == nil) {
				t.Fatalf("DecodeConfig error = %v, Decode error = %v", configErr, decodeErr)
			}
			if tt.wantErr == nil {
				return
			}
			if !errors.Is(configErr, tt.wantErr) {
				t.Fatalf("DecodeConfig error = %v, want %v", configErr, tt.wantErr)
			}
			if !errors.Is(decodeErr, tt.wantErr) {
				t.Fatalf("Decode error = %v, want %v", decodeErr, tt.wantErr)
			}
		})
	}
}

func TestDecode_Synthetic24Bit(t *testing.T) {
	// 2x2 image, top-left origin, one red pixel (0,0), rest black
	header := [18]byte{
		0, 0, 2,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		2, 0, 2, 0,
		24, 0x20,
	}

	// Row 0: BGR (0,0)=red -> 0,0,255; (1,0)=black -> 0,0,0
	// Row 1: (0,1)=black, (1,1)=black
	pixels := []byte{
		0, 0, 255, 0, 0, 0,
		0, 0, 0, 0, 0, 0,
	}

	data := append(header[:], pixels...)
	img, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("expected *image.NRGBA, got %T", img)
	}

	r := nrgba.NRGBAAt(0, 0)
	if r.R != 255 || r.G != 0 || r.B != 0 {
		t.Errorf("pixel (0,0): expected red, got R=%d G=%d B=%d", r.R, r.G, r.B)
	}
}

func TestDecode_Origins(t *testing.T) {
	want := [][2]color.NRGBA{
		{{R: 255, A: 255}, {G: 255, A: 255}},
		{{B: 255, A: 255}, {R: 255, G: 255, B: 255, A: 255}},
	}

	for _, tt := range []struct {
		name       string
		descriptor uint8
		rle        bool
	}{
		{name: "bottom_left_raw"},
		{name: "bottom_right_raw", descriptor: maskOriginRight},
		{name: "top_left_raw", descriptor: maskOriginTop},
		{name: "top_right_raw", descriptor: maskOriginTop | maskOriginRight},
		{name: "bottom_left_rle", rle: true},
		{name: "bottom_right_rle", descriptor: maskOriginRight, rle: true},
		{name: "top_left_rle", descriptor: maskOriginTop, rle: true},
		{name: "top_right_rle", descriptor: maskOriginTop | maskOriginRight, rle: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := Decode(bytes.NewReader(makeOriginTGA24(tt.descriptor, tt.rle, want)))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			got := decoded.(*image.NRGBA)
			for y := range 2 {
				for x := range 2 {
					if actual := got.NRGBAAt(x, y); actual != want[y][x] {
						t.Fatalf("pixel (%d,%d) = %+v, want %+v", x, y, actual, want[y][x])
					}
				}
			}
		})
	}
}

func TestDecode_InterleavedImageUnsupported(t *testing.T) {
	data := makeRawTGA24(1, 1, true)
	data[17] |= 0x40

	_, configErr := DecodeConfig(bytes.NewReader(data))
	if !errors.Is(configErr, ErrUnsupported) {
		t.Fatalf("DecodeConfig error = %v, want ErrUnsupported", configErr)
	}

	_, decodeErr := Decode(bytes.NewReader(data))
	if !errors.Is(decodeErr, ErrUnsupported) {
		t.Fatalf("Decode error = %v, want ErrUnsupported", decodeErr)
	}
}

func makeOriginTGA24(descriptor uint8, rle bool, pixels [][2]color.NRGBA) []byte {
	imageType := byte(typeTrueColor)
	if rle {
		imageType = typeRLETrueColor
	}
	header := []byte{
		0, 0, imageType,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		2, 0, 2, 0,
		24, descriptor,
	}

	payload := make([]byte, 0, 12)
	for inputY := range 2 {
		y := inputY
		if descriptor&maskOriginTop == 0 {
			y = 1 - inputY
		}
		for inputX := range 2 {
			x := inputX
			if descriptor&maskOriginRight != 0 {
				x = 1 - inputX
			}
			c := pixels[y][x]
			payload = append(payload, c.B, c.G, c.R)
		}
	}

	if rle {
		payload = append([]byte{0x03}, payload...)
	}

	return append(header, payload...)
}

func TestDecode_HeaderTooShort(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte{0, 0, 0}))
	if err == nil {
		t.Fatal("expected error for short header")
	}

	if !errors.Is(err, ErrHeaderTooShort) {
		t.Errorf("expected ErrHeaderTooShort, got %v", err)
	}
}

func TestDecode_UnsupportedType(t *testing.T) {
	// Type 0 (no image data)
	header := [18]byte{
		0, 0, 0,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		1, 0, 1, 0,
		24, 0x20,
	}

	_, err := Decode(bytes.NewReader(header[:]))
	if err == nil {
		t.Fatal("expected error for type 0")
	}

	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("expected ErrUnsupported, got %v", err)
	}
}

func TestDecode_InvalidDimensions(t *testing.T) {
	header := [18]byte{
		0, 0, 2,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0, // width=0, height=0
		24, 0x20,
	}

	_, err := Decode(bytes.NewReader(header[:]))
	if err == nil {
		t.Fatal("expected error for zero dimensions")
	}

	if !errors.Is(err, ErrFormat) {
		t.Errorf("expected ErrFormat, got %v", err)
	}
}

func TestDecode_RLEOverrun(t *testing.T) {
	// Minimal header: 1x1, type 10 (RLE true color), 24 bpp
	header := [18]byte{
		0, 0, 10,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		1, 0, 1, 0,
		24, 0x20,
	}

	// One RLE packet: raw, count=2 (header 0x01, then 6 bytes). We only have 1 pixel expected -> overrun
	payload := []byte{0x01, 0, 0, 0, 0, 0, 0}
	data := append(header[:], payload...)
	_, err := Decode(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for RLE overrun")
	}

	if !errors.Is(err, ErrRLEOverrun) {
		t.Errorf("expected ErrRLEOverrun, got %v", err)
	}
}

func TestDecode_RLETrueColor24(t *testing.T) {
	header := [18]byte{
		0, 0, 10,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		3, 0, 1, 0,
		24, 0x20,
	}

	// Packet 1: RLE run, 2 red pixels (BGR 0,0,255)
	// Packet 2: raw, 1 blue pixel (BGR 255,0,0)
	payload := []byte{
		0x81, 0, 0, 255,
		0x00, 255, 0, 0,
	}

	data := append(header[:], payload...)
	img, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode RLE true color: %v", err)
	}

	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("expected *image.NRGBA, got %T", img)
	}

	p0 := nrgba.NRGBAAt(0, 0)
	p1 := nrgba.NRGBAAt(1, 0)
	p2 := nrgba.NRGBAAt(2, 0)
	if p0 != (color.NRGBA{R: 255, G: 0, B: 0, A: 255}) {
		t.Fatalf("pixel 0 mismatch: %+v", p0)
	}
	if p1 != (color.NRGBA{R: 255, G: 0, B: 0, A: 255}) {
		t.Fatalf("pixel 1 mismatch: %+v", p1)
	}
	if p2 != (color.NRGBA{R: 0, G: 0, B: 255, A: 255}) {
		t.Fatalf("pixel 2 mismatch: %+v", p2)
	}
}

func TestDecode_RLEGrayscale8(t *testing.T) {
	header := [18]byte{
		0, 0, 11,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		4, 0, 1, 0,
		8, 0x20,
	}

	// Packet 1: RLE run of 3 pixels with value 42.
	// Packet 2: raw packet with 1 pixel value 200.
	payload := []byte{
		0x82, 42,
		0x00, 200,
	}

	data := append(header[:], payload...)
	img, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode RLE grayscale: %v", err)
	}

	gray, ok := img.(*image.Gray)
	if !ok {
		t.Fatalf("expected *image.Gray, got %T", img)
	}

	if got := gray.GrayAt(0, 0).Y; got != 42 {
		t.Fatalf("pixel 0 mismatch: %d", got)
	}
	if got := gray.GrayAt(1, 0).Y; got != 42 {
		t.Fatalf("pixel 1 mismatch: %d", got)
	}
	if got := gray.GrayAt(2, 0).Y; got != 42 {
		t.Fatalf("pixel 2 mismatch: %d", got)
	}
	if got := gray.GrayAt(3, 0).Y; got != 200 {
		t.Fatalf("pixel 3 mismatch: %d", got)
	}
}

func TestDecode_RLEPaletted8(t *testing.T) {
	header := [18]byte{
		0, 1, 9,
		0, 0,
		2, 0,
		24,
		0, 0, 0, 0,
		2, 0, 2, 0,
		8, 0x20,
	}

	// Palette (24-bit BGR): idx 0 = black, idx 1 = red.
	palette := []byte{
		0, 0, 0,
		0, 0, 255,
	}

	// One RLE run packet: 4 pixels of index 1.
	payload := []byte{0x83, 0x01}

	data := append(header[:], palette...)
	data = append(data, payload...)
	img, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode RLE paletted: %v", err)
	}

	paletted, ok := img.(*image.Paletted)
	if !ok {
		t.Fatalf("expected *image.Paletted, got %T", img)
	}

	for i, v := range paletted.Pix {
		if v != 1 {
			t.Fatalf("pixel index %d mismatch: %d", i, v)
		}
	}
}

func TestDecode_PaletteIndices(t *testing.T) {
	palette := []byte{
		0, 0, 255,
		0, 255, 0,
	}

	for _, tt := range []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{name: "non_zero_first_raw", data: makePalettedTGA(2, 2, 24, palette, []byte{2, 3}, false)},
		{name: "non_zero_first_rle", data: makePalettedTGA(2, 2, 24, palette, []byte{2, 3}, true)},
		{name: "below_first_raw", data: makePalettedTGA(2, 1, 24, palette[:3], []byte{1}, false), wantErr: ErrPaletteIndex},
		{name: "below_first_rle", data: makePalettedTGA(2, 1, 24, palette[:3], []byte{1}, true), wantErr: ErrPaletteIndex},
		{name: "at_or_above_end_raw", data: makePalettedTGA(2, 1, 24, palette[:3], []byte{3}, false), wantErr: ErrPaletteIndex},
		{name: "at_or_above_end_rle", data: makePalettedTGA(2, 1, 24, palette[:3], []byte{3}, true), wantErr: ErrPaletteIndex},
		{name: "unsupported_entry_depth", data: makePalettedTGA(0, 1, 8, []byte{0}, []byte{0}, false), wantErr: ErrUnsupported},
	} {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := Decode(bytes.NewReader(tt.data))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Decode error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			paletted, ok := decoded.(*image.Paletted)
			if !ok {
				t.Fatalf("decoded image = %T, want *image.Paletted", decoded)
			}
			if len(paletted.Palette) != 2 {
				t.Fatalf("palette length = %d, want 2", len(paletted.Palette))
			}
			if got := paletted.Pix; !bytes.Equal(got, []byte{0, 1}) {
				t.Fatalf("normalized palette indices = %v, want [0 1]", got)
			}
			if got := color.NRGBAModel.Convert(paletted.At(0, 0)); got != (color.NRGBA{R: 255, A: 255}) {
				t.Fatalf("palette color 0 = %+v, want red", got)
			}
			if got := color.NRGBAModel.Convert(paletted.At(1, 0)); got != (color.NRGBA{G: 255, A: 255}) {
				t.Fatalf("palette color 1 = %+v, want green", got)
			}
		})
	}
}

func TestDecodeConfig_UnsupportedPaletteDepth(t *testing.T) {
	data := makePalettedTGA(0, 1, 8, []byte{0}, []byte{0}, false)

	_, err := DecodeConfig(bytes.NewReader(data))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("DecodeConfig error = %v, want ErrUnsupported", err)
	}
}

func makePalettedTGA(first, length int, depth byte, palette, indices []byte, rle bool) []byte {
	imageType := byte(typePaletted)
	if rle {
		imageType = typeRLEPaletted
	}
	header := []byte{
		0, 1, imageType,
		byte(first), byte(first >> 8),
		byte(length), byte(length >> 8),
		depth,
		0, 0, 0, 0,
		byte(len(indices)), byte(len(indices) >> 8),
		1, 0,
		8, maskOriginTop,
	}

	payload := append([]byte(nil), indices...)
	if rle {
		payload = append([]byte{byte(len(indices) - 1)}, payload...)
	}

	data := append(header, palette...)
	return append(data, payload...)
}

func TestDecode_UnsupportedDepthForRLEType(t *testing.T) {
	header := [18]byte{
		0, 0, 10,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		1, 0, 1, 0,
		12, 0x20,
	}

	_, err := Decode(bytes.NewReader(header[:]))
	if err == nil {
		t.Fatal("expected error for unsupported true-color depth")
	}

	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestDecode_PalettedWithoutColorMap(t *testing.T) {
	header := [18]byte{
		0, 0, 1,
		0, 0,
		0, 0,
		0,
		0, 0, 0, 0,
		1, 0, 1, 0,
		8, 0x20,
	}

	_, err := Decode(bytes.NewReader(header[:]))
	if err == nil {
		t.Fatal("expected error for paletted image without color map")
	}

	if !errors.Is(err, ErrFormat) {
		t.Fatalf("expected ErrFormat, got %v", err)
	}
}

func TestDecodeRGB555(t *testing.T) {
	tests := []struct {
		v    uint16
		want color.NRGBA
	}{
		{0, color.NRGBA{0, 0, 0, 0xff}},
		{0x7fff, color.NRGBA{0xff, 0xff, 0xff, 0xff}}, // R=31, G=31, B=31
		{0x7c00, color.NRGBA{0xff, 0, 0, 0xff}},       // R=31
		{0x03e0, color.NRGBA{0, 0xff, 0, 0xff}},       // G=31
		{0x001f, color.NRGBA{0, 0, 0xff, 0xff}},       // B=31
	}

	for _, tt := range tests {
		got := decodeRGB555(tt.v)
		if got != tt.want {
			t.Errorf("decodeRGB555(0x%04x) = %+v, want %+v", tt.v, got, tt.want)
		}
	}
}

func TestDecode_TrueColor16AlphaRoundTrip(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 64, B: 32, A: 0})
	src.SetNRGBA(1, 0, color.NRGBA{R: 32, G: 128, B: 255, A: 255})
	src.SetNRGBA(0, 1, color.NRGBA{R: 96, G: 192, B: 16, A: 0})
	src.SetNRGBA(1, 1, color.NRGBA{R: 224, G: 16, B: 160, A: 255})

	for _, tt := range []struct {
		name         string
		rle          bool
		originBottom bool
	}{
		{name: "raw_top"},
		{name: "raw_bottom", originBottom: true},
		{name: "rle_top", rle: true},
		{name: "rle_bottom", rle: true, originBottom: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := EncodeWithOptions(&buf, src, &EncodeOptions{
				PixelDepth:   16,
				RLE:          tt.rle,
				OriginBottom: tt.originBottom,
			})
			if err != nil {
				t.Fatalf("EncodeWithOptions: %v", err)
			}

			data := buf.Bytes()
			if data[17]&0x0f != 1 {
				t.Fatalf("descriptor alpha bits=%d, want 1", data[17]&0x0f)
			}

			decoded, err := Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			got := decoded.(*image.NRGBA)
			for y := range 2 {
				for x := range 2 {
					wantSrc := src.NRGBAAt(x, y)
					want := decode16BitTrueColor(
						encodeRGB555(wantSrc.R, wantSrc.G, wantSrc.B, wantSrc.A),
						true,
					)
					if actual := got.NRGBAAt(x, y); actual != want {
						t.Fatalf("pixel (%d,%d) = %+v, want %+v", x, y, actual, want)
					}
				}
			}
		})
	}
}

func TestDecode_TrueColor15IgnoresHighBitForAlpha(t *testing.T) {
	data := [20]byte{
		0, 0, typeTrueColor,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		1, 0, 1, 0,
		15, maskOriginTop,
		0x1f, 0x80,
	}

	decoded, err := Decode(bytes.NewReader(data[:]))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := decoded.(*image.NRGBA).NRGBAAt(0, 0); got != (color.NRGBA{B: 255, A: 255}) {
		t.Fatalf("pixel = %+v, want opaque blue", got)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// Create small NRGBA, encode to buffer, decode, compare bounds and a pixel
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 128, B: 64, A: 255})

	var buf bytes.Buffer
	if err := Encode(&buf, img); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	dec, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode after Encode: %v", err)
	}

	if !dec.Bounds().Eq(img.Bounds()) {
		t.Errorf("bounds: got %v, want %v", dec.Bounds(), img.Bounds())
	}

	// Compare center pixel
	r0 := img.NRGBAAt(1, 1)
	r1 := dec.(*image.NRGBA).NRGBAAt(1, 1)
	if r0 != r1 {
		t.Errorf("pixel (1,1): got %+v, want %+v", r1, r0)
	}
}

func TestEncodeGrayRoundTrip(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 4, 4))
	img.SetGray(2, 2, color.Gray{Y: 200})

	var buf bytes.Buffer
	if err := Encode(&buf, img); err != nil {
		t.Fatalf("Encode Gray: %v", err)
	}

	dec, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	g, ok := dec.(*image.Gray)
	if !ok {
		t.Fatalf("expected *image.Gray, got %T", dec)
	}
	if g.GrayAt(2, 2).Y != 200 {
		t.Errorf("pixel (2,2): got %d", g.GrayAt(2, 2).Y)
	}
}

// testdataTGA is the list of TGA files for round-trip tests.
// Regenerate with: go run ./testdata/gen
var testdataTGA = []string{
	"testdata/bw_32x32_8.tga",
	"testdata/color_32x32_24.tga",
	"testdata/color_32x32_32.tga",
	"testdata/bw_4096x16_8.tga",
	"testdata/color_4096x16_24.tga",
	"testdata/sample_truecolor_16.tga",
	"testdata/sample_truecolor_24.tga",
	"testdata/sample_truecolor_rle24.tga",
	"testdata/sample_paletted.tga",
	"testdata/sample_paletted_rle.tga",
	"testdata/sample_metadata.tga",
}

// imagesEqual reports whether two images have the same bounds and pixel values.
func imagesEqual(a, b image.Image) bool {
	ba := a.Bounds()
	bb := b.Bounds()
	if !ba.Eq(bb) {
		return false
	}
	for y := ba.Min.Y; y < ba.Max.Y; y++ {
		for x := ba.Min.X; x < ba.Max.X; x++ {
			ca, cb := a.At(x, y), b.At(x, y)
			ra, ga, ba_, aa := ca.RGBA()
			rb, gb, bb_, ab := cb.RGBA()
			if ra != rb || ga != gb || ba_ != bb_ || aa != ab {
				return false
			}
		}
	}
	return true
}

// TestRoundTrip_Testdata decodes each testdata TGA, re-encodes it, decodes again, and compares.
// Direction: file -> decode -> encode -> decode -> compare to first decode.
// Run after generating testdata: go run ./testdata/gen
func TestRoundTrip_Testdata(t *testing.T) {
	for _, path := range testdataTGA {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("testdata missing: %v (run: go run ./testdata/gen)", err)
			return
		}

		img1, err := Decode(bytes.NewReader(data))
		if err != nil {
			t.Errorf("%s: first decode: %v", path, err)
			continue
		}

		var buf bytes.Buffer
		if err := Encode(&buf, img1); err != nil {
			t.Errorf("%s: encode: %v", path, err)
			continue
		}

		img2, err := Decode(&buf)
		if err != nil {
			t.Errorf("%s: second decode: %v", path, err)
			continue
		}

		if !imagesEqual(img1, img2) {
			t.Errorf("%s: round-trip pixel mismatch", path)
		}
	}
}

// TestRoundTrip_Testdata_EncodeFirst builds an image, encodes to TGA, decodes, and compares.
// Direction: memory -> encode -> decode -> compare to original.
func TestRoundTrip_Testdata_EncodeFirst(t *testing.T) {
	dir := "testdata"
	// Use same list; we only need at least one file to exist to know testdata is there
	first := filepath.Join(dir, "bw_32x32_8.tga")
	if _, err := os.Stat(first); err != nil {
		t.Skipf("testdata missing (run: go run ./testdata/gen): %v", err)
		return
	}

	// Gray: encode -> decode -> compare
	gray := image.NewGray(image.Rect(0, 0, 32, 32))
	for i := 0; i < 32*32; i++ {
		gray.Pix[i] = byte(i % 256)
	}
	var buf bytes.Buffer
	if err := Encode(&buf, gray); err != nil {
		t.Fatalf("encode gray: %v", err)
	}
	dec, err := Decode(&buf)
	if err != nil {
		t.Fatalf("decode after encode gray: %v", err)
	}
	if !imagesEqual(gray, dec) {
		t.Error("gray round-trip (encode first): pixel mismatch")
	}

	// NRGBA with alpha: encode -> decode -> compare
	nrgba := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			nrgba.SetNRGBA(x, y, color.NRGBA{
				R: byte(x * 8), G: byte(y * 8), B: byte((x + y) * 4), A: byte(128 + (x+y)%128),
			})
		}
	}

	buf.Reset()
	if err := Encode(&buf, nrgba); err != nil {
		t.Fatalf("encode nrgba: %v", err)
	}

	dec, err = Decode(&buf)
	if err != nil {
		t.Fatalf("decode after encode nrgba: %v", err)
	}
	if !imagesEqual(nrgba, dec) {
		t.Error("nrgba round-trip (encode first): pixel mismatch")
	}
}
