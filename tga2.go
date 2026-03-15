// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"encoding/binary"
	"image"
	"image/color"
	"io"
	"math"
	"time"
)

const (
	tga2ExtensionSize = 495
	tga2FooterSize    = 26

	tga2OffAuthor       = 2
	tga2OffComments     = 43
	tga2OffTimestamp    = 367
	tga2OffJobName      = 379
	tga2OffJobTime      = 420
	tga2OffSoftwareID   = 426
	tga2OffSoftwareVer  = 467
	tga2OffGammaNum     = 478
	tga2OffGammaDen     = 480
	tga2OffPostageStamp = 486
	tga2OffAttrType     = 494
)

const tga2FooterSignature = "TRUEVISION-XFILE.\x00"

// TGA2Metadata describes optional TGA 2.0 extension/developer metadata.
type TGA2Metadata struct {
	// Timestamp writes local date/time fields if non-zero.
	Timestamp time.Time
	// Thumbnail writes TGA 2.0 postage stamp block (24-bit BGR).
	Thumbnail image.Image
	// Author is stored in the 41-byte Author Name field.
	Author string
	// JobName is written to the Job Name/ID field.
	JobName string
	// SoftwareID is written to the Software ID field.
	SoftwareID string
	// Comments stores up to 4 lines, 81 bytes each.
	Comments []string
	// DeveloperArea writes raw developer-area bytes.
	DeveloperArea []byte
	// JobDuration is encoded as hours/minutes/seconds.
	JobDuration time.Duration
	// Gamma writes gamma as rational value if > 0 and explicit ratio is unset.
	Gamma float64
	// SoftwareVersion is written as numeric version value.
	SoftwareVersion uint16
	// GammaNumerator overrides gamma ratio numerator when set with denominator.
	GammaNumerator uint16
	// GammaDenominator overrides gamma ratio denominator when set with numerator.
	GammaDenominator uint16
	// SoftwareVersionLetter is written next to SoftwareVersion.
	SoftwareVersionLetter byte
	// AttributesType writes image attribute type byte.
	AttributesType byte
}

// countingWriter wraps io.Writer and tracks written bytes.
type countingWriter struct {
	w io.Writer
	n int64
}

// Write writes bytes and increments written counter.
func (cw *countingWriter) Write(p []byte) (int, error) {
	written, err := cw.w.Write(p)
	cw.n += int64(written)
	return written, err
}

// writeTGA2Tail writes optional extension/developer areas and required footer.
func writeTGA2Tail(w *countingWriter, meta *TGA2Metadata) error {
	if meta == nil {
		return nil
	}

	extOffset, err := uint32FromInt64(w.n)
	if err != nil {
		return err
	}

	postageStamp, err := buildPostageStamp(meta.Thumbnail)
	if err != nil {
		return err
	}

	postageOffset := uint32(0)
	if len(postageStamp) > 0 {
		postageOffset = extOffset + tga2ExtensionSize
	}

	ext := buildExtensionArea(meta, postageOffset)
	if _, err := w.Write(ext); err != nil {
		return err
	}

	if len(postageStamp) > 0 {
		if _, err := w.Write(postageStamp); err != nil {
			return err
		}
	}

	devOffset := uint32(0)
	if len(meta.DeveloperArea) > 0 {
		devOffset, err = uint32FromInt64(w.n)
		if err != nil {
			return err
		}

		if _, err := w.Write(meta.DeveloperArea); err != nil {
			return err
		}
	}

	return writeFooter(w, extOffset, devOffset)
}

// writeFooter writes TGA 2.0 footer with extension/developer offsets.
func writeFooter(w io.Writer, extOffset, devOffset uint32) error {
	footer := make([]byte, tga2FooterSize)
	binary.LittleEndian.PutUint32(footer[0:4], extOffset)
	binary.LittleEndian.PutUint32(footer[4:8], devOffset)
	copy(footer[8:], []byte(tga2FooterSignature))

	_, err := w.Write(footer)
	return err
}

// buildExtensionArea builds a fixed-size TGA 2.0 extension area block.
func buildExtensionArea(meta *TGA2Metadata, postageOffset uint32) []byte {
	ext := make([]byte, tga2ExtensionSize)
	binary.LittleEndian.PutUint16(ext[0:2], tga2ExtensionSize)

	writeASCIIZ(ext[tga2OffAuthor:tga2OffAuthor+41], meta.Author)
	writeCommentLines(ext[tga2OffComments:tga2OffComments+324], meta.Comments)
	writeTimestamp(ext[tga2OffTimestamp:tga2OffTimestamp+12], meta.Timestamp)
	writeASCIIZ(ext[tga2OffJobName:tga2OffJobName+41], meta.JobName)
	writeJobDuration(ext[tga2OffJobTime:tga2OffJobTime+6], meta.JobDuration)
	writeASCIIZ(ext[tga2OffSoftwareID:tga2OffSoftwareID+41], meta.SoftwareID)
	binary.LittleEndian.PutUint16(ext[tga2OffSoftwareVer:tga2OffSoftwareVer+2], meta.SoftwareVersion)
	ext[tga2OffSoftwareVer+2] = meta.SoftwareVersionLetter

	if num, den, ok := resolveGamma(meta); ok {
		binary.LittleEndian.PutUint16(ext[tga2OffGammaNum:tga2OffGammaNum+2], num)
		binary.LittleEndian.PutUint16(ext[tga2OffGammaDen:tga2OffGammaDen+2], den)
	}

	binary.LittleEndian.PutUint32(ext[tga2OffPostageStamp:tga2OffPostageStamp+4], postageOffset)
	ext[tga2OffAttrType] = meta.AttributesType

	return ext
}

// resolveGamma returns gamma ratio from metadata fields.
func resolveGamma(meta *TGA2Metadata) (uint16, uint16, bool) {
	if meta.GammaNumerator > 0 && meta.GammaDenominator > 0 {
		return meta.GammaNumerator, meta.GammaDenominator, true
	}

	if meta.Gamma <= 0 || math.IsNaN(meta.Gamma) || math.IsInf(meta.Gamma, 0) {
		return 0, 0, false
	}

	const den = 1000.0
	numFloat := math.Round(meta.Gamma * den)
	if numFloat <= 0 || numFloat > math.MaxUint16 {
		return 0, 0, false
	}

	// numFloat range is checked above.
	// #nosec G115 -- conversion is bounded.
	num := uint16(numFloat)
	return num, uint16(den), true
}

// writeASCIIZ writes a zero-terminated field with truncation.
func writeASCIIZ(dst []byte, src string) {
	if len(dst) == 0 {
		return
	}

	limit := min(len(dst)-1, len(src))
	copy(dst[:limit], src[:limit])
}

// writeCommentLines writes up to 4 comment lines into 4x81-byte field.
func writeCommentLines(dst []byte, comments []string) {
	lineSize := 81
	for i := 0; i < 4 && i < len(comments); i++ {
		start := i * lineSize
		writeASCIIZ(dst[start:start+lineSize], comments[i])
	}
}

// writeTimestamp writes local date/time to extension timestamp field.
func writeTimestamp(dst []byte, ts time.Time) {
	if ts.IsZero() {
		return
	}

	parts := []int{
		int(ts.Month()),
		ts.Day(),
		ts.Year(),
		ts.Hour(),
		ts.Minute(),
		ts.Second(),
	}

	for i, part := range parts {
		v, err := uint16FromInt(part)
		if err != nil {
			return
		}
		binary.LittleEndian.PutUint16(dst[i*2:i*2+2], v)
	}
}

// writeJobDuration writes job duration as hours/minutes/seconds.
func writeJobDuration(dst []byte, d time.Duration) {
	if d <= 0 {
		return
	}

	totalSeconds := int(d / time.Second)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	parts := []int{hours, minutes, seconds}
	for i, part := range parts {
		v, err := uint16FromInt(part)
		if err != nil {
			return
		}
		binary.LittleEndian.PutUint16(dst[i*2:i*2+2], v)
	}
}

// buildPostageStamp builds TGA 2.0 postage stamp block from image.
func buildPostageStamp(img image.Image) ([]byte, error) {
	if img == nil {
		return nil, nil
	}

	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w <= 0 || h <= 0 {
		return nil, ErrFormat
	}
	if w > 255 || h > 255 {
		return nil, ErrFormat
	}

	wb, err := byteFromInt(w)
	if err != nil {
		return nil, err
	}
	hb, err := byteFromInt(h)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 2+w*h*3)
	out[0] = wb
	out[1] = hb

	di := 2
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			nc := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			out[di+0] = nc.B
			out[di+1] = nc.G
			out[di+2] = nc.R
			di += 3
		}
	}

	return out, nil
}

// uint16FromInt converts checked int to uint16.
func uint16FromInt(v int) (uint16, error) {
	if v < 0 || v > math.MaxUint16 {
		return 0, ErrFormat
	}

	// Range is checked above.
	// #nosec G115 -- conversion is bounded.
	return uint16(v), nil
}

// byteFromInt converts checked int to byte.
func byteFromInt(v int) (byte, error) {
	if v < 0 || v > math.MaxUint8 {
		return 0, ErrFormat
	}

	// Range is checked above.
	// #nosec G115 -- conversion is bounded.
	return byte(v), nil
}

// uint32FromInt64 converts checked int64 to uint32.
func uint32FromInt64(v int64) (uint32, error) {
	if v < 0 || v > math.MaxUint32 {
		return 0, ErrFormat
	}

	// Range is checked above.
	// #nosec G115 -- conversion is bounded.
	return uint32(v), nil
}
