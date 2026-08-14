package tga

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"testing"
)

// benchPalette is the small fixed palette shared by all benchmark images.
var benchPalette = color.Palette{
	color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	color.NRGBA{R: 220, G: 30, B: 10, A: 255},
	color.NRGBA{R: 30, G: 140, B: 220, A: 255},
	color.NRGBA{R: 250, G: 250, B: 60, A: 255},
}

// patternIndex maps (x,y) to a palette index in ~16px horizontal runs,
// so RLE benchmarks compress realistically instead of degenerating to all-same data.
func patternIndex(x, y int) int { return (x/16 + y/16) % len(benchPalette) }

func buildNRGBA(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, benchPalette[patternIndex(x, y)].(color.NRGBA))
		}
	}
	return img
}

type genericBenchImage struct {
	image.Image
}

func buildGeneric(w, h int) image.Image {
	return genericBenchImage{Image: buildNRGBA(w, h)}
}

func buildGray(w, h int) image.Image {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8(patternIndex(x, y) * 80)})
		}
	}
	return img
}

func buildPaletted(w, h int) image.Image {
	img := image.NewPaletted(image.Rect(0, 0, w, h), benchPalette)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetColorIndex(x, y, uint8(patternIndex(x, y)))
		}
	}
	return img
}

var benchSizes = []struct {
	name string
	w, h int
}{
	{"64x64", 64, 64},
	{"1920x1080", 1920, 1080},
}

var benchKinds = []struct {
	name  string
	depth int // true-color PixelDepth; 0 (default) for gray/paletted
	build func(w, h int) image.Image
}{
	{"Gray8", 0, buildGray},
	{"Paletted8", 0, buildPaletted},
	{"TrueColor16", 16, buildNRGBA},
	{"TrueColor24", 24, buildNRGBA},
	{"TrueColor32", 32, buildNRGBA},
}

func rleLabel(rle bool) string {
	if rle {
		return "RLE"
	}
	return "Raw"
}

// BenchmarkDecode covers Decode over {size} x {kind/depth} x {RLE on/off}.
func BenchmarkDecode(b *testing.B) {
	for _, sz := range benchSizes {
		for _, k := range benchKinds {
			img := k.build(sz.w, sz.h)
			for _, rle := range []bool{false, true} {
				var buf bytes.Buffer
				if err := EncodeWithOptions(&buf, img, &EncodeOptions{PixelDepth: k.depth, RLE: rle}); err != nil {
					b.Fatalf("encode %s/%s: %v", sz.name, k.name, err)
				}
				data := buf.Bytes()

				b.Run(sz.name+"/"+k.name+"/"+rleLabel(rle), func(b *testing.B) {
					r := bytes.NewReader(data)
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						r.Reset(data)
						if _, err := Decode(r); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}

// BenchmarkDecodeRLEBottomOrigin measures the RLE path that writes logical rows directly
// instead of decoding linearly and vertically flipping the frame.
func BenchmarkDecodeRLEBottomOrigin(b *testing.B) {
	for _, sz := range benchSizes {
		for _, k := range benchKinds {
			img := k.build(sz.w, sz.h)
			var buf bytes.Buffer
			if err := EncodeWithOptions(&buf, img, &EncodeOptions{
				PixelDepth:   k.depth,
				RLE:          true,
				OriginBottom: true,
			}); err != nil {
				b.Fatalf("encode %s/%s: %v", sz.name, k.name, err)
			}
			data := buf.Bytes()

			b.Run(sz.name+"/"+k.name, func(b *testing.B) {
				r := bytes.NewReader(data)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					r.Reset(data)
					if _, err := Decode(r); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkEncode covers Encode over {size} x {kind/depth} x {RLE on/off}.
func BenchmarkEncode(b *testing.B) {
	for _, sz := range benchSizes {
		for _, k := range benchKinds {
			img := k.build(sz.w, sz.h)
			for _, rle := range []bool{false, true} {
				opts := &EncodeOptions{PixelDepth: k.depth, RLE: rle}

				b.Run(sz.name+"/"+k.name+"/"+rleLabel(rle), func(b *testing.B) {
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := EncodeWithOptions(io.Discard, img, opts); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}

// BenchmarkEncodeGeneric measures row-wise conversion for image.Image values
// that do not match one of the encoder's concrete fast-path types.
func BenchmarkEncodeGeneric(b *testing.B) {
	img := buildGeneric(1920, 1080)
	for _, depth := range []int{24, 32} {
		for _, rle := range []bool{false, true} {
			b.Run(fmt.Sprintf("%dbit/%s", depth, rleLabel(rle)), func(b *testing.B) {
				opts := &EncodeOptions{PixelDepth: depth, RLE: rle}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := EncodeWithOptions(io.Discard, img, opts); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
