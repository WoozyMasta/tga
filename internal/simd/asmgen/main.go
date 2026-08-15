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
	rgb555Mask := constBytes("rgb555Mask", []byte{
		0x1f, 0x00, 0x1f, 0x00, 0x1f, 0x00, 0x1f, 0x00,
		0x1f, 0x00, 0x1f, 0x00, 0x1f, 0x00, 0x1f, 0x00,
	})
	rgb555Alpha := constBytes("rgb555Alpha", []byte{
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	})

	genSwapRB32SSE(swapMask16)
	genSwapRB32AVX2(swapMask32)
	genBGRToRGBASSE(bgrShuf, bgrAlpha)
	genRGBAToBGRSSE(rgbShuf)
	genRGB555ToRGBASSE(rgb555Mask, rgb555Alpha, bgrAlpha)

	Generate()
}

// genRGB555ToRGBASSE converts four little-endian RGB555 pixels to opaque RGBA.
func genRGB555ToRGBASSE(mask, alpha, outputAlpha Mem) {
	TEXT("rgb555ToRGBASSE", NOSPLIT, "func(dst, src *byte, pixels int)")
	Pragma("noescape")
	Doc("rgb555ToRGBASSE converts RGB555 pixels to opaque RGBA using SSE2; pixels must be a multiple of 4.")
	dst := Load(Param("dst"), GP64())
	src := Load(Param("src"), GP64())
	pixels := Load(Param("pixels"), GP64())

	maskReg := XMM()
	MOVOU(mask, maskReg)
	alphaReg := XMM()
	MOVOU(alpha, alphaReg)

	Label("loop")
	CMPQ(pixels, Imm(4))
	JL(LabelRef("done"))

	v := XMM()
	MOVQ(Mem{Base: src}, v)

	r := XMM()
	MOVO(v, r)
	PSRLW(Imm(10), r)
	PAND(maskReg, r)
	PSLLW(Imm(3), r)
	rShift := XMM()
	MOVO(r, rShift)
	PSRLW(Imm(5), rShift)
	POR(rShift, r)
	PACKUSWB(r, r)

	g := XMM()
	MOVO(v, g)
	PSRLW(Imm(5), g)
	PAND(maskReg, g)
	PSLLW(Imm(3), g)
	gShift := XMM()
	MOVO(g, gShift)
	PSRLW(Imm(5), gShift)
	POR(gShift, g)
	PACKUSWB(g, g)

	b := XMM()
	MOVO(v, b)
	PAND(maskReg, b)
	PSLLW(Imm(3), b)
	bShift := XMM()
	MOVO(b, bShift)
	PSRLW(Imm(5), bShift)
	POR(bShift, b)
	PACKUSWB(b, b)

	a := XMM()
	MOVO(alphaReg, a)
	PACKUSWB(a, a)
	rg := XMM()
	PUNPCKLBW(g, r)
	PUNPCKLBW(a, b)
	MOVO(r, rg)
	PUNPCKLWL(b, rg)
	outAlpha := XMM()
	MOVOU(outputAlpha, outAlpha)
	POR(outAlpha, rg)
	MOVOU(rg, Mem{Base: dst})

	ADDQ(Imm(8), src)
	ADDQ(Imm(16), dst)
	SUBQ(Imm(4), pixels)
	JMP(LabelRef("loop"))

	Label("done")
	RET()
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
