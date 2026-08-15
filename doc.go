// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

/*
Package tga implements decoding and encoding of TGA (Truevision TARGA) image files.

The decoder supports uncompressed and RLE-compressed true-color, grayscale, and color-mapped images.
Supported pixel depths are 8, 15, 16, 24, and 32 bits.
It handles image origins, palette origins, image IDs, and resource limits for untrusted input.

Decode returns *image.Gray for 8-bit grayscale, *image.Paletted for color-mapped images,
and *image.NRGBA for true-color and 16-bit grayscale images.
TGA 2.0 premultiplied alpha metadata returns *image.RGBA from DecodeWithMetadata.

DecodeWithMetadata reads optional TGA 2.0 footer, extension area,
developer area, metadata, and postage-stamp thumbnail.
The input must implement io.ReadSeeker.
DecodeWithMetadataOptions applies DecodeOptions resource limits,
including MaxMetadataBytes for TGA 2.0 data.

The encoder writes grayscale, true-color, and paletted images, with optional RLE compression.
EncodeOptions controls pixel depth, color-map depth, image origin, image ID, and TGA 2.0 metadata.

Registration with image.Decode is not done by default
because TGA has no magic bytes and can conflict with other formats.
To enable image.Decode, either call RegisterFormat once or blank-import the register subpackage:

	import _ "github.com/woozymasta/tga/register"

Runnable usage examples are provided by ExampleDecode and ExampleDecodeWithMetadata.
*/
package tga
