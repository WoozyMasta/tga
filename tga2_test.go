package tga

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
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
	min := binary.LittleEndian.Uint16(ext[tga2OffTimestamp+8 : tga2OffTimestamp+10])
	sec := binary.LittleEndian.Uint16(ext[tga2OffTimestamp+10 : tga2OffTimestamp+12])
	if mon != 3 || day != 16 || year != 2026 || hour != 14 || min != 5 || sec != 9 {
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
