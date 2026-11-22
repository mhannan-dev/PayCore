package helpers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand"
	"strings"
	"testing"

	"github.com/HugoSmits86/nativewebp"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	"paycore/api/internal/models"
	"paycore/api/internal/storage"
)

type UploadedFile struct {
	FileName, ContentType, StorageKey, SHA256 string
	SizeBytes                                 int64
}

func uploadImageFixture(t *testing.T, filename string, data []byte) (UploadedFile, []byte, error) {
	t.Helper()
	files, err := storage.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()

	prepared, err := PrepareUpload(context.Background(), models.AttachmentUpload{FileName: filename, Content: bytes.NewReader(data)}, NewWebPConverter(2))
	if err != nil {
		return UploadedFile{}, nil, err
	}
	key, hash, size, err := files.Save(prepared.Content, prepared.FileName)
	if err != nil {
		return UploadedFile{}, nil, err
	}
	result := UploadedFile{FileName: prepared.FileName, ContentType: prepared.ContentType, StorageKey: key, SHA256: hash, SizeBytes: size}
	file, err := files.Open(result.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	stored, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return result, stored, nil
}

func TestImagesSavedAsWebP(t *testing.T) {
	picture := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	picture.SetNRGBA(0, 0, color.NRGBA{R: 120, G: 60, B: 30, A: 100})
	picture.SetNRGBA(2, 1, color.NRGBA{R: 10, G: 20, B: 200, A: 255})
	for _, tc := range []struct {
		filename string
		encode   func(io.Writer, image.Image) error
	}{
		{"avatar.PNG", png.Encode},
		{"avatar.jpg", func(w io.Writer, i image.Image) error { return jpeg.Encode(w, i, nil) }},
		{"avatar.gif", func(w io.Writer, i image.Image) error { return gif.Encode(w, i, nil) }},
		{"avatar.bmp", bmp.Encode},
		{"avatar.tiff", func(w io.Writer, i image.Image) error { return tiff.Encode(w, i, nil) }},
		{"avatar.webp", func(w io.Writer, i image.Image) error { return nativewebp.Encode(w, i, nil) }},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			var input bytes.Buffer
			if err := tc.encode(&input, picture); err != nil {
				t.Fatal(err)
			}
			result, stored, err := uploadImageFixture(t, tc.filename, input.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if result.FileName != "avatar.webp" || result.ContentType != WebPContentType || !strings.HasSuffix(result.StorageKey, ".webp") {
				t.Fatalf("wrong converted metadata: %+v", result)
			}
			digest := sha256.Sum256(stored)
			if result.SizeBytes != int64(len(stored)) || result.SHA256 != hex.EncodeToString(digest[:]) {
				t.Fatal("metadata describes source bytes instead of stored WebP")
			}
			decoded, format, err := image.Decode(bytes.NewReader(stored))
			if err != nil || format != "webp" || decoded.Bounds() != picture.Bounds() {
				t.Fatalf("invalid converted WebP: %s %v", format, err)
			}
			if tc.filename == "avatar.PNG" && color.NRGBAModel.Convert(decoded.At(0, 0)) != picture.NRGBAAt(0, 0) {
				t.Fatal("lossless conversion did not preserve transparency/pixel colour")
			}
		})
	}
}

func TestLargeImageCompressedBelowThreeMiB(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 1200, 1200))
	random := rand.New(rand.NewSource(1))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			source.SetNRGBA(x, y, color.NRGBA{R: uint8(random.Intn(256)), G: uint8(random.Intn(256)), B: uint8(random.Intn(256)), A: 255})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, source); err != nil {
		t.Fatal(err)
	}
	_, stored, err := uploadImageFixture(t, "noisy.png", input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(stored)) > MaxWebPImageBytes {
		t.Fatalf("converted image is %d bytes, want at most %d", len(stored), MaxWebPImageBytes)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(stored))
	if err != nil || format != "webp" {
		t.Fatalf("invalid converted WebP: %s %v", format, err)
	}
	if cfg.Width > source.Bounds().Dx() || cfg.Height > source.Bounds().Dy() {
		t.Fatalf("compressed dimensions %dx%d exceed source", cfg.Width, cfg.Height)
	}
}

func TestAnimatedGIFStaysAnimated(t *testing.T) {
	palette := color.Palette{color.Transparent, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255}}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	first.SetColorIndex(0, 0, 1)
	second := image.NewPaletted(first.Bounds(), palette)
	second.SetColorIndex(1, 1, 2)
	var input bytes.Buffer
	if err := gif.EncodeAll(&input, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{5, 12}, Disposal: []byte{gif.DisposalPrevious, gif.DisposalBackground}, LoopCount: 3}); err != nil {
		t.Fatal(err)
	}
	_, stored, err := uploadImageFixture(t, "animated.gif", input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var durations []int
	for pos := 12; pos+8 <= len(stored); {
		size := int(binary.LittleEndian.Uint32(stored[pos+4 : pos+8]))
		if size > len(stored)-pos-8 {
			t.Fatal("invalid WebP chunk")
		}
		if string(stored[pos:pos+4]) == "ANMF" {
			if size < 16 {
				t.Fatal("invalid animation frame")
			}
			durations = append(durations, int(stored[pos+20])|int(stored[pos+21])<<8|int(stored[pos+22])<<16)
		}
		pos += 8 + size + size%2
	}
	if len(durations) != 2 || durations[0] != 50 || durations[1] != 120 {
		t.Fatalf("animation frames/timing changed: %v", durations)
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(stored)); err != nil || cfg.Width != 2 || cfg.Height != 2 {
		t.Fatalf("animated WebP photo dimensions unreadable: %v", err)
	}
}

func TestInvalidImagesRejectedBeforeSaving(t *testing.T) {
	var small bytes.Buffer
	if err := png.Encode(&small, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	oversized := append([]byte(nil), small.Bytes()...)
	binary.BigEndian.PutUint32(oversized[16:20], 10001)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	for _, tc := range []struct {
		name string
		data []byte
		code string
	}{
		{"corrupt.png", []byte("\x89PNG\r\n\x1a\ninvalid"), "unsupported_image"},
		{"vector.svg", []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"), "unsupported_image"},
		{"too-wide.png", oversized, "image_dimensions_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := uploadImageFixture(t, tc.name, tc.data)
			var validation *models.UploadError
			if !errors.As(err, &validation) || string(validation.Code) != tc.code {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
		})
	}
}
