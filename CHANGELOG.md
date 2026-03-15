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

* `EncodeWithOptions` and `EncodeOptions{RLE:true}` for RLE-compressed output.
* TGA encode support for true-color `16-bit` and `24-bit` output.
* TGA encode support for color-mapped (`paletted`) images.
* `EncodeOptions.PixelDepth` and `EncodeOptions.ColorMapDepth`.
* TGA 2.0 metadata writing via `EncodeOptions.Metadata`
  (footer, extension area, developer area, and thumbnail/postage stamp).

### Changed

* Decoder now validates image type/bit-depth combinations before pixel decode.
* Improved decode performance and reduced internal overhead, especially for
  large images and RLE-compressed files.

### Removed

## [1.0.0][] - 2026-02-10

### Added

* First public release

[1.0.0]: https://github.com/WoozyMasta/tga/tree/v1.0.0

<!--links-->
[Keep a Changelog]: https://keepachangelog.com/en/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
