# ImageMagick parity fixtures

These files were generated locally with ImageMagick 7.1.1-37 Q16-HDR.
They contain no third-party source images;
the pixel data comes from ImageMagick's built-in `plasma:fractal` generator.

The PNG file in each pair is the independent pixel oracle
and the TGA file is the ImageMagick TGA writer output.
ImageMagick is distributed under the Apache License 2.0;
these generated fixtures are retained only for tests.

Generation commands:

```shell
magick \
  -size 8x4 plasma:fractal \
  -type TrueColor \
  plasma24.png

magick plasma24.png \
  -type TrueColor \
  -define tga:bits-per-pixel=24 \
  plasma24.tga

magick \
  -size 7x3 plasma:fractal \
  -alpha on \
  -channel A \
  -evaluate set 70% \
  -type TrueColorAlpha \
  plasma32.png

magick plasma32.png \
  -type TrueColorAlpha \
  -define tga:bits-per-pixel=32 \
  plasma32.tga
```

Fixtures cover independent raw true-color 24-bit and 32-bit alpha pixel data.
Malformed inputs, palette origins,
and RLE boundary cases remain covered by deterministic synthetic tests.
