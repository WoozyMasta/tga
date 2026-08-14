package tga

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"testing"
)

// goldenImageSpec describes expected properties for one golden sample file.
type goldenImageSpec struct {
	path        string
	wantType    byte
	wantDepth   byte
	wantCMap    byte
	expectTGA2  bool
	expectedImg image.Image
}

// makeGoldenPatternNRGBA creates deterministic pattern used by golden files.
func makeGoldenPatternNRGBA(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: byte((x * 17) & 0xff),
				G: byte((y * 29) & 0xff),
				B: byte(((x + y) * 11) & 0xff),
				A: byte(100 + ((x*3 + y*5) & 0x7f)),
			})
		}
	}

	return img
}

// makeGoldenRLEFriendlyNRGBA creates pattern with long runs.
func makeGoldenRLEFriendlyNRGBA(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		var c color.NRGBA
		if y%2 == 0 {
			c = color.NRGBA{R: 220, G: 30, B: 10, A: 255}
		} else {
			c = color.NRGBA{R: 30, G: 140, B: 220, A: 255}
		}

		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}

	return img
}

// makeGoldenPaletted creates deterministic paletted image.
func makeGoldenPaletted(w, h int) *image.Paletted {
	pal := color.Palette{
		color.NRGBA{R: 0, G: 0, B: 0, A: 255},
		color.NRGBA{R: 220, G: 30, B: 10, A: 255},
		color.NRGBA{R: 30, G: 140, B: 220, A: 255},
		color.NRGBA{R: 250, G: 250, B: 60, A: 255},
	}
	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := uint8((x/4 + y/4) % len(pal))
			img.SetColorIndex(x, y, idx)
		}
	}

	return img
}

// quantizeTo16Bit applies A1R5G5B5 quantization used by the 16-bit fixture.
func quantizeTo16Bit(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := src.NRGBAAt(x, y)
			v := encodeRGB555(c.R, c.G, c.B, c.A)
			q := decode16BitTrueColor(v, true)
			dst.SetNRGBA(x-b.Min.X, y-b.Min.Y, q)
		}
	}

	return dst
}

// forceOpaque copies image and sets alpha channel to 0xff.
func forceOpaque(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := src.NRGBAAt(x, y)
			c.A = 0xff
			dst.SetNRGBA(x-b.Min.X, y-b.Min.Y, c)
		}
	}

	return dst
}

// loadHeader reads and returns TGA header bytes.
func loadHeader(path string) ([18]byte, []byte, error) {
	var header [18]byte
	data, err := os.ReadFile(path)
	if err != nil {
		return header, nil, err
	}
	if len(data) < 18 {
		return header, nil, ErrHeaderTooShort
	}
	copy(header[:], data[:18])
	return header, data, nil
}

func TestGolden_TestdataSamples(t *testing.T) {
	basePattern := makeGoldenPatternNRGBA(16, 16)
	cases := []goldenImageSpec{
		{
			path:        "testdata/sample_truecolor_16.tga",
			wantType:    typeTrueColor,
			wantDepth:   16,
			wantCMap:    0,
			expectTGA2:  false,
			expectedImg: quantizeTo16Bit(basePattern),
		},
		{
			path:        "testdata/sample_truecolor_24.tga",
			wantType:    typeTrueColor,
			wantDepth:   24,
			wantCMap:    0,
			expectTGA2:  false,
			expectedImg: forceOpaque(basePattern),
		},
		{
			path:        "testdata/sample_truecolor_rle24.tga",
			wantType:    typeRLETrueColor,
			wantDepth:   24,
			wantCMap:    0,
			expectTGA2:  false,
			expectedImg: makeGoldenRLEFriendlyNRGBA(16, 16),
		},
		{
			path:        "testdata/sample_paletted.tga",
			wantType:    typePaletted,
			wantDepth:   8,
			wantCMap:    24,
			expectTGA2:  false,
			expectedImg: makeGoldenPaletted(16, 16),
		},
		{
			path:        "testdata/sample_paletted_rle.tga",
			wantType:    typeRLEPaletted,
			wantDepth:   8,
			wantCMap:    24,
			expectTGA2:  false,
			expectedImg: makeGoldenPaletted(16, 16),
		},
		{
			path:        "testdata/sample_metadata.tga",
			wantType:    typeRLETrueColor,
			wantDepth:   32,
			wantCMap:    0,
			expectTGA2:  true,
			expectedImg: makeGoldenRLEFriendlyNRGBA(16, 16),
		},
	}

	for _, tt := range cases {
		header, raw, err := loadHeader(tt.path)
		if err != nil {
			t.Fatalf("%s: load header: %v", tt.path, err)
		}

		if header[2] != tt.wantType {
			t.Fatalf("%s: type=%d, want=%d", tt.path, header[2], tt.wantType)
		}
		if header[16] != tt.wantDepth {
			t.Fatalf("%s: depth=%d, want=%d", tt.path, header[16], tt.wantDepth)
		}
		if header[7] != tt.wantCMap {
			t.Fatalf("%s: color-map-depth=%d, want=%d", tt.path, header[7], tt.wantCMap)
		}

		decoded, err := Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("%s: decode: %v", tt.path, err)
		}
		if !imagesEqual(tt.expectedImg, decoded) {
			t.Fatalf("%s: decoded pixels do not match golden expectation", tt.path)
		}

		hasFooter := false
		if len(raw) >= tga2FooterSize {
			footer := raw[len(raw)-tga2FooterSize:]
			if string(footer[8:26]) == tga2FooterSignature {
				hasFooter = true
				extOff := binary.LittleEndian.Uint32(footer[0:4])
				if tt.expectTGA2 && extOff == 0 {
					t.Fatalf("%s: extension offset is zero in TGA2 footer", tt.path)
				}
			}
		}
		if hasFooter != tt.expectTGA2 {
			t.Fatalf("%s: hasTGA2Footer=%v, want=%v", tt.path, hasFooter, tt.expectTGA2)
		}
	}
}
