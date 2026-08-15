// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package simd

import "testing"

const benchmarkPixels = 1920

// benchmarkBuffers returns reusable source and destination scanline buffers.
func benchmarkBuffers(bytesPerSourcePixel, bytesPerDestinationPixel int) ([]byte, []byte) {
	src := make([]byte, benchmarkPixels*bytesPerSourcePixel)
	dst := make([]byte, benchmarkPixels*bytesPerDestinationPixel)
	for i := range src {
		src[i] = byte(i*31 + 7)
	}

	return dst, src
}

// BenchmarkBGRToRGBA measures the dispatched BGR-to-RGBA conversion
// on a representative scanline-sized buffer.
func BenchmarkBGRToRGBA(b *testing.B) {
	dst, src := benchmarkBuffers(3, 4)
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BGRToRGBA(dst, src)
	}
}

// BenchmarkSwapRB32 measures the dispatched four-byte channel swap.
func BenchmarkSwapRB32(b *testing.B) {
	dst, src := benchmarkBuffers(4, 4)
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SwapRB32(dst, src)
	}
}

// BenchmarkRGBAToBGR measures the dispatched RGBA-to-BGR conversion.
func BenchmarkRGBAToBGR(b *testing.B) {
	dst, src := benchmarkBuffers(4, 3)
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RGBAToBGR(dst, src)
	}
}

// BenchmarkRGB555ToRGBA measures RGB555 expansion on a representative scanline.
func BenchmarkRGB555ToRGBA(b *testing.B) {
	dst, src := benchmarkBuffers(2, 4)
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RGB555ToRGBA(dst, src)
	}
}
