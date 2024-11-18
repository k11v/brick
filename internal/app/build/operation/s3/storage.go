package s3

import (
	"context"

	"github.com/k11v/brick/internal/app/build/operation"
)

var _ operation.Storage = (*Storage)(nil)

type Storage struct{}

func NewStorage() *Storage {
	panic("unimplemented")
}

// UploadFiles implements operation.Storage.
func (s *Storage) UploadFiles(ctx context.Context, params *operation.StorageUploadFilesParams) error {
	panic("unimplemented")
}

// DownloadFiles implements operation.Storage.
func (s *Storage) DownloadFiles(ctx context.Context, params *operation.StorageDownloadFilesParams) error {
	panic("unimplemented")
}
