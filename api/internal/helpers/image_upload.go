package helpers

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path"
	"strings"

	"paycore/api/internal/models"
	"paycore/api/internal/storage"

	"github.com/HugoSmits86/nativewebp"
	_ "golang.org/x/image/bmp"
	imagedraw "golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
)

const WebPContentType = "image/webp"
const MaxWebPImageBytes int64 = 3 << 20
const maxImagePixels int64 = 25_000_000
const maxImageSide = 10000
const webPImageAttempts = 16
const webPImageShrinkNumerator = 7
const webPImageShrinkDenominator = 10

type uploadImageFormat string

const (
	imageFormatJPEG uploadImageFormat = "jpeg"
	imageFormatPNG  uploadImageFormat = "png"
	imageFormatGIF  uploadImageFormat = "gif"
	imageFormatBMP  uploadImageFormat = "bmp"
	imageFormatTIFF uploadImageFormat = "tiff"
	imageFormatWebP uploadImageFormat = "webp"
)

// Bound concurrent conversion memory/CPU independently of the request byte limit.
type WebPConverter struct{ slots chan struct{} }

func NewWebPConverter(concurrency int) *WebPConverter {
	if concurrency < 1 {
		concurrency = 1
	}
	return &WebPConverter{slots: make(chan struct{}, concurrency)}
}

// ValidImageSize is shared by upload conversion and profile-photo assignment.
func ValidImageSize(cfg image.Config) bool {
	return cfg.Width > 0 && cfg.Height > 0 && cfg.Width <= maxImageSide && cfg.Height <= maxImageSide && int64(cfg.Width)*int64(cfg.Height) <= maxImagePixels
}

func imageUpload(prefix []byte, contentType, declared, filename string) bool {
	// net/http's sniffer does not identify TIFF, so recognize its byte-order header.
	if bytes.HasPrefix(prefix, []byte{'I', 'I', 42, 0}) || bytes.HasPrefix(prefix, []byte{'M', 'M', 0, 42}) {
		return true
	}
	if strings.HasPrefix(contentType, "image/") || strings.HasPrefix(declared, "image/") {
		return true
	}
	switch strings.ToLower(path.Ext(filename)) {
	case ".svg", ".heic", ".heif", ".avif", ".ico", ".jxl", ".tiff", ".tif":
		return true
	}
	return false
}

func (c *WebPConverter) Convert(ctx context.Context, source io.Reader, filename string) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	default:
		return nil, "", &models.UploadError{Code: models.UploadImageBusy}
	}
	data, err := io.ReadAll(io.LimitReader(source, storage.MaxFileSize+1))
	if err != nil {
		return nil, "", &models.UploadError{Code: models.UploadInvalidImage}
	}
	if len(data) == 0 || int64(len(data)) > storage.MaxFileSize {
		return nil, "", storage.ErrFileSize
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", &models.UploadError{Code: models.UploadUnsupportedImage}
	}
	if !ValidImageSize(cfg) {
		return nil, "", &models.UploadError{Code: models.UploadImageDimensions}
	}
	if uploadImageFormat(format) == imageFormatWebP && len(data) >= 21 && string(data[12:16]) == "VP8X" && data[20]&2 != 0 {
		return nil, "", &models.UploadError{Code: models.UploadUnsupportedAnimation}
	}
	var encoded []byte
	switch uploadImageFormat(format) {
	case imageFormatGIF:
		if err = checkGIFBudget(data, cfg); err != nil {
			return nil, "", err
		}
		var animation *gif.GIF
		animation, err = gif.DecodeAll(bytes.NewReader(data))
		if err == nil {
			encoded, err = encodeGIF(animation)
		}
	case imageFormatJPEG, imageFormatPNG, imageFormatBMP, imageFormatTIFF, imageFormatWebP:
		if uploadImageFormat(format) == imageFormatPNG && animatedPNG(data) {
			return nil, "", &models.UploadError{Code: models.UploadUnsupportedAnimation}
		}
		var decoded image.Image
		decoded, _, err = image.Decode(bytes.NewReader(data))
		if err == nil {
			encoded, err = encodeStillWebP(decoded)
		}
	default:
		return nil, "", &models.UploadError{Code: models.UploadUnsupportedImage}
	}
	if errors.Is(err, storage.ErrFileSize) {
		return nil, "", err
	}
	if err != nil {
		return nil, "", &models.UploadError{Code: models.UploadInvalidImage}
	}
	if err = ctx.Err(); err != nil {
		return nil, "", err
	}
	base := strings.TrimSuffix(filename, path.Ext(filename))
	if base == "" {
		base = "image"
	}
	runes := []rune(base)
	if len(runes) > 250 {
		base = string(runes[:250])
	}
	return encoded, base + ".webp", nil
}

// The encoder may ignore writer errors, so also check overflow after encoding.
type imageBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *imageBuffer) Write(p []byte) (int, error) {
	if int64(b.Len())+int64(len(p)) > MaxWebPImageBytes {
		b.overflow = true
		return 0, storage.ErrFileSize
	}
	return b.Buffer.Write(p)
}

func animatedPNG(data []byte) bool {
	for offset := 8; offset+12 <= len(data); {
		size := int64(binary.BigEndian.Uint32(data[offset : offset+4]))
		if string(data[offset+4:offset+8]) == "acTL" {
			return true
		}
		if size > int64(len(data)-offset-12) {
			return false
		}
		offset += int(size) + 12
	}
	return false
}

// Preflight GIF block sizes before DecodeAll allocates frame pixel buffers.
func checkGIFBudget(data []byte, cfg image.Config) error {
	invalid := &models.UploadError{Code: models.UploadInvalidImage}
	if len(data) < 13 {
		return invalid
	}
	pos := 13
	if data[10]&0x80 != 0 {
		pos += 3 * (1 << ((data[10] & 7) + 1))
	}
	frames := 0
	for pos < len(data) {
		block := data[pos]
		pos++
		switch block {
		case 0x3b:
			return nil
		case 0x21:
			pos++
		case 0x2c:
			if pos+9 >= len(data) {
				return invalid
			}
			x := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			y := int(binary.LittleEndian.Uint16(data[pos+2 : pos+4]))
			width := int(binary.LittleEndian.Uint16(data[pos+4 : pos+6]))
			height := int(binary.LittleEndian.Uint16(data[pos+6 : pos+8]))
			if width == 0 || height == 0 || x+width > cfg.Width || y+height > cfg.Height {
				return invalid
			}
			frames++
			if frames > 200 || int64(frames)*int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
				return &models.UploadError{Code: models.UploadImageDimensions}
			}
			packed := data[pos+8]
			pos += 9
			if packed&0x80 != 0 {
				pos += 3 * (1 << ((packed & 7) + 1))
			}
			pos++ // LZW minimum code size
		default:
			return invalid
		}
		for {
			if pos >= len(data) {
				return invalid
			}
			size := int(data[pos])
			pos++
			if size == 0 {
				break
			}
			if size > len(data)-pos {
				return invalid
			}
			pos += size
		}
	}
	return invalid
}

func encodeStillWebP(src image.Image) ([]byte, error) {
	width, height := src.Bounds().Dx(), src.Bounds().Dy()
	for attempt := 0; attempt < webPImageAttempts; attempt++ {
		frame := image.Image(src)
		if attempt > 0 {
			w, h := scaledImageSize(width, height, attempt)
			frame = scaleImage(src, w, h)
		}
		encoded, err := encodeWebPImage(frame)
		if err != nil {
			return nil, err
		}
		if encoded == nil {
			continue
		}
		return encoded, nil
	}
	return nil, storage.ErrFileSize
}

func encodeGIF(animation *gif.GIF) ([]byte, error) {
	if len(animation.Image) == 0 {
		return nil, errors.New("empty GIF")
	}
	for attempt := 0; attempt < webPImageAttempts; attempt++ {
		var out imageBuffer
		if err := writeAnimation(&out, animation, attempt); err != nil {
			return nil, err
		}
		if out.overflow {
			continue
		}
		return out.Bytes(), nil
	}
	return nil, storage.ErrFileSize
}

func writeAnimation(w io.Writer, animation *gif.GIF, attempt int) error {
	canvas := image.NewNRGBA(image.Rect(0, 0, animation.Config.Width, animation.Config.Height))
	var background color.Color = color.Transparent
	if int(animation.BackgroundIndex) < len(animation.Image[0].Palette) {
		background = animation.Image[0].Palette[animation.BackgroundIndex]
	}
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	sequence := nativewebp.Animation{}
	if animation.LoopCount > 0 {
		sequence.LoopCount = uint16(min(animation.LoopCount+1, 65535))
	}
	if animation.LoopCount == -1 {
		sequence.LoopCount = 1
	}
	for i, frame := range animation.Image {
		previous := image.NewNRGBA(canvas.Bounds())
		draw.Draw(previous, previous.Bounds(), canvas, image.Point{}, draw.Src)
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		var snapshot image.Image
		if attempt > 0 {
			width, height := scaledImageSize(animation.Config.Width, animation.Config.Height, attempt)
			snapshot = scaleImage(canvas, width, height)
		} else {
			copy := image.NewNRGBA(canvas.Bounds())
			draw.Draw(copy, copy.Bounds(), canvas, image.Point{}, draw.Src)
			snapshot = copy
		}
		sequence.Images = append(sequence.Images, snapshot)
		sequence.Durations = append(sequence.Durations, uint(animation.Delay[i])*10)
		sequence.Disposals = append(sequence.Disposals, 1)
		if len(animation.Disposal) > i {
			switch animation.Disposal[i] {
			case gif.DisposalBackground:
				draw.Draw(canvas, frame.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
			case gif.DisposalPrevious:
				canvas = previous
			}
		}
	}
	options := &nativewebp.Options{CompressionLevel: nativewebp.BestSpeed}
	if len(sequence.Images) == 1 {
		return nativewebp.Encode(w, sequence.Images[0], options)
	}
	return nativewebp.EncodeAll(w, &sequence, options)
}

func encodeWebPImage(src image.Image) ([]byte, error) {
	var out imageBuffer
	if err := nativewebp.Encode(&out, src, &nativewebp.Options{CompressionLevel: nativewebp.BestSpeed}); err != nil {
		return nil, err
	}
	if out.overflow {
		return nil, nil
	}
	return out.Bytes(), nil
}

func scaledImageSize(width, height, attempt int) (int, int) {
	for i := 0; i < attempt; i++ {
		width = max(1, width*webPImageShrinkNumerator/webPImageShrinkDenominator)
		height = max(1, height*webPImageShrinkNumerator/webPImageShrinkDenominator)
	}
	return width, height
}

func scaleImage(src image.Image, width, height int) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	imagedraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), imagedraw.Src, nil)
	return dst
}
