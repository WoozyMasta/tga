// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

/*
Package tga implements decoding and encoding of TGA (Truevision TARGA) image files.

Supported image types: uncompressed and RLE-compressed true color (2, 10),
grayscale (3, 11), and color-mapped (1, 9). Bit depths: 8, 15, 16, 24, 32.

Decode returns `*image.Gray` for 8-bit grayscale,
`*image.Paletted` for color-mapped images,
and `*image.NRGBA` for true-color and 16-bit grayscale images.
TGA 2.0 metadata with premultiplied alpha returns `*image.RGBA` from DecodeWithMetadata.

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

Decode with resource limits:

	f, _ := os.Open("untrusted.tga")
	defer f.Close()
	img, err := tga.DecodeWithOptions(f, tga.DecodeOptions{
		MaxPixels:        16 * 1024 * 1024,
		MaxDecodedBytes:  64 * 1024 * 1024,
	})

Decode TGA 2.0 metadata (the input must be seekable):

	f, _ := os.Open("image-with-metadata.tga")
	defer f.Close()
	img, info, err := tga.DecodeWithMetadata(f)
	// info.HasFooter and info.Metadata describe the optional TGA 2.0 areas.

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

Encode with TGA 2.0 metadata:

	outMeta, _ := os.Create("out-meta.tga")
	defer outMeta.Close()
	err := tga.EncodeWithOptions(outMeta, img, &tga.EncodeOptions{
		Metadata: &tga.TGA2Metadata{
			Author: "Woozy",
			Gamma:  2.2,
		},
	})

Encode with bottom-left origin and image ID:

	outBottom, _ := os.Create("out-bottom.tga")
	defer outBottom.Close()
	err := tga.EncodeWithOptions(outBottom, img, &tga.EncodeOptions{
		OriginBottom: true,
		ImageID:      []byte("preview-id"),
	})

Encode with explicit TGA 2.0 alpha semantics:

	outAlpha, _ := os.Create("out-alpha.tga")
	defer outAlpha.Close()
	err := tga.EncodeWithOptions(outAlpha, img, &tga.EncodeOptions{
		Metadata: &tga.TGA2Metadata{
			AttributesType: 3,
		},
	})

With image.Decode (after import _ "github.com/woozymasta/tga/register" or tga.RegisterFormat()):

	img, format, err := image.Decode(f)
*/
package tga

// Specification: http://www.paulbourke.net/dataformats/tga/
