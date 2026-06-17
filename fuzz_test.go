package tga

import (
	"bytes"
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
