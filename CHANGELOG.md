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
  with unchanged memory use.
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
