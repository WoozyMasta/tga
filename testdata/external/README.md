# External TGA fixtures

This directory contains a small, curated set of external TGA fixtures.
The files are kept flat so the test data is easy to discover;
the test names and provenance below identify
the source collection for each group.

## ftrvxmtrx

The PNG-oracle pairs are selected from:

<https://github.com/ftrvxmtrx/tga/tree/master/testdata>

They cover raw and RLE images, grayscale, true-color,
color-mapped pixels, and top-left/bottom-left origins.
The source collection documents that its 16-bit grayscale PNGs
were decoded incorrectly by Krita,
so those TGA files are not included in the oracle pairs.

## Selected conformance fixtures

`utc16.tga` and `ccm8.tga`
are selected from the local `lunapaint/tga-test-suite` mirror:

<https://github.com/lunapaint/tga-test-suite>

They cover external 16-bit A1R5G5B5 true-color data
and RLE color-mapped decoding.
The two 16-bit grayscale TGA files are decode-only smoke fixtures
because their external PNG oracles are documented as incorrectly decoded.
The legacy TGA 2.0 footer references in these samples are not used as
metadata oracles because their postage-stamp payloads are truncated.

The source collection attributes these conformance files to historical
TrueVision TGA 2.0 materials without a specific redistribution license.
Keep this provenance with the fixtures when redistributing the repository.
