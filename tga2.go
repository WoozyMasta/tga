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
	tga2ExtensionSize = 495 // tga2ExtensionSize is the fixed extension-area size.
	tga2FooterSize    = 26  // tga2FooterSize is the footer size including its signature.

	tga2OffAuthor       = 2   // tga2OffAuthor points to the author name field.
	tga2OffComments     = 43  // tga2OffComments points to the four comment lines.
	tga2OffTimestamp    = 367 // tga2OffTimestamp points to the date/time fields.
	tga2OffJobName      = 379 // tga2OffJobName points to the job name or ID field.
	tga2OffJobTime      = 420 // tga2OffJobTime points to the job duration fields.
	tga2OffSoftwareID   = 426 // tga2OffSoftwareID points to the software ID field.
	tga2OffSoftwareVer  = 467 // tga2OffSoftwareVer points to the software version.
	tga2OffKeyColor     = 470 // tga2OffKeyColor points to the A:R:G:B key color.
	tga2OffPixelAspect  = 474 // tga2OffPixelAspect points to the pixel aspect ratio.
	tga2OffGammaNum     = 478 // tga2OffGammaNum points to the gamma numerator.
	tga2OffGammaDen     = 480 // tga2OffGammaDen points to the gamma denominator.
	tga2OffColorCorrect = 482 // tga2OffColorCorrect points to the color correction table.
	tga2OffPostageStamp = 486 // tga2OffPostageStamp points to the postage stamp.
	tga2OffScanLine     = 490 // tga2OffScanLine points to the scan-line table.
	tga2OffAttrType     = 494 // tga2OffAttrType stores the image attribute type.
)

const tga2FooterSignature = "TRUEVISION-XFILE.\x00"

const (
	tga2ColorCorrectionEntries = 256
	tga2ColorCorrectionSize    = tga2ColorCorrectionEntries * 4 * 2
)

// Info contains metadata read by DecodeWithMetadata.
type Info struct {
	// Metadata contains the parsed TGA 2.0 extension area, if present.
	Metadata *TGA2Metadata `json:"metadata,omitempty"`
	// ImageID contains the optional TGA image identification field.
	ImageID []byte `json:"image_id,omitempty"`
	// DeveloperFields contains fields parsed from the TGA 2.0 developer directory.
	DeveloperFields []DeveloperField `json:"developer_fields,omitempty"`
	// DeveloperArea is a deprecated compatibility view
	// containing field data concatenated in directory order.
	DeveloperArea []byte `json:"developer_area,omitempty"`
	// HasFooter reports whether the TGA 2.0 footer signature was found.
	HasFooter bool `json:"has_footer"`
}

// TGA2Metadata describes optional TGA 2.0 extension/developer metadata.
type TGA2Metadata struct {
	// Timestamp writes local date/time fields if non-zero.
	Timestamp time.Time `json:"timestamp,omitzero"`
	// Thumbnail writes an uncompressed TGA 2.0 postage stamp in the main image format.
	Thumbnail image.Image `json:"thumbnail,omitempty"`
	// Author is stored in the 41-byte Author Name field.
	Author string `json:"author,omitempty"`
	// JobName is written to the Job Name/ID field.
	JobName string `json:"job_name,omitempty"`
	// SoftwareID is written to the Software ID field.
	SoftwareID string `json:"software_id,omitempty"`
	// Comments stores up to 4 lines, 81 bytes each.
	Comments []string `json:"comments,omitempty"`
	// ColorCorrectionTable contains exactly 256 A:R:G:B entries when present.
	ColorCorrectionTable []TGA2ColorCorrectionEntry `json:"color_correction_table,omitempty"`
	// ScanLineTable contains file-relative offsets for image scan lines.
	ScanLineTable []uint32 `json:"scan_line_table,omitempty"`
	// DeveloperFields contains the TGA 2.0 developer fields and their tags.
	DeveloperFields []DeveloperField `json:"developer_fields,omitempty"`
	// DeveloperArea is a deprecated compatibility field. When DeveloperFields
	// is empty, its bytes are written as one field with tag zero.
	DeveloperArea []byte `json:"developer_area,omitempty"`
	// JobDuration is encoded as hours/minutes/seconds.
	JobDuration time.Duration `json:"job_duration,omitempty"`
	// Gamma writes gamma as rational value if > 0 and explicit ratio is unset.
	Gamma float64 `json:"gamma,omitempty"`
	// PixelAspectRatio describes the pixel width-to-height ratio.
	PixelAspectRatio TGA2PixelAspectRatio `json:"pixel_aspect_ratio"`
	// SoftwareVersion is written as numeric version value.
	SoftwareVersion uint16 `json:"software_version,omitempty"`
	// GammaNumerator overrides gamma ratio numerator when set with denominator.
	GammaNumerator uint16 `json:"gamma_numerator,omitempty"`
	// GammaDenominator overrides gamma ratio denominator when set with numerator.
	GammaDenominator uint16 `json:"gamma_denominator,omitempty"`
	// KeyColor is the A:R:G:B key color from the extension area.
	KeyColor color.NRGBA `json:"key_color"`
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

// TGA2ColorCorrectionEntry contains one A:R:G:B correction-table entry.
type TGA2ColorCorrectionEntry struct {
	A uint16 `json:"a"` // A is the alpha correction value.
	R uint16 `json:"r"` // R is the red correction value.
	G uint16 `json:"g"` // G is the green correction value.
	B uint16 `json:"b"` // B is the blue correction value.
}

// DeveloperField describes one TGA 2.0 developer field.
type DeveloperField struct {
	Data []byte `json:"data,omitempty"` // Data contains the field payload.
	Tag  uint16 `json:"tag"`            // Tag identifies the application-defined field.
}

// TGA2PixelAspectRatio describes the pixel width-to-height ratio.
type TGA2PixelAspectRatio struct {
	// Numerator is the pixel width component.
	Numerator uint16 `json:"numerator,omitempty"`
	// Denominator is the pixel height component.
	Denominator uint16 `json:"denominator,omitempty"`
}

// postageStampFormat describes the uncompressed pixel format of a thumbnail.
type postageStampFormat struct {
	palette       color.Palette // palette contains the main image palette for indexed thumbnails.
	depth         int           // depth is the main image pixel depth in bits.
	colorMapStart int           // colorMapStart is the first file index declared by the main color map.
	imageHeight   int           // imageHeight is the number of scan lines in the main image.
	grayscale     bool          // grayscale selects luminance encoding instead of true-color encoding.
	alpha         bool          // alpha reports whether 16-bit true-color pixels contain an alpha bit.
}

// developerDirectoryEntry stores one serialized TGA 2.0 directory record.
type developerDirectoryEntry struct {
	offset uint32 // offset points to the field payload from the start of the file.
	size   uint32 // size is the field payload length in bytes.
	tag    uint16 // tag identifies the developer field.
}

// metadataBudget tracks allocations made while reading TGA 2.0 metadata.
type metadataBudget struct {
	limit uint64
	used  uint64
}

// DecodeWithMetadata decodes a seekable TGA stream and reads its TGA 2.0 metadata.
// AttributesType values 0 and 1 produce opaque pixels, 2 and 3 preserve straight
// alpha as *image.NRGBA, and 4 returns premultiplied pixels as *image.RGBA.
// The reader must implement io.ReadSeeker and is not closed by this function.
func DecodeWithMetadata(r io.ReadSeeker) (img image.Image, info Info, err error) {
	return DecodeWithMetadataOptions(r, DecodeOptions{})
}

// DecodeWithMetadataOptions is DecodeWithMetadata with resource limits.
// MaxDecodedBytes limits image pixels and MaxMetadataBytes limits TGA 2.0 data.
func DecodeWithMetadataOptions(r io.ReadSeeker, opts DecodeOptions) (img image.Image, info Info, err error) {
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
	img, err = DecodeWithOptions(r, opts)
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
	budget := metadataBudget{limit: opts.MaxMetadataBytes}
	if extOffset != 0 {
		if err = budget.reserve(tga2ExtensionSize); err != nil {
			return nil, Info{}, err
		}
		ext, err := readTGA2Extension(r, extOffset, dataEnd)
		if err != nil {
			return nil, Info{}, err
		}

		format := postageStampFormat{
			depth:       int(h[16]),
			alpha:       h[16] == 16 && h[17]&0x0f == 1,
			imageHeight: int(binary.LittleEndian.Uint16(h[14:16])),
		}
		switch h[2] {
		case typeGrayscale, typeRLEGrayscale:
			format.grayscale = true

		case typePaletted, typeRLEPaletted:
			paletted, ok := img.(*image.Paletted)
			if !ok {
				return nil, Info{}, ErrFormat
			}

			format.depth = 8
			format.palette = paletted.Palette
			format.colorMapStart = int(binary.LittleEndian.Uint16(h[3:5]))
		}

		info.Metadata, err = parseTGA2Metadata(r, ext, extOffset, dataEnd, format, &budget)
		if err != nil {
			return nil, Info{}, err
		}
		img, err = applyTGA2AlphaSemantics(
			img,
			info.Metadata.AttributesType,
			hasPhysicalAlpha(h),
			h[17]&0x0f != 0,
		)
		if err != nil {
			return nil, Info{}, err
		}
	}

	if devOffset != 0 {
		info.DeveloperFields, info.DeveloperArea, err = readDeveloperDirectory(r, devOffset, dataEnd, &budget)
		if err != nil {
			return nil, Info{}, err
		}
	}

	return img, info, nil
}

// reserve accounts for a metadata allocation and enforces the configured limit.
func (b *metadataBudget) reserve(size int64) error {
	if size < 0 {
		return ErrFormat
	}

	amount := uint64(size)
	if b.limit > 0 && (b.used > b.limit || amount > b.limit-b.used) {
		return ErrResourceLimit
	}
	b.used += amount

	return nil
}

// readDeveloperDirectory reads the directory and all fields it references.
func readDeveloperDirectory(r io.ReadSeeker, offset, dataEnd int64, budget *metadataBudget) ([]DeveloperField, []byte, error) {
	if offset < int64(headerSize) || offset > dataEnd-2 {
		return nil, nil, ErrFormat
	}
	if _, err := r.Seek(offset, io.SeekStart); err != nil {
		return nil, nil, err
	}

	var countBytes [2]byte
	if _, err := io.ReadFull(r, countBytes[:]); err != nil {
		return nil, nil, err
	}
	count := int(binary.LittleEndian.Uint16(countBytes[:]))
	directorySize := int64(2) + int64(count)*10
	if directorySize > dataEnd-offset {
		return nil, nil, ErrFormat
	}
	if err := budget.reserve(directorySize); err != nil {
		return nil, nil, err
	}

	directory := make([]byte, directorySize-2)
	if _, err := io.ReadFull(r, directory); err != nil {
		return nil, nil, err
	}

	var fieldBytes int64
	fields := make([]DeveloperField, 0, count)
	for i := range count {
		entry := directory[i*10 : (i+1)*10]
		fieldOffset := int64(binary.LittleEndian.Uint32(entry[2:6]))
		fieldSize := int64(binary.LittleEndian.Uint32(entry[6:10]))
		if fieldOffset < int64(headerSize) || fieldOffset > dataEnd || fieldSize > dataEnd-fieldOffset {
			return nil, nil, ErrFormat
		}

		fieldBytes += fieldSize
	}
	if err := budget.reserve(fieldBytes * 2); err != nil {
		return nil, nil, err
	}

	areaSize, err := checkedInt64ToInt(fieldBytes)
	if err != nil {
		return nil, nil, err
	}

	area := make([]byte, 0, areaSize)
	for i := range count {
		entry := directory[i*10 : (i+1)*10]
		fieldOffset := int64(binary.LittleEndian.Uint32(entry[2:6]))
		fieldSize := int64(binary.LittleEndian.Uint32(entry[6:10]))

		size, err := checkedInt64ToInt(fieldSize)
		if err != nil {
			return nil, nil, err
		}
		data := make([]byte, size)
		if _, err := r.Seek(fieldOffset, io.SeekStart); err != nil {
			return nil, nil, err
		}
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, nil, err
		}
		area = append(area, data...)

		fields = append(fields, DeveloperField{
			Tag:  binary.LittleEndian.Uint16(entry[:2]),
			Data: data,
		})
	}

	return fields, area, nil
}

// applyTGA2AlphaSemantics maps TGA 2.0 alpha attributes to Go image models.
// hasPhysicalAlpha reports whether the encoded pixel format contains alpha data.
func hasPhysicalAlpha(header [headerSize]byte) bool {
	switch header[2] {
	case typeGrayscale, typeRLEGrayscale:
		return header[16] == 16

	case typeTrueColor, typeRLETrueColor:
		return header[16] == 32 || (header[16] == 16 && header[17]&0x0f == 1)

	case typePaletted, typeRLEPaletted:
		return header[7] == 32

	default:
		return false
	}
}

// applyTGA2AlphaSemantics applies metadata semantics to physical alpha data.
func applyTGA2AlphaSemantics(img image.Image, attributesType byte, physicalAlpha, descriptorAlpha bool) (image.Image, error) {
	switch attributesType {
	case 0, 1:
		// No alpha or ignorable alpha is represented as fully opaque pixels.
		if physicalAlpha {
			return makeOpaqueNRGBA(img), nil
		}
		return img, nil

	case 2, 3:
		// Undefined-but-preserved and useful unassociated alpha are straight.
		if !descriptorAlpha {
			return nil, ErrFormat
		}
		return img, nil

	case 4:
		// Useful associated alpha is represented by the premultiplied RGBA model.
		if !descriptorAlpha {
			return nil, ErrFormat
		}
		return makePremultipliedRGBA(img), nil

	default:
		return nil, ErrFormat
	}
}

// makeOpaqueNRGBA discards alpha in-place for decoder-owned NRGBA storage.
func makeOpaqueNRGBA(src image.Image) *image.NRGBA {
	if nrgba, ok := src.(*image.NRGBA); ok {
		bounds := nrgba.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			row := nrgba.Pix[nrgba.PixOffset(bounds.Min.X, y):]
			for i := 3; i < bounds.Dx()*4; i += 4 {
				row[i] = 0xff
			}
		}
		return nrgba
	}

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

// makePremultipliedRGBA reuses decoder-owned NRGBA storage as premultiplied RGBA.
func makePremultipliedRGBA(src image.Image) *image.RGBA {
	if nrgba, ok := src.(*image.NRGBA); ok {
		return &image.RGBA{Pix: nrgba.Pix, Stride: nrgba.Stride, Rect: nrgba.Rect}
	}

	b := src.Bounds()
	dst := image.NewRGBA(b)
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
func parseTGA2Metadata(r io.ReadSeeker, ext []byte, extOffset, dataEnd int64, format postageStampFormat, budget *metadataBudget) (*TGA2Metadata, error) {
	meta := &TGA2Metadata{
		Author:                readASCIIZ(ext[tga2OffAuthor : tga2OffAuthor+41]),
		Comments:              readCommentLines(ext[tga2OffComments : tga2OffComments+324]),
		JobName:               readASCIIZ(ext[tga2OffJobName : tga2OffJobName+41]),
		SoftwareID:            readASCIIZ(ext[tga2OffSoftwareID : tga2OffSoftwareID+41]),
		SoftwareVersion:       binary.LittleEndian.Uint16(ext[tga2OffSoftwareVer : tga2OffSoftwareVer+2]),
		SoftwareVersionLetter: ext[tga2OffSoftwareVer+2],
		AttributesType:        ext[tga2OffAttrType],
	}
	keyColor := binary.LittleEndian.Uint32(ext[tga2OffKeyColor : tga2OffKeyColor+4])
	meta.KeyColor = color.NRGBA{
		R: uint8(keyColor >> 16), // #nosec G115 -- the shift extracts one byte from a uint32 field.
		G: uint8(keyColor >> 8),  // #nosec G115 -- the shift extracts one byte from a uint32 field.
		B: uint8(keyColor),       // #nosec G115 -- the conversion extracts the low byte from a uint32 field.
		A: uint8(keyColor >> 24), // #nosec G115 -- the shift extracts one byte from a uint32 field.
	}
	meta.PixelAspectRatio = TGA2PixelAspectRatio{
		Numerator:   binary.LittleEndian.Uint16(ext[tga2OffPixelAspect : tga2OffPixelAspect+2]),
		Denominator: binary.LittleEndian.Uint16(ext[tga2OffPixelAspect+2 : tga2OffPixelAspect+4]),
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

	colorCorrectionOffset := int64(binary.LittleEndian.Uint32(ext[tga2OffColorCorrect : tga2OffColorCorrect+4]))
	if colorCorrectionOffset != 0 {
		data, err := readTGA2Block(r, colorCorrectionOffset, tga2ColorCorrectionSize, dataEnd, budget)
		if err != nil {
			return nil, err
		}
		if err := budget.reserve(int64(tga2ColorCorrectionEntries * 8)); err != nil {
			return nil, err
		}

		meta.ColorCorrectionTable = make([]TGA2ColorCorrectionEntry, tga2ColorCorrectionEntries)
		for i := range meta.ColorCorrectionTable {
			offset := i * 8
			meta.ColorCorrectionTable[i] = TGA2ColorCorrectionEntry{
				A: binary.LittleEndian.Uint16(data[offset : offset+2]),
				R: binary.LittleEndian.Uint16(data[offset+2 : offset+4]),
				G: binary.LittleEndian.Uint16(data[offset+4 : offset+6]),
				B: binary.LittleEndian.Uint16(data[offset+6 : offset+8]),
			}
		}
	}

	scanLineOffset := int64(binary.LittleEndian.Uint32(ext[tga2OffScanLine : tga2OffScanLine+4]))
	if scanLineOffset != 0 {
		size := int64(format.imageHeight) * 4
		data, err := readTGA2Block(r, scanLineOffset, size, dataEnd, budget)
		if err != nil {
			return nil, err
		}
		if err := budget.reserve(size); err != nil {
			return nil, err
		}

		meta.ScanLineTable = make([]uint32, format.imageHeight)
		for i := range meta.ScanLineTable {
			meta.ScanLineTable[i] = binary.LittleEndian.Uint32(data[i*4 : i*4+4])
		}
	}

	postageOffset := int64(binary.LittleEndian.Uint32(ext[tga2OffPostageStamp : tga2OffPostageStamp+4]))
	if postageOffset != 0 {
		// A zero offset means no thumbnail.
		// Otherwise, constrain the offset and decoded size to the payload
		// before allocating the thumbnail buffer.
		// TGA 2.0 permits the postage stamp to be stored before the extension area;
		// when present, the extension area is the next known block boundary.
		postageEnd := dataEnd
		if extOffset > postageOffset {
			postageEnd = extOffset
		}
		if postageOffset < int64(headerSize) || postageOffset > postageEnd-2 {
			return nil, ErrFormat
		}

		if _, err := r.Seek(postageOffset, io.SeekStart); err != nil {
			return nil, err
		}

		var dimensions [2]byte
		if _, err := io.ReadFull(r, dimensions[:]); err != nil {
			return nil, err
		}

		bytesPerPixel, err := postageBytesPerPixel(format)
		if err != nil {
			return nil, err
		}
		size := int64(dimensions[0])*int64(dimensions[1])*int64(bytesPerPixel) + 2
		if size > postageEnd-postageOffset {
			return nil, ErrFormat
		}
		decodedBytes := int64(dimensions[0]) * int64(dimensions[1])
		if len(format.palette) == 0 {
			decodedBytes *= 4
		}
		if err := budget.reserve(size + decodedBytes); err != nil {
			return nil, err
		}

		pixels := make([]byte, int(size-2))
		if _, err := io.ReadFull(r, pixels); err != nil {
			return nil, err
		}

		width := int(dimensions[0])
		height := int(dimensions[1])
		thumb, err := decodePostageStamp(pixels, width, height, format)
		if err != nil {
			return nil, err
		}
		meta.Thumbnail = thumb
	}

	return meta, nil
}

// readTGA2Block reads a bounded file-relative metadata block.
func readTGA2Block(r io.ReadSeeker, offset, size, dataEnd int64, budget *metadataBudget) ([]byte, error) {
	if size < 0 || offset < int64(headerSize) || offset > dataEnd-size {
		return nil, ErrFormat
	}
	if err := budget.reserve(size); err != nil {
		return nil, err
	}

	blockSize, err := checkedInt64ToInt(size)
	if err != nil {
		return nil, err
	}
	if _, err := r.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	block := make([]byte, blockSize)
	if _, err := io.ReadFull(r, block); err != nil {
		return nil, err
	}

	return block, nil
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
	comments := make([]string, 4)
	last := -1
	for i := range 4 {
		comments[i] = readASCIIZ(src[i*81 : (i+1)*81])
		if comments[i] != "" {
			last = i
		}
	}

	if last < 0 {
		return nil
	}

	return comments[:last+1]
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
func writeTGA2Tail(w *countingWriter, meta *TGA2Metadata, format postageStampFormat, originBottom bool) error {
	// A footer is emitted only when the caller supplied TGA 2.0 metadata.
	if meta == nil {
		return nil
	}

	// All extension-area offsets are file-relative,
	// so capture the current position before reserving the fixed-size extension block.
	extOffset, err := uint32FromInt64(w.n)
	if err != nil {
		return err
	}

	// Build optional variable-size sections before calculating their offsets.
	postageStamp, err := buildPostageStamp(meta.Thumbnail, format, originBottom)
	if err != nil {
		return err
	}
	developerFields, err := developerFieldsForEncoding(meta)
	if err != nil {
		return err
	}

	extEnd, err := uint32FromInt64(int64(extOffset) + int64(tga2ExtensionSize))
	if err != nil {
		return err
	}

	// The scan-line table has one entry per main-image row;
	// an empty table means that the extension area does not reference one.
	if len(meta.ScanLineTable) > 0 && len(meta.ScanLineTable) != format.imageHeight {
		return metadataError("scan_line_table", "length must match image height")
	}

	// The color-correction table is defined as exactly 256 four-component SHORT entries;
	// unlike the other sections, it has no variable length.
	if len(meta.ColorCorrectionTable) != 0 && len(meta.ColorCorrectionTable) != tga2ColorCorrectionEntries {
		return metadataError("color_correction_table", "must contain 256 entries")
	}

	// Sections are laid out in the same order as their offsets are assigned.
	// Keeping this cursor separate from the writer position avoids seeking
	// and makes every offset stable before the extension area is serialized.
	cursor := int64(extEnd)
	scanLineOffset := uint32(0)
	if len(meta.ScanLineTable) > 0 {
		scanLineOffset, err = uint32FromInt64(cursor)
		if err != nil {
			return err
		}
		cursor += int64(len(meta.ScanLineTable)) * 4
	}
	postageOffset := uint32(0)
	if len(postageStamp) > 0 {
		postageOffset, err = uint32FromInt64(cursor)
		if err != nil {
			return err
		}
		cursor += int64(len(postageStamp))
	}
	colorCorrectionOffset := uint32(0)
	if len(meta.ColorCorrectionTable) > 0 {
		colorCorrectionOffset, err = uint32FromInt64(cursor)
		if err != nil {
			return err
		}
	}

	// Write the fixed extension area first;
	// it contains references to all variable-size sections written immediately after it.
	ext := buildExtensionArea(meta, postageOffset, colorCorrectionOffset, scanLineOffset)
	if _, err := w.Write(ext); err != nil {
		return err
	}

	if len(meta.ScanLineTable) > 0 {
		// Scan-line entries are stored as little-endian file offsets.
		table := make([]byte, len(meta.ScanLineTable)*4)
		for i, offset := range meta.ScanLineTable {
			binary.LittleEndian.PutUint32(table[i*4:i*4+4], offset)
		}
		if _, err := w.Write(table); err != nil {
			return err
		}
	}

	if len(postageStamp) > 0 {
		// The postage stamp is already encoded in the main image pixel format.
		if _, err := w.Write(postageStamp); err != nil {
			return err
		}
	}

	if len(meta.ColorCorrectionTable) > 0 {
		// Each correction entry is serialized as A, R, G, B 16-bit values.
		table := make([]byte, tga2ColorCorrectionSize)
		for i, entry := range meta.ColorCorrectionTable {
			offset := i * 8
			binary.LittleEndian.PutUint16(table[offset:offset+2], entry.A)
			binary.LittleEndian.PutUint16(table[offset+2:offset+4], entry.R)
			binary.LittleEndian.PutUint16(table[offset+4:offset+6], entry.G)
			binary.LittleEndian.PutUint16(table[offset+6:offset+8], entry.B)
		}
		if _, err := w.Write(table); err != nil {
			return err
		}
	}

	devOffset := uint32(0)
	if len(developerFields) > 0 {
		// Developer payloads follow the extension sections
		// and are indexed by a directory written after all payloads.
		entries := make([]developerDirectoryEntry, len(developerFields))
		for i, field := range developerFields {
			fieldOffset, err := uint32FromInt64(w.n)
			if err != nil {
				return err
			}
			if _, err := w.Write(field.Data); err != nil {
				return err
			}
			fieldSize, err := uint32FromInt64(int64(len(field.Data)))
			if err != nil {
				return err
			}
			entries[i] = developerDirectoryEntry{
				tag:    field.Tag,
				offset: fieldOffset,
				size:   fieldSize,
			}
		}

		devOffset, err = uint32FromInt64(w.n)
		if err != nil {
			return err
		}

		directory := make([]byte, 2+len(entries)*10)
		fieldCount, err := uint16FromInt(len(entries))
		if err != nil {
			return err
		}

		binary.LittleEndian.PutUint16(directory[:2], fieldCount)
		for i, entry := range entries {
			offset := 2 + i*10
			binary.LittleEndian.PutUint16(directory[offset:offset+2], entry.tag)
			binary.LittleEndian.PutUint32(directory[offset+2:offset+6], entry.offset)
			binary.LittleEndian.PutUint32(directory[offset+6:offset+10], entry.size)
		}
		if _, err := w.Write(directory); err != nil {
			return err
		}
	}

	return writeFooter(w, extOffset, devOffset)
}

// developerFieldsForEncoding converts the legacy raw field into one tagged field.
func developerFieldsForEncoding(meta *TGA2Metadata) ([]DeveloperField, error) {
	if len(meta.DeveloperFields) > 0 && len(meta.DeveloperArea) > 0 {
		return nil, metadataError("developer_fields", "cannot be combined with developer_area")
	}
	if len(meta.DeveloperFields) > math.MaxUint16 {
		return nil, metadataError("developer_fields", "more than 65535 fields")
	}
	if len(meta.DeveloperFields) > 0 {
		return meta.DeveloperFields, nil
	}
	if len(meta.DeveloperArea) == 0 {
		return nil, nil
	}

	return []DeveloperField{{Data: meta.DeveloperArea}}, nil
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
func buildExtensionArea(meta *TGA2Metadata, postageOffset, colorCorrectionOffset, scanLineOffset uint32) []byte {
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

	keyColor := uint32(meta.KeyColor.A)<<24 |
		uint32(meta.KeyColor.R)<<16 |
		uint32(meta.KeyColor.G)<<8 |
		uint32(meta.KeyColor.B)

	binary.LittleEndian.PutUint32(ext[tga2OffKeyColor:tga2OffKeyColor+4], keyColor)
	binary.LittleEndian.PutUint16(ext[tga2OffPixelAspect:tga2OffPixelAspect+2], meta.PixelAspectRatio.Numerator)
	binary.LittleEndian.PutUint16(ext[tga2OffPixelAspect+2:tga2OffPixelAspect+4], meta.PixelAspectRatio.Denominator)

	if num, den, ok := resolveGamma(meta); ok {
		binary.LittleEndian.PutUint16(ext[tga2OffGammaNum:tga2OffGammaNum+2], num)
		binary.LittleEndian.PutUint16(ext[tga2OffGammaDen:tga2OffGammaDen+2], den)
	}

	binary.LittleEndian.PutUint32(ext[tga2OffColorCorrect:tga2OffColorCorrect+4], colorCorrectionOffset)
	binary.LittleEndian.PutUint32(ext[tga2OffPostageStamp:tga2OffPostageStamp+4], postageOffset)
	binary.LittleEndian.PutUint32(ext[tga2OffScanLine:tga2OffScanLine+4], scanLineOffset)
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

// postageBytesPerPixel returns the uncompressed postage pixel size.
func postageBytesPerPixel(format postageStampFormat) (int, error) {
	if len(format.palette) > 0 || format.grayscale {
		if format.grayscale && format.depth != 8 && format.depth != 16 {
			return 0, ErrUnsupported
		}
		if len(format.palette) > 0 && format.depth != 8 {
			return 0, ErrUnsupported
		}
		return (format.depth + 7) / 8, nil
	}

	if format.depth != 15 && format.depth != 16 && format.depth != 24 && format.depth != 32 {
		return 0, ErrUnsupported
	}

	return format.depth / 8, nil
}

// decodePostageStamp decodes uncompressed thumbnail pixels in the main format.
func decodePostageStamp(pixels []byte, width, height int, format postageStampFormat) (image.Image, error) {
	if len(format.palette) > 0 {
		thumb := image.NewPaletted(image.Rect(0, 0, width, height), format.palette)
		for i, value := range pixels {
			index := int(value) - format.colorMapStart
			if index < 0 || index >= len(format.palette) {
				return nil, ErrFormat
			}

			indexByte, err := byteFromInt(index)
			if err != nil {
				return nil, err
			}

			thumb.Pix[i] = indexByte
		}

		return thumb, nil
	}

	thumb := image.NewNRGBA(image.Rect(0, 0, width, height))
	bytesPerPixel, err := postageBytesPerPixel(format)
	if err != nil {
		return nil, err
	}

	for i, offset := 0, 0; i < width*height; i++ {
		var pixel color.NRGBA
		switch {
		case format.grayscale && format.depth == 8:
			pixel = color.NRGBA{R: pixels[offset], G: pixels[offset], B: pixels[offset], A: 0xff}

		case format.grayscale && format.depth == 16:
			pixel = color.NRGBA{R: pixels[offset], G: pixels[offset], B: pixels[offset], A: pixels[offset+1]}

		case format.depth == 15 || format.depth == 16:
			pixel = decode16BitTrueColor(binary.LittleEndian.Uint16(pixels[offset:offset+2]), format.alpha)

		case format.depth == 24:
			pixel = color.NRGBA{R: pixels[offset+2], G: pixels[offset+1], B: pixels[offset], A: 0xff}

		case format.depth == 32:
			pixel = color.NRGBA{R: pixels[offset+2], G: pixels[offset+1], B: pixels[offset], A: pixels[offset+3]}

		default:
			return nil, ErrUnsupported
		}
		thumb.SetNRGBA(i%width, i/width, pixel)
		offset += bytesPerPixel
	}

	return thumb, nil
}

// buildPostageStamp builds an uncompressed TGA 2.0 thumbnail in the main format.
func buildPostageStamp(img image.Image, format postageStampFormat, originBottom bool) ([]byte, error) {
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

	bytesPerPixel, err := postageBytesPerPixel(format)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 2+w*h*bytesPerPixel)
	out[0] = wb
	out[1] = hb

	for row := range h {
		y := b.Min.Y + row
		if originBottom {
			y = b.Max.Y - 1 - row
		}

		dst := out[2+row*w*bytesPerPixel:]
		switch {
		case len(format.palette) > 0:
			for x := range w {
				index, err := byteFromInt(format.palette.Index(img.At(b.Min.X+x, y)))
				if err != nil {
					return nil, err
				}
				dst[x] = index
			}

		case format.grayscale:
			for x := range w {
				gray := color.GrayModel.Convert(img.At(b.Min.X+x, y)).(color.Gray)
				dst[x*bytesPerPixel] = gray.Y
				if bytesPerPixel == 2 {
					nrgba := color.NRGBAModel.Convert(img.At(b.Min.X+x, y)).(color.NRGBA)
					dst[x*2+1] = nrgba.A
				}
			}

		default:
			rowPixels := make([]byte, w*4)
			fillNRGBARow(rowPixels, img, b.Min.X, y)
			packNRGBARow(dst[:w*bytesPerPixel], rowPixels, format.depth)
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
