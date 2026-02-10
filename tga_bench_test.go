package tga

import (
	"bytes"
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
