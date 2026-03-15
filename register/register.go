// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

/*
Package register registers the TGA format with image.Decode and image.DecodeConfig on import.

Use a blank import to enable image.Decode for TGA:

	import _ "github.com/woozymasta/tga/register"
*/
package register

import "github.com/woozymasta/tga"

// init registers TGA decoder in the global image registry.
func init() {
	tga.RegisterFormat()
}
