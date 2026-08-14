<!-- markdownlint-disable MD024 -->
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog][],
and this project adheres to [Semantic Versioning][].

<!--
## Unreleased

### Added
### Changed
### Removed
-->

## Unreleased

### Added

* A deterministic conformance matrix now exercises supported pixel depths,
  origins, raw/RLE packets, palette origins, and malformed input cases.
* `DecodeWithMetadata` for reading TGA 2.0 image IDs,
  extension metadata, thumbnails, and developer areas.
* `DecodeWithOptions` for opt-in pixel and decoded-memory limits.
* 16-bit grayscale+alpha decoding for raw and RLE TGA images.

### Changed

* Generic `image.Image` encoding now converts one row at a time,
  avoiding a full-image intermediate `NRGBA` allocation.
* Encoder palette data and RLE packets now use bounded buffers
  to reduce the number of underlying `io.Writer.Write` calls.
* RLE decoding now writes bottom-origin pixels directly to logical rows,
  avoiding the full-frame vertical flip pass.
* Raw grayscale and paletted decoding now reads directly
  into final pixel storage, removing temporary per-row buffers.
* Encoder depth options are now validated only for applicable image modes;
  irrelevant `PixelDepth` and `ColorMapDepth` values are ignored.
* TGA 2.0 metadata encoding now validates fixed-width fields,
  timestamps, durations, gamma, thumbnails, and offsets,
  and rejects alpha attributes inconsistent
  with the emitted pixel representation.
* `DecodeWithMetadata` now applies TGA 2.0 alpha attribute semantics
  and distinguishes straight and premultiplied alpha image models.
* Public encoding, decoding, and TGA 2.0 metadata option structs now expose
  stable JSON field names.

### Fixed

* RLE paletted decoding no longer indexes raw packet data past the packet
  when a packet crosses a scanline boundary.
* 16-bit true-color decoding now preserves the A1R5G5B5 alpha bit
  when the image descriptor declares one attribute bit;
  15-bit RGB555 remains opaque.
* `Decode` and `DecodeConfig` now share header parsing and validation,
  including consistent errors for truncated and malformed headers.
* Decoder now handles all four TGA image origins
  and rejects unsupported interleaved image data.
* Indexed TGA decoding now rejects out-of-range palette indices
  and unsupported color-map entry depths.

## [1.2.0][] - 2026-06-17

### Added

* SIMD-accelerated (`SSSE3`/`AVX2`) true-color pixel conversion on `amd64`
  for both decode and encode, selected at runtime via CPU feature detection
  with an automatic scalar fallback on other CPUs and architectures.
* `purego` build tag (compile-time) and `TGA_PUREGO=1` environment variable
  (runtime) to force the pure-Go paths (no assembly).

### Changed

* Significant performance improvements:
  true-color decode is ~40-65% faster,
  true-color encode ~75-87% faster,
  and RLE true-color decode ~55% faster (~60% faster overall, geomean),
  without increasing memory use.
* True-color RLE encoding no longer allocates a scratch buffer per scan line,
  cutting allocations from roughly one-per-row to a small constant
  (e.g. 1082 -> 3 allocs/op at 1920x1080).
* True-color RLE encoding no longer emits packets that span scan lines,
  which is friendlier to strict TGA 2.0 readers;
  the decoded image is identical.
* Added `golang.org/x/sys` as the only new dependency (CPU feature detection);
  the avo assembly generator lives in an isolated tooling module
  and is not pulled into consumer builds.

### Fixed

* Decoding a malformed color-mapped image no longer panics
  when the pixel data references a palette index outside the supplied color map.
* Decoding no longer panics on a color map that declares a `0`-bit entry size.

[1.2.0]: https://github.com/WoozyMasta/tga/compare/v1.1.0...v1.2.0

## [1.1.0][] - 2026-03-16

### Added

* `EncodeWithOptions` and `EncodeOptions{RLE:true}` for RLE-compressed output.
* TGA encode support for true-color `16-bit` and `24-bit` output.
* TGA encode support for color-mapped (`paletted`) images.
* `EncodeOptions.PixelDepth` and `EncodeOptions.ColorMapDepth`.
* Added encode options for origin and image ID field.
* TGA 2.0 metadata writing via `EncodeOptions.Metadata`
  (footer, extension area, developer area, and thumbnail/postage stamp).

### Changed

* Decoder now validates image type/bit-depth combinations before pixel decode.
* Improved decode performance and reduced internal overhead, especially for
  large images and RLE-compressed files.
* Expanded test and benchmark coverage for new format capabilities.

[1.1.0]: https://github.com/WoozyMasta/tga/compare/v1.0.0...v1.1.0

### Removed

## [1.0.0][] - 2026-02-10

### Added

* First public release

[1.0.0]: https://github.com/WoozyMasta/tga/tree/v1.0.0

<!--links-->
[Keep a Changelog]: https://keepachangelog.com/en/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
