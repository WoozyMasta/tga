// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga_test

import (
	"bytes"
	"fmt"
	"image"

	"github.com/woozymasta/tga"
)

func ExampleDecode() {
	var encoded bytes.Buffer
	if err := tga.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		panic(err)
	}

	decoded, err := tga.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		panic(err)
	}

	fmt.Println(decoded.Bounds().Dx(), decoded.Bounds().Dy())
	// Output:
	// 1 1
}

func ExampleDecodeWithMetadata() {
	var encoded bytes.Buffer
	err := tga.EncodeWithOptions(&encoded, image.NewNRGBA(image.Rect(0, 0, 1, 1)), &tga.EncodeOptions{
		Metadata: &tga.TGA2Metadata{Author: "Example"},
	})
	if err != nil {
		panic(err)
	}

	_, info, err := tga.DecodeWithMetadata(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		panic(err)
	}

	fmt.Println(info.HasFooter)
	fmt.Println(info.Metadata.Author)
	// Output:
	// true
	// Example
}

func ExampleDecodeConfig() {
	var encoded bytes.Buffer
	if err := tga.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		panic(err)
	}

	config, err := tga.DecodeConfig(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		panic(err)
	}

	fmt.Println(config.Width, config.Height)
	// Output:
	// 3 2
}

func ExampleDecodeWithOptions() {
	var encoded bytes.Buffer
	if err := tga.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		panic(err)
	}

	_, err := tga.DecodeWithOptions(bytes.NewReader(encoded.Bytes()), tga.DecodeOptions{
		MaxPixels:       4,
		MaxDecodedBytes: 64,
	})
	fmt.Println(err == nil)
	// Output:
	// true
}

func ExampleEncodeWithOptions() {
	var encoded bytes.Buffer
	err := tga.EncodeWithOptions(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2)), &tga.EncodeOptions{
		PixelDepth: 24,
		RLE:        true,
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(encoded.Len() > 18)
	// Output:
	// true
}
