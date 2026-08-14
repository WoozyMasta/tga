// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import "errors"

// Sentinel errors for decode/encode operations.
// Use errors.Is to check for them in callers.
var (
	// ErrFormat is returned when the TGA file format is invalid.
	ErrFormat = errors.New("tga: invalid format")

	// ErrUnsupported is returned when the TGA file is unsupported.
	ErrUnsupported = errors.New("tga: unsupported bit depth or image type")

	// ErrRLEOverrun is returned when the RLE data overrun.
	ErrRLEOverrun = errors.New("tga: rle data overrun")

	// ErrHeaderTooShort is returned when the TGA header is too short.
	ErrHeaderTooShort = errors.New("tga: header too short")

	// ErrPaletteIndex is returned when an indexed pixel is outside the declared color map.
	ErrPaletteIndex = errors.New("tga: palette index out of range")

	// ErrResourceLimit is returned when DecodeWithOptions would exceed a configured limit.
	ErrResourceLimit = errors.New("tga: decode resource limit exceeded")

	// ErrMetadata is returned when TGA 2.0 metadata is invalid.
	ErrMetadata = errors.New("tga: invalid metadata")
)
