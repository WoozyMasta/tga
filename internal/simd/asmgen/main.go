// Command asmgen generates the SIMD kernels used by package simd.
//
// It emits ../kernels_amd64.s and ../kernels_stub_amd64.go. Regenerate with:
//
//	make generate
//
// The kernels operate on whole groups of pixels only;
// callers (the Go wrappers in package simd) are responsible
// for feeding a safe pixel count and handling the scalar tail.
package main

import (
	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
)

func main() {
	// Keep the generated assembly out of purego builds.
	// Combined with the _amd64 file suffix
	// the effective constraint is "amd64 && !purego".
	ConstraintExpr("!purego")

	// PSHUFB control byte 0x80 selects a zero byte in the destination.
	const z = 0x80

	// 16-byte shuffle masks (one 128-bit register = 4 pixels).
	swapMask16 := constBytes("swapRB32Mask16", []byte{
		2, 1, 0, 3, 6, 5, 4, 7, 10, 9, 8, 11, 14, 13, 12, 15,
	})
	// 32-byte mask for AVX2: the 16-byte pattern repeated per 128-bit lane.
	swapMask32 := constBytes("swapRB32Mask32", []byte{
		2, 1, 0, 3, 6, 5, 4, 7, 10, 9, 8, 11, 14, 13, 12, 15,
		2, 1, 0, 3, 6, 5, 4, 7, 10, 9, 8, 11, 14, 13, 12, 15,
	})
	// BGR(3) -> RGBA(4): gather R,G,B and zero the alpha slot.
	bgrShuf := constBytes("bgrToRGBAShuf", []byte{
		2, 1, 0, z, 5, 4, 3, z, 8, 7, 6, z, 11, 10, 9, z,
	})
	// OR mask that sets every alpha byte to 0xff.
	bgrAlpha := constBytes("bgrToRGBAAlpha", []byte{
		0, 0, 0, 0xff, 0, 0, 0, 0xff, 0, 0, 0, 0xff, 0, 0, 0, 0xff,
	})
	// RGBA(4) -> BGR(3): pack B,G,R for 4 pixels into the low 12 bytes.
	rgbShuf := constBytes("rgbaToBGRShuf", []byte{
		2, 1, 0, 6, 5, 4, 10, 9, 8, 14, 13, 12, z, z, z, z,
	})

	genSwapRB32SSE(swapMask16)
	genSwapRB32AVX2(swapMask32)
	genBGRToRGBASSE(bgrShuf, bgrAlpha)
	genRGBAToBGRSSE(rgbShuf)

	Generate()
}

// constBytes defines a read-only data constant and returns its address.
func constBytes(name string, b []byte) Mem {
	return ConstData(name, String(string(b)))
}

// genSwapRB32SSE swaps R and B of every RGBA pixel, 4 pixels per iteration.
func genSwapRB32SSE(mask Mem) {
	TEXT("swapRB32SSE", NOSPLIT, "func(dst, src *byte, pixels int)")
	Pragma("noescape")
	Doc("swapRB32SSE swaps R<->B in pixels RGBA pixels (pixels must be a multiple of 4) using SSSE3.")
	dst := Load(Param("dst"), GP64())
	src := Load(Param("src"), GP64())
	pixels := Load(Param("pixels"), GP64())

	m := XMM()
	MOVOU(mask, m)

	Label("loop")
	CMPQ(pixels, Imm(4))
	JL(LabelRef("done"))

	x := XMM()
	MOVOU(Mem{Base: src}, x)
	PSHUFB(m, x)
	MOVOU(x, Mem{Base: dst})

	ADDQ(Imm(16), src)
	ADDQ(Imm(16), dst)
	SUBQ(Imm(4), pixels)
	JMP(LabelRef("loop"))

	Label("done")
	RET()
}

// genSwapRB32AVX2 swaps R and B of every RGBA pixel, 8 pixels per iteration.
func genSwapRB32AVX2(mask Mem) {
	TEXT("swapRB32AVX2", NOSPLIT, "func(dst, src *byte, pixels int)")
	Pragma("noescape")
	Doc("swapRB32AVX2 swaps R<->B in pixels RGBA pixels (pixels must be a multiple of 8) using AVX2.")
	dst := Load(Param("dst"), GP64())
	src := Load(Param("src"), GP64())
	pixels := Load(Param("pixels"), GP64())

	m := YMM()
	VMOVDQU(mask, m)

	Label("loop")
	CMPQ(pixels, Imm(8))
	JL(LabelRef("done"))

	y := YMM()
	VMOVDQU(Mem{Base: src}, y)
	VPSHUFB(m, y, y)
	VMOVDQU(y, Mem{Base: dst})

	ADDQ(Imm(32), src)
	ADDQ(Imm(32), dst)
	SUBQ(Imm(8), pixels)
	JMP(LabelRef("loop"))

	Label("done")
	VZEROUPPER()
	RET()
}

// genBGRToRGBASSE expands BGR (3 bytes) to RGBA (4 bytes), 4 pixels per iteration.
// It reads 16 bytes per group, so the caller must guarantee
// at least 16 readable source bytes for every group it requests.
func genBGRToRGBASSE(shuf, alpha Mem) {
	TEXT("bgrToRGBASSE", NOSPLIT, "func(dst, src *byte, pixels int)")
	Pragma("noescape")
	Doc("bgrToRGBASSE converts pixels BGR pixels to RGBA (alpha 0xff) using SSSE3; pixels must be a multiple of 4 and the source must hold 16 readable bytes per group.")
	dst := Load(Param("dst"), GP64())
	src := Load(Param("src"), GP64())
	pixels := Load(Param("pixels"), GP64())

	s := XMM()
	MOVOU(shuf, s)
	a := XMM()
	MOVOU(alpha, a)

	Label("loop")
	CMPQ(pixels, Imm(4))
	JL(LabelRef("done"))

	x := XMM()
	MOVOU(Mem{Base: src}, x)
	PSHUFB(s, x)
	POR(a, x)
	MOVOU(x, Mem{Base: dst})

	ADDQ(Imm(12), src)
	ADDQ(Imm(16), dst)
	SUBQ(Imm(4), pixels)
	JMP(LabelRef("loop"))

	Label("done")
	RET()
}

// genRGBAToBGRSSE packs RGBA (4 bytes) to BGR (3 bytes), 4 pixels per iteration.
// It writes 16 bytes per group, so the caller must guarantee
// at least 16 writable destination bytes for every group it requests.
func genRGBAToBGRSSE(shuf Mem) {
	TEXT("rgbaToBGRSSE", NOSPLIT, "func(dst, src *byte, pixels int)")
	Pragma("noescape")
	Doc("rgbaToBGRSSE converts pixels RGBA pixels to BGR using SSSE3; pixels must be a multiple of 4 and the destination must hold 16 writable bytes per group.")
	dst := Load(Param("dst"), GP64())
	src := Load(Param("src"), GP64())
	pixels := Load(Param("pixels"), GP64())

	s := XMM()
	MOVOU(shuf, s)

	Label("loop")
	CMPQ(pixels, Imm(4))
	JL(LabelRef("done"))

	x := XMM()
	MOVOU(Mem{Base: src}, x)
	PSHUFB(s, x)
	MOVOU(x, Mem{Base: dst})

	ADDQ(Imm(16), src)
	ADDQ(Imm(12), dst)
	SUBQ(Imm(4), pixels)
	JMP(LabelRef("loop"))

	Label("done")
	RET()
}
