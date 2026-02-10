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

// testdataTGA is the list of TGA files for round-trip tests (run: go run ./testdata/gen or ./testdata).
var testdataTGA = []string{
	"testdata/bw_32x32_8.tga",
	"testdata/color_32x32_24.tga",
	"testdata/color_32x32_32.tga",
	"testdata/bw_4096x16_8.tga",
	"testdata/color_4096x16_24.tga",
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
// Run after generating testdata: go run ./testdata/gen or go run ./testdata
func TestRoundTrip_Testdata(t *testing.T) {
	for _, path := range testdataTGA {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("testdata missing: %v (run: go run ./testdata)", err)
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
		t.Skipf("testdata missing (run: go run ./testdata): %v", err)
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
