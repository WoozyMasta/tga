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

With image.Decode (after import _ "github.com/woozymasta/tga/register" or tga.RegisterFormat()):

	img, format, err := image.Decode(f)
*/
package tga

// Specification: http://www.paulbourke.net/dataformats/tga/
