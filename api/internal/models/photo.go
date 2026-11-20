package models

type ImageFormat string

const (
	ImageJPEG ImageFormat = "jpeg"
	ImagePNG  ImageFormat = "png"
	ImageGIF  ImageFormat = "gif"
	ImageWebP ImageFormat = "webp"
)

type ImageInfo struct {
	Width, Height int
	Format        ImageFormat
}

func (i ImageInfo) ValidPhoto() bool {
	switch i.Format {
	case ImageJPEG, ImagePNG, ImageGIF, ImageWebP:
	default:
		return false
	}
	return i.Width > 0 && i.Height > 0 && i.Width <= 10000 && i.Height <= 10000 && int64(i.Width)*int64(i.Height) <= 25_000_000
}

type PhotoResult struct {
	AttachmentID *string `json:"photo_attachment_id"`
	Photo        *string `json:"photo"`
}
