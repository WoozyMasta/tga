package tga

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"
)

func TestEncode_ZeroSize(t *testing.T) {
	empty := image.NewNRGBA(image.Rect(0, 0, 0, 0))
	err := Encode(bytes.NewBuffer(nil), empty)
	if err == nil {
		t.Fatal("expected error for zero-size image")
	}
	if !errors.Is(err, ErrFormat) {
		t.Errorf("expected ErrFormat, got %v", err)
	}

	zeroH := image.NewNRGBA(image.Rect(0, 0, 10, 0))
	if err := Encode(bytes.NewBuffer(nil), zeroH); err == nil {
		t.Fatal("expected error for zero height")
	} else if !errors.Is(err, ErrFormat) {
		t.Errorf("expected ErrFormat, got %v", err)
	}

	zeroW := image.NewNRGBA(image.Rect(0, 0, 0, 10))
	if err := Encode(bytes.NewBuffer(nil), zeroW); err == nil {
		t.Fatal("expected error for zero width")
	} else if !errors.Is(err, ErrFormat) {
		t.Errorf("expected ErrFormat, got %v", err)
	}
}

func TestEncode_TooLarge(t *testing.T) {
	// Width or height > 65535 must be rejected (uint16 overflow in header)
	wide := image.NewNRGBA(image.Rect(0, 0, 70000, 1))
	err := Encode(bytes.NewBuffer(nil), wide)
	if err == nil {
		t.Fatal("expected error for width > 65535")
	}
	if err != ErrFormat {
		t.Errorf("expected ErrFormat, got %v", err)
	}

	tall := image.NewNRGBA(image.Rect(0, 0, 1, 70000))
	err = Encode(bytes.NewBuffer(nil), tall)
	if err == nil {
		t.Fatal("expected error for height > 65535")
	}
}

func TestEncode_OtherImageType(t *testing.T) {
	// *image.RGBA and other types go through default path (convert to NRGBA via draw.Draw)
	rgba := image.NewRGBA(image.Rect(0, 0, 4, 4))
	rgba.Set(1, 1, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	var buf bytes.Buffer
	if err := Encode(&buf, rgba); err != nil {
		t.Fatalf("Encode RGBA: %v", err)
	}
	dec, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	// Decoded as NRGBA; compare RGBA at (1,1)
	r0, g0, b0, a0 := rgba.At(1, 1).RGBA()
	r1, g1, b1, a1 := dec.At(1, 1).RGBA()
	if r0 != r1 || g0 != g1 || b0 != b1 || a0 != a1 {
		t.Errorf("pixel (1,1): got %v, want %v", dec.At(1, 1), rgba.At(1, 1))
	}
}

func TestEncode_NonZeroBounds(t *testing.T) {
	// Image with Min != (0,0) must encode the visible rectangle correctly
	nrgba := image.NewNRGBA(image.Rect(5, 5, 15, 15))
	nrgba.SetNRGBA(5, 5, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	nrgba.SetNRGBA(14, 14, color.NRGBA{R: 4, G: 5, B: 6, A: 255})

	var buf bytes.Buffer
	if err := Encode(&buf, nrgba); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	dec, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if dec.Bounds().Dx() != 10 || dec.Bounds().Dy() != 10 {
		t.Errorf("decoded bounds: got %v", dec.Bounds())
	}
	// Decoded image has Rect(0,0,10,10); pixel that was at (5,5) is now at (0,0)
	c := dec.(*image.NRGBA).NRGBAAt(0, 0)
	if c.R != 1 || c.G != 2 || c.B != 3 {
		t.Errorf("pixel (0,0): got %v", c)
	}
	c = dec.(*image.NRGBA).NRGBAAt(9, 9)
	if c.R != 4 || c.G != 5 || c.B != 6 {
		t.Errorf("pixel (9,9): got %v", c)
	}
}
