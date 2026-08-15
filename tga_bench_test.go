// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"bytes"
	"encoding/binary"
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

// buildGeneric wraps an NRGBA image to force the generic encoder path.
func buildGeneric(w, h int) image.Image {
	return genericBenchImage{Image: buildNRGBA(w, h)}
}

// buildRGBA creates the concrete premultiplied-alpha benchmark input.
func buildRGBA(w, h int) image.Image {
	src := buildNRGBA(w, h).(*image.NRGBA)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(dst.Pix, src.Pix)
	return dst
}

// buildRLEPattern creates a small image with a controlled compression profile.
func buildRLEPattern(w, h int, mode string) image.Image {
	if mode == "solid" {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for i := range img.Pix {
			img.Pix[i] = 255
		}
		return img
	}
	if mode == "mixed" {
		return buildNRGBA(w, h)
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			img.Pix[i+0] = byte(x*37 + y*13)
			img.Pix[i+1] = byte(x*17 + y*29)
			img.Pix[i+2] = byte(x*11 + y*43)
			img.Pix[i+3] = 255
		}
	}
	return img
}

// buildGray16TGA creates a bounded raw or RLE Gray16 fixture for decoding.
func buildGray16TGA(w, h int, rle bool) []byte {
	imageType := byte(typeGrayscale)
	if rle {
		imageType = typeRLEGrayscale
	}
	data := []byte{0, 0, imageType, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(w), byte(w >> 8), byte(h), byte(h >> 8), 16, 0x20}
	pixels := make([]byte, w*h*2)
	for i := 0; i < len(pixels); i += 2 {
		pixels[i] = byte(i)
		pixels[i+1] = byte(i >> 1)
	}
	if !rle {
		return append(data, pixels...)
	}
	for offset := 0; offset < w*h; {
		count := min(128, w*h-offset)
		data = append(data, byte(count-1))
		data = append(data, pixels[offset*2:(offset+count)*2]...)
		offset += count
	}
	return data
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

// BenchmarkDecodeWithMetadataAlpha measures TGA 2.0 alpha postprocessing
// on a representative full-HD true-color image.
func BenchmarkDecodeWithMetadataAlpha(b *testing.B) {
	src := buildNRGBA(1920, 1080)
	for _, attributes := range []byte{1, 4} {
		var encoded bytes.Buffer
		if err := EncodeWithOptions(&encoded, src, &EncodeOptions{
			Metadata: &TGA2Metadata{AttributesType: 3},
		}); err != nil {
			b.Fatalf("EncodeWithOptions attributes=%d: %v", attributes, err)
		}
		data := append([]byte(nil), encoded.Bytes()...)
		footer := len(data) - tga2FooterSize
		extOffset := int(binary.LittleEndian.Uint32(data[footer : footer+4]))
		data[extOffset+tga2OffAttrType] = attributes

		b.Run(fmt.Sprintf("attributes-%d", attributes), func(b *testing.B) {
			r := bytes.NewReader(data)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r.Reset(data)
				if _, _, err := DecodeWithMetadata(r); err != nil {
					b.Fatal(err)
				}
			}
		})
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

// BenchmarkEncodeRGBA covers the concrete premultiplied-alpha input path
// without multiplying the main size/depth benchmark matrix.
func BenchmarkEncodeRGBA(b *testing.B) {
	img := buildRGBA(1920, 1080)
	for _, rle := range []bool{false, true} {
		b.Run(rleLabel(rle), func(b *testing.B) {
			opts := &EncodeOptions{PixelDepth: 32, RLE: rle}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := EncodeWithOptions(io.Discard, img, opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEncodeRLEPatterns compares highly compressible,
// mixed, and noisy input without adding each pattern to the full benchmark matrix.
func BenchmarkEncodeRLEPatterns(b *testing.B) {
	for _, mode := range []string{"solid", "mixed", "noise"} {
		img := buildRLEPattern(64, 64, mode)
		b.Run(mode, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := EncodeWithOptions(io.Discard, img, &EncodeOptions{PixelDepth: 24, RLE: true}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkDecodeGray16 covers raw and RLE Gray16 without expanding
// the main benchmark matrix with a format that has no encoder option.
func BenchmarkDecodeGray16(b *testing.B) {
	for _, rle := range []bool{false, true} {
		data := buildGray16TGA(64, 64, rle)
		b.Run(rleLabel(rle), func(b *testing.B) {
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
