// SPDX-License-Identifier: MIT
// Copyright (c) 2026 WoozyMasta
// Source: github.com/woozymasta/tga

package tga

import (
	"bytes"
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
	Timestamp time.Time `json:"timestamp,omitzero"`
	// Thumbnail writes TGA 2.0 postage stamp block (24-bit BGR).
	Thumbnail image.Image `json:"thumbnail,omitempty"`
	// Author is stored in the 41-byte Author Name field.
	Author string `json:"author,omitempty"`
	// JobName is written to the Job Name/ID field.
	JobName string `json:"job_name,omitempty"`
	// SoftwareID is written to the Software ID field.
	SoftwareID string `json:"software_id,omitempty"`
	// Comments stores up to 4 lines, 81 bytes each.
	Comments []string `json:"comments,omitempty"`
	// DeveloperArea writes raw developer-area bytes.
	DeveloperArea []byte `json:"developer_area,omitempty"`
	// JobDuration is encoded as hours/minutes/seconds.
	JobDuration time.Duration `json:"job_duration,omitempty"`
	// Gamma writes gamma as rational value if > 0 and explicit ratio is unset.
	Gamma float64 `json:"gamma,omitempty"`
	// SoftwareVersion is written as numeric version value.
	SoftwareVersion uint16 `json:"software_version,omitempty"`
	// GammaNumerator overrides gamma ratio numerator when set with denominator.
	GammaNumerator uint16 `json:"gamma_numerator,omitempty"`
	// GammaDenominator overrides gamma ratio denominator when set with numerator.
	GammaDenominator uint16 `json:"gamma_denominator,omitempty"`
	// SoftwareVersionLetter is written next to SoftwareVersion.
	SoftwareVersionLetter byte `json:"software_version_letter,omitempty"`
	// AttributesType controls the image attribute type:
	//
	//   - 0 means automatic when encoding,
	//   - 1 means ignorable alpha,
	//   - 2 means preserve undefined alpha,
	//   - 3 means useful straight alpha,
	//   - 4 means useful premultiplied alpha.
	AttributesType byte `json:"attributes_type,omitempty"`
}

// Info contains metadata read by DecodeWithMetadata.
type Info struct {
	// Metadata contains the parsed TGA 2.0 extension area, if present.
	Metadata *TGA2Metadata `json:"metadata,omitempty"`
	// ImageID contains the optional TGA image identification field.
	ImageID []byte `json:"image_id,omitempty"`
	// DeveloperArea contains bytes between the developer-area offset and footer.
	DeveloperArea []byte `json:"developer_area,omitempty"`
	// HasFooter reports whether the TGA 2.0 footer signature was found.
	HasFooter bool `json:"has_footer"`
}

// DecodeWithMetadata decodes a seekable TGA stream and reads its TGA 2.0 metadata.
// AttributesType values 0 and 1 produce opaque pixels, 2 and 3 preserve straight
// alpha as *image.NRGBA, and 4 returns premultiplied pixels as *image.RGBA.
// The reader must implement io.ReadSeeker and is not closed by this function.
func DecodeWithMetadata(r io.ReadSeeker) (img image.Image, info Info, err error) {
	defer func() {
		err = wrapTruncated(err)
	}()

	// Read the ID first so metadata decoding can reuse the regular streaming decoder.
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, Info{}, err
	}
	var h [headerSize]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, Info{}, err
	}

	id := make([]byte, h[0])
	if _, err := io.ReadFull(r, id); err != nil {
		return nil, Info{}, err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, Info{}, err
	}

	// Decode validates and decodes pixels using the same path as Decode(io.Reader).
	img, err = Decode(r)
	if err != nil {
		return nil, Info{}, err
	}
	info = Info{ImageID: id}
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil || end < tga2FooterSize {
		return img, info, err
	}

	// A TGA 2.0 footer is optional; legacy files simply return the decoded image.
	if _, err = r.Seek(-tga2FooterSize, io.SeekEnd); err != nil {
		return nil, Info{}, err
	}

	footer := make([]byte, tga2FooterSize)
	if _, err = io.ReadFull(r, footer); err != nil {
		return nil, Info{}, err
	}
	if string(footer[8:]) != tga2FooterSignature {
		return img, info, nil
	}
	info.HasFooter = true

	// Footer offsets are file-relative.
	// Keep all subsequent reads inside the payload preceding the footer
	// before seeking to any offset supplied by input.
	extOffset := int64(binary.LittleEndian.Uint32(footer[:4]))
	devOffset := int64(binary.LittleEndian.Uint32(footer[4:8]))
	dataEnd := end - tga2FooterSize
	if extOffset != 0 {
		ext, err := readTGA2Extension(r, extOffset, dataEnd)
		if err != nil {
			return nil, Info{}, err
		}
		info.Metadata, err = parseTGA2Metadata(r, ext, extOffset, dataEnd)
		if err != nil {
			return nil, Info{}, err
		}
		img, err = applyTGA2AlphaSemantics(img, info.Metadata.AttributesType, h[17])
		if err != nil {
			return nil, Info{}, err
		}
	}

	if devOffset != 0 {
		// The writer emits the developer area immediately before the footer,
		// so its size is derived from the developer offset and the footer boundary.
		if devOffset >= dataEnd || (extOffset != 0 && devOffset < extOffset) {
			return nil, Info{}, ErrFormat
		}
		if _, err = r.Seek(devOffset, io.SeekStart); err != nil {
			return nil, Info{}, err
		}
		developerSize, err := checkedInt64ToInt(dataEnd - devOffset)
		if err != nil {
			return nil, Info{}, err
		}
		info.DeveloperArea = make([]byte, developerSize)
		if _, err = io.ReadFull(r, info.DeveloperArea); err != nil {
			return nil, Info{}, err
		}
	}

	return img, info, nil
}

// applyTGA2AlphaSemantics maps TGA 2.0 alpha attributes to Go image models.
func applyTGA2AlphaSemantics(img image.Image, attributesType, descriptor byte) (image.Image, error) {
	hasAlpha := descriptor&0x0f != 0
	switch attributesType {
	case 0, 1:
		// No alpha or ignorable alpha is represented as fully opaque pixels.
		if hasAlpha {
			return makeOpaqueNRGBA(img), nil
		}
		return img, nil

	case 2, 3:
		// Undefined-but-preserved and useful unassociated alpha are straight.
		if !hasAlpha {
			return nil, ErrFormat
		}
		return img, nil

	case 4:
		// Useful associated alpha is represented by the premultiplied RGBA model.
		if !hasAlpha {
			return nil, ErrFormat
		}
		return makePremultipliedRGBA(img), nil

	default:
		return nil, ErrFormat
	}
}

// makeOpaqueNRGBA returns a copy with alpha discarded and RGB preserved.
func makeOpaqueNRGBA(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			c.A = 0xff
			dst.SetNRGBA(x, y, c)
		}
	}
	return dst
}

// makePremultipliedRGBA preserves decoded channel bytes in image.RGBA.
func makePremultipliedRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	if nrgba, ok := src.(*image.NRGBA); ok {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := nrgba.NRGBAAt(x, y)
				dst.SetRGBA(x, y, color.RGBA(c))
			}
		}
		return dst
	}

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.SetRGBA(x, y, color.RGBAModel.Convert(src.At(x, y)).(color.RGBA))
		}
	}
	return dst
}

// readTGA2Extension reads and validates the fixed-size extension area.
func readTGA2Extension(r io.ReadSeeker, offset, dataEnd int64) ([]byte, error) {
	if offset < 0 || offset > dataEnd-int64(tga2ExtensionSize) {
		return nil, ErrFormat
	}

	if _, err := r.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	ext := make([]byte, tga2ExtensionSize)
	if _, err := io.ReadFull(r, ext); err != nil {
		return nil, err
	}
	if int64(binary.LittleEndian.Uint16(ext[:2])) != int64(tga2ExtensionSize) {
		return nil, ErrFormat
	}

	return ext, nil
}

// checkedInt64ToInt converts a file-derived size without overflowing int.
func checkedInt64ToInt(value int64) (int, error) {
	if value < 0 || value > int64(maxInt) {
		return 0, ErrFormat
	}

	// #nosec G115 -- value is bounded by maxInt above.
	return int(value), nil
}

// parseTGA2Metadata converts extension fields and reads the optional thumbnail.
func parseTGA2Metadata(r io.ReadSeeker, ext []byte, extOffset, dataEnd int64) (*TGA2Metadata, error) {
	meta := &TGA2Metadata{
		Author:                readASCIIZ(ext[tga2OffAuthor : tga2OffAuthor+41]),
		Comments:              readCommentLines(ext[tga2OffComments : tga2OffComments+324]),
		JobName:               readASCIIZ(ext[tga2OffJobName : tga2OffJobName+41]),
		SoftwareID:            readASCIIZ(ext[tga2OffSoftwareID : tga2OffSoftwareID+41]),
		SoftwareVersion:       binary.LittleEndian.Uint16(ext[tga2OffSoftwareVer : tga2OffSoftwareVer+2]),
		SoftwareVersionLetter: ext[tga2OffSoftwareVer+2],
		AttributesType:        ext[tga2OffAttrType],
	}

	// Zero timestamp fields represent an unset optional timestamp;
	// non-zero fields must round-trip through time.Date without normalization.
	timestamp, err := readTimestamp(ext[tga2OffTimestamp : tga2OffTimestamp+12])
	if err != nil {
		return nil, err
	}
	meta.Timestamp = timestamp

	// Validate minute and second ranges before converting the duration to nanoseconds.
	duration, err := readJobDuration(ext[tga2OffJobTime : tga2OffJobTime+6])
	if err != nil {
		return nil, err
	}

	meta.JobDuration = duration
	meta.GammaNumerator = binary.LittleEndian.Uint16(ext[tga2OffGammaNum : tga2OffGammaNum+2])
	meta.GammaDenominator = binary.LittleEndian.Uint16(ext[tga2OffGammaDen : tga2OffGammaDen+2])

	// Preserve the stored ratio and expose its numeric value when a denominator exists.
	if meta.GammaDenominator != 0 {
		meta.Gamma = float64(meta.GammaNumerator) / float64(meta.GammaDenominator)
	}

	postageOffset := int64(binary.LittleEndian.Uint32(ext[tga2OffPostageStamp : tga2OffPostageStamp+4]))
	if postageOffset != 0 {
		// A zero offset means no thumbnail.
		// Otherwise, constrain the offset and decoded size to the payload
		// before allocating the thumbnail buffer.
		if postageOffset < extOffset+int64(tga2ExtensionSize) ||
			postageOffset > dataEnd-2 {
			return nil, ErrFormat
		}

		if _, err := r.Seek(postageOffset, io.SeekStart); err != nil {
			return nil, err
		}

		var dimensions [2]byte
		if _, err := io.ReadFull(r, dimensions[:]); err != nil {
			return nil, err
		}

		size := int64(dimensions[0])*int64(dimensions[1])*3 + 2
		if size > dataEnd-postageOffset {
			return nil, ErrFormat
		}

		pixels := make([]byte, size-2)
		if _, err := io.ReadFull(r, pixels); err != nil {
			return nil, err
		}

		width := int(dimensions[0])
		height := int(dimensions[1])
		thumb := image.NewNRGBA(image.Rect(0, 0, width, height))
		for i, y := 0, 0; y < thumb.Bounds().Dy(); y++ {
			for x := 0; x < thumb.Bounds().Dx(); x++ {
				thumb.SetNRGBA(x, y, color.NRGBA{R: pixels[i+2], G: pixels[i+1], B: pixels[i], A: 255})
				i += 3
			}
		}

		meta.Thumbnail = thumb
	}

	return meta, nil
}

// readASCIIZ reads a fixed-width, zero-terminated TGA text field.
func readASCIIZ(src []byte) string {
	if end := bytes.IndexByte(src, 0); end >= 0 {
		src = src[:end]
	}

	return string(src)
}

// readCommentLines reads the four fixed-width comment fields.
func readCommentLines(src []byte) []string {
	comments := make([]string, 0, 4)
	for i := range 4 {
		line := readASCIIZ(src[i*81 : (i+1)*81])
		if line != "" {
			comments = append(comments, line)
		}
	}

	return comments
}

// readTimestamp decodes and validates the extension-area timestamp.
func readTimestamp(src []byte) (time.Time, error) {
	var parts [6]int
	for i := range parts {
		parts[i] = int(binary.LittleEndian.Uint16(src[i*2 : i*2+2]))
	}
	if parts == [6]int{} {
		return time.Time{}, nil
	}

	ts := time.Date(parts[2], time.Month(parts[0]), parts[1], parts[3], parts[4], parts[5], 0, time.Local)
	if ts.Month() != time.Month(parts[0]) ||
		ts.Day() != parts[1] ||
		ts.Year() != parts[2] ||
		ts.Hour() != parts[3] ||
		ts.Minute() != parts[4] ||
		ts.Second() != parts[5] {
		return time.Time{}, ErrFormat
	}

	return ts, nil
}

// readJobDuration decodes the extension-area hours/minutes/seconds fields.
func readJobDuration(src []byte) (time.Duration, error) {
	hours := int64(binary.LittleEndian.Uint16(src[0:2]))
	minutes := int64(binary.LittleEndian.Uint16(src[2:4]))
	seconds := int64(binary.LittleEndian.Uint16(src[4:6]))

	if minutes >= 60 || seconds >= 60 {
		return 0, ErrFormat
	}

	return time.Duration((hours*3600 + minutes*60 + seconds) * int64(time.Second)), nil
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

	extEnd, err := uint32FromInt64(int64(extOffset) + int64(tga2ExtensionSize))
	if err != nil {
		return err
	}
	postageEnd := int64(extEnd)
	postageOffset := uint32(0)
	if len(postageStamp) > 0 {
		postageOffset = extEnd
		postageEndOffset, err := uint32FromInt64(postageEnd + int64(len(postageStamp)))
		if err != nil {
			return err
		}
		postageEnd = int64(postageEndOffset)
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
		devOffset, err = uint32FromInt64(postageEnd)
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
