package models

import (
	"errors"
	"io"
)

type UploadCode string

const (
	UploadTooLarge             UploadCode = "file_too_large"
	UploadInvalidMultipart     UploadCode = "invalid_multipart"
	UploadOneFileRequired      UploadCode = "one_file_required"
	UploadInvalidName          UploadCode = "invalid_file_name"
	UploadInvalidFile          UploadCode = "invalid_file"
	UploadInvalidSize          UploadCode = "file_size_must_be_1_byte_to_10_mib"
	UploadImageBusy            UploadCode = "image_conversion_busy"
	UploadUnsupportedImage     UploadCode = "unsupported_image"
	UploadInvalidImage         UploadCode = "invalid_image"
	UploadUnsupportedAnimation UploadCode = "unsupported_animated_image"
	UploadImageDimensions      UploadCode = "image_dimensions_exceeded"
)

// UploadError carries a domain validation code; HTTP mapping lives in server/.
type UploadError struct{ Code UploadCode }

func (e *UploadError) Error() string { return string(e.Code) }

// AttachmentUpload is a single file stream, independent of HTTP multipart parsing.
// The caller owns the stream and closes it after Upload returns.
type AttachmentUpload struct {
	FileName            string
	DeclaredContentType string
	Content             io.Reader
}

type StoredFile struct {
	Key, SHA256 string
	Size        int64
}

// FileContent is an opened regular file. The recipient must close Reader.
type FileContent struct {
	Reader io.ReadCloser
	Size   int64
}

// AttachmentDownload transfers ownership of Content to the transport layer.
type AttachmentDownload struct {
	Attachment Attachment
	Content    io.ReadCloser
}

var ErrAttachmentNotFound = errors.New("attachment not found")

// A failed commit acknowledgement cannot prove whether the row was saved.
var ErrAttachmentCommitUnknown = errors.New("attachment commit outcome unknown")
