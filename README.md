# tga

Go package for decoding and encoding TGA (Truevision TARGA) images.

* Decode: uncompressed and RLE image types `1`, `2`, `3`, `9`, `10`, `11`;
  bit depths `8`, `15`, `16`, `24`, `32`.
* Encode: grayscale, true-color (`16`/`24`/`32`), and paletted (`8`) output;
  optional RLE compression.
* TGA 2.0 write support: footer, extension/developer areas, and metadata fields.
* Encode options: `PixelDepth`, `ColorMapDepth`, `OriginBottom`, `ImageID`.

> [!NOTE]  
> TGA has no stable magic bytes, so global registration via
> `image.RegisterFormat` can conflict with other decoders.
> Use direct `tga.Decode`/`tga.DecodeConfig` by default.
> If you still need `image.Decode`, opt in with blank import:
> `import _ "github.com/woozymasta/tga/register"`.

```go
import "github.com/woozymasta/tga"

// Decode (direct; no registration)
img, err := tga.Decode(r)
cfg, err := tga.DecodeConfig(r)

// To use image.Decode with TGA: import _ "github.com/woozymasta/tga/register"

// Decode with resource limits for untrusted input.
img, err := tga.DecodeWithOptions(r, tga.DecodeOptions{
  MaxPixels:       16 * 1024 * 1024,
  MaxDecodedBytes: 64 * 1024 * 1024,
})

// Decode TGA 2.0 metadata from a seekable reader.
img, info, err := tga.DecodeWithMetadata(readSeeker)

// Encode
err := tga.Encode(w, img)

// Encode with RLE compression
err := tga.EncodeWithOptions(w, img, &tga.EncodeOptions{RLE: true})

// Encode true-color as 24-bit
err := tga.EncodeWithOptions(w, img, &tga.EncodeOptions{PixelDepth: 24})

// Encode true-color as 16-bit
err := tga.EncodeWithOptions(w, img, &tga.EncodeOptions{PixelDepth: 16})

// Encode paletted image with 32-bit palette entries
err := tga.EncodeWithOptions(w, palettedImg, &tga.EncodeOptions{ColorMapDepth: 32})

// Encode with TGA 2.0 metadata (footer + extension/developer areas)
err := tga.EncodeWithOptions(w, img, &tga.EncodeOptions{
  Metadata: &tga.TGA2Metadata{
    Author:    "WoozyMasta",
    Gamma:     2.2,
    Timestamp: time.Now(),
  },
})

// Encode with bottom-left origin and Image ID field
err := tga.EncodeWithOptions(w, img, &tga.EncodeOptions{
  OriginBottom: true,
  ImageID:      []byte("preview-id"),
})

// Set explicit TGA2 alpha-attribute semantics (advanced)
err := tga.EncodeWithOptions(w, img, &tga.EncodeOptions{
  Metadata: &tga.TGA2Metadata{
    AttributesType: 3,
  },
})
```
