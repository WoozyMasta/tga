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

// scalarRGB555ToRGBA converts little-endian RGB555 pixels to opaque RGBA pixels.
func scalarRGB555ToRGBA(dst, src []byte) {
	di := 0
	for si := 0; si < len(src); si += 2 {
		v := uint16(src[si]) | uint16(src[si+1])<<8
		r := byte((v >> 10) & 0x1f)
		g := byte((v >> 5) & 0x1f)
		b := byte(v & 0x1f)
		dst[di+0] = (r << 3) | (r >> 2)
		dst[di+1] = (g << 3) | (g >> 2)
		dst[di+2] = (b << 3) | (b >> 2)
		dst[di+3] = 0xff
		di += 4
	}
}

// scalarRGB555AlphaToRGBA converts little-endian A1R5G5B5 pixels to RGBA pixels.
func scalarRGB555AlphaToRGBA(dst, src []byte) {
	di := 0
	for si := 0; si < len(src); si += 2 {
		v := uint16(src[si]) | uint16(src[si+1])<<8
		r := byte((v >> 10) & 0x1f)
		g := byte((v >> 5) & 0x1f)
		b := byte(v & 0x1f)
		dst[di+0] = (r << 3) | (r >> 2)
		dst[di+1] = (g << 3) | (g >> 2)
		dst[di+2] = (b << 3) | (b >> 2)
		dst[di+3] = byte((v >> 15) * 0xff)
		di += 4
	}
}
