// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"testing"
)

func TestImageMagickParityFixtures(t *testing.T) {
	fixtures := []struct {
		name  string
		depth byte
		alpha byte
	}{
		{name: "plasma24", depth: 24},
		{name: "plasma32", depth: 32, alpha: 8},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			tgaData, err := os.ReadFile("testdata/parity/imagemagick/" + fixture.name + ".tga")
			if err != nil {
				t.Fatal(err)
			}
			pngData, err := os.ReadFile("testdata/parity/imagemagick/" + fixture.name + ".png")
			if err != nil {
				t.Fatal(err)
			}
			if len(tgaData) < 18 || tgaData[2] != 2 ||
				tgaData[16] != fixture.depth ||
				tgaData[17]&0x0f != fixture.alpha {
				t.Fatalf("unexpected TGA header: type=%d depth=%d alpha=%d", tgaData[2], tgaData[16], tgaData[17]&0x0f)
			}

			got, err := Decode(bytes.NewReader(tgaData))
			if err != nil {
				t.Fatalf("Decode TGA: %v", err)
			}
			want, err := png.Decode(bytes.NewReader(pngData))
			if err != nil {
				t.Fatalf("Decode PNG oracle: %v", err)
			}
			if !parityImagesEqual(got, want) {
				t.Fatalf("TGA pixels differ from ImageMagick PNG oracle; got=%T bounds=%v want=%T bounds=%v", got, got.Bounds(), want, want.Bounds())
			}
		})
	}
}

func parityChannelDelta(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

func parityImagesEqual(a, b image.Image) bool {
	if !a.Bounds().Eq(b.Bounds()) {
		return false
	}

	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			if parityChannelDelta(ar>>8, br>>8) > 1 ||
				parityChannelDelta(ag>>8, bg>>8) > 1 ||
				parityChannelDelta(ab>>8, bb>>8) > 1 ||
				parityChannelDelta(aa>>8, ba>>8) > 1 {
				return false
			}
		}
	}

	return true
}
