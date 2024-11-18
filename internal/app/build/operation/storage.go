package operation

import (
	"context"
	"mime/multipart"

	"github.com/google/uuid"
)

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
