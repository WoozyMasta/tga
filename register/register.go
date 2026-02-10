// Package register registers the TGA format with image.Decode and image.DecodeConfig on import.
//
// Use a blank import to enable image.Decode for TGA:
//
//	import _ "github.com/woozymasta/tga/register"
package register

import "github.com/woozymasta/tga"

func init() {
	tga.RegisterFormat()
}
