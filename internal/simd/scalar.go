// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package simd

// scalarBGRToRGBA is the portable reference for BGRToRGBA.
func scalarBGRToRGBA(dst, src []byte) {
	di := 0
	for si := 0; si < len(src); si += 3 {
		dst[di+0] = src[si+2]
		dst[di+1] = src[si+1]
		dst[di+2] = src[si+0]
		dst[di+3] = 0xff
		di += 4
	}
}

// scalarSwapRB32 is the portable reference for SwapRB32.
func scalarSwapRB32(dst, src []byte) {
	for i := 0; i < len(src); i += 4 {
		dst[i+0] = src[i+2]
		dst[i+1] = src[i+1]
		dst[i+2] = src[i+0]
		dst[i+3] = src[i+3]
	}
}

// scalarRGBAToBGR is the portable reference for RGBAToBGR.
func scalarRGBAToBGR(dst, src []byte) {
	di := 0
	for si := 0; si < len(src); si += 4 {
		dst[di+0] = src[si+2]
		dst[di+1] = src[si+1]
		dst[di+2] = src[si+0]
		di += 3
	}
}
