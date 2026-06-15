// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

// Package simd provides SIMD-accelerated pixel layout conversions used by the tga codec.
// On amd64 it dispatches to SSSE3/AVX2 kernels when the running CPU supports them;
// otherwise (and on every other architecture) it falls back to the scalar implementations,
// which are also the source of truth for behavior.
//
// Set the TGA_PUREGO environment variable (or build with the `purego` tag)
// to force the scalar path on amd64.
//
// The amd64 assembly is generated with github.com/mmcloughlin/avo
// from the generator in ./asmgen. Regenerate with `make generate`.
package simd

// BGRToRGBA converts BGR pixels (3 bytes each) in src to RGBA pixels
// (4 bytes each, alpha forced to 0xff) in dst.
// len(src) must be a multiple of 3 and len(dst) must be at least len(src)/3*4.
// src and dst must not overlap.
func BGRToRGBA(dst, src []byte) { bgrToRGBAFn(dst, src) }

// SwapRB32 copies src to dst swapping the first
// and third byte of every 4-byte pixel (RGBA<->BGRA; alpha is preserved).
// len(src) and len(dst) must be equal and a multiple of 4.
// src and dst must not overlap.
func SwapRB32(dst, src []byte) { swapRB32Fn(dst, src) }

// RGBAToBGR converts RGBA pixels (4 bytes each) in src to BGR pixels
// (3 bytes each, alpha dropped) in dst. len(src) must be a multiple of 4
// and len(dst) must be at least len(src)/4*3. src and dst must not overlap.
func RGBAToBGR(dst, src []byte) { rgbaToBGRFn(dst, src) }

// Dispatch points. They default to the scalar implementations and are replaced
// by SIMD wrappers in init on amd64 when the CPU advertises the needed features.
var (
	bgrToRGBAFn = scalarBGRToRGBA
	swapRB32Fn  = scalarSwapRB32
	rgbaToBGRFn = scalarRGBAToBGR
)
