// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"testing"
)

// makeConformanceTrueColor builds a deterministic synthetic true-color sample.
func makeConformanceTrueColor(depth int, rle bool, descriptor byte) []byte {
	imageType := byte(typeTrueColor)
	if rle {
		imageType = typeRLETrueColor
	}
	header := []byte{
		0, 0, imageType,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		2, 0, 2, 0,
		byte(depth), descriptor,
	}
	bytesPerPixel := (depth + 7) / 8
	pixels := make([]byte, 4*bytesPerPixel)
	for i := range pixels {
		pixels[i] = byte(i*17 + 3)
	}
	if rle {
		pixels = append([]byte{0x03}, pixels...)
	}

	return append(header, pixels...)
}

// makeConformanceGray builds a deterministic synthetic grayscale sample.
func makeConformanceGray(depth int, rle bool, descriptor byte) []byte {
	imageType := byte(typeGrayscale)
	if rle {
		imageType = typeRLEGrayscale
	}
	header := []byte{
		0, 0, imageType,
		0, 0, 0, 0, 0,
		0, 0, 0, 0,
		2, 0, 2, 0,
		byte(depth), descriptor,
	}
	bytesPerPixel := depth / 8
	pixels := make([]byte, 4*bytesPerPixel)
	for i := range pixels {
		pixels[i] = byte(i*29 + 5)
	}
	if rle {
		pixels = append([]byte{0x03}, pixels...)
	}

	return append(header, pixels...)
}

// makeConformancePaletted builds a palette with a non-zero first index.
func makeConformancePaletted(rle bool, descriptor byte) []byte {
	imageType := byte(typePaletted)
	if rle {
		imageType = typeRLEPaletted
	}
	header := []byte{
		0, 1, imageType,
		2, 0, 2, 0, 24,
		0, 0, 0, 0,
		2, 0, 2, 0,
		8, descriptor,
	}
	palette := []byte{
		0, 0, 255,
		0, 255, 0,
	}
	indices := []byte{2, 3, 2, 3}
	if rle {
		indices = append([]byte{0x03}, indices...)
	}

	return append(append(header, palette...), indices...)
}

func TestConformanceMatrix(t *testing.T) {
	for _, depth := range []int{15, 16, 24, 32} {
		for _, rle := range []bool{false, true} {
			for _, descriptor := range []byte{0, maskOriginRight, maskOriginTop, maskOriginTop | maskOriginRight} {
				name := "true-color"
				if rle {
					name += "/rle"
				}
				t.Run(fmt.Sprintf("%s/%dbit/descriptor-%02x", name, depth, descriptor), func(t *testing.T) {
					data := makeConformanceTrueColor(depth, rle, descriptor)
					if _, err := Decode(bytes.NewReader(data)); err != nil {
						t.Fatalf("Decode depth=%d rle=%t descriptor=0x%02x: %v", depth, rle, descriptor, err)
					}
				})
			}
		}
	}

	for _, depth := range []int{8, 16} {
		for _, rle := range []bool{false, true} {
			for _, descriptor := range []byte{0, maskOriginRight, maskOriginTop, maskOriginTop | maskOriginRight} {
				data := makeConformanceGray(depth, rle, descriptor)
				if _, err := Decode(bytes.NewReader(data)); err != nil {
					t.Fatalf("Decode grayscale depth=%d rle=%t descriptor=0x%02x: %v", depth, rle, descriptor, err)
				}
			}
		}
	}

	for _, rle := range []bool{false, true} {
		for _, descriptor := range []byte{0, maskOriginRight, maskOriginTop, maskOriginTop | maskOriginRight} {
			data := makeConformancePaletted(rle, descriptor)
			decoded, err := Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Decode palette rle=%t descriptor=0x%02x: %v", rle, descriptor, err)
			}
			if len(decoded.(*image.Paletted).Palette) != 2 {
				t.Fatalf("palette length = %d, want 2", len(decoded.(*image.Paletted).Palette))
			}
		}
	}
}

func TestConformanceMalformedMatrix(t *testing.T) {
	valid := makeConformanceTrueColor(24, false, maskOriginTop)
	validRLE := makeConformanceTrueColor(24, true, maskOriginTop)
	validPalette := makeConformancePaletted(false, maskOriginTop)
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{name: "truncated_header", data: valid[:headerSize-1], want: ErrHeaderTooShort},
		{name: "truncated_raw", data: valid[:len(valid)-1], want: io.ErrUnexpectedEOF},
		{name: "truncated_rle", data: validRLE[:len(validRLE)-1], want: io.ErrUnexpectedEOF},
		{name: "truncated_palette", data: validPalette[:len(validPalette)-1], want: io.ErrUnexpectedEOF},
		{
			name: "invalid_palette_index",
			data: append(append([]byte(nil), validPalette[:24]...), 9, 2, 2, 3),
			want: ErrPaletteIndex,
		},
	}

	interleaved := append([]byte(nil), valid...)
	interleaved[17] |= 0x40
	cases = append(cases, struct {
		name string
		data []byte
		want error
	}{"unsupported_interleave", interleaved, ErrUnsupported})

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(bytes.NewReader(tt.data))
			if !errors.Is(err, tt.want) {
				t.Fatalf("Decode error = %v, want %v", err, tt.want)
			}
		})
	}

	large := makeConformanceTrueColor(24, false, maskOriginTop)
	large[12], large[13], large[14], large[15] = 0xff, 0xff, 0xff, 0xff
	if _, err := DecodeWithOptions(bytes.NewReader(large), DecodeOptions{MaxPixels: 1}); !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("large image error = %v, want %v", err, ErrResourceLimit)
	}
}
