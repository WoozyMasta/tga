// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEncodeWithOptions_TGA2MetadataWritesFooterAndAreas(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 255})

	thumb := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	thumb.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	thumb.SetNRGBA(1, 0, color.NRGBA{R: 40, G: 50, B: 60, A: 255})

	ts := time.Date(2026, time.March, 16, 14, 5, 9, 0, time.Local)
	dev := []byte{0xde, 0xad, 0xbe, 0xef}
	meta := &TGA2Metadata{
		Author:                "Woozy",
		Comments:              []string{"line-1", "line-2"},
		Timestamp:             ts,
		SoftwareID:            "tga-tests",
		Gamma:                 2.2,
		AttributesType:        3,
		Thumbnail:             thumb,
		DeveloperArea:         dev,
		SoftwareVersion:       100,
		SoftwareVersionLetter: 'a',
	}

	var buf bytes.Buffer
	err := EncodeWithOptions(&buf, img, &EncodeOptions{
		ImageID:  []byte("image-id"),
		Metadata: meta,
	})
	if err != nil {
		t.Fatalf("EncodeWithOptions metadata: %v", err)
	}

	data := buf.Bytes()
	if len(data) < tga2FooterSize {
		t.Fatalf("encoded data too short: %d", len(data))
	}

	footer := data[len(data)-tga2FooterSize:]
	if string(footer[8:26]) != tga2FooterSignature {
		t.Fatalf("missing TGA 2.0 footer signature")
	}

	extOffset := binary.LittleEndian.Uint32(footer[0:4])
	devOffset := binary.LittleEndian.Uint32(footer[4:8])
	if extOffset == 0 {
		t.Fatal("extension offset is zero")
	}
	if devOffset == 0 {
		t.Fatal("developer offset is zero")
	}

	extStart := int(extOffset)
	extEnd := extStart + tga2ExtensionSize
	if extStart < 0 || extEnd > len(data) {
		t.Fatalf("extension area offset out of range")
	}

	ext := data[extStart:extEnd]
	if got := binary.LittleEndian.Uint16(ext[0:2]); got != tga2ExtensionSize {
		t.Fatalf("extension size=%d, want=%d", got, tga2ExtensionSize)
	}
	if !strings.HasPrefix(string(ext[tga2OffAuthor:tga2OffAuthor+41]), "Woozy") {
		t.Fatalf("author field mismatch")
	}

	mon := binary.LittleEndian.Uint16(ext[tga2OffTimestamp+0 : tga2OffTimestamp+2])
	day := binary.LittleEndian.Uint16(ext[tga2OffTimestamp+2 : tga2OffTimestamp+4])
	year := binary.LittleEndian.Uint16(ext[tga2OffTimestamp+4 : tga2OffTimestamp+6])
	hour := binary.LittleEndian.Uint16(ext[tga2OffTimestamp+6 : tga2OffTimestamp+8])
	minute := binary.LittleEndian.Uint16(ext[tga2OffTimestamp+8 : tga2OffTimestamp+10])
	sec := binary.LittleEndian.Uint16(ext[tga2OffTimestamp+10 : tga2OffTimestamp+12])
	if mon != 3 || day != 16 || year != 2026 || hour != 14 || minute != 5 || sec != 9 {
		t.Fatalf("timestamp fields mismatch")
	}

	gammaNum := binary.LittleEndian.Uint16(ext[tga2OffGammaNum : tga2OffGammaNum+2])
	gammaDen := binary.LittleEndian.Uint16(ext[tga2OffGammaDen : tga2OffGammaDen+2])
	if gammaNum == 0 || gammaDen == 0 {
		t.Fatalf("gamma was not written")
	}
	if ext[tga2OffAttrType] != 3 {
		t.Fatalf("attributes type mismatch: %d", ext[tga2OffAttrType])
	}

	postageOffset := binary.LittleEndian.Uint32(
		ext[tga2OffPostageStamp : tga2OffPostageStamp+4],
	)
	if postageOffset == 0 {
		t.Fatalf("postage stamp offset is zero")
	}

	postageStart := int(postageOffset)
	if postageStart < 0 || postageStart+2 > len(data) {
		t.Fatalf("postage area offset out of range")
	}

	postage := data[postageStart:]
	if postage[0] != 2 || postage[1] != 1 {
		t.Fatalf("postage dimensions mismatch: %dx%d", postage[0], postage[1])
	}

	devStart := int(devOffset)
	devEnd := devStart + len(dev)
	if devStart < 0 || devEnd > len(data) {
		t.Fatalf("developer area offset out of range")
	}

	if !bytes.Equal(data[devStart:devEnd], dev) {
		t.Fatalf("developer area payload mismatch")
	}

	decoded, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode with metadata tail: %v", err)
	}
	if !imagesEqual(img, decoded) {
		t.Fatalf("decoded image mismatch with metadata tail")
	}

	decoded, info, err := DecodeWithMetadata(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeWithMetadata: %v", err)
	}
	if !imagesEqual(img, decoded) {
		t.Fatalf("metadata decoded image mismatch")
	}
	if !bytes.Equal(info.ImageID, []byte("image-id")) || !info.HasFooter || info.Metadata == nil {
		t.Fatalf("metadata info missing: %+v", info)
	}
	got := info.Metadata
	if got.Author != meta.Author || got.JobName != meta.JobName || got.SoftwareID != meta.SoftwareID ||
		got.SoftwareVersion != meta.SoftwareVersion || got.SoftwareVersionLetter != meta.SoftwareVersionLetter ||
		got.AttributesType != meta.AttributesType || len(got.Comments) != len(meta.Comments) {
		t.Fatalf("metadata fields mismatch: got=%+v want=%+v", got, meta)
	}
	if !got.Timestamp.Equal(meta.Timestamp) || got.JobDuration != meta.JobDuration || got.Gamma != meta.Gamma {
		t.Fatalf("metadata numeric fields mismatch: got=%+v want=%+v", got, meta)
	}
	if !imagesEqual(meta.Thumbnail, got.Thumbnail) {
		t.Fatalf("thumbnail mismatch")
	}
	if !bytes.Equal(info.DeveloperArea, dev) {
		t.Fatalf("developer area mismatch: got=%x want=%x", info.DeveloperArea, dev)
	}
}

func TestDecodeWithMetadata_RejectsInvalidOffsets(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	var buf bytes.Buffer
	if err := EncodeWithOptions(&buf, img, &EncodeOptions{Metadata: &TGA2Metadata{Author: "test"}}); err != nil {
		t.Fatalf("EncodeWithOptions: %v", err)
	}
	data := append([]byte(nil), buf.Bytes()...)
	footer := len(data) - tga2FooterSize
	binary.LittleEndian.PutUint32(data[footer:footer+4], uint32(len(data)))
	if _, _, err := DecodeWithMetadata(bytes.NewReader(data)); err != ErrFormat {
		t.Fatalf("invalid extension offset error=%v, want=%v", err, ErrFormat)
	}

	data = append([]byte(nil), buf.Bytes()...)
	binary.LittleEndian.PutUint32(data[footer:footer+4], 1)
	if _, _, err := DecodeWithMetadata(bytes.NewReader(data)); err != ErrFormat {
		t.Fatalf("invalid extension size error=%v, want=%v", err, ErrFormat)
	}
}

func TestDecodeWithMetadata_AlphaAttributes(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 100, G: 50, B: 25, A: 128})

	tests := []struct {
		name       string
		attributes byte
		want       color.NRGBA
		wantType   any
	}{
		{name: "none", attributes: 0, want: color.NRGBA{R: 100, G: 50, B: 25, A: 255}, wantType: &image.NRGBA{}},
		{name: "ignore", attributes: 1, want: color.NRGBA{R: 100, G: 50, B: 25, A: 255}, wantType: &image.NRGBA{}},
		{name: "preserve", attributes: 2, want: color.NRGBA{R: 100, G: 50, B: 25, A: 128}, wantType: &image.NRGBA{}},
		{name: "straight", attributes: 3, want: color.NRGBA{R: 100, G: 50, B: 25, A: 128}, wantType: &image.NRGBA{}},
		{name: "premultiplied", attributes: 4, want: color.NRGBA{R: 100, G: 50, B: 25, A: 128}, wantType: &image.RGBA{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := EncodeWithOptions(&buf, src, &EncodeOptions{
				Metadata: &TGA2Metadata{AttributesType: 3},
			})
			if err != nil {
				t.Fatalf("EncodeWithOptions: %v", err)
			}

			data := append([]byte(nil), buf.Bytes()...)
			footer := len(data) - tga2FooterSize
			extOffset := binary.LittleEndian.Uint32(data[footer : footer+4])
			data[extOffset+uint32(tga2OffAttrType)] = test.attributes
			decoded, _, err := DecodeWithMetadata(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("DecodeWithMetadata: %v", err)
			}
			if reflect.TypeOf(decoded) != reflect.TypeOf(test.wantType) {
				t.Fatalf("decoded type=%T, want=%T", decoded, test.wantType)
			}
			if test.attributes == 4 {
				got := decoded.(*image.RGBA).RGBAAt(0, 0)
				want := color.RGBA(test.want)
				if got != want {
					t.Fatalf("decoded pixel=%+v, want=%+v", got, want)
				}
			} else if got := color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA); got != test.want {
				t.Fatalf("decoded pixel=%+v, want=%+v", got, test.want)
			}
		})
	}

	var buf bytes.Buffer
	if err := EncodeWithOptions(&buf, src, &EncodeOptions{
		PixelDepth: 24,
		Metadata:   &TGA2Metadata{},
	}); err != nil {
		t.Fatalf("EncodeWithOptions without alpha: %v", err)
	}
	data := append([]byte(nil), buf.Bytes()...)
	footer := len(data) - tga2FooterSize
	extOffset := binary.LittleEndian.Uint32(data[footer : footer+4])
	data[extOffset+uint32(tga2OffAttrType)] = 3
	if _, _, err := DecodeWithMetadata(bytes.NewReader(data)); err != ErrFormat {
		t.Fatalf("missing descriptor alpha error=%v, want=%v", err, ErrFormat)
	}
}

func TestEncodeWithOptions_RejectsInvalidMetadataBeforeWriting(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	tooLargeThumbnail := image.NewNRGBA(image.Rect(0, 0, 256, 1))
	tests := []struct {
		name string
		meta TGA2Metadata
	}{
		{name: "author too long", meta: TGA2Metadata{Author: strings.Repeat("a", 41)}},
		{name: "author contains NUL", meta: TGA2Metadata{Author: "a\x00b"}},
		{name: "too many comments", meta: TGA2Metadata{Comments: []string{"1", "2", "3", "4", "5"}}},
		{name: "comment too long", meta: TGA2Metadata{Comments: []string{strings.Repeat("a", 81)}}},
		{name: "job name too long", meta: TGA2Metadata{JobName: strings.Repeat("a", 41)}},
		{name: "software ID too long", meta: TGA2Metadata{SoftwareID: strings.Repeat("a", 41)}},
		{name: "negative gamma", meta: TGA2Metadata{Gamma: -1}},
		{name: "non-finite gamma", meta: TGA2Metadata{Gamma: math.Inf(1)}},
		{name: "NaN gamma", meta: TGA2Metadata{Gamma: math.NaN()}},
		{name: "partial gamma ratio", meta: TGA2Metadata{GammaNumerator: 1}},
		{name: "conflicting gamma values", meta: TGA2Metadata{Gamma: 2.2, GammaNumerator: 2, GammaDenominator: 1}},
		{name: "gamma out of range", meta: TGA2Metadata{Gamma: 65.536}},
		{name: "timestamp precision", meta: TGA2Metadata{Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 1, time.UTC)}},
		{name: "timestamp year", meta: TGA2Metadata{Timestamp: time.Date(65536, 1, 1, 0, 0, 0, 0, time.UTC)}},
		{name: "negative duration", meta: TGA2Metadata{JobDuration: -time.Second}},
		{name: "duration precision", meta: TGA2Metadata{JobDuration: 500 * time.Millisecond}},
		{name: "duration hours", meta: TGA2Metadata{JobDuration: 65536 * time.Hour}},
		{name: "attribute type", meta: TGA2Metadata{AttributesType: 5}},
		{name: "thumbnail dimensions", meta: TGA2Metadata{Thumbnail: tooLargeThumbnail}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := EncodeWithOptions(&buf, img, &EncodeOptions{Metadata: &test.meta})
			if err == nil {
				t.Fatal("expected metadata validation error")
			}
			if !errors.Is(err, ErrMetadata) || !errors.Is(err, ErrFormat) {
				t.Fatalf("error=%v, want ErrMetadata and ErrFormat", err)
			}
			if buf.Len() != 0 {
				t.Fatalf("validation wrote %d bytes", buf.Len())
			}
		})
	}
}

func TestWriteTGA2Tail_RejectsOffsetOverflowBeforeWriting(t *testing.T) {
	var buf bytes.Buffer
	cw := &countingWriter{
		w: &buf,
		n: int64(math.MaxUint32) - int64(tga2ExtensionSize) + 1,
	}
	err := writeTGA2Tail(cw, &TGA2Metadata{})
	if !errors.Is(err, ErrFormat) {
		t.Fatalf("error=%v, want ErrFormat", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("offset validation wrote %d bytes", buf.Len())
	}
}

func TestEncode_DefaultKeepsNoTGA2Footer(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))

	var buf bytes.Buffer
	if err := Encode(&buf, img); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	data := buf.Bytes()
	if len(data) >= tga2FooterSize {
		tail := data[len(data)-tga2FooterSize:]
		if string(tail[8:26]) == tga2FooterSignature {
			t.Fatalf("unexpected TGA 2.0 footer in default encoding")
		}
	}
}
