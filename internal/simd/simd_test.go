// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package simd

import (
	"bytes"
	"math/rand"
	"testing"
)

// pixelCounts exercises every tail/edge case around the 4- and 8-pixel SIMD
// group sizes plus a few large buffers.
var pixelCounts = []int{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 11, 12, 15, 16, 17,
	31, 32, 33, 63, 64, 65, 100, 255, 256, 257, 1000, 1920,
}

const guard = 0xAA

// makeSrc returns n deterministic pseudo-random bytes.
func makeSrc(n int) []byte {
	r := rand.New(rand.NewSource(int64(n)*1009 + 7))
	b := make([]byte, n)
	_, _ = r.Read(b)
	return b
}

// dstWithGuard allocates need+32 bytes filled with the guard byte and returns the full buffer;
// callers pass buf[:need] to the function under test.
func dstWithGuard(need int) []byte {
	buf := make([]byte, need+32)
	for i := range buf {
		buf[i] = guard
	}
	return buf
}

func checkGuard(t *testing.T, buf []byte, need int) {
	t.Helper()
	for i := need; i < len(buf); i++ {
		if buf[i] != guard {
			t.Fatalf("out-of-bounds write at +%d (need=%d)", i-need, need)
			return
		}
	}
}

func TestBGRToRGBA_MatchesScalar(t *testing.T) {
	for _, px := range pixelCounts {
		src := makeSrc(px * 3)
		want := make([]byte, px*4)
		scalarBGRToRGBA(want, src)

		buf := dstWithGuard(px * 4)
		BGRToRGBA(buf[:px*4], src)
		if !bytes.Equal(buf[:px*4], want) {
			t.Fatalf("px=%d: BGRToRGBA != scalar", px)
		}
		checkGuard(t, buf, px*4)
	}
}

func TestSwapRB32_MatchesScalar(t *testing.T) {
	for _, px := range pixelCounts {
		src := makeSrc(px * 4)
		want := make([]byte, px*4)
		scalarSwapRB32(want, src)

		buf := dstWithGuard(px * 4)
		SwapRB32(buf[:px*4], src)
		if !bytes.Equal(buf[:px*4], want) {
			t.Fatalf("px=%d: SwapRB32 != scalar", px)
		}
		checkGuard(t, buf, px*4)
	}
}

func TestRGBAToBGR_MatchesScalar(t *testing.T) {
	for _, px := range pixelCounts {
		src := makeSrc(px * 4)
		want := make([]byte, px*3)
		scalarRGBAToBGR(want, src)

		buf := dstWithGuard(px * 3)
		RGBAToBGR(buf[:px*3], src)
		if !bytes.Equal(buf[:px*3], want) {
			t.Fatalf("px=%d: RGBAToBGR != scalar", px)
		}
		checkGuard(t, buf, px*3)
	}
}
