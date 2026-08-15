// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"bytes"
	"image/png"
	"os"
	"testing"
)

func TestExternalFtrvxmtrxFixtures(t *testing.T) {
	fixtures := []string{
		"monochrome8_bottom_left",
		"monochrome8_bottom_left_rle",
		"rgb24_bottom_left_rle",
		"rgb24_top_left",
		"rgb24_top_left_colormap",
		"rgb32_bottom_left",
		"rgb32_top_left_rle",
		"rgb32_top_left_rle_colormap",
	}

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			base := "testdata/external/" + name
			tgaData, err := os.ReadFile(base + ".tga")
			if err != nil {
				t.Fatal(err)
			}
			pngData, err := os.ReadFile(base + ".png")
			if err != nil {
				t.Fatal(err)
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
				t.Fatalf("TGA pixels differ from external PNG oracle")
			}
		})
	}
}

func TestExternalConformanceSmokeFixtures(t *testing.T) {
	fixtures := []struct {
		name      string
		wantType  byte
		wantDepth byte
		width     int
		height    int
	}{
		{name: "ccm8", wantType: typeRLEPaletted, wantDepth: 8, width: 128, height: 128},
		{name: "monochrome16_top_left", wantType: typeGrayscale, wantDepth: 16, width: 64, height: 64},
		{name: "monochrome16_top_left_rle", wantType: typeRLEGrayscale, wantDepth: 16, width: 64, height: 64},
		{name: "utc16", wantType: typeTrueColor, wantDepth: 16, width: 128, height: 128},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			path := "testdata/external/" + fixture.name + ".tga"
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) < 18 {
				t.Fatalf("fixture is shorter than the TGA header: %d bytes", len(data))
			}
			if data[2] != fixture.wantType || data[16] != fixture.wantDepth {
				t.Fatalf("header type/depth = %d/%d, want %d/%d", data[2], data[16], fixture.wantType, fixture.wantDepth)
			}

			config, err := DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("DecodeConfig: %v", err)
			}
			if config.Width != fixture.width || config.Height != fixture.height {
				t.Fatalf("config size = %dx%d, want %dx%d", config.Width, config.Height, fixture.width, fixture.height)
			}
			if _, err := Decode(bytes.NewReader(data)); err != nil {
				t.Fatalf("Decode: %v", err)
			}
		})
	}
}
