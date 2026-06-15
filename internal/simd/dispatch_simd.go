// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

//go:build amd64 && !purego

package simd

import "golang.org/x/sys/cpu"

// init selects the fastest kernel the running CPU supports. SSSE3 covers all
// three conversions; AVX2 additionally doubles the swap throughput.
// Anything the kernels can't handle (and the trailing pixels of every call)
// is left to the scalar reference implementations.
func init() {
	if cpu.X86.HasSSSE3 {
		bgrToRGBAFn = bgrToRGBASSEWrapper
		rgbaToBGRFn = rgbaToBGRSSEWrapper
		swapRB32Fn = swapRB32SSEWrapper
	}
	if cpu.X86.HasAVX2 {
		swapRB32Fn = swapRB32AVX2Wrapper
	}
}

// swapRB32SSEWrapper processes whole 4-pixel groups in SSSE3, the rest scalar.
func swapRB32SSEWrapper(dst, src []byte) {
	pixels := len(src) / 4
	bulk := pixels &^ 3
	if bulk > 0 {
		swapRB32SSE(&dst[0], &src[0], bulk)
	}
	if bulk < pixels {
		scalarSwapRB32(dst[bulk*4:], src[bulk*4:])
	}
}

// swapRB32AVX2Wrapper processes whole 8-pixel groups in AVX2, the rest scalar.
func swapRB32AVX2Wrapper(dst, src []byte) {
	pixels := len(src) / 4
	bulk := pixels &^ 7
	if bulk > 0 {
		swapRB32AVX2(&dst[0], &src[0], bulk)
	}
	if bulk < pixels {
		scalarSwapRB32(dst[bulk*4:], src[bulk*4:])
	}
}

// bgrToRGBASSEWrapper vectorizes only groups for which the 16-byte source load stays in bounds
// (a 16-byte read covers 4 BGR pixels plus 4 slack bytes),
// and leaves the final pixels to the scalar path.
func bgrToRGBASSEWrapper(dst, src []byte) {
	pixels := len(src) / 3
	safe := min(safeGroups(len(src)), pixels)
	if safe > 0 {
		bgrToRGBASSE(&dst[0], &src[0], safe)
	}
	if safe < pixels {
		scalarBGRToRGBA(dst[safe*4:], src[safe*3:])
	}
}

// rgbaToBGRSSEWrapper vectorizes only groups for which the 16-byte destination
// store stays in bounds (a 16-byte write covers 4 BGR pixels plus 4 slack bytes),
// and leaves the final pixels to the scalar path.
func rgbaToBGRSSEWrapper(dst, src []byte) {
	pixels := len(src) / 4
	safe := min(safeGroups(len(dst)), pixels)
	if safe > 0 {
		rgbaToBGRSSE(&dst[0], &src[0], safe)
	}
	if safe < pixels {
		scalarRGBAToBGR(dst[safe*3:], src[safe*4:])
	}
}

// safeGroups returns the number of pixels (a multiple of 4)
// that can be processed by the 24-bit kernels given a buffer of n bytes,
// such that every 16-byte group access stays within n.
// The last group is always left to the scalar path
// so a 16-byte access never runs past the buffer.
func safeGroups(n int) int {
	if n < 16 {
		return 0
	}
	return 4 * ((n - 4) / 12)
}
