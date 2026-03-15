# tga

Go package for decoding and encoding TGA (Truevision TARGA) images.  
Supports uncompressed and RLE types 1, 2, 3, 9, 10, 11;
bit depths 8, 15, 16, 24, 32.

```go
import "github.com/woozymasta/tga"

// Decode (direct; no registration)
img, err := tga.Decode(r)
cfg, err := tga.DecodeConfig(r)

// To use image.Decode with TGA: import _ "github.com/woozymasta/tga/register"

// Encode
err := tga.Encode(w, img)

// Encode with RLE compression
err := tga.EncodeWithOptions(w, img, &tga.EncodeOptions{RLE: true})
```
