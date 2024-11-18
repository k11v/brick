package operation

import (
	"context"
	"errors"
	"mime/multipart"

	"github.com/google/uuid"
)

var FileTooLarge = errors.New("file too large")

type StorageUploadFilesParams struct {
	BuildID         uuid.UUID
	MultipartReader *multipart.Reader
}

type StorageDownloadFilesParams struct {
	BuildID         uuid.UUID
	MultipartWriter *multipart.Writer
}

type Storage interface {
	UploadFiles(ctx context.Context, params *StorageUploadFilesParams) error
	DownloadFiles(ctx context.Context, params *StorageDownloadFilesParams) error
}
