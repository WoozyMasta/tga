package tga

import (
	"bytes"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// maxFuzzPixels caps the image size the fuzzer is allowed to request
// so a header claiming huge dimensions does not OOM the fuzzing process.
// This is a test-only guard and does not change Decode's behavior.
const maxFuzzPixels = 1 << 22 // ~4M px (~16 MiB as NRGBA)

// declaredPixels parses width*height from a TGA header, or 0 if too short.
func declaredPixels(data []byte) int {
	if len(data) < headerSize {
		return 0
	}
	w := int(data[12]) | int(data[13])<<8
	h := int(data[14]) | int(data[15])<<8
	return w * h
}

// addTGASeeds seeds f with every testdata/*.tga file
// plus a couple of tiny synthetic headers,
// so the fuzzer starts from structurally valid inputs.
func addTGASeeds(f *testing.F) {
	f.Add(makeRawTGA24(2, 2, true))
	f.Add(makeRawTGA24(4, 4, false))
	f.Add(makeConformanceTrueColor(16, true, maskOriginTop|maskOriginRight))
	f.Add(makeConformanceGray(16, true, maskOriginTop))
	f.Add(makeConformancePaletted(true, maskOriginRight))

	entries, err := os.ReadDir("testdata")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".tga" {
			continue
		}
		data, err := os.ReadFile(filepath.Join("testdata", e.Name()))
		if err != nil {
			continue
		}
		f.Add(data)
	}
}

// FuzzParseHeader asserts that arbitrary fixed-size headers never panic while
// being parsed and validated.
func FuzzParseHeader(f *testing.F) {
	f.Add(makeRawTGA24(1, 1, true)[:headerSize])
	f.Fuzz(func(t *testing.T, data []byte) {
		var raw [headerSize]byte
		copy(raw[:], data)
		_, _ = parseHeader(raw)
	})
}

// FuzzPaletteIndex checks the normalization boundary for arbitrary palette
// origins and lengths without allocating a decoded image.
func FuzzPaletteIndex(f *testing.F) {
	f.Add(byte(0), uint8(0), uint8(1))
	f.Add(byte(255), uint8(255), uint8(1))
	f.Fuzz(func(t *testing.T, index byte, start, length uint8) {
		_, _ = normalizePaletteIndex(index, int(start), int(length))
	})
}

// FuzzDecodeWithMetadata exercises footer, offset, extension, and thumbnail
// parsing with the same bounded input used by the ordinary decoder fuzzer.
func FuzzDecodeWithMetadata(f *testing.F) {
	addTGASeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, _, _ = DecodeWithMetadata(bytes.NewReader(data))
	})
}

// FuzzRoundTripBounded generates small images from fuzz bytes and verifies that
// encoding followed by decoding remains bounded and panic-free.
func FuzzRoundTripBounded(f *testing.F) {
	f.Add([]byte{4, 3, 17, 29, 41, 53})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 {
			t.Skip()
		}
		w := int(data[0]%32) + 1
		h := int(data[1]%32) + 1
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for i := range img.Pix {
			img.Pix[i] = data[(i+2)%len(data)]
		}

		var buf bytes.Buffer
		if err := Encode(&buf, img); err != nil {
			t.Fatalf("Encode: %v", err)
		}
		decoded, err := Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("Decode encoded image: %v", err)
		}
		if decoded.Bounds().Dx() != w || decoded.Bounds().Dy() != h {
			t.Fatalf("decoded bounds=%v, want %dx%d", decoded.Bounds(), w, h)
		}
	})
}

// FuzzDecode asserts that Decode never panics on arbitrary input,
// and that any image it does return is self-consistent and safe to use
// (bounds, pixel access, and re-encoding must not panic either).
func FuzzDecode(f *testing.F) {
	addTGASeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		if declaredPixels(data) > maxFuzzPixels {
			return // intentional huge allocation, not a logic bug
		}

		img, err := Decode(bytes.NewReader(data))
		if err != nil {
			return // malformed input is expected to error, not panic
		}

		b := img.Bounds()
		if b.Dx() <= 0 || b.Dy() <= 0 {
			t.Fatalf("decoded image has non-positive bounds: %v", b)
		}
		// Touch the corners to surface any lazy out-of-bounds access.
		_ = img.At(b.Min.X, b.Min.Y)
		_ = img.At(b.Max.X-1, b.Max.Y-1)

		// Re-encoding the decoder's own output must not panic; an error is fine.
		_ = Encode(io.Discard, img)
	})
}

// FuzzDecodeConfig asserts that DecodeConfig never panics on arbitrary input.
func FuzzDecodeConfig(f *testing.F) {
	addTGASeeds(f)
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = DecodeConfig(bytes.NewReader(data))
	})
}
