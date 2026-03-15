// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

/*
Package tga implements decoding and encoding of TGA (Truevision TARGA) image files.

Supported image types: uncompressed and RLE-compressed true color (2, 10),
grayscale (3, 11), and color-mapped (1, 9). Bit depths: 8, 15, 16, 24, 32.

Registration with image.Decode is not done by default (TGA has no magic bytes and
can conflict with other formats). To enable image.Decode for TGA, either call
tga.RegisterFormat() once or blank-import the register subpackage:

	import _ "github.com/woozymasta/tga/register"

Decode example (direct, no registration needed):

	f, _ := os.Open("image.tga")
	defer f.Close()
	img, err := tga.Decode(f)

DecodeConfig (dimensions without full decode):

	f, _ := os.Open("image.tga")
	defer f.Close()
	cfg, err := tga.DecodeConfig(f)
	// cfg.Width, cfg.Height

Encode example:

	out, _ := os.Create("out.tga")
	defer out.Close()
	err := tga.Encode(out, img)

Encode with RLE compression:

	out, _ := os.Create("out-rle.tga")
	defer out.Close()
	err := tga.EncodeWithOptions(out, img, &tga.EncodeOptions{RLE: true})

Encode true-color as 24-bit:

	out24, _ := os.Create("out-24.tga")
	defer out24.Close()
	err := tga.EncodeWithOptions(out24, img, &tga.EncodeOptions{PixelDepth: 24})

Encode true-color as 16-bit:

	out16, _ := os.Create("out-16.tga")
	defer out16.Close()
	err := tga.EncodeWithOptions(out16, img, &tga.EncodeOptions{PixelDepth: 16})

Encode paletted image with 32-bit color map:

	outPal, _ := os.Create("out-pal.tga")
	defer outPal.Close()
	err := tga.EncodeWithOptions(outPal, palettedImg, &tga.EncodeOptions{ColorMapDepth: 32})

With image.Decode (after import _ "github.com/woozymasta/tga/register" or tga.RegisterFormat()):

	img, format, err := image.Decode(f)
*/
package tga

// Specification: http://www.paulbourke.net/dataformats/tga/
