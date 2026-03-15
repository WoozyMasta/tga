package tga

import (
	"bytes"
	"image/color"
	"io"
	"testing"

	"image"
)

func BenchmarkDecode_64x64(b *testing.B) {
	data := makeRawTGA24(64, 64, true)
	r := bytes.NewReader(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Reset(data)
		_, _ = Decode(r)
	}
}

func BenchmarkDecode_1920x1080(b *testing.B) {
	data := makeRawTGA24(1920, 1080, true)
	r := bytes.NewReader(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Reset(data)
		_, _ = Decode(r)
	}
}

func BenchmarkEncode_64x64(b *testing.B) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Encode(io.Discard, img)
	}
}

func BenchmarkEncode_1920x1080(b *testing.B) {
	img := image.NewNRGBA(image.Rect(0, 0, 1920, 1080))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Encode(io.Discard, img)
	}
}

func BenchmarkDecode_RLE24_1920x1080(b *testing.B) {
	src := image.NewNRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		c := color.NRGBA{R: 20, G: 30, B: 40, A: 255}
		if y%2 == 1 {
			c = color.NRGBA{R: 180, G: 20, B: 60, A: 255}
		}
		for x := 0; x < 1920; x++ {
			src.SetNRGBA(x, y, c)
		}
	}

	var encoded bytes.Buffer
	_ = EncodeWithOptions(&encoded, src, &EncodeOptions{PixelDepth: 24, RLE: true})
	data := encoded.Bytes()
	r := bytes.NewReader(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Reset(data)
		_, _ = Decode(r)
	}
}

func BenchmarkEncode_PalettedRLE_1920x1080(b *testing.B) {
	pal := color.Palette{
		color.NRGBA{R: 0, G: 0, B: 0, A: 255},
		color.NRGBA{R: 220, G: 30, B: 10, A: 255},
		color.NRGBA{R: 30, G: 140, B: 220, A: 255},
		color.NRGBA{R: 250, G: 250, B: 60, A: 255},
	}
	img := image.NewPaletted(image.Rect(0, 0, 1920, 1080), pal)
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			img.SetColorIndex(x, y, uint8((x/16+y/16)%len(pal)))
		}
	}

	opts := &EncodeOptions{RLE: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EncodeWithOptions(io.Discard, img, opts)
	}
}
